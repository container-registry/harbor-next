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
	"fmt"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/pkg/reg"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func init() {
	feature.RegisterDisableGuard(feature.ModelSync, modelSyncDisableGuard{regMgr: reg.Mgr}.Validate)
}

// modelSyncDisableGuard keeps Model Sync on while Hugging Face endpoints exist,
// so their replication rules and proxy cache projects are not left behind.
type modelSyncDisableGuard struct {
	regMgr reg.Manager
}

func (g modelSyncDisableGuard) Validate(ctx context.Context) error {
	count, err := g.regMgr.Count(ctx, &q.Query{
		Keywords: map[string]any{
			"Type": model.RegistryTypeHuggingFace,
		},
	})
	if err != nil {
		return fmt.Errorf("count Hugging Face registry endpoints: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("model sync cannot be disabled while %d Hugging Face registry endpoint(s) exist", count)
	}
	return nil
}
