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

package exporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveStatus(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	InitHarborClient(&HarborClient{
		HarborScheme: "http",
		HarborHost:   u.Hostname(),
		HarborPort:   port,
		Client:       srv.Client(),
	})
}

// A non-2xx response must fail rather than decode into a zero value: decoding a
// 401 body would publish harbor_health 0 and an empty auth_mode as if they were
// real observations.
func TestRESTBackendFailsClosedOnNon2xx(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError, http.StatusNotFound} {
		serveStatus(t, status, `{"status":"healthy"}`)

		health, err := NewRESTBackend().Health(context.Background())
		assert.Errorf(t, err, "status %d must be an error", status)
		assert.Nil(t, health)

		info, err := NewRESTBackend().SystemInfo(context.Background())
		assert.Errorf(t, err, "status %d must be an error", status)
		assert.Nil(t, info)
	}
}

func TestRESTBackendDecodesOn2xx(t *testing.T) {
	serveStatus(t, http.StatusOK, `{"status":"healthy","components":[{"name":"core","status":"healthy"}]}`)

	health, err := NewRESTBackend().Health(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "healthy", health.Status)
	require.Len(t, health.Components, 1)
	assert.Equal(t, "core", health.Components[0].Name)
}
