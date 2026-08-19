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

package branding

import (
	"context"

	"github.com/goharbor/harbor/src/pkg/branding/dao"
	model "github.com/goharbor/harbor/src/pkg/branding/models"
)

var (
	// Mgr is a global variable for the default branding manager implementation
	Mgr = NewManager()
)

// Manager ...
type Manager interface {
	// Get ...
	Get(ctx context.Context) (*model.Branding, error)

	// Update updates the branding configuration
	Update(ctx context.Context, params string) (err error)
}

var _ Manager = &manager{}

type manager struct {
	dao dao.DAO
}

// NewManager returns a new instance of the default branding manager
func NewManager() Manager {
	return &manager{
		dao: dao.New(),
	}
}

// Get ...
func (m *manager) Get(ctx context.Context) (*model.Branding, error) {
	return m.dao.Get(ctx)
}

// Update ...
func (m *manager) Update(ctx context.Context, params string) (err error) {
	return m.dao.Update(ctx, params)
}
