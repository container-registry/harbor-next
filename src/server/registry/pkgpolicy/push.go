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

// Package pkgpolicy enforces project policy for native package protocols.
package pkgpolicy

import (
	"context"

	projectctl "github.com/goharbor/harbor/src/controller/project"
	registryctl "github.com/goharbor/harbor/src/controller/registry"
	"github.com/goharbor/harbor/src/lib/errors"
	projectmodel "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
)

// AuthorizePush verifies that a native package can be published to projectName.
func AuthorizePush(ctx context.Context, projectName, registryType string) error {
	project, err := projectctl.Ctl.GetByName(ctx, projectName, projectctl.Metadata(true))
	if err != nil {
		return err
	}
	if !project.IsProxy() {
		return nil
	}
	if !project.ProxyCacheAllowPush() {
		return pushDenied(project.Name, "client publishing is disabled")
	}

	registry, err := registryctl.Ctl.Get(ctx, project.RegistryID)
	if err != nil {
		return err
	}
	return ValidatePush(project, registry, registryType)
}

// ValidatePush validates an already resolved project and upstream registry.
func ValidatePush(project *projectmodel.Project, registry *regmodel.Registry, registryType string) error {
	if project == nil {
		return errors.NotFoundError(nil).WithMessage("project not found")
	}
	if !project.IsProxy() {
		return nil
	}
	if !project.ProxyCacheAllowPush() {
		return pushDenied(project.Name, "client publishing is disabled")
	}
	if registry == nil || !regmodel.RegistryTypesCompatible(registry.Type, registryType) {
		return pushDenied(project.Name, "package type does not match the configured upstream registry")
	}
	return nil
}

// ValidateOCIPush verifies that an OCI artifact can be published to a project.
func ValidateOCIPush(project *projectmodel.Project, registry *regmodel.Registry) error {
	if project == nil {
		return errors.NotFoundError(nil).WithMessage("project not found")
	}
	if !project.IsProxy() {
		return nil
	}
	if !project.ProxyCacheAllowPush() {
		return pushDenied(project.Name, "client publishing is disabled")
	}
	if registry == nil || isNativePackageRegistry(registry.Type) {
		return pushDenied(project.Name, "artifact type does not match the configured upstream registry")
	}
	return nil
}

func isNativePackageRegistry(registryType string) bool {
	switch regmodel.CanonicalRegistryType(registryType) {
	case regmodel.RegistryTypeNPM,
		regmodel.RegistryTypePyPI,
		regmodel.RegistryTypeMaven,
		regmodel.RegistryTypeCargo,
		regmodel.RegistryTypeGo,
		regmodel.RegistryTypeGoSumDB,
		regmodel.RegistryTypeHomebrew:
		return true
	default:
		return false
	}
}

func pushDenied(project, reason string) error {
	return errors.DeniedError(nil).WithMessagef("cannot publish to proxy project %s: %s", project, reason)
}
