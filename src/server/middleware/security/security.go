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

package security

import (
	"net/http"

	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/server/middleware"
)

var (
	generators = []generator{
		&secret{},
		&oidcCli{},
		&v2Token{},
		&idToken{},
		&authProxy{},
		&robot{},
		&basicAuth{},
		&session{},
		&proxyCacheSecret{},
	}
)

// security context generator
type generator interface {
	// Generate returns nil when the request carries no credential the generator
	// handles or the credential is invalid, and an error only when a backend it
	// depends on fails, so the credential could be neither accepted nor rejected.
	Generate(req *http.Request) (security.Context, error)
}

// Middleware returns a security context middleware that populates the security context into the request context
func Middleware(skippers ...middleware.Skipper) func(http.Handler) http.Handler {
	return middleware.New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		log := log.G(r.Context())
		mode, err := config.AuthMode(r.Context())
		if err != nil {
			// Fail closed: determining the security context depends on the configured auth
			// mode, so a failure to load it must reject the request rather than continue with
			// an undetermined mode (CWE-636 fail-open of an authorization decision).
			log.Errorf("failed to get auth mode, rejecting request: %v", err)
			lib_http.SendError(w, errors.UnknownError(err).WithMessage("failed to determine authentication mode"))
			return
		}
		r = r.WithContext(lib.WithAuthMode(r.Context(), mode))
		for _, generator := range generators {
			ctx, err := generator.Generate(r)
			if err != nil {
				// Fail closed, and not as 401: continuing as anonymous would tell a client
				// with valid credentials that they are wrong, and many give up instead of
				// retrying.
				log.WithField("client IP", GetClientIP(r)).WithField("user agent", GetUserAgent(r)).
					Errorf("failed to verify credentials, rejecting request: %v", err)
				lib_http.SendServiceUnavailable(w)
				return
			}
			if ctx != nil {
				r = r.WithContext(security.NewContext(r.Context(), ctx))
				break
			}
		}
		next.ServeHTTP(w, r)
	}, skippers...)
}

// UnauthorizedMiddleware returns a security context middleware
// that populates the unauthorized security context when not security context found in the request context
func UnauthorizedMiddleware(skippers ...middleware.Skipper) func(http.Handler) http.Handler {
	return middleware.New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if _, ok := security.FromContext(r.Context()); !ok {
			u := &unauthorized{}
			ctx, _ := u.Generate(r)
			r = r.WithContext(security.NewContext(r.Context(), ctx))
		}

		next.ServeHTTP(w, r)
	}, skippers...)
}
