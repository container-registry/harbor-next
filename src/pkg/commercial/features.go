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

// Package commercial centralizes commercial feature gates. Feature packages use
// Enabled at their API and runtime boundaries and may register a guard that
// prevents disabling a feature while its persisted state is still in use.
package commercial

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
)

// Feature identifies a commercial capability.
type Feature string

const (
	Branding             Feature = "branding"
	SFTPReplication      Feature = "sftp_replication"
	IdentityProviders    Feature = "identity_providers"
	PGXMonitoring        Feature = "pgx_monitoring"
	AWSRDSIAMAuth        Feature = "aws_rds_iam_auth"
	MultiFormatArtifacts Feature = "multi_format_artifacts"
	AuditLogOTLP         Feature = "audit_log_otlp"
)

// Definition describes the configuration and environment override for a feature.
type Definition struct {
	Feature   Feature
	ConfigKey string
	EnvKey    string
}

var definitions = []Definition{
	{Feature: Branding, ConfigKey: common.EnableCommercialBranding, EnvKey: "HARBOR_ENABLE_COMMERCIAL_BRANDING"},
	{Feature: SFTPReplication, ConfigKey: common.EnableCommercialSFTPReplication, EnvKey: "HARBOR_ENABLE_COMMERCIAL_SFTP_REPLICATION"},
	{Feature: IdentityProviders, ConfigKey: common.EnableCommercialIdentityProviders, EnvKey: "HARBOR_ENABLE_COMMERCIAL_IDENTITY_PROVIDERS"},
	{Feature: PGXMonitoring, ConfigKey: common.EnableCommercialPGXMonitoring, EnvKey: "HARBOR_ENABLE_COMMERCIAL_PGX_MONITORING"},
	{Feature: AWSRDSIAMAuth, ConfigKey: common.EnableCommercialAWSRDSIAMAuth, EnvKey: "HARBOR_ENABLE_COMMERCIAL_AWS_RDS_IAM_AUTH"},
	{Feature: MultiFormatArtifacts, ConfigKey: common.EnableCommercialMultiFormat, EnvKey: "HARBOR_ENABLE_COMMERCIAL_MULTI_FORMAT_ARTIFACTS"},
	{Feature: AuditLogOTLP, ConfigKey: common.EnableCommercialAuditLogOTLP, EnvKey: "HARBOR_ENABLE_COMMERCIAL_AUDIT_LOG_OTLP"},
}

// DisableGuard returns an error when persisted feature state prevents disabling it.
type DisableGuard func(context.Context) error

var (
	guardsMu         sync.RWMutex
	guards           = map[Feature]DisableGuard{}
	registryFeatures = map[string]Feature{}
)

// Definitions returns all commercial feature definitions.
func Definitions() []Definition {
	return append([]Definition(nil), definitions...)
}

// RegisterDisableGuard registers the check used before a feature can be disabled.
func RegisterDisableGuard(feature Feature, guard DisableGuard) {
	guardsMu.Lock()
	defer guardsMu.Unlock()
	guards[feature] = guard
}

// RegisterRegistryTypes associates registry provider types with a commercial feature.
func RegisterRegistryTypes(feature Feature, registryTypes ...string) {
	guardsMu.Lock()
	defer guardsMu.Unlock()
	for _, registryType := range registryTypes {
		registryFeatures[registryType] = feature
	}
}

// RegistryTypeFeature returns the commercial feature controlling registryType.
func RegistryTypeFeature(registryType string) (Feature, bool) {
	guardsMu.RLock()
	defer guardsMu.RUnlock()
	feature, ok := registryFeatures[registryType]
	return feature, ok
}

// RegistryTypeEnabled reports whether registryType is not commercially gated
// or its controlling feature is enabled.
func RegistryTypeEnabled(ctx context.Context, registryType string) bool {
	feature, gated := RegistryTypeFeature(registryType)
	return !gated || Enabled(ctx, feature)
}

// Enabled returns the effective state for feature. An explicitly set environment
// variable takes precedence over the administrator-managed configuration.
func Enabled(ctx context.Context, feature Feature) bool {
	definition, ok := definitionForFeature(feature)
	if !ok {
		return false
	}
	if enabled, overridden := EnvironmentOverride(feature); overridden {
		return enabled
	}
	mgr := config.DefaultMgr()
	if mgr == nil {
		return false
	}
	value := mgr.Get(ctx, definition.ConfigKey)
	return value != nil && value.GetBool()
}

// EnvironmentOverride returns the explicitly configured environment value for
// feature. Invalid values fail closed while still locking the UI control.
func EnvironmentOverride(feature Feature) (bool, bool) {
	definition, ok := definitionForFeature(feature)
	if !ok {
		return false, false
	}
	value, exists := os.LookupEnv(definition.EnvKey)
	if !exists {
		return false, false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, true
	}
	return enabled, true
}

// ConfigEditable reports whether configKey can be changed through the UI.
func ConfigEditable(configKey string) bool {
	definition, ok := definitionForConfigKey(configKey)
	if !ok {
		return true
	}
	_, overridden := EnvironmentOverride(definition.Feature)
	return !overridden
}

// ValidateConfigUpdate rejects environment-controlled changes and checks that a
// feature is safe to disable before its persisted configuration is changed.
func ValidateConfigUpdate(ctx context.Context, cfgs map[string]any) error {
	for _, definition := range definitions {
		value, updated := cfgs[definition.ConfigKey]
		if !updated {
			continue
		}
		enabled, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%s must be a boolean", definition.ConfigKey)
		}
		if !ConfigEditable(definition.ConfigKey) {
			return fmt.Errorf("%s is controlled by %s", definition.ConfigKey, definition.EnvKey)
		}
		if enabled || !Enabled(ctx, definition.Feature) {
			continue
		}
		guardsMu.RLock()
		guard := guards[definition.Feature]
		guardsMu.RUnlock()
		if guard != nil {
			if err := guard(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func definitionForFeature(feature Feature) (Definition, bool) {
	for _, definition := range definitions {
		if definition.Feature == feature {
			return definition, true
		}
	}
	return Definition{}, false
}

func definitionForConfigKey(configKey string) (Definition, bool) {
	for _, definition := range definitions {
		if definition.ConfigKey == configKey {
			return definition, true
		}
	}
	return Definition{}, false
}
