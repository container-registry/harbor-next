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

package mirror

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestParseNamespaces(t *testing.T) {
	cases := []struct {
		raw    string
		expect map[string]string
	}{
		{"", map[string]string{}},
		{"docker.io=dockerhub", map[string]string{"docker.io": "dockerhub"}},
		{" docker.io = dockerhub , ghcr.io=ghcr ", map[string]string{"docker.io": "dockerhub", "ghcr.io": "ghcr"}},
		{"*=dockerhub", map[string]string{"*": "dockerhub"}},
		{"docker.io", map[string]string{}},
		{"docker.io=", map[string]string{}},
		{"=dockerhub", map[string]string{}},
		{"quay.io:443=quay", map[string]string{"quay.io:443": "quay"}},
	}
	for _, c := range cases {
		assert.Equal(t, c.expect, ParseNamespaces(c.raw), c.raw)
	}
}

// withConfig installs the mirror settings under test and returns a context
// bound to the in-memory config manager.
func withConfig(t *testing.T, enabled bool, namespaces string) context.Context {
	t.Helper()
	ctx := context.TODO()
	config.DefaultMgr().Set(ctx, common.RegistryMirrorEnabled, enabled)
	config.DefaultMgr().Set(ctx, common.RegistryMirrorNamespaces, namespaces)
	return ctx
}

func TestRoute(t *testing.T) {
	config.Init()

	local := map[string]bool{"library/local-only": true, "team/app": true}
	orig := repoExists
	repoExists = func(_ context.Context, name string) bool { return local[name] }
	t.Cleanup(func() { repoExists = orig })

	cases := []struct {
		name       string
		enabled    bool
		namespaces string
		repo       string
		ns         string
		want       string
		wantOK     bool
	}{
		{
			name:       "a namespace hint picks the project mapped to it",
			enabled:    true,
			namespaces: "docker.io=dockerhub,ghcr.io=ghcr",
			repo:       "library/nginx",
			ns:         "docker.io",
			want:       "dockerhub/library/nginx",
			wantOK:     true,
		},
		{
			name:       "a namespace hint wins over a local repository of the same name",
			enabled:    true,
			namespaces: "docker.io=dockerhub",
			repo:       "library/local-only",
			ns:         "docker.io",
			want:       "dockerhub/library/local-only",
			wantOK:     true,
		},
		{
			name:       "an unmapped namespace falls back to the catch-all",
			enabled:    true,
			namespaces: "docker.io=dockerhub,*=upstream",
			repo:       "prometheus/node-exporter",
			ns:         "quay.io",
			want:       "upstream/prometheus/node-exporter",
			wantOK:     true,
		},
		{
			name:       "an unmapped namespace without a catch-all is left alone",
			enabled:    true,
			namespaces: "docker.io=dockerhub",
			repo:       "prometheus/node-exporter",
			ns:         "quay.io",
			want:       "prometheus/node-exporter",
		},
		{
			name:       "a hintless pull of an unknown repository takes the catch-all",
			enabled:    true,
			namespaces: "*=dockerhub",
			repo:       "library/nginx",
			want:       "dockerhub/library/nginx",
			wantOK:     true,
		},
		{
			name:       "a hintless pull of a local repository stays local",
			enabled:    true,
			namespaces: "*=dockerhub",
			repo:       "team/app",
			want:       "team/app",
		},
		{
			name:       "a repository already under the proxy project is not routed twice",
			enabled:    true,
			namespaces: "docker.io=dockerhub",
			repo:       "dockerhub/library/nginx",
			ns:         "docker.io",
			want:       "dockerhub/library/nginx",
		},
		{
			name:       "the proxy project itself is not routed into itself",
			enabled:    true,
			namespaces: "docker.io=dockerhub",
			repo:       "dockerhub",
			ns:         "docker.io",
			want:       "dockerhub",
		},
		{
			name:       "disabled mirror routes nothing",
			enabled:    false,
			namespaces: "*=dockerhub",
			repo:       "library/nginx",
			ns:         "docker.io",
			want:       "library/nginx",
		},
		{
			name:       "an enabled mirror with no mapping routes nothing",
			enabled:    true,
			namespaces: "",
			repo:       "library/nginx",
			ns:         "docker.io",
			want:       "library/nginx",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := withConfig(t, c.enabled, c.namespaces)
			got, ok := Route(ctx, c.repo, c.ns)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.wantOK, ok)
		})
	}
}
