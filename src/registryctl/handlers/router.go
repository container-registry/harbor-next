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

package handlers

import (
	"net/http"
	"strings"

	"github.com/goharbor/harbor/src/registryctl/api"
	"github.com/goharbor/harbor/src/registryctl/api/registry/blob"
	"github.com/goharbor/harbor/src/registryctl/api/registry/manifest"
	"github.com/goharbor/harbor/src/registryctl/config"
)

// manifestsSeparator divides a repository name from a manifest reference in the
// registry delete path.
const manifestsSeparator = "/manifests/"

func newRouter(conf config.Configuration) http.Handler {
	// create the root rooter
	rootRouter := http.NewServeMux()
	rootRouter.HandleFunc("GET /api/health", api.Health)

	rootRouter.Handle("DELETE /api/registry/blob/{reference}", blob.NewHandler(conf.StorageDriver))
	rootRouter.Handle("DELETE /api/registry/{path...}", manifestRoute(manifest.NewHandler(conf.StorageDriver)))
	return rootRouter
}

// manifestRoute recovers the repository name and the manifest reference from the
// rest of the path. A repository name spans several segments, and only a
// trailing "..." wildcard matches more than one, so "{name}/manifests/{reference}"
// has no ServeMux spelling. Splitting on the last separator reproduces what the
// greedy "{name:.*}" pattern matched.
func manifestRoute(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := r.PathValue("path")

		index := strings.LastIndex(rest, manifestsSeparator)
		if index < 0 {
			http.NotFound(w, r)
			return
		}

		name := rest[:index]
		reference := rest[index+len(manifestsSeparator):]
		// The reference was a single-segment wildcard, so keep rejecting the rest.
		if reference == "" || strings.Contains(reference, "/") {
			http.NotFound(w, r)
			return
		}

		r.SetPathValue("name", name)
		r.SetPathValue("reference", reference)

		next.ServeHTTP(w, r)
	}
}
