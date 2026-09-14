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

package systemquota

import (
	"context"

	"github.com/goharbor/harbor/src/pkg/systemquota/dao"
	"github.com/goharbor/harbor/src/pkg/systemquota/model"
)

// Mgr is the default manager.
var Mgr = New()

// Manager manages the global storage quota.
type Manager interface {
	// Get returns the global quota, or a NotFound error when none is set.
	Get(ctx context.Context) (*model.SystemQuota, error)
	// Upsert creates or replaces the global quota.
	Upsert(ctx context.Context, quota *model.SystemQuota) error
	// Delete unsets the global quota.
	Delete(ctx context.Context) error
	// ProjectAllocation aggregates the per-project hard limits.
	ProjectAllocation(ctx context.Context) (*model.ProjectAllocation, error)
}

// New creates a manager backed by the default DAO.
func New() Manager {
	return &manager{dao: dao.New()}
}

type manager struct {
	dao dao.DAO
}

func (m *manager) Get(ctx context.Context) (*model.SystemQuota, error) {
	return m.dao.Get(ctx)
}

func (m *manager) Upsert(ctx context.Context, quota *model.SystemQuota) error {
	return m.dao.Upsert(ctx, quota)
}

func (m *manager) Delete(ctx context.Context) error {
	return m.dao.Delete(ctx)
}

func (m *manager) ProjectAllocation(ctx context.Context) (*model.ProjectAllocation, error) {
	return m.dao.ProjectAllocation(ctx)
}
