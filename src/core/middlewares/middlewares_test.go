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
