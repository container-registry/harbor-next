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

// Package systemquota holds the threshold gate that enforces the global
// storage quota on registry writes (harbor-next #839, theme A). It is a pure
// read of cached state: no reservation row, no CAS, so it adds no lock to the
// push path. Mount it only on write verbs; deletes must stay open so users can
// free space.
package systemquota

import (
	"net/http"

	"github.com/goharbor/harbor/src/controller/systemquota"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	"github.com/goharbor/harbor/src/server/middleware"
)

var ctl = func() systemquota.Controller { return systemquota.Ctl }

// Gate denies the request when the global storage quota is enforced and the
// current usage plus the request body would exceed the limit. The body size is
// only known for blob uploads that send Content-Length; everything else is
// checked against usage alone.
func Gate(skippers ...middleware.Skipper) func(http.Handler) http.Handler {
	return middleware.New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		var extra int64
		if r.Method == http.MethodPatch || r.Method == http.MethodPut {
			if r.ContentLength > 0 {
				extra = r.ContentLength
			}
		}
		if err := ctl().CheckCapacity(r.Context(), extra); err != nil {
			lib_http.SendError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	}, skippers...)
}
