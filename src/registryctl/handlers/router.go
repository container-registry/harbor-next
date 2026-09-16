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

	rootRouter.Handle("DELETE /api/registry/blob/{reference}", blobRoute(blob.NewHandler(conf.StorageDriver)))
	rootRouter.Handle("DELETE /api/registry/{path...}", manifestRoute(manifest.NewHandler(conf.StorageDriver)))

	// gorilla/mux ran with StrictSlash(true), which redirected a trailing-slash
	// request onto the slashless route. ServeMux offers no such match and would
	// newly 404, so fold a trailing slash away before dispatch to keep those URLs
	// reachable. Normalising in place, rather than issuing gorilla's 301, also
	// spares the DELETE callers a redirect that would drop their method.
	return trimTrailingSlash(rootRouter)
}

// trimTrailingSlash strips a single literal trailing slash from the request path
// before the mux matches it, standing in for gorilla's StrictSlash redirect. It
// tests the escaped path so an encoded "%2F" — which belongs to a reference, not
// a path separator — is left in place and still reaches the route's slash guard,
// and it preserves RawPath so that encoding survives to the guard unchanged.
func trimTrailingSlash(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.EscapedPath(); len(p) > 1 && strings.HasSuffix(p, "/") {
			r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
			if r.URL.RawPath != "" {
				r.URL.RawPath = strings.TrimSuffix(r.URL.RawPath, "/")
			}
		}
		next.ServeHTTP(w, r)
	}
}

// blobRoute rejects a reference that carries a path separator. The ServeMux
// "{reference}" wildcard keeps an encoded slash as part of the segment and hands
// the decoded value through, where gorilla's single-segment match answered 404,
// so keep answering 404.
func blobRoute(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.PathValue("reference"), "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
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
