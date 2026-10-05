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

package bearer

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scope decides which repository a token is requested for, so pin it for
// awkward names; the patterns come from the distribution reference grammar.
func TestParseScopesTable(t *testing.T) {
	const dgst = "sha256:eec76eedea59f7bf39a2713bfd995c82cfaa97724ee5b7f5aba253e07423d0ae"
	cases := []struct {
		method string
		url    string
		want   []string
	}{
		{http.MethodGet, "/v2/a/b/c/manifests/v1", []string{"repository:a/b/c:pull"}},
		{http.MethodHead, "/v2/p/a--b__c.d/manifests/" + dgst, []string{"repository:p/a--b__c.d:pull"}},
		{http.MethodPut, "/v2/p/a/manifests/v1", []string{"repository:p/a:pull,push"}},
		{http.MethodDelete, "/v2/p/a/manifests/" + dgst, []string{"repository:p/a:*"}},
		{http.MethodGet, "/v2/p/manifests/manifests/v1", []string{"repository:p/manifests:pull"}},
		{http.MethodGet, "/v2/p/a/manifests/b/manifests/v1", []string{"repository:p/a/manifests/b:pull"}},
		{http.MethodGet, "/v2/localhost:5000/a/manifests/v1", []string{"repository:localhost:5000/a:pull"}},
		{http.MethodGet, "/v2/p/a/manifests/v1/", []string{"repository:p/a:pull"}},
		{http.MethodGet, "/v2/p/a/blobs/" + dgst, []string{"repository:p/a:pull"}},
		{http.MethodPost, "/v2/p/a/blobs/uploads/", []string{"repository:p/a:pull,push"}},
		{http.MethodPost, "/v2/p/a/blobs/uploads/?mount=" + dgst + "&from=other/src", []string{"repository:other/src:pull", "repository:p/a:pull,push"}},
		{http.MethodPatch, "/v2/p/a/blobs/uploads/uuid-1", []string{"repository:p/a:pull,push"}},
		{http.MethodGet, "/v2/p/a/tags/list", []string{"repository:p/a:pull"}},
		{http.MethodGet, "/v2/p/a/referrers/" + dgst, []string{"repository:p/a:pull"}},
		{http.MethodGet, "/v2/_catalog", []string{"registry:catalog:*"}},
		{http.MethodGet, "/v2/", nil},
		{http.MethodGet, "/v2/p/a/unknown", nil},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.url, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.url, nil)
			require.NoError(t, err)
			var got []string
			for _, s := range parseScopes(req) {
				got = append(got, s.String())
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
