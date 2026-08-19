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

package commercial

import (
	"testing"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config/metadata"
)

func TestBrandingEnabledByDefault(t *testing.T) {
	item, ok := metadata.Instance().GetByName(common.EnableCommercialBranding)
	if !ok {
		t.Fatal("commercial branding configuration metadata is missing")
	}
	if got, want := item.DefaultValue, "true"; got != want {
		t.Errorf("commercial branding default = %q, want %q", got, want)
	}
}

func TestEnvironmentOverride(t *testing.T) {
	t.Setenv("HARBOR_ENABLE_COMMERCIAL_BRANDING", "true")

	enabled, overridden := EnvironmentOverride(Branding)
	if !overridden {
		t.Fatal("expected environment override")
	}
	if !enabled {
		t.Fatal("expected branding to be enabled by environment override")
	}
	if ConfigEditable("enable_commercial_branding") {
		t.Fatal("expected environment-controlled configuration to be read-only")
	}
}

func TestInvalidEnvironmentOverrideFailsClosed(t *testing.T) {
	t.Setenv("HARBOR_ENABLE_COMMERCIAL_BRANDING", "not-a-boolean")

	enabled, overridden := EnvironmentOverride(Branding)
	if !overridden {
		t.Fatal("expected invalid environment value to remain an override")
	}
	if enabled {
		t.Fatal("expected invalid environment value to disable the feature")
	}
}

func TestRegistryTypeEnabled(t *testing.T) {
	const registryType = "test-commercial-registry"
	RegisterRegistryTypes(SFTPReplication, registryType)

	t.Setenv("HARBOR_ENABLE_COMMERCIAL_SFTP_REPLICATION", "false")
	if RegistryTypeEnabled(t.Context(), registryType) {
		t.Fatal("expected registered registry type to be disabled")
	}
	if !RegistryTypeEnabled(t.Context(), "unregistered-registry") {
		t.Fatal("expected unregistered registry type to be enabled")
	}
}
