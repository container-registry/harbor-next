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

func Test_dbTxSkippers(t *testing.T) {
	tests := []struct {
		name string
		r    *http.Request
		want bool
	}{
		{"post initiate blob upload", httptest.NewRequest(http.MethodPost, "/v2/library/photon/blobs/uploads", nil), true},
		{"post initiate blob upload with mount", httptest.NewRequest(http.MethodPost, "/v2/library/photon/blobs/uploads?mount=sha256:aaa&from=library/app", nil), true},
		{"patch blob upload", httptest.NewRequest(http.MethodPatch, "/v2/library/photon/blobs/uploads/uuid-123", nil), true},
		{"put blob upload", httptest.NewRequest(http.MethodPut, "/v2/library/photon/blobs/uploads/uuid-123?digest=sha256:aaa", nil), true},
		{"put manifest", httptest.NewRequest(http.MethodPut, "/v2/library/photon/manifests/latest", nil), false},
		{"post api", httptest.NewRequest(http.MethodPost, "/api/v2.0/projects", nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			for _, skipper := range dbTxSkippers {
				if skipper(tt.r) {
					got = true
					break
				}
			}
			if got != tt.want {
				t.Errorf("dbTxSkippers(%s %s) = %v, want %v", tt.r.Method, tt.r.URL.Path, got, tt.want)
			}
		})
	}
}

func Test_securityHeaderSkipper(t *testing.T) {
	for path, skip := range map[string]bool{"/api/": false, "/api/v2.0/ping": false, "/v2/library/photon/manifests/latest": true, "/apikeys": true} {
		if got := securityHeaderSkipper(httptest.NewRequest(http.MethodGet, path, nil)); got != skip {
			t.Errorf("securityHeaderSkipper(%s) = %v, want %v", path, got, skip)
		}
	}
}

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

// TestTransactionRunsAfterTheDatabaseReadingMiddlewares covers harbor-next #92.
// transaction.Middleware holds a pool connection for the whole request. Any
// middleware inside it that reads config or user records needs a second
// connection while holding the first, and the config-cache builder it queues
// behind needs one too, so the pool deadlocks against itself under mixed read
// and write traffic.
func TestTransactionRunsAfterTheDatabaseReadingMiddlewares(t *testing.T) {
	chain := middlewareChain()
	position := map[string]int{}
	for i, entry := range chain {
		if _, dup := position[entry.name]; dup {
			t.Fatalf("middleware %q appears twice in the chain", entry.name)
		}
		position[entry.name] = i
	}

	if last := chain[len(chain)-1].name; last != "transaction" {
		t.Errorf("%q runs inside the request transaction; transaction must be the innermost middleware so nothing before the handler holds a pool connection while it waits for another", last)
	}

	for _, name := range []string{"security", "log", "security-unauthorized"} {
		if _, ok := position[name]; !ok {
			t.Fatalf("no %q middleware in the chain", name)
		}
	}
	if position["log"] < position["security"] || position["log"] > position["security-unauthorized"] {
		t.Error("log must run between security and security-unauthorized so the request's user is logged and 401s produce an audit event")
	}

	if _, ok := position["dbadmission"]; !ok {
		t.Fatal("no \"dbadmission\" middleware in the chain")
	}
	for _, name := range []string{"orm", "notification", "security"} {
		if position["dbadmission"] > position[name] {
			t.Errorf("dbadmission must run before %q so a rejected request never waits for a pool connection", name)
		}
	}
}
