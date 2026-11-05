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

package registry

import (
	"context"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/pkg/reg"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/huggingface"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func init() {
	feature.RegisterDisableGuard(huggingface.Feature, registriesOfTypeGuard(reg.Mgr, model.RegistryTypeHuggingFace))
}

// CheckTypeEnabled returns a bad request error when registryType belongs to a
// disabled optional feature.
func CheckTypeEnabled(ctx context.Context, registryType string) error {
	if feature.RegistryTypeEnabled(ctx, registryType) {
		return nil
	}
	return errors.New(nil).WithCode(errors.BadRequestCode).
		WithMessagef("the registry type %s is disabled by an optional feature, see Configuration > Features", registryType)
}

// registriesOfTypeGuard refuses disabling a feature while registries of its
// type exist. Replication policies and proxy cache projects reference a
// registry and block its deletion, so the registry count covers them too.
func registriesOfTypeGuard(mgr reg.Manager, registryType string) feature.DisableGuard {
	return func(ctx context.Context) error {
		count, err := mgr.Count(ctx, q.New(q.KeyWords{"type": registryType}))
		if err != nil {
			return err
		}
		if count > 0 {
			return errors.New(nil).WithCode(errors.PreconditionCode).
				WithMessagef("the feature is still used by %d %s registry endpoint(s), delete them before disabling it", count, registryType)
		}
		return nil
	}
}
