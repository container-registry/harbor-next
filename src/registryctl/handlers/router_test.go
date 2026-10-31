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
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/registryctl/config"
)

// TestManifestRoute pins the split that replaces gorilla's greedy "{name:.*}"
// wildcard. Each case was checked against gorilla/mux and matches it.
func TestManifestRoute(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		wantCode  int
		wantName  string
		wantRef   string
		reachable bool
	}{
		{
			name:      "single segment repository",
			path:      "nginx/manifests/latest",
			wantCode:  http.StatusOK,
			wantName:  "nginx",
			wantRef:   "latest",
			reachable: true,
		},
		{
			name:      "two segment repository",
			path:      "library/hello-world/manifests/latest",
			wantCode:  http.StatusOK,
			wantName:  "library/hello-world",
			wantRef:   "latest",
			reachable: true,
		},
		{
			name:      "deeply nested repository",
			path:      "a/b/c/nginx/manifests/sha256:deadbeef",
			wantCode:  http.StatusOK,
			wantName:  "a/b/c/nginx",
			wantRef:   "sha256:deadbeef",
			reachable: true,
		},
		{
			name:      "a repository named blob is not the blob route",
			path:      "blob/manifests/latest",
			wantCode:  http.StatusOK,
			wantName:  "blob",
			wantRef:   "latest",
			reachable: true,
		},
		{
			name:      "repository containing the separator splits on the last one",
			path:      "a/manifests/b/manifests/c",
			wantCode:  http.StatusOK,
			wantName:  "a/manifests/b",
			wantRef:   "c",
			reachable: true,
		},
		{
			name:     "no separator",
			path:     "library/nginx",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "empty reference",
			path:     "library/nginx/manifests/",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "reference spanning segments",
			path:     "library/nginx/manifests/a/b",
			wantCode: http.StatusNotFound,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotName, gotRef string
			reached := false

			route := manifestRoute(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotName, gotRef, reached = r.PathValue("name"), r.PathValue("reference"), true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodDelete, "/api/registry/"+c.path, nil)
			req.SetPathValue("path", c.path)
			rec := httptest.NewRecorder()

			route(rec, req)

			assert.Equal(t, c.wantCode, rec.Code)
			assert.Equal(t, c.reachable, reached, "handler reachability")
			if c.reachable {
				assert.Equal(t, c.wantName, gotName)
				assert.Equal(t, c.wantRef, gotRef)
			}
		})
	}
}

func TestNewRouterDispatch(t *testing.T) {
	router := newRouter(config.Configuration{})

	cases := []struct {
		name     string
		method   string
		target   string
		wantCode int
	}{
		{"health", http.MethodGet, "/api/health", http.StatusOK},
		{"health rejects other methods", http.MethodPost, "/api/health", http.StatusMethodNotAllowed},
		{"blob rejects other methods", http.MethodGet, "/api/registry/blob/sha256:abc", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/api/nope", http.StatusNotFound},
		{"registry path without a manifest reference", http.MethodDelete, "/api/registry/library/nginx", http.StatusNotFound},
		// Routing succeeds and the manifest handler rejects "latest" as a digest,
		// which is how this case reaches a verdict without a storage driver.
		{"manifest route reaches the handler", http.MethodDelete, "/api/registry/library/nginx/manifests/latest", http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(c.method, c.target, nil))

			assert.Equal(t, c.wantCode, rec.Code)
		})
	}
}

// TestTrailingSlashStaysReachable pins the StrictSlash stand-in. gorilla's
// StrictSlash(true) answered "/api/health/" with a 301 to "/api/health";
// ServeMux has no equivalent, so trimTrailingSlash folds the slash away before
// dispatch and the route is reached directly instead of newly 404-ing.
func TestTrailingSlashStaysReachable(t *testing.T) {
	router := newRouter(config.Configuration{})

	cases := []struct {
		name     string
		method   string
		target   string
		wantCode int
	}{
		{"health with a trailing slash", http.MethodGet, "/api/health/", http.StatusOK},
		// The manifest handler rejects "latest" as a digest, so a routed request
		// reaches BadRequest; a 404 would mean the trailing slash lost the route.
		{"manifest delete with a trailing slash", http.MethodDelete, "/api/registry/library/nginx/manifests/latest/", http.StatusBadRequest},
		// An encoded trailing slash is part of the reference, not a separator: it
		// must survive the fold and be refused by the blob guard, not stripped into
		// a valid single-segment reference that deletes "sha256:ab".
		{"encoded trailing slash on a blob reference is not stripped", http.MethodDelete, "/api/registry/blob/sha256:ab%2F", http.StatusNotFound},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(c.method, c.target, nil))

			assert.Equal(t, c.wantCode, rec.Code)
		})
	}
}

// TestBlobRoute pins the encoded-slash guard. The ServeMux "{reference}" wildcard
// keeps an encoded slash inside the segment, where gorilla's single-segment match
// answered 404, so a reference carrying a separator must still be refused.
func TestBlobRoute(t *testing.T) {
	cases := []struct {
		name      string
		reference string
		reachable bool
	}{
		{"plain digest", "sha256:deadbeef", true},
		{"reference with a decoded slash", "sha256:deadbeef/../evil", false},
		{"reference that is only a slash", "/", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reached := false
			route := blobRoute(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodDelete, "/api/registry/blob/x", nil)
			req.SetPathValue("reference", c.reference)
			rec := httptest.NewRecorder()

			route(rec, req)

			assert.Equal(t, c.reachable, reached, "handler reachability")
			if !c.reachable {
				assert.Equal(t, http.StatusNotFound, rec.Code)
			}
		})
	}
}
