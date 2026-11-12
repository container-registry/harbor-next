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

package test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mapping answers with its own id so a test can tell which route replied.
func mapping(id, method, pattern string) *RequestHandlerMapping {
	return &RequestHandlerMapping{
		Method:  method,
		Pattern: pattern,
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, id)
		},
	}
}

// TestNewServerRouting pins the dispatch rules callers rely on. Every case here
// was checked against the gorilla/mux router this replaced and matches it.
func TestNewServerRouting(t *testing.T) {
	server := NewServer(
		mapping("catalog", http.MethodGet, "/v2/_catalog"),
		mapping("tags", http.MethodGet, "/v2/{repo}/tags/list"),
		mapping("digest", http.MethodGet, "/v2/library/hello-world/blobs/digest"),
		mapping("digest1", http.MethodGet, "/v2/library/hello-world/blobs/digest1"),
		mapping("uploads", http.MethodPost, "/v2/library/hello-world/blobs/uploads/"),
		mapping("health", http.MethodGet, "/api/health"),
		mapping("v2root", http.MethodGet, "/v2/"),
		mapping("root", http.MethodGet, "/"),
	)
	defer server.Close()

	cases := []struct {
		name     string
		method   string
		path     string
		wantCode int
		wantBody string
	}{
		{"exact match", http.MethodGet, "/v2/_catalog", http.StatusOK, "catalog"},
		{"placeholder stands for one segment", http.MethodGet, "/v2/test1/tags/list", http.StatusOK, "tags"},
		{"placeholder does not span segments", http.MethodGet, "/v2/a/b/tags/list", http.StatusOK, "v2root"},
		{"a pattern is a prefix, not a whole path", http.MethodGet, "/api/healthcheck", http.StatusOK, "health"},
		{"registration order wins over a longer match", http.MethodGet, "/v2/library/hello-world/blobs/digest1", http.StatusOK, "digest"},
		{"prefix carries past the pattern", http.MethodPost, "/v2/library/hello-world/blobs/uploads/abc-123", http.StatusOK, "uploads"},
		{"unmatched path falls through to the catch-all", http.MethodGet, "/anything", http.StatusOK, "root"},
		{"method is part of matching", http.MethodPost, "/v2/_catalog", http.StatusMethodNotAllowed, ""},
		{"a query string is not matched", http.MethodGet, "/api/health?page=2", http.StatusOK, "health"},
	}

	client := server.Client()
	// A redirect must be observable rather than followed.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, server.URL+c.path, nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, c.wantCode, resp.StatusCode)

			if c.wantBody != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Equal(t, c.wantBody, string(body))
			}
		})
	}
}

// TestNewServerCleansPath covers the normalisation the router applied before
// matching: a redirect, not a match against the uncleaned path.
func TestNewServerCleansPath(t *testing.T) {
	server := NewServer(mapping("catalog", http.MethodGet, "/v2/_catalog"))
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for _, path := range []string{"/v2//_catalog", "/v2/./_catalog", "/v2/foo/../_catalog"} {
		t.Run(path, func(t *testing.T) {
			resp, err := client.Get(server.URL + path)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
			assert.Equal(t, "/v2/_catalog", resp.Header.Get("Location"))
		})
	}
}

func TestCompilePathPrefix(t *testing.T) {
	cases := []struct {
		pattern string
		matches []string
		misses  []string
	}{
		{
			pattern: "/v2/_catalog",
			matches: []string{"/v2/_catalog", "/v2/_catalogue", "/v2/_catalog/more"},
			misses:  []string{"/v2/catalog", "/v2"},
		},
		{
			pattern: "/v2/{repo}/tags/list",
			matches: []string{"/v2/test1/tags/list", "/v2/x/tags/list/extra"},
			misses:  []string{"/v2/a/b/tags/list", "/v2//tags/list", "/v2/tags/list"},
		},
		{
			pattern: "/v2/{repo:[a-z]+}/tags/list",
			matches: []string{"/v2/abc/tags/list"},
			misses:  []string{"/v2/ABC/tags/list", "/v2/a1/tags/list"},
		},
		{
			// A pattern with regexp metacharacters is matched literally.
			pattern: "/api/v2.0/health",
			matches: []string{"/api/v2.0/health"},
			misses:  []string{"/api/v2x0/health"},
		},
		{
			pattern: "/",
			matches: []string{"/", "/anything"},
		},
	}

	for _, c := range cases {
		t.Run(c.pattern, func(t *testing.T) {
			prefix := compilePathPrefix(c.pattern)

			for _, path := range c.matches {
				assert.True(t, prefix.MatchString(path), "%q should match %q", c.pattern, path)
			}
			for _, path := range c.misses {
				assert.False(t, prefix.MatchString(path), "%q should not match %q", c.pattern, path)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"/v2/_catalog", "/v2/_catalog"},
		{"/v2//_catalog", "/v2/_catalog"},
		{"/v2/./_catalog", "/v2/_catalog"},
		{"/v2/foo/../_catalog", "/v2/_catalog"},
		{"/v2/", "/v2/"},
		{"/v2", "/v2"},
		{"v2/_catalog", "/v2/_catalog"},
		{"/v2//", "/v2/"},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, cleanPath(c.in), "cleanPath(%q)", c.in)
	}
}
