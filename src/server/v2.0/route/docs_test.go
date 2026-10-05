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

package route

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestDocsMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		enabled    bool
		path       string
		wantServed bool
	}{
		{name: "disabled by default", path: "/api/v2.0/docs", wantServed: false},
		{name: "disabled, trailing slash", path: "/api/v2.0/docs/", wantServed: false},
		{name: "enabled", enabled: true, path: "/api/v2.0/docs", wantServed: true},
		{name: "enabled, trailing slash", enabled: true, path: "/api/v2.0/docs/", wantServed: true},
		{name: "another API path is untouched", path: "/api/v2.0/projects", wantServed: true},
		{name: "a path that only starts alike is untouched", path: "/api/v2.0/docsomething", wantServed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config.InitWithSettings(map[string]any{common.APIDocsEnable: c.enabled})

			served := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				served = true
				w.WriteHeader(http.StatusOK)
			})

			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080"+c.path, nil)
			require.Nil(t, err)
			rec := httptest.NewRecorder()
			docsMiddleware()(next).ServeHTTP(rec, req)

			assert.Equal(t, c.wantServed, served)
			if !c.wantServed {
				assert.Equal(t, http.StatusNotFound, rec.Result().StatusCode)
				assert.Contains(t, rec.Body.String(), "NOT_FOUND")
			}
		})
	}
}
