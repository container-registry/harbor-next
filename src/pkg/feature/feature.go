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

// Package feature centralizes feature gates. A feature calls Register once
// from its own file; that single definition provides the configuration item,
// the environment override, the Configuration > Features toggle and the
// features API. Code checks Enabled at its API and runtime boundaries.
//
// The configuration key is also the flag key, so the gates can later be
// served through an OpenFeature provider built on Resolve and OnChange.
package feature

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	"github.com/goharbor/harbor/src/lib/log"
)

// Feature identifies a gated capability.
type Feature string

// OptionalSection is the section a feature lands in when Definition.Section is empty.
const OptionalSection = "optional"

// Definition describes a feature. ConfigKey is the stored configuration key
// and the flag key; EnvKey is the environment variable that overrides it.
// Name and Description go through the portal's translate pipe, so they may
// be i18n keys or plain text.
type Definition struct {
	Feature   Feature
	ConfigKey string
	EnvKey    string
	// LegacyEnvKey is read when EnvKey is unset, for deployments still
	// using a renamed variable.
	LegacyEnvKey string
	Section      string
	Name         string
	Description  string
	Beta         bool
	Default      bool
}

// Section groups features on the Configuration > Features page. Link, when
// set, is resolved on every read so it can reflect the current state.
type Section struct {
	ID          string
	Title       string
	Description string
	LinkText    string
	Link        func(ctx context.Context, enabled []Definition) string
	// Order sorts sections ascending; ties sort by ID.
	Order int
}

// Source tells where an effective value came from.
type Source string

const (
	SourceEnv     Source = "env"
	SourceConfig  Source = "config"
	SourceDefault Source = "default"
)

// DisableGuard returns an error when persisted feature state prevents disabling it.
type DisableGuard func(context.Context) error

var (
	mu               sync.RWMutex
	definitions      []Definition
	sections         = map[string]Section{}
	guards           = map[Feature][]DisableGuard{}
	registryFeatures = map[string]Feature{}
	listeners        []func(configKey string)
)

func init() {
	RegisterSection(Section{ID: OptionalSection, Title: "CONFIG.OPTIONAL_FEATURES", Order: 100})
}

// Register adds feature definitions and their boolean configuration items.
// Call it from an init function in the feature's own file.
func Register(defs ...Definition) {
	mu.Lock()
	defer mu.Unlock()
	for _, def := range defs {
		if def.Feature == "" || def.ConfigKey == "" || def.EnvKey == "" {
			panic(fmt.Sprintf("feature definition %+v needs Feature, ConfigKey and EnvKey", def))
		}
		if def.Section == "" {
			def.Section = OptionalSection
		}
		for _, existing := range definitions {
			if existing.Feature == def.Feature || existing.ConfigKey == def.ConfigKey {
				panic(fmt.Sprintf("feature %q or config key %q registered twice", def.Feature, def.ConfigKey))
			}
		}
		registerConfigItem(def)
		definitions = append(definitions, def)
	}
}

// registerConfigItem tolerates an item a feature declared itself in the
// metadata package, as long as it matches the definition.
func registerConfigItem(def Definition) {
	item, added := metadata.Register(metadata.Item{
		Name:         def.ConfigKey,
		Scope:        metadata.UserScope,
		Group:        metadata.BasicGroup,
		EnvKey:       def.EnvKey,
		DefaultValue: strconv.FormatBool(def.Default),
		ItemType:     &metadata.BoolType{},
		Editable:     true,
		Description:  def.Name,
	})
	if added {
		return
	}
	if _, ok := item.ItemType.(*metadata.BoolType); !ok || item.EnvKey != def.EnvKey {
		panic(fmt.Sprintf("config item %q exists with a different type or env key", def.ConfigKey))
	}
}

// RegisterSection adds or replaces a section of the Features page.
func RegisterSection(section Section) {
	if section.ID == "" {
		panic("feature section needs an ID")
	}
	mu.Lock()
	defer mu.Unlock()
	sections[section.ID] = section
}

// Definitions returns all registered feature definitions.
func Definitions() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Definition(nil), definitions...)
}

// Sections returns the sections that have at least one feature, ordered.
func Sections() []Section {
	mu.RLock()
	defer mu.RUnlock()
	used := map[string]bool{}
	for _, def := range definitions {
		used[def.Section] = true
	}
	result := make([]Section, 0, len(used))
	for id := range used {
		section, ok := sections[id]
		if !ok {
			section = Section{ID: id, Title: id}
		}
		result = append(result, section)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Order != result[j].Order {
			return result[i].Order < result[j].Order
		}
		return result[i].ID < result[j].ID
	})
	return result
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

// RegistryTypeFeature returns the feature gating registryType, if any.
func RegistryTypeFeature(registryType string) (Feature, bool) {
	mu.RLock()
	defer mu.RUnlock()
	feature, gated := registryFeatures[registryType]
	return feature, gated
}

// RegistryTypeEnabled reports whether registryType is not gated by a feature
// or its controlling feature is enabled.
func RegistryTypeEnabled(ctx context.Context, registryType string) bool {
	feature, gated := RegistryTypeFeature(registryType)
	return !gated || Enabled(ctx, feature)
}

// OnChange registers a callback run after an administrator changed the
// stored value of a feature. Environment-controlled features never change.
func OnChange(listener func(configKey string)) {
	mu.Lock()
	defer mu.Unlock()
	listeners = append(listeners, listener)
}

// NotifyChanged runs the OnChange callbacks for every feature key in cfgs.
// The configuration controller calls it after a successful update.
func NotifyChanged(cfgs map[string]any) {
	mu.RLock()
	current := append(([]func(string))(nil), listeners...)
	var changed []string
	for _, def := range definitions {
		if _, ok := cfgs[def.ConfigKey]; ok {
			changed = append(changed, def.ConfigKey)
		}
	}
	mu.RUnlock()
	for _, key := range changed {
		for _, listener := range current {
			listener(key)
		}
	}
}

// Enabled returns the effective state of feature.
func Enabled(ctx context.Context, feature Feature) bool {
	definition, ok := definitionFor(func(d Definition) bool { return d.Feature == feature })
	if !ok {
		return false
	}
	enabled, _ := resolve(ctx, definition)
	return enabled
}

// Resolve returns the effective value of a feature by its configuration key:
// an explicitly set environment variable wins over the stored value, which
// wins over the registered default. known is false for unregistered keys.
func Resolve(ctx context.Context, configKey string) (enabled bool, source Source, known bool) {
	definition, ok := definitionFor(func(d Definition) bool { return d.ConfigKey == configKey })
	if !ok {
		return false, "", false
	}
	enabled, source = resolve(ctx, definition)
	return enabled, source, true
}

func resolve(ctx context.Context, definition Definition) (bool, Source) {
	if enabled, _, overridden := environmentOverride(definition); overridden {
		return enabled, SourceEnv
	}
	mgr := config.DefaultMgr()
	if mgr == nil {
		return definition.Default, SourceDefault
	}
	// pick up a toggle another replica stored; a failed refresh keeps the last value
	if err := mgr.Load(ctx); err != nil {
		log.Warningf("failed to refresh the configuration for feature %s: %v", definition.ConfigKey, err)
	}
	// A key never saved comes back as an empty value, not the item default,
	// because the item may have been registered after the defaults loaded.
	value := mgr.Get(ctx, definition.ConfigKey)
	if value == nil || value.Value == "" {
		return definition.Default, SourceDefault
	}
	enabled, err := strconv.ParseBool(value.Value)
	if err != nil {
		return definition.Default, SourceDefault
	}
	return enabled, SourceConfig
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

// EnvironmentKey returns the variable that currently controls configKey, if any.
func EnvironmentKey(configKey string) (string, bool) {
	definition, ok := definitionFor(func(d Definition) bool { return d.ConfigKey == configKey })
	if !ok {
		return "", false
	}
	_, envKey, overridden := environmentOverride(definition)
	return envKey, overridden
}

// environmentOverride treats an unparsable value as disabled but still as an
// override, so a typo fails closed and keeps the UI control locked.
func environmentOverride(definition Definition) (enabled bool, envKey string, overridden bool) {
	envKey = definition.EnvKey
	value, exists := os.LookupEnv(envKey)
	if !exists && definition.LegacyEnvKey != "" {
		envKey = definition.LegacyEnvKey
		value, exists = os.LookupEnv(envKey)
	}
	if !exists {
		return false, "", false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, envKey, true
	}
	return enabled, envKey, true
}

// ValidateConfigUpdate rejects changes to environment-controlled features and
// runs the disable guards on every update that switches a feature off.
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
		if enabled {
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
