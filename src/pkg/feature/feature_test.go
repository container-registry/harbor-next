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

package feature

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

// The config metadata is frozen once a config manager registers at package
// init, so a test cannot add its own item. The test feature borrows the
// existing boolean read_only item instead.
const (
	testFeature   Feature = "test_optional_feature"
	testConfigKey         = common.ReadOnly
	testEnvKey            = "READ_ONLY"
)

func init() {
	Register(Definition{Feature: testFeature, ConfigKey: testConfigKey, EnvKey: testEnvKey})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

func storeEnabled(t *testing.T, enabled bool) {
	t.Helper()
	unsetEnv(t, testEnvKey)
	config.InitWithSettings(map[string]any{testConfigKey: enabled})
}

func TestEnabledReadsStoredConfig(t *testing.T) {
	unsetEnv(t, testEnvKey)
	storeEnabled(t, true)
	if !Enabled(t.Context(), testFeature) {
		t.Fatal("expected stored true to enable the feature")
	}
	storeEnabled(t, false)
	if Enabled(t.Context(), testFeature) {
		t.Fatal("expected stored false to disable the feature")
	}
}

func TestUnknownFeatureIsDisabled(t *testing.T) {
	if Enabled(t.Context(), "not_registered") {
		t.Fatal("expected unregistered feature to be disabled")
	}
	if _, overridden := ConfigOverride("not_a_feature_key"); overridden {
		t.Fatal("expected keys outside the feature gate to have no override")
	}
}

func TestEnvironmentOverride(t *testing.T) {
	cases := []struct {
		value       string
		wantEnabled bool
	}{
		{value: "true", wantEnabled: true},
		{value: "false"},
		{value: "not-a-boolean"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			storeEnabled(t, !tc.wantEnabled)
			t.Setenv(testEnvKey, tc.value)

			enabled, overridden := ConfigOverride(testConfigKey)
			if !overridden || enabled != tc.wantEnabled {
				t.Fatalf("ConfigOverride = (%v, %v), want (%v, true)", enabled, overridden, tc.wantEnabled)
			}
			if got := Enabled(t.Context(), testFeature); got != tc.wantEnabled {
				t.Fatalf("Enabled = %v, want %v", got, tc.wantEnabled)
			}
		})
	}
}

func TestValidateConfigUpdateRejectsEnvironmentControlledFeature(t *testing.T) {
	t.Setenv(testEnvKey, "true")

	err := ValidateConfigUpdate(t.Context(), map[string]any{testConfigKey: false})
	if err == nil || !strings.Contains(err.Error(), testEnvKey) {
		t.Fatalf("error = %v, want one naming %s", err, testEnvKey)
	}
}

func TestValidateConfigUpdateRejectsNonBoolean(t *testing.T) {
	unsetEnv(t, testEnvKey)
	if err := ValidateConfigUpdate(t.Context(), map[string]any{testConfigKey: "true"}); err == nil {
		t.Fatal("expected a non-boolean value to be rejected")
	}
}

func TestValidateConfigUpdateRunsDisableGuard(t *testing.T) {
	unsetEnv(t, testEnvKey)
	inUse := errors.New("feature still in use")
	passed := 0
	RegisterDisableGuard(testFeature, func(context.Context) error { passed++; return nil })
	RegisterDisableGuard(testFeature, func(context.Context) error { return inUse })
	t.Cleanup(func() { resetGuards(testFeature) })

	storeEnabled(t, true)
	if err := ValidateConfigUpdate(t.Context(), map[string]any{testConfigKey: false}); !errors.Is(err, inUse) {
		t.Fatalf("disabling an enabled feature: error = %v, want the second guard's error", err)
	}
	if passed != 1 {
		t.Fatalf("first guard ran %d times, want 1: a later guard must not replace it", passed)
	}
	if err := ValidateConfigUpdate(t.Context(), map[string]any{testConfigKey: true}); err != nil {
		t.Fatalf("enabling: unexpected error %v", err)
	}
	storeEnabled(t, false)
	if err := ValidateConfigUpdate(t.Context(), map[string]any{testConfigKey: false}); err != nil {
		t.Fatalf("disabling an already disabled feature: unexpected error %v", err)
	}
}

func TestRegistryTypeEnabled(t *testing.T) {
	const registryType = "test-optional-registry"
	RegisterRegistryTypes(testFeature, registryType)

	t.Setenv(testEnvKey, "false")
	if RegistryTypeEnabled(t.Context(), registryType) {
		t.Fatal("expected gated registry type to be disabled")
	}
	t.Setenv(testEnvKey, "true")
	if !RegistryTypeEnabled(t.Context(), registryType) {
		t.Fatal("expected gated registry type to be enabled")
	}
	if !RegistryTypeEnabled(t.Context(), "ungated-registry") {
		t.Fatal("expected ungated registry type to be enabled")
	}
}

func resetGuards(feature Feature) {
	mu.Lock()
	defer mu.Unlock()
	delete(guards, feature)
}

func TestRegisterDisableGuardRejectsNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a nil guard to panic")
		}
	}()
	RegisterDisableGuard(testFeature, nil)
}

func TestRegisterRegistryTypesRejectsSecondOwner(t *testing.T) {
	const registryType = "test-owned-registry"
	RegisterRegistryTypes(testFeature, registryType)
	RegisterRegistryTypes(testFeature, registryType)
	defer func() {
		if recover() == nil {
			t.Fatal("expected a registry type claimed by another feature to panic")
		}
	}()
	RegisterRegistryTypes("other_feature", registryType)
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate registration to panic")
		}
	}()
	Register(Definition{Feature: testFeature, ConfigKey: "other_key", EnvKey: "OTHER"})
}

func TestDefinitionsMatchConfigMetadata(t *testing.T) {
	for _, definition := range Definitions() {
		item, ok := metadata.Instance().GetByName(definition.ConfigKey)
		if !ok {
			t.Errorf("%s: no configuration metadata", definition.ConfigKey)
			continue
		}
		if _, ok := item.ItemType.(*metadata.BoolType); !ok {
			t.Errorf("%s: metadata type is %T, want bool", definition.ConfigKey, item.ItemType)
		}
		if item.EnvKey != definition.EnvKey {
			t.Errorf("%s: metadata env key %q, definition env key %q", definition.ConfigKey, item.EnvKey, definition.EnvKey)
		}
	}
}
