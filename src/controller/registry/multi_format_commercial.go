// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package registry

import (
	"context"
	"fmt"

	"github.com/goharbor/harbor/src/pkg/commercial"
	"github.com/goharbor/harbor/src/pkg/reg"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func init() {
	commercial.RegisterRegistryTypes(commercial.MultiFormatArtifacts,
		model.RegistryTypeNPM,
		model.RegistryTypeNPMJS,
		model.RegistryTypePyPI,
		model.RegistryTypePyPIRegistry,
		model.RegistryTypeMaven,
		model.RegistryTypeMavenCentral,
		model.RegistryTypeCargo,
		model.RegistryTypeCratesIO,
		model.RegistryTypeGo,
		model.RegistryTypeGoRegistry,
		model.RegistryTypeGoSumDB,
		model.RegistryTypeHomebrew,
		model.RegistryTypeHomebrewRegistry,
	)
	commercial.RegisterDisableGuard(commercial.MultiFormatArtifacts, multiFormatDisableGuard{regMgr: reg.Mgr}.Validate)
}

type multiFormatDisableGuard struct {
	regMgr reg.Manager
}

func (g multiFormatDisableGuard) Validate(ctx context.Context) error {
	registries, err := g.regMgr.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("list package registry endpoints: %w", err)
	}
	count := 0
	for _, registry := range registries {
		if model.IsPackageRegistryType(registry.Type) {
			count++
		}
	}
	if count > 0 {
		return fmt.Errorf("multi-format artifacts cannot be disabled while %d package registry endpoint(s) exist", count)
	}
	return nil
}
