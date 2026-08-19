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

package model

import "testing"

func TestRegistryTypesCompatible(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{name: "npmjs and npm", left: RegistryTypeNPMJS, right: RegistryTypeNPM, want: true},
		{name: "PyPI and generic PyPI", left: RegistryTypePyPI, right: RegistryTypePyPIRegistry, want: true},
		{name: "Maven Central and Maven", left: RegistryTypeMavenCentral, right: RegistryTypeMaven, want: true},
		{name: "crates.io and Cargo", left: RegistryTypeCratesIO, right: RegistryTypeCargo, want: true},
		{name: "Go proxy and custom Go registry", left: RegistryTypeGo, right: RegistryTypeGoRegistry, want: true},
		{name: "Homebrew and generic Homebrew", left: RegistryTypeHomebrew, right: RegistryTypeHomebrewRegistry, want: true},
		{name: "different protocols", left: RegistryTypeNPMJS, right: RegistryTypeMaven, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RegistryTypesCompatible(tt.left, tt.right); got != tt.want {
				t.Errorf("RegistryTypesCompatible(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestIsPackageRegistryType(t *testing.T) {
	packageTypes := []string{
		RegistryTypeNPM,
		RegistryTypeNPMJS,
		RegistryTypePyPI,
		RegistryTypePyPIRegistry,
		RegistryTypeMaven,
		RegistryTypeMavenCentral,
		RegistryTypeCargo,
		RegistryTypeCratesIO,
		RegistryTypeGo,
		RegistryTypeGoRegistry,
		RegistryTypeGoSumDB,
		RegistryTypeHomebrew,
		RegistryTypeHomebrewRegistry,
	}
	for _, registryType := range packageTypes {
		if !IsPackageRegistryType(registryType) {
			t.Errorf("IsPackageRegistryType(%q) = false, want true", registryType)
		}
	}
	if IsPackageRegistryType(RegistryTypeDockerHub) {
		t.Errorf("IsPackageRegistryType(%q) = true, want false", RegistryTypeDockerHub)
	}
}
