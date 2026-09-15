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
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ts is a fixed timestamp so the rendered lines are comparable.
var ts = time.Date(2026, time.September, 14, 6, 34, 23, 0, time.FixedZone("", 2*60*60))

func TestCommonLogLine(t *testing.T) {
	cases := []struct {
		name    string
		request func() *http.Request
		reqURL  func(*http.Request) url.URL
		status  int
		size    int64
		want    string
	}{
		{
			name: "delete with host and port",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodDelete, "/api/registry/blob/sha256:abc", nil)
				r.RemoteAddr = "10.1.2.3:52918"
				r.RequestURI = "/api/registry/blob/sha256:abc"
				return r
			},
			status: http.StatusOK,
			size:   0,
			want:   `10.1.2.3 - - [14/Sep/2026:06:34:23 +0200] "DELETE /api/registry/blob/sha256:abc HTTP/1.1" 200 0` + "\n",
		},
		{
			name: "remote address without a port is kept whole",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
				r.RemoteAddr = "unix"
				r.RequestURI = "/api/health"
				return r
			},
			status: http.StatusOK,
			size:   17,
			want:   `unix - - [14/Sep/2026:06:34:23 +0200] "GET /api/health HTTP/1.1" 200 17` + "\n",
		},
		{
			name: "userinfo on the URL becomes the authuser field",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
				r.RemoteAddr = "10.1.2.3:52918"
				r.RequestURI = "/api/health"
				return r
			},
			reqURL: func(r *http.Request) url.URL {
				u := *r.URL
				u.User = url.User("jobservice")
				return u
			},
			status: http.StatusUnauthorized,
			size:   13,
			want:   `10.1.2.3 - jobservice [14/Sep/2026:06:34:23 +0200] "GET /api/health HTTP/1.1" 401 13` + "\n",
		},
		{
			name: "a quote in the URI cannot break out of the request field",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
				r.RemoteAddr = "10.1.2.3:52918"
				r.RequestURI = `/api/health?x="` + "\n" + `evil`
				return r
			},
			status: http.StatusBadRequest,
			size:   9,
			want:   `10.1.2.3 - - [14/Sep/2026:06:34:23 +0200] "GET /api/health?x=\"\nevil HTTP/1.1" 400 9` + "\n",
		},
		{
			name: "empty RequestURI falls back to the URL",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/registry/gc?dry_run=true", nil)
				r.RemoteAddr = "10.1.2.3:52918"
				r.RequestURI = ""
				return r
			},
			status: http.StatusOK,
			size:   4,
			want:   `10.1.2.3 - - [14/Sep/2026:06:34:23 +0200] "GET /api/registry/gc?dry_run=true HTTP/1.1" 200 4` + "\n",
		},
		{
			name: "HTTP/2 CONNECT logs the authority instead of the URI",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodConnect, "/", nil)
				r.RemoteAddr = "10.1.2.3:52918"
				r.RequestURI = "/ignored"
				r.Host = "registryctl:8080"
				r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/2.0", 2, 0
				return r
			},
			status: http.StatusOK,
			size:   0,
			want:   `10.1.2.3 - - [14/Sep/2026:06:34:23 +0200] "CONNECT registryctl:8080 HTTP/2.0" 200 0` + "\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.request()
			reqURL := *r.URL
			if c.reqURL != nil {
				reqURL = c.reqURL(r)
			}
			assert.Equal(t, c.want, string(commonLogLine(r, reqURL, ts, c.status, c.size)))
		})
	}
}

func TestLoggingHandler(t *testing.T) {
	var out bytes.Buffer

	h := loggingHandler(&out, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rewriting the URL must not change what gets logged.
		r.URL.Path = "/rewritten"
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}))

	r := httptest.NewRequest(http.MethodDelete, "/api/registry/blob/sha256:abc", nil)
	r.RemoteAddr = "10.1.2.3:52918"
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusTeapot, rec.Code)

	line := out.String()
	require.NotEmpty(t, line)
	assert.Contains(t, line, `"DELETE /api/registry/blob/sha256:abc HTTP/1.1" 418 15`)
	assert.NotContains(t, line, "/rewritten")
	assert.True(t, line[len(line)-1] == '\n', "log line must be newline terminated")
}

// TestLoggingHandlerDefaultsToOK pins the status a handler that never calls
// WriteHeader is logged with.
func TestLoggingHandlerDefaultsToOK(t *testing.T) {
	var out bytes.Buffer

	h := loggingHandler(&out, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))

	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.RemoteAddr = "10.1.2.3:52918"

	h.ServeHTTP(httptest.NewRecorder(), r)

	assert.Contains(t, out.String(), `"GET /api/health HTTP/1.1" 200 0`)
}

func TestEscapeQuoted(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/api/health", "/api/health"},
		{`/a"b`, `/a\"b`},
		{`/a\b`, `/a\\b`},
		{"/a\nb", `/a\nb`},
		{"/a\tb", `/a\tb`},
		{"/é", "/é"},
		{"/\x00", `/\x00`},
		// DEL escapes as \x7f, where Apache would write \u007f. Unreachable in
		// practice: net/http rejects a request line holding a raw control byte.
		{"/\x7f", `/\x7f`},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, escapeQuoted(c.in), "escapeQuoted(%q)", c.in)
	}
}
