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

// Package feature centralizes optional feature gates. A feature registers its
// Definition from its own file, checks Enabled at its API and runtime
// boundaries, and may register a guard that prevents disabling it while its
// persisted state is still in use.
package feature

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/goharbor/harbor/src/lib/config"
)

// Feature identifies an optional capability.
type Feature string

// Definition describes the configuration key and environment override of a feature.
type Definition struct {
	Feature   Feature
	ConfigKey string
	EnvKey    string
}

// DisableGuard returns an error when persisted feature state prevents disabling it.
type DisableGuard func(context.Context) error

var (
	mu               sync.RWMutex
	definitions      []Definition
	guards           = map[Feature][]DisableGuard{}
	registryFeatures = map[string]Feature{}
)

// Register adds feature definitions. Call it from an init function in the
// feature's own file, next to the matching config metadata item.
func Register(defs ...Definition) {
	mu.Lock()
	defer mu.Unlock()
	for _, def := range defs {
		for _, existing := range definitions {
			if existing.Feature == def.Feature || existing.ConfigKey == def.ConfigKey {
				panic(fmt.Sprintf("feature %q or config key %q registered twice", def.Feature, def.ConfigKey))
			}
		}
		definitions = append(definitions, def)
	}
}

// Definitions returns all registered feature definitions.
func Definitions() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Definition(nil), definitions...)
}

// RegisterDisableGuard adds a check run before a feature can be disabled.
// Several packages may guard the same feature; every guard must pass.
func RegisterDisableGuard(feature Feature, guard DisableGuard) {
	if guard == nil {
		panic(fmt.Sprintf("nil disable guard for feature %q", feature))
	}
	mu.Lock()
	defer mu.Unlock()
	guards[feature] = append(guards[feature], guard)
}

// RegisterRegistryTypes associates registry provider types with a feature.
// A registry type can belong to one feature only.
func RegisterRegistryTypes(feature Feature, registryTypes ...string) {
	mu.Lock()
	defer mu.Unlock()
	for _, registryType := range registryTypes {
		if owner, ok := registryFeatures[registryType]; ok && owner != feature {
			panic(fmt.Sprintf("registry type %q already gated by feature %q", registryType, owner))
		}
		registryFeatures[registryType] = feature
	}
}

// RegistryTypeEnabled reports whether registryType is not gated by a feature
// or its controlling feature is enabled.
func RegistryTypeEnabled(ctx context.Context, registryType string) bool {
	mu.RLock()
	feature, gated := registryFeatures[registryType]
	mu.RUnlock()
	return !gated || Enabled(ctx, feature)
}

// Enabled returns the effective state of feature. An explicitly set environment
// variable takes precedence over the administrator-managed configuration.
func Enabled(ctx context.Context, feature Feature) bool {
	definition, ok := definitionFor(func(d Definition) bool { return d.Feature == feature })
	if !ok {
		return false
	}
	if enabled, _, overridden := environmentOverride(definition); overridden {
		return enabled
	}
	mgr := config.DefaultMgr()
	if mgr == nil {
		return false
	}
	value := mgr.Get(ctx, definition.ConfigKey)
	return value != nil && value.GetBool()
}

// ConfigOverride returns the environment value controlling configKey, if any.
// The config API reports it in place of the stored value and marks the item
// read-only, so the UI shows the state the feature actually runs with.
func ConfigOverride(configKey string) (enabled bool, overridden bool) {
	definition, ok := definitionFor(func(d Definition) bool { return d.ConfigKey == configKey })
	if !ok {
		return false, false
	}
	enabled, _, overridden = environmentOverride(definition)
	return enabled, overridden
}

// environmentOverride treats an unparsable value as disabled but still as an
// override, so a typo fails closed and keeps the UI control locked.
func environmentOverride(definition Definition) (enabled bool, envKey string, overridden bool) {
	value, exists := os.LookupEnv(definition.EnvKey)
	if !exists {
		return false, "", false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, definition.EnvKey, true
	}
	return enabled, definition.EnvKey, true
}

// ValidateConfigUpdate rejects changes to environment-controlled features and
// runs the disable guard before an enabled feature is switched off.
func ValidateConfigUpdate(ctx context.Context, cfgs map[string]any) error {
	for _, definition := range Definitions() {
		value, updated := cfgs[definition.ConfigKey]
		if !updated {
			continue
		}
		enabled, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%s must be a boolean", definition.ConfigKey)
		}
		if _, envKey, overridden := environmentOverride(definition); overridden {
			return fmt.Errorf("%s is controlled by %s", definition.ConfigKey, envKey)
		}
		if enabled || !Enabled(ctx, definition.Feature) {
			continue
		}
		mu.RLock()
		featureGuards := append([]DisableGuard(nil), guards[definition.Feature]...)
		mu.RUnlock()
		for _, guard := range featureGuards {
			if err := guard(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func definitionFor(match func(Definition) bool) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, definition := range definitions {
		if match(definition) {
			return definition, true
		}
	}
	return Definition{}, false
}
