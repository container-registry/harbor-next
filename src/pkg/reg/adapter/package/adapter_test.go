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

package packageadapter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func TestGoSumDBHealthCheckUsesLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/latest", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &adapter{
		provider: provider{registryType: model.RegistryTypeGoSumDB, protocolType: model.RegistryTypeGoSumDB},
		registry: &model.Registry{URL: server.URL},
	}
	status, err := adapter.HealthCheck()
	require.NoError(t, err)
	require.Equal(t, model.Healthy, status)
}

func TestProviderPatterns(t *testing.T) {
	tests := []struct {
		registryType string
		protocolType string
		endpoint     string
	}{
		{registryType: model.RegistryTypeNPMJS, protocolType: model.RegistryTypeNPM, endpoint: "https://registry.npmjs.org"},
		{registryType: model.RegistryTypeNPM, protocolType: model.RegistryTypeNPM},
		{registryType: model.RegistryTypeMavenCentral, protocolType: model.RegistryTypeMaven, endpoint: "https://repo.maven.apache.org/maven2"},
		{registryType: model.RegistryTypeMaven, protocolType: model.RegistryTypeMaven},
		{registryType: model.RegistryTypePyPI, protocolType: model.RegistryTypePyPI, endpoint: "https://pypi.org/simple"},
		{registryType: model.RegistryTypePyPIRegistry, protocolType: model.RegistryTypePyPI},
		{registryType: model.RegistryTypeCratesIO, protocolType: model.RegistryTypeCargo, endpoint: "https://index.crates.io"},
		{registryType: model.RegistryTypeCargo, protocolType: model.RegistryTypeCargo},
		{registryType: model.RegistryTypeGo, protocolType: model.RegistryTypeGo, endpoint: "https://proxy.golang.org"},
		{registryType: model.RegistryTypeGoRegistry, protocolType: model.RegistryTypeGo},
		{registryType: model.RegistryTypeHomebrew, protocolType: model.RegistryTypeHomebrew, endpoint: "https://formulae.brew.sh/api"},
		{registryType: model.RegistryTypeHomebrewRegistry, protocolType: model.RegistryTypeHomebrew},
	}

	providers := make(map[string]provider, len(packageProviders))
	for _, provider := range packageProviders {
		providers[provider.registryType] = provider
	}
	for _, tt := range tests {
		t.Run(tt.registryType, func(t *testing.T) {
			provider, ok := providers[tt.registryType]
			require.True(t, ok)
			require.Equal(t, tt.protocolType, provider.protocolType)

			pattern := (&factory{provider: provider}).AdapterPattern()
			require.Equal(t, model.EndpointPatternTypeStandard, pattern.EndpointPattern.EndpointType)
			require.Nil(t, pattern.CredentialPattern)
			if tt.endpoint == "" {
				require.Empty(t, pattern.EndpointPattern.Endpoints)
				return
			}
			require.Len(t, pattern.EndpointPattern.Endpoints, 1)
			require.Equal(t, tt.endpoint, pattern.EndpointPattern.Endpoints[0].Value)
		})
	}
}
