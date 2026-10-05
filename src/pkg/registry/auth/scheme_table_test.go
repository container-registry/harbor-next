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

package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Which authorizer the WWW-Authenticate challenge selects.
func TestInitializeSchemeSelection(t *testing.T) {
	cases := []struct {
		name    string
		headers []string
		want    string
		err     bool
	}{
		{"no challenge means no auth", nil, "*null.authorizer", false},
		{"basic", []string{`Basic realm="r"`}, "*basic.authorizer", false},
		{"bearer", []string{`Bearer realm="https://a/token",service="s"`}, "*bearer.authorizer", false},
		{"bearer wins over basic", []string{`Basic realm="r"`, `Bearer realm="https://a/token",service="s"`}, "*bearer.authorizer", false},
		{"scheme is case-insensitive", []string{`BASIC realm="r"`}, "*basic.authorizer", false},
		{"unknown scheme", []string{`Digest realm="r"`}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for _, h := range tc.headers {
					w.Header().Add("WWW-Authenticate", h)
				}
				if len(tc.headers) > 0 {
					w.WriteHeader(http.StatusUnauthorized)
				}
			}))
			defer srv.Close()

			a := &authorizer{username: "u", password: "p", client: srv.Client()}
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/p/a/manifests/v1", nil)
			err := a.initialize(req.URL)
			if tc.err {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, fmt.Sprintf("%T", a.authorizer))
		})
	}
}
