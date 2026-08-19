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

package gosum

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/goharbor/harbor/src/common/rbac"
	rbacproject "github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	projectcontroller "github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

type handler struct {
	mirror          *Mirror
	authorize       projectAuthorizer
	proxyForProject proxyResolver
}

type projectAuthorizer func(http.ResponseWriter, *http.Request, string) (int64, bool)
type proxyResolver func(context.Context, string, string) (*pkgproxy.Proxy, error)

func newHandler() http.Handler {
	return newHandlerWithDeps(NewMirror(NewStore()), authorize, pkgproxy.ForProject)
}

func newHandlerWithDeps(mirror *Mirror, authorize projectAuthorizer, proxyForProject proxyResolver) http.Handler {
	return &handler{mirror: mirror, authorize: authorize, proxyForProject: proxyForProject}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := parseRequest(r.Method, r.URL.EscapedPath())
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	projectID, ok := h.authorize(w, r, req.Project)
	if !ok {
		return
	}
	proxy, err := h.proxyForProject(r.Context(), req.Project, model.RegistryTypeGoSumDB)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if proxy == nil || proxy.Registry == nil {
		writeError(w, http.StatusNotFound, "Go checksum database proxy is not enabled for project")
		return
	}
	databaseName := proxy.Registry.Name
	if databaseName == "" {
		databaseName = proxy.Registry.URL
	}
	database := Database{Name: databaseName, URL: strings.TrimRight(proxy.Registry.URL, "/")}
	response, err := h.mirror.Resolve(r.Context(), projectID, req.Project, database, req.Path, func(ctx context.Context) (*pkgproxy.Response, error) {
		return proxy.Get(ctx, req.Path, nil)
	})
	if err != nil {
		writeProxyError(w, err)
		return
	}
	contentType := response.ContentType
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(response.Body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(response.Body)
	}
}

func authorize(w http.ResponseWriter, r *http.Request, projectName string) (int64, bool) {
	securityCtx, ok := security.FromContext(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return 0, false
	}
	project, err := projectcontroller.Ctl.Get(r.Context(), projectName)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return 0, false
	}
	resource := rbacproject.NewNamespace(project.ProjectID).Resource(rbac.ResourceRepository)
	if !securityCtx.Can(r.Context(), rbac.ActionPull, resource) {
		if !securityCtx.IsAuthenticated() {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusForbidden, "forbidden")
		}
		return 0, false
	}
	return project.ProjectID, true
}

func writeProxyError(w http.ResponseWriter, err error) {
	if errors.IsNotFoundErr(err) {
		writeError(w, http.StatusNotFound, "checksum response not found")
		return
	}
	if errors.IsRateLimitError(err) {
		writeError(w, http.StatusTooManyRequests, "too many requests to upstream registry")
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}
