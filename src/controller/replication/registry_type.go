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

package replication

import (
	"context"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/feature"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
)

// checkRegistryTypeEnabled rejects a registry whose type belongs to a disabled feature.
func checkRegistryTypeEnabled(ctx context.Context, registry *regmodel.Registry) error {
	if registry == nil || feature.RegistryTypeEnabled(ctx, registry.Type) {
		return nil
	}
	gate, _ := feature.RegistryTypeFeature(registry.Type)
	return errors.ForbiddenError(nil).WithMessagef("registry type %s needs the %s feature to be enabled", registry.Type, gate)
}
