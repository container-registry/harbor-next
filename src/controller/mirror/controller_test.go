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
	"github.com/stretchr/testify/mock"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/controller/repository"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	"github.com/goharbor/harbor/src/pkg/repository/model"
	repotesting "github.com/goharbor/harbor/src/testing/controller/repository"
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

// useMirrorConfig installs mirror settings on the in-memory config manager
// and puts the previous manager and the defaults back when the test ends.
func useMirrorConfig(t *testing.T, enabled bool, namespaces string) {
	t.Helper()
	prev := config.DefaultCfgManager
	config.InitWithSettings(map[string]any{
		common.RegistryMirrorEnabled:    enabled,
		common.RegistryMirrorNamespaces: namespaces,
	})
	t.Cleanup(func() {
		_ = config.DefaultMgr().UpdateConfig(context.Background(), map[string]any{
			common.RegistryMirrorEnabled:    false,
			common.RegistryMirrorNamespaces: "",
		})
		config.DefaultCfgManager = prev
	})
}

func TestRoute(t *testing.T) {

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
			useMirrorConfig(t, c.enabled, c.namespaces)
			ctx := context.TODO()
			got, ok := Route(ctx, c.repo, c.ns)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.wantOK, ok)
		})
	}
}

func TestValidateNamespaces(t *testing.T) {
	cases := []struct {
		raw     string
		want    map[string]string
		wantErr bool
	}{
		{raw: "", want: map[string]string{}},
		{raw: " docker.io = dockerhub , *=dockerhub ", want: map[string]string{"docker.io": "dockerhub", "*": "dockerhub"}},
		{raw: "docker.io", wantErr: true},
		{raw: "=dockerhub", wantErr: true},
		{raw: "docker io=dockerhub", wantErr: true},
		{raw: "docker.io=a,docker.io=b", wantErr: true},
		{raw: "docker.io=Not A Project", wantErr: true},
	}
	for _, c := range cases {
		got, err := ValidateNamespaces(c.raw)
		if c.wantErr {
			assert.Error(t, err, c.raw)
			continue
		}
		assert.NoError(t, err, c.raw)
		assert.Equal(t, c.want, got, c.raw)
	}
}

func TestRepoExists(t *testing.T) {
	orig := repository.Ctl
	t.Cleanup(func() { repository.Ctl = orig })
	ctl := &repotesting.Controller{}
	repository.Ctl = ctl
	ctx := context.TODO()

	ctl.On("GetByName", mock.Anything, "team/app").Return(&model.RepoRecord{Name: "team/app"}, nil)
	ctl.On("GetByName", mock.Anything, "library/nginx").Return(nil, errors.NotFoundError(nil))
	ctl.On("GetByName", mock.Anything, "team/broken").Return(nil, errors.New("db down"))

	assert.True(t, repoExists(ctx, "team/app"))
	assert.False(t, repoExists(ctx, "library/nginx"))
	assert.True(t, repoExists(ctx, "team/broken"), "an unknown lookup result keeps the pull local")
}
