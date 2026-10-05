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

package blob

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/registryctl/api/registry/test"
)

// GC sweeps storage through this handler, so pin exactly which files a blob
// deletion removes and which status codes GC sees.
func TestDeleteBlobStorageScope(t *testing.T) {
	all := test.AllFixturePaths()
	sort.Strings(all)

	cases := []struct {
		name       string
		method     string
		ref        string
		wantStatus int
		wantFiles  []string
	}{
		{
			name:       "removes only the blob data, keeps shard sibling and repo links",
			method:     http.MethodDelete,
			ref:        test.DigestA,
			wantStatus: http.StatusOK,
			wantFiles:  test.Without(all, test.BlobA),
		},
		{
			name:       "missing blob is 404 so GC can ignore it",
			method:     http.MethodDelete,
			ref:        test.DigestD,
			wantStatus: http.StatusNotFound,
			wantFiles:  all,
		},
		{
			name:       "invalid digest deletes nothing",
			method:     http.MethodDelete,
			ref:        "sha256:nothex",
			wantStatus: http.StatusInternalServerError,
			wantFiles:  all,
		},
		{
			name:       "empty reference is rejected",
			method:     http.MethodDelete,
			ref:        "",
			wantStatus: http.StatusBadRequest,
			wantFiles:  all,
		},
		{
			name:       "only DELETE is served",
			method:     http.MethodGet,
			ref:        test.DigestA,
			wantStatus: http.StatusMethodNotAllowed,
			wantFiles:  all,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := inmemory.New()
			test.SeedFiles(t, d, all)

			req, err := http.NewRequest(tc.method, "http://api/registry/blob/"+tc.ref, nil)
			require.NoError(t, err)
			req = mux.SetURLVars(req, map[string]string{"reference": tc.ref})
			rec := httptest.NewRecorder()
			NewHandler(d).ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantFiles, test.ListFiles(t, d, test.StorageRoot))
		})
	}
}
