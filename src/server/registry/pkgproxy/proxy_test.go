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

package pkgproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
)

func TestCredentialsAreScopedToRegistryOrigin(t *testing.T) {
	var registryAuth string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		registryAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	var artifactAuth string
	artifact := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		artifactAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer artifact.Close()

	proxy := New(&proModels.Project{}, &regmodel.Registry{
		URL: registry.URL,
		Credential: &regmodel.Credential{
			AccessKey:    "registry-user",
			AccessSecret: "registry-password",
		},
	})

	_, err := proxy.Get(t.Context(), "index", nil)
	require.NoError(t, err)
	require.NotEmpty(t, registryAuth)

	_, err = proxy.Get(t.Context(), artifact.URL+"/package", nil)
	require.NoError(t, err)
	require.Empty(t, artifactAuth, "registry credentials leaked to a cross-origin artifact URL")
}

func TestGetPreservesEscapedPathSegments(t *testing.T) {
	var escapedPath string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	proxy := New(&proModels.Project{}, &regmodel.Registry{URL: registry.URL})
	_, err := proxy.Get(t.Context(), "%40angular%2Fcli", nil)
	require.NoError(t, err)
	require.Equal(t, "/%40angular%2Fcli", escapedPath)
}

func TestGetPreservesTrailingSlashWithRegistryBasePath(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/simple/demo/" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	proxy := New(&proModels.Project{}, &regmodel.Registry{URL: registry.URL + "/simple"})
	_, err := proxy.Get(t.Context(), "demo/", nil)
	require.NoError(t, err)
}

func TestGetUsesPackageProxyUserAgent(t *testing.T) {
	var userAgent string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.UserAgent()
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	proxy := New(&proModels.Project{}, &regmodel.Registry{URL: registry.URL})
	_, err := proxy.Get(t.Context(), "artifact", nil)
	require.NoError(t, err)
	require.Equal(t, defaultUserAgent, userAgent)
}

func TestGetPreservesCallerUserAgent(t *testing.T) {
	var userAgent string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.UserAgent()
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	proxy := New(&proModels.Project{}, &regmodel.Registry{URL: registry.URL})
	_, err := proxy.Get(t.Context(), "artifact", http.Header{"User-Agent": {"Apache-Maven/3.9.11"}})
	require.NoError(t, err)
	require.Equal(t, "Apache-Maven/3.9.11", userAgent)
}

func TestGetOCINegotiatesBearerAuthentication(t *testing.T) {
	const token = "homebrew-token"
	var serverURL string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+serverURL+`/token",service="ghcr.test"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			require.Equal(t, "ghcr.test", r.URL.Query().Get("service"))
			require.Equal(t, "repository:homebrew/core/cmake:pull", r.URL.Query().Get("scope"))
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"token": token}))
		case "/v2/homebrew/core/cmake/manifests/4.1.0":
			require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			_, _ = w.Write([]byte(`{"schemaVersion":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()
	serverURL = registry.URL

	proxy := New(&proModels.Project{}, &regmodel.Registry{URL: registry.URL})
	resp, err := proxy.GetOCI(t.Context(), "v2/homebrew/core/cmake/manifests/4.1.0", nil)
	require.NoError(t, err)
	require.Equal(t, "application/vnd.oci.image.manifest.v1+json", resp.ContentType)
	require.JSONEq(t, `{"schemaVersion":2}`, string(resp.Body))
}
