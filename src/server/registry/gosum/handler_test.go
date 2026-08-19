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

package gosum

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

func TestHandlerPersistsCustomChecksumResponse(t *testing.T) {
	const body = "signed lookup response\n"
	var upstreamDisabled atomic.Bool
	var upstreamRequests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		require.Equal(t, "/lookup/github.com/!azure/module@v1.0.0", r.URL.Path)
		if upstreamDisabled.Load() {
			http.Error(w, "disabled", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, _ = io.WriteString(w, body)
	}))
	defer upstream.Close()

	proxy := pkgproxy.New(
		&proModels.Project{Name: "checksums"},
		&regmodel.Registry{Name: "corporate-sumdb", URL: upstream.URL, Type: regmodel.RegistryTypeGoSumDB, Status: regmodel.Healthy},
	)
	h := newHandlerWithDeps(
		NewMirror(newMemoryStore()),
		func(http.ResponseWriter, *http.Request, string) (int64, bool) { return 9, true },
		func(_ context.Context, project, registryType string) (*pkgproxy.Proxy, error) {
			require.Equal(t, "checksums", project)
			require.Equal(t, regmodel.RegistryTypeGoSumDB, registryType)
			return proxy, nil
		},
	)

	path := "/go-sumdb/checksums/lookup/github.com/%21azure/module@v1.0.0"
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, body, first.Body.String())
	require.Equal(t, "text/plain; charset=UTF-8", first.Header().Get("Content-Type"))

	upstreamDisabled.Store(true)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, body, second.Body.String())
	require.Equal(t, int64(1), upstreamRequests.Load())

	head := httptest.NewRecorder()
	h.ServeHTTP(head, httptest.NewRequest(http.MethodHead, path, nil))
	require.Equal(t, http.StatusOK, head.Code)
	require.Empty(t, head.Body.String())
	require.Equal(t, "23", head.Header().Get("Content-Length"))
}

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		want    *request
		wantErr bool
	}{
		{name: "lookup", method: http.MethodGet, path: "/go-sumdb/checksums/lookup/github.com/%21azure/module@v1.0.0", want: &request{Project: "checksums", Path: "lookup/github.com/!azure/module@v1.0.0"}},
		{name: "complete tile", method: http.MethodGet, path: "/go-sumdb/checksums/tile/8/0/x001/002", want: &request{Project: "checksums", Path: "tile/8/0/x001/002"}},
		{name: "partial tile", method: http.MethodHead, path: "/go-sumdb/checksums/tile/8/data/001.p/7", want: &request{Project: "checksums", Path: "tile/8/data/001.p/7"}},
		{name: "latest", method: http.MethodGet, path: "/go-sumdb/checksums/latest", want: &request{Project: "checksums", Path: "latest"}},
		{name: "post", method: http.MethodPost, path: "/go-sumdb/checksums/latest", wantErr: true},
		{name: "traversal", method: http.MethodGet, path: "/go-sumdb/checksums/lookup/../latest", wantErr: true},
		{name: "noncanonical tile", method: http.MethodGet, path: "/go-sumdb/checksums/tile/8/0/1", wantErr: true},
		{name: "unknown endpoint", method: http.MethodGet, path: "/go-sumdb/checksums/supported", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRequest(tt.method, tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
