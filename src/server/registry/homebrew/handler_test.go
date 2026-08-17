// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package homebrew

import (
	"testing"

	"github.com/stretchr/testify/require"

	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		project  string
		kind     routeKind
		upstream string
		ok       bool
	}{
		{
			name:     "formula API",
			path:     "/homebrew/library/api/formula/cmake.json",
			project:  "library",
			kind:     routeAPI,
			upstream: "formula/cmake.json",
			ok:       true,
		},
		{
			name:     "GHCR bottle manifest",
			path:     "/homebrew/library/v2/homebrew/core/cmake/manifests/4.1.0",
			project:  "library",
			kind:     routeArtifact,
			upstream: "v2/homebrew/core/cmake/manifests/4.1.0",
			ok:       true,
		},
		{name: "unscoped legacy path", path: "/homebrew/library/formula/cmake.json"},
		{name: "path traversal", path: "/homebrew/library/api/../secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, kind, upstream, ok := parsePath(tt.path)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.project, project)
			require.Equal(t, tt.kind, kind)
			require.Equal(t, tt.upstream, upstream)
		})
	}
}

func TestBottleProxy(t *testing.T) {
	project := &proModels.Project{Name: "library"}

	preset := bottleProxy(pkgproxy.New(project, &regmodel.Registry{
		Type:       regmodel.RegistryTypeHomebrew,
		URL:        "https://formulae.brew.sh/api",
		Credential: &regmodel.Credential{AccessKey: "metadata-user", AccessSecret: "metadata-password"},
	}))
	require.Equal(t, defaultBottleHost, preset.Registry.URL)
	require.Nil(t, preset.Registry.Credential)

	custom := bottleProxy(pkgproxy.New(project, &regmodel.Registry{
		Type: regmodel.RegistryTypeHomebrewRegistry,
		URL:  "https://brew-mirror.example",
	}))
	require.Equal(t, "https://brew-mirror.example", custom.Registry.URL)
	require.Equal(t, "api/formula/cmake.json", apiUpstreamPath(custom, "formula/cmake.json"))
	require.Equal(t, "formula/cmake.json", apiUpstreamPath(preset, "formula/cmake.json"))
}
