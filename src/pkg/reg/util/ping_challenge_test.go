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

package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

// Ping reads the token realm and service from WWW-Authenticate; replication
// and proxy cache authenticate against whatever it returns.
func TestPingChallengeParsing(t *testing.T) {
	cases := []struct {
		name        string
		headers     []string
		realm       string
		service     string
		unsupported bool
	}{
		{"docker hub", []string{`Bearer realm="https://auth.docker.io/token",service="registry.docker.io"`},
			"https://auth.docker.io/token", "registry.docker.io", false},
		{"lowercase scheme", []string{`bearer realm="https://a/token",service="svc"`}, "https://a/token", "svc", false},
		{"uppercase scheme", []string{`BEARER realm="https://a/token",service="svc"`}, "https://a/token", "svc", false},
		{"param names are case-insensitive", []string{`Bearer Realm="https://a/token",SERVICE="svc"`}, "https://a/token", "svc", false},
		// quirks of the v2 parser, kept as they are
		{"spaces around separators drop the params", []string{`Bearer realm = "https://a/token" , service = "svc"`}, "", "", false},
		{"extra scope param", []string{`Bearer realm="https://a/token",service="svc",scope="repository:x:pull"`}, "https://a/token", "svc", false},
		{"escaped quote in value", []string{`Bearer realm="https://a/token",service="s\"vc"`}, "https://a/token", `s"vc`, false},
		{"unquoted value stops at colon", []string{`Bearer realm=https://a/token,service=svc`}, "https", "", false},
		{"no service", []string{`Bearer realm="https://a/token"`}, "https://a/token", "", false},
		{"basic then bearer in separate headers", []string{`Basic realm="x"`, `Bearer realm="https://a/token",service="svc"`}, "https://a/token", "svc", false},
		{"basic and bearer in one header", []string{`Basic realm="x", Bearer realm="https://a/token",service="svc"`}, "", "", true},
		{"basic only", []string{`Basic realm="registry"`}, "", "", true},
		{"no header", nil, "", "", true},
		{"garbage", []string{`!!!`}, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for _, h := range tc.headers {
					w.Header().Add("WWW-Authenticate", h)
				}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer srv.Close()

			realm, service, err := Ping(&model.Registry{URL: srv.URL})
			if tc.unsupported {
				assert.True(t, errors.IsErr(err, errors.ChallengesUnsupportedCode), "got %v", err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.realm, realm)
			assert.Equal(t, tc.service, service)
		})
	}
}
