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

package storage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
)

func serve(t *testing.T, h http.Handler, method string) (*httptest.ResponseRecorder, *imagestorage.VolumeInfo) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/api/registry/storage", nil))
	info := &imagestorage.VolumeInfo{}
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), info))
	}
	return rec, info
}

func TestFilesystemIsMeasured(t *testing.T) {
	rec, info := serve(t, NewHandler("filesystem", t.TempDir()), http.MethodGet)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "filesystem", info.Driver)
	assert.True(t, info.Supported)
	assert.NotEmpty(t, info.Path)
	assert.Greater(t, info.Total, uint64(0))
	assert.Equal(t, info.Total-info.Free, info.Used)
	assert.False(t, info.MeasuredAt.IsZero())
}

func TestObjectStoreIsUnsupported(t *testing.T) {
	rec, info := serve(t, NewHandler("s3", ""), http.MethodGet)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "s3", info.Driver)
	assert.False(t, info.Supported)
	assert.Zero(t, info.Total)
	assert.True(t, info.MeasuredAt.IsZero())
}

func TestMethodNotAllowed(t *testing.T) {
	rec, _ := serve(t, NewHandler("filesystem", t.TempDir()), http.MethodDelete)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
