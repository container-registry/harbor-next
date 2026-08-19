// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pkgpolicy

import (
	"testing"

	projectmodel "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
)

func TestValidatePush(t *testing.T) {
	tests := []struct {
		name         string
		project      *projectmodel.Project
		registry     *regmodel.Registry
		registryType string
		wantErr      bool
	}{
		{
			name:         "hosted project",
			project:      &projectmodel.Project{Name: "hosted"},
			registryType: regmodel.RegistryTypeNPM,
		},
		{
			name:         "push enabled npm proxy",
			project:      proxyProject(true),
			registry:     &regmodel.Registry{Type: regmodel.RegistryTypeNPM},
			registryType: regmodel.RegistryTypeNPM,
		},
		{
			name:         "push enabled npmjs proxy uses npm protocol",
			project:      proxyProject(true),
			registry:     &regmodel.Registry{Type: regmodel.RegistryTypeNPMJS},
			registryType: regmodel.RegistryTypeNPM,
		},
		{
			name:         "read only proxy",
			project:      proxyProject(false),
			registry:     &regmodel.Registry{Type: regmodel.RegistryTypeNPM},
			registryType: regmodel.RegistryTypeNPM,
			wantErr:      true,
		},
		{
			name:         "wrong package type",
			project:      proxyProject(true),
			registry:     &regmodel.Registry{Type: regmodel.RegistryTypeMaven},
			registryType: regmodel.RegistryTypeNPM,
			wantErr:      true,
		},
		{
			name:         "missing registry",
			project:      proxyProject(true),
			registryType: regmodel.RegistryTypeNPM,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePush(tt.project, tt.registry, tt.registryType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePush() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateOCIPush(t *testing.T) {
	tests := []struct {
		name     string
		project  *projectmodel.Project
		registry *regmodel.Registry
		wantErr  bool
	}{
		{name: "hosted project", project: &projectmodel.Project{Name: "hosted"}},
		{
			name:     "push enabled OCI proxy",
			project:  proxyProject(true),
			registry: &regmodel.Registry{Type: regmodel.RegistryTypeDockerRegistry},
		},
		{
			name:     "read only OCI proxy",
			project:  proxyProject(false),
			registry: &regmodel.Registry{Type: regmodel.RegistryTypeDockerRegistry},
			wantErr:  true,
		},
		{
			name:     "npm proxy",
			project:  proxyProject(true),
			registry: &regmodel.Registry{Type: regmodel.RegistryTypeNPM},
			wantErr:  true,
		},
		{
			name:     "npmjs proxy",
			project:  proxyProject(true),
			registry: &regmodel.Registry{Type: regmodel.RegistryTypeNPMJS},
			wantErr:  true,
		},
		{name: "missing registry", project: proxyProject(true), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOCIPush(tt.project, tt.registry)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOCIPush() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func proxyProject(allowPush bool) *projectmodel.Project {
	return &projectmodel.Project{
		Name:       "proxy",
		RegistryID: 1,
		Metadata: map[string]string{
			projectmodel.ProMetaProxyCacheAllowPush: boolString(allowPush),
		},
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
