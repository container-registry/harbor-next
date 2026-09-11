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

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The admission gate must cover ONLY the allowlisted user-facing routes;
// gating an internal machine-to-machine path with an unexpected 429 can break
// flows (e.g. GC callbacks) destructively.
func Test_dbAdmissionSkipper(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		wantSkip bool
	}{
		{"login gated", http.MethodPost, "/c/login", false},
		{"create project gated", http.MethodPost, "/api/v2.0/projects", false},
		{"create project trailing slash gated", http.MethodPost, "/api/v2.0/projects/", false},
		{"create user gated", http.MethodPost, "/api/v2.0/users", false},
		{"create robot gated", http.MethodPost, "/api/v2.0/robots", false},

		{"login page not gated", http.MethodGet, "/c/login", true},
		{"list projects not gated", http.MethodGet, "/api/v2.0/projects", true},
		{"project sub-resource not gated", http.MethodPost, "/api/v2.0/projects/1/members", true},
		{"jobservice callback not gated", http.MethodPost, "/service/notifications/jobs/adminjob/123", true},
		{"token auth not gated", http.MethodGet, "/service/token", true},
		{"manifest push not gated", http.MethodPut, "/v2/library/nginx/manifests/latest", true},
		{"blob pull not gated", http.MethodGet, "/v2/library/nginx/blobs/sha256:abc", true},
		{"ping not gated", http.MethodGet, "/api/v2.0/ping", true},
		{"health not gated", http.MethodGet, "/api/v2.0/health", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			if got := dbAdmissionSkipper(r); got != tt.wantSkip {
				t.Errorf("dbAdmissionSkipper(%s %s) = %v, want %v", tt.method, tt.path, got, tt.wantSkip)
			}
		})
	}
}

func Test_readonlySkipper(t *testing.T) {
	type args struct {
		r *http.Request
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"login", args{httptest.NewRequest(http.MethodPost, "/c/login", nil)}, true},
		{"login get", args{httptest.NewRequest(http.MethodGet, "/c/login", nil)}, false},
		{"onboard", args{httptest.NewRequest(http.MethodPost, "/c/oidc/onboard", nil)}, true},
		{"user exist", args{httptest.NewRequest(http.MethodPost, "/c/userExists", nil)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pass bool
			for _, skipper := range readonlySkippers {
				if got := skipper(tt.args.r); got == tt.want {
					pass = true
				}
			}
			if !pass {
				t.Errorf("readonlySkippers() = %v, want %v", tt.args, tt.want)
			}
		})
	}
}
