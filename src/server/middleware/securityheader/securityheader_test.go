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
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/server/middleware"
)

func TestMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	Middleware()(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2.0/systeminfo", nil))

	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestMiddlewareOnErrorResponse(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	rec := httptest.NewRecorder()
	Middleware()(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2.0/users/current", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestMiddlewareSkipped(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	skipper := middleware.NegativeSkipper(middleware.MethodAndPathSkipper("*", regexp.MustCompile("^/api/")))

	rec := httptest.NewRecorder()
	Middleware(skipper)(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v2/library/hello-world/manifests/latest", nil))

	assert.Equal(t, "", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "", rec.Header().Get("Cache-Control"))

	rec = httptest.NewRecorder()
	Middleware(skipper)(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2.0/docs/", nil))

	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}
