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

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

const (
	testFeature   Feature = "test_optional_feature"
	testConfigKey         = "enable_test_optional_feature"
	testEnvKey            = "HARBOR_ENABLE_TEST_OPTIONAL_FEATURE"

	defaultOnFeature      Feature = "test_default_on_feature"
	defaultOnConfigKey            = "enable_test_default_on_feature"
	defaultOnEnvKey               = "HARBOR_ENABLE_TEST_DEFAULT_ON_FEATURE"
	defaultOnLegacyEnvKey         = "HARBOR_ENABLE_TEST_DEFAULT_ON_LEGACY"
)

func init() {
	Register(
		Definition{Feature: testFeature, ConfigKey: testConfigKey, EnvKey: testEnvKey, Name: "Test", Beta: true},
		Definition{Feature: defaultOnFeature, ConfigKey: defaultOnConfigKey, EnvKey: defaultOnEnvKey, LegacyEnvKey: defaultOnLegacyEnvKey, Section: "test_section", Default: true},
	)
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

func storeConfig(t *testing.T, cfgs map[string]any) {
	t.Helper()
	unsetEnv(t, testEnvKey)
	unsetEnv(t, defaultOnEnvKey)
	unsetEnv(t, defaultOnLegacyEnvKey)
	config.InitWithSettings(cfgs)
}

func storeEnabled(t *testing.T, enabled bool) {
	t.Helper()
	storeConfig(t, map[string]any{testConfigKey: enabled})
}

func TestRegisterCreatesBooleanConfigItem(t *testing.T) {
	item, ok := metadata.Instance().GetByName(testConfigKey)
	if !ok {
		t.Fatal("Register did not create the configuration item")
	}
	if _, ok := item.ItemType.(*metadata.BoolType); !ok || item.EnvKey != testEnvKey || item.Scope != metadata.UserScope {
		t.Fatalf("unexpected item %+v", item)
	}
}

func TestRegisterAcceptsMatchingPreexistingItem(t *testing.T) {
	metadata.Register(metadata.Item{Name: "enable_test_predeclared", EnvKey: "HARBOR_ENABLE_TEST_PREDECLARED", ItemType: &metadata.BoolType{}, Scope: metadata.UserScope})
	Register(Definition{Feature: "test_predeclared", ConfigKey: "enable_test_predeclared", EnvKey: "HARBOR_ENABLE_TEST_PREDECLARED"})

	defer func() {
		if recover() == nil {
			t.Fatal("expected a conflicting preexisting item to panic")
		}
	}()
	metadata.Register(metadata.Item{Name: "enable_test_conflict", ItemType: &metadata.StringType{}})
	Register(Definition{Feature: "test_conflict", ConfigKey: "enable_test_conflict", EnvKey: "HARBOR_ENABLE_TEST_CONFLICT"})
}

func TestResolveSources(t *testing.T) {
	storeConfig(t, map[string]any{})
	if enabled, source, known := Resolve(t.Context(), testConfigKey); !known || enabled || source != SourceDefault {
		t.Fatalf("unsaved, default off: (%v, %v, %v)", enabled, source, known)
	}
	if enabled, source, _ := Resolve(t.Context(), defaultOnConfigKey); !enabled || source != SourceDefault {
		t.Fatalf("unsaved, default on: (%v, %v)", enabled, source)
	}

	storeConfig(t, map[string]any{defaultOnConfigKey: false})
	if enabled, source, _ := Resolve(t.Context(), defaultOnConfigKey); enabled || source != SourceConfig {
		t.Fatalf("stored false overrides default on: (%v, %v)", enabled, source)
	}
	if Enabled(t.Context(), defaultOnFeature) {
		t.Fatal("Enabled disagrees with Resolve")
	}

	t.Setenv(defaultOnEnvKey, "true")
	if enabled, source, _ := Resolve(t.Context(), defaultOnConfigKey); !enabled || source != SourceEnv {
		t.Fatalf("env overrides stored value: (%v, %v)", enabled, source)
	}

	if _, _, known := Resolve(t.Context(), "not_a_feature"); known {
		t.Fatal("unregistered key reported as known")
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
			if envKey, ok := EnvironmentKey(testConfigKey); !ok || envKey != testEnvKey {
				t.Fatalf("EnvironmentKey = (%q, %v)", envKey, ok)
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

func resetGuards(feature Feature) {
	mu.Lock()
	defer mu.Unlock()
	delete(guards, feature)
}

func TestValidateConfigUpdateRunsDisableGuards(t *testing.T) {
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
	// a replica may still read false after another one enabled the feature
	storeEnabled(t, false)
	if err := ValidateConfigUpdate(t.Context(), map[string]any{testConfigKey: false}); !errors.Is(err, inUse) {
		t.Fatalf("disabling with a stale false: error = %v, want the guards to run", err)
	}
}

func TestRegisterDisableGuardRejectsNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a nil guard to panic")
		}
	}()
	RegisterDisableGuard(testFeature, nil)
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
	if feature, gated := RegistryTypeFeature(registryType); !gated || feature != testFeature {
		t.Fatalf("RegistryTypeFeature = (%q, %v)", feature, gated)
	}
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

func TestRegisterRejectsDuplicatesAndIncompleteDefinitions(t *testing.T) {
	for name, def := range map[string]Definition{
		"duplicate":  {Feature: testFeature, ConfigKey: "other_key", EnvKey: "OTHER"},
		"no env key": {Feature: "test_no_env", ConfigKey: "enable_test_no_env"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected Register to panic")
				}
			}()
			Register(def)
		})
	}
}

func TestSectionsAreOrderedAndOnlyNonEmpty(t *testing.T) {
	RegisterSection(Section{ID: "test_empty", Title: "Empty", Order: 1})
	RegisterSection(Section{ID: "test_section", Title: "Test Section", Order: 50})

	var ids []string
	for _, section := range Sections() {
		ids = append(ids, section.ID)
	}
	if strings.Contains(strings.Join(ids, ","), "test_empty") {
		t.Fatalf("empty section listed: %v", ids)
	}
	want := []string{"test_section", OptionalSection}
	got := []string{}
	for _, id := range ids {
		if id == "test_section" || id == OptionalSection {
			got = append(got, id)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("section order = %v, want %v", got, want)
	}
	for _, def := range Definitions() {
		if def.Feature == testFeature && def.Section != OptionalSection {
			t.Fatalf("definition without section landed in %q", def.Section)
		}
	}
}

func TestNotifyChangedCallsListenersForFeatureKeysOnly(t *testing.T) {
	var changed []string
	OnChange(func(key string) { changed = append(changed, key) })
	t.Cleanup(func() {
		mu.Lock()
		listeners = listeners[:len(listeners)-1]
		mu.Unlock()
	})

	NotifyChanged(map[string]any{testConfigKey: true, "auth_mode": "db_auth"})
	if strings.Join(changed, ",") != testConfigKey {
		t.Fatalf("changed = %v, want only %s", changed, testConfigKey)
	}
}

func TestLegacyEnvironmentKey(t *testing.T) {
	storeConfig(t, map[string]any{defaultOnConfigKey: true})
	t.Setenv(defaultOnLegacyEnvKey, "false")
	if enabled, source, _ := Resolve(t.Context(), defaultOnConfigKey); enabled || source != SourceEnv {
		t.Fatalf("legacy variable: (%v, %v), want (false, env)", enabled, source)
	}
	if envKey, _ := EnvironmentKey(defaultOnConfigKey); envKey != defaultOnLegacyEnvKey {
		t.Fatalf("EnvironmentKey = %q, want the legacy variable that is set", envKey)
	}
	t.Setenv(defaultOnEnvKey, "true")
	if !Enabled(t.Context(), defaultOnFeature) {
		t.Fatal("the current variable must win over the legacy one")
	}
}
