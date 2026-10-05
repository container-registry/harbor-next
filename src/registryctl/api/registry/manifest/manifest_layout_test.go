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

package manifest

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/registryctl/api/registry/test"
)

// GC removes manifest revisions through this handler; tags are left to the v2
// DELETE API. Pin exactly which files go and which status codes GC sees.
func TestDeleteManifestStorageScope(t *testing.T) {
	all := test.AllFixturePaths()
	sort.Strings(all)

	cases := []struct {
		name       string
		method     string
		repo       string
		ref        string
		query      string
		wantStatus int
		wantFiles  []string
	}{
		{
			name:       "removes only this repo's revision link",
			method:     http.MethodDelete,
			repo:       "library/app",
			ref:        test.DigestC,
			wantStatus: http.StatusOK,
			wantFiles:  test.Without(all, test.AppRevisionC),
		},
		{
			name:       "tags in the query are ignored, tag links stay",
			method:     http.MethodDelete,
			repo:       "library/app",
			ref:        test.DigestC,
			query:      "?tags=v1",
			wantStatus: http.StatusOK,
			wantFiles:  test.Without(all, test.AppRevisionC),
		},
		{
			name:       "same digest in another repo is untouched",
			method:     http.MethodDelete,
			repo:       "library/other",
			ref:        test.DigestC,
			wantStatus: http.StatusOK,
			wantFiles:  test.Without(all, test.OtherRevC),
		},
		{
			name:       "missing revision is 404 so GC can ignore it",
			method:     http.MethodDelete,
			repo:       "library/app",
			ref:        test.DigestD,
			wantStatus: http.StatusNotFound,
			wantFiles:  all,
		},
		{
			name:       "missing repo is 404",
			method:     http.MethodDelete,
			repo:       "library/nope",
			ref:        test.DigestC,
			wantStatus: http.StatusNotFound,
			wantFiles:  all,
		},
		{
			name:       "invalid digest is rejected",
			method:     http.MethodDelete,
			repo:       "library/app",
			ref:        "sha256:nothex",
			wantStatus: http.StatusBadRequest,
			wantFiles:  all,
		},
		{
			name:       "empty repo name is rejected",
			method:     http.MethodDelete,
			repo:       "",
			ref:        test.DigestC,
			wantStatus: http.StatusBadRequest,
			wantFiles:  all,
		},
		{
			name:       "only DELETE is served",
			method:     http.MethodGet,
			repo:       "library/app",
			ref:        test.DigestC,
			wantStatus: http.StatusMethodNotAllowed,
			wantFiles:  all,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := inmemory.New()
			test.SeedFiles(t, d, all)

			req, err := http.NewRequest(tc.method, "http://api/registry/"+tc.repo+"/manifests/"+tc.ref+tc.query, nil)
			require.NoError(t, err)
			req = mux.SetURLVars(req, map[string]string{"name": tc.repo, "reference": tc.ref})
			rec := httptest.NewRecorder()
			NewHandler(d).ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantFiles, test.ListFiles(t, d, test.StorageRoot))
		})
	}
}
