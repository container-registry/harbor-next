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

package securityheader

import (
	"net/http"

	"github.com/goharbor/harbor/src/server/middleware"
)

var headers = map[string]string{
	"X-Frame-Options":         "DENY",
	"Content-Security-Policy": "frame-ancestors 'none'",
	"X-Content-Type-Options":  "nosniff",
	"Cache-Control":           "no-store",
}

// Middleware sets the security related response headers
func Middleware(skippers ...middleware.Skipper) func(http.Handler) http.Handler {
	return middleware.New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		// set before the handler runs so that error responses carry the headers too
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		next.ServeHTTP(w, r)
	}, skippers...)
}
