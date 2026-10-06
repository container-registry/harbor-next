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

package route

import (
	"net/http"
	"path"

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	"github.com/goharbor/harbor/src/server/middleware"
)

// docsPath is where the generated server serves its ReDoc documentation page.
const docsPath = "/api/" + APIVersion + "/docs"

// docsMiddleware answers the API documentation page with 404 unless
// API_DOCS_ENABLE is set. The page is unauthenticated and loads ReDoc and two
// font families from public CDNs, which a deployment that never opens it has no
// reason to serve.
func docsMiddleware() middleware.Middleware {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if path.Clean(r.URL.Path) == docsPath && !config.APIDocsEnable(r.Context()) {
				lib_http.SendError(w, errors.New(nil).WithCode(errors.NotFoundCode).
					WithMessagef("path %s was not found", r.URL.Path))
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
}
