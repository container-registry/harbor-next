// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package multiformatauth enforces project-scoped RBAC for the native npm and maven
// (multiformat) endpoints. registry.Cli authenticates to the backing registry as
// Harbor's internal credential, never as the requesting user, so every store
// access MUST be gated here first.
package multiformatauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/goharbor/harbor/src/common/rbac"
	rbac_project "github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/lib/errors"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	regmultiformat "github.com/goharbor/harbor/src/server/registry/multiformat"
)

const (
	// basicChallenge is returned on auth failure. Both clients (npm `_auth`,
	// maven `<server>`) send HTTP Basic, so a Bearer/token redirect is wrong here.
	basicChallenge = `Basic realm="harbor"`
)

type projectKey struct{}

// FromContext returns the resolved project ID stashed by the middleware. It is
// consumed downstream (e.g. ensureVisible) to avoid resolving the project twice.
func FromContext(ctx context.Context) (int64, bool) {
	pid, ok := ctx.Value(projectKey{}).(int64)
	return pid, ok
}

type reqChecker struct {
	ctl project.Controller
}

// check authorizes the request and returns the resolved project ID. A non-nil
// error means access is denied and a Basic challenge must be sent.
func (rc *reqChecker) check(req *http.Request) (int64, error) {
	securityCtx, ok := security.FromContext(req.Context())
	if !ok {
		return 0, errors.New("the security context got from request is nil")
	}

	name := projectName(req.URL.Path)
	if name == "" {
		return 0, errors.New("un-recognized request: missing project name")
	}

	p, err := rc.ctl.Get(req.Context(), name)
	if err != nil {
		// Treat a missing project as an auth failure so we never disclose its
		// existence; mirrors docker's 401-over-404 behaviour.
		if errors.IsNotFoundErr(err) {
			return 0, errors.New("project not found").WithCause(err)
		}
		return 0, err
	}

	action := actionForMethod(req.Method)
	if action == "" {
		return 0, fmt.Errorf("un-recognized method: %s", req.Method)
	}

	// Anonymous pull from a public project is allowed; everything else requires
	// the security context to carry the action on the project's repository resource.
	if action == rbac.ActionPull && p.IsPublic() {
		return p.ProjectID, nil
	}

	resource := rbac_project.NewNamespace(p.ProjectID).Resource(rbac.ResourceRepository)
	if !securityCtx.Can(req.Context(), action, resource) {
		return 0, fmt.Errorf("unauthorized to %s project %s", action, name)
	}
	return p.ProjectID, nil
}

// projectName extracts the project, the first path segment after the /npm or
// /maven prefix.
func projectName(path string) string {
	p := strings.TrimPrefix(path, "/")
	segs := strings.SplitN(p, "/", 3)
	if len(segs) < 2 {
		return ""
	}
	switch segs[0] {
	case "npm", "maven":
		return segs[1]
	default:
		return ""
	}
}

func actionForMethod(method string) types.Action {
	switch method {
	case http.MethodGet, http.MethodHead:
		return rbac.ActionPull
	case http.MethodPut, http.MethodPost:
		return rbac.ActionPush
	case http.MethodDelete:
		return rbac.ActionDelete
	default:
		return ""
	}
}

var (
	once    sync.Once
	checker reqChecker
)

// Middleware enforces project-scoped RBAC for the npm and maven endpoints.
func Middleware() func(http.Handler) http.Handler {
	once.Do(func() {
		if checker.ctl == nil { // for UT, where ctl has been set to a mock value
			checker = reqChecker{
				ctl: project.Ctl,
			}
		}
	})
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			pid, err := checker.check(req)
			if err != nil {
				rw.Header().Set("Www-Authenticate", basicChallenge)
				lib_http.SendError(rw, errors.UnauthorizedError(err).WithMessage(err.Error()))
				return
			}
			ctx := context.WithValue(req.Context(), projectKey{}, pid)
			ctx = regmultiformat.WithProjectID(ctx, pid)
			next.ServeHTTP(rw, req.WithContext(ctx))
		})
	}
}
