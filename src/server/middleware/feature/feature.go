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

// Package feature provides the HTTP middleware for feature gates.
package feature

import (
	"fmt"
	"net/http"

	"github.com/goharbor/harbor/src/lib/errors"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	featuregate "github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/server/middleware"
)

// Require answers 404 for routes of a disabled feature, so a switched-off
// capability looks absent rather than forbidden.
// It panics for a feature nobody registered, which would otherwise hide the
// routes for good without any error.
func Require(feature featuregate.Feature) middleware.Middleware {
	if !registered(feature) {
		panic(fmt.Sprintf("feature %q is not registered", feature))
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !featuregate.Enabled(r.Context(), feature) {
				lib_http.SendError(w, errors.NotFoundError(nil).WithMessage("requested capability is not enabled"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func registered(feature featuregate.Feature) bool {
	for _, def := range featuregate.Definitions() {
		if def.Feature == feature {
			return true
		}
	}
	return false
}
