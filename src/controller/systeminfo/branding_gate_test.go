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

package systeminfo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/branding/dao"
	model "github.com/goharbor/harbor/src/pkg/branding/models"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	mdl "github.com/goharbor/harbor/src/server/v2.0/models"
)

type fakeBrandingManager struct {
	getCalls    int
	updateCalls int
}

func (m *fakeBrandingManager) Get(context.Context) (*model.Branding, error) {
	m.getCalls++
	return &model.Branding{Config: dao.DefaultBrandingJSON}, nil
}

func (m *fakeBrandingManager) Update(context.Context, string) error {
	m.updateCalls++
	return nil
}

func TestBrandingDisabledReturnsNeutralConfig(t *testing.T) {
	config.InitWithSettings(map[string]any{common.EnableCommercialBranding: false})
	mgr := &fakeBrandingManager{}
	ctl := &controller{brandMgr: mgr}

	branding, err := ctl.GetBrandingConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, &mdl.BrandingConfig{}, branding)
	require.Zero(t, mgr.getCalls)
}

func TestBrandingDisabledRejectsUpdates(t *testing.T) {
	config.InitWithSettings(map[string]any{common.EnableCommercialBranding: false})
	mgr := &fakeBrandingManager{}
	ctl := &controller{brandMgr: mgr}

	err := ctl.UpdateBrandingConfig(context.Background(), &mdl.BrandingConfig{})

	require.True(t, errors.IsErr(err, errors.ForbiddenCode))
	require.Zero(t, mgr.updateCalls)
}

func TestBrandingEnabledReturnsConfiguredBranding(t *testing.T) {
	config.InitWithSettings(map[string]any{common.EnableCommercialBranding: true})
	mgr := &fakeBrandingManager{}
	ctl := &controller{brandMgr: mgr}

	branding, err := ctl.GetBrandingConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, "8gears Container Registry", *branding.Product.Name)
	require.Equal(t, 1, mgr.getCalls)
}
