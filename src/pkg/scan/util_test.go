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

package scan

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/pkg/robot/model"
	v1sq "github.com/goharbor/harbor/src/pkg/scan/rest/v1"
)

func TestGenAccessoryArt(t *testing.T) {
	server := httptest.NewServer(registry.New(registry.WithReferrersSupport(true)))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	sq := v1sq.ScanRequest{
		Registry: &v1sq.Registry{
			URL: u.Host,
		},
		Artifact: &v1sq.Artifact{
			Repository: "library/hello-world",
			Tag:        "latest",
			Size:       1234,
			MimeType:   "application/vnd.docker.distribution.manifest.v2+json",
			Digest:     "sha256:d37ada95d47ad12224c205a938129df7a3e52345828b4fa27b03a98825d1e2e7",
		},
	}
	r := &model.Robot{
		Name:   "admin",
		Secret: "Harbor12345",
	}

	annotations := map[string]string{
		"created-by": "trivy",
		"org.opencontainers.artifact.description": "SPDX JSON SBOM",
	}
	s, err := GenAccessoryArt(sq, []byte(`{"name": "harborAccTest", "version": "1.0"}`), annotations, "application/vnd.goharbor.harbor.main.v1", r)
	assert.Nil(t, err)
	assert.Equal(t, "sha256:a39c6456d3cd1d87b7ee5706f67133d7a6d27a2dbc9ed66d50e504ff8920efc3", s)
}

func TestAccessoryRef(t *testing.T) {
	const dgst = "sha256:13b9614bcc1fe99b09889f037935912fab2d98196d6090aef1719aef0511ceb4"

	cases := []struct {
		name         string
		registryURL  string
		repository   string
		insecure     bool
		wantRegistry string
		wantScheme   string
	}{
		{
			// A single-label host without a port is what the in-cluster
			// CORE_URL resolves to when the chart omits the service port.
			// name.ParseReference would classify it as a Docker Hub namespace.
			name:         "portless single-label host stays the registry",
			registryURL:  "harbor-core",
			repository:   "library/app",
			insecure:     true,
			wantRegistry: "harbor-core",
			wantScheme:   "http",
		},
		{
			name:         "host with port",
			registryURL:  "harbor-core:80",
			repository:   "library/app",
			insecure:     true,
			wantRegistry: "harbor-core:80",
			wantScheme:   "http",
		},
		{
			name:         "fqdn without port, secure",
			registryURL:  "harbor.example.com",
			repository:   "library/app",
			insecure:     false,
			wantRegistry: "harbor.example.com",
			wantScheme:   "https",
		},
		{
			// Harbor project names may contain dots; the repository must
			// never be re-parsed for a registry component.
			name:         "dotted project name is not mistaken for a registry",
			registryURL:  "harbor-core",
			repository:   "my.project/app",
			insecure:     true,
			wantRegistry: "harbor-core",
			wantScheme:   "http",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := accessoryRef(tc.registryURL, tc.repository, dgst, tc.insecure)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantRegistry, ref.Context().RegistryStr())
			assert.NotEqual(t, "index.docker.io", ref.Context().RegistryStr())
			assert.Equal(t, tc.repository, ref.Context().RepositoryStr())
			assert.Equal(t, tc.wantScheme, ref.Context().Registry.Scheme())
			assert.Equal(t, dgst, ref.Identifier())
		})
	}
}

func TestAccessoryRefInvalidRegistry(t *testing.T) {
	_, err := accessoryRef("harbor core with spaces", "library/app",
		"sha256:13b9614bcc1fe99b09889f037935912fab2d98196d6090aef1719aef0511ceb4", false)
	assert.Error(t, err)
}
