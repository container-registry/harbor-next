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
	"os"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/huggingface"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
	"github.com/goharbor/harbor/src/testing/mock"
)

func (r *registryTestSuite) setHuggingFace(enabled bool) {
	r.T().Setenv(huggingface.EnvKey, "")
	r.Require().NoError(os.Unsetenv(huggingface.EnvKey))
	config.InitWithSettings(map[string]any{
		common.ReplicationAdapterWhiteList: "docker-hub,huggingface",
		huggingface.ConfigKey:              enabled,
	})
}

func (r *registryTestSuite) TestProviderListsHideDisabledType() {
	r.regMgr.On("ListRegistryProviderTypes", mock.Anything).Return([]string{"docker-hub", "huggingface"}, nil)
	r.regMgr.On("ListRegistryProviderInfos", mock.Anything).Return(map[string]*model.AdapterPattern{
		"docker-hub": {}, "huggingface": {},
	}, nil)

	r.setHuggingFace(false)
	types, err := r.ctl.ListRegistryProviderTypes(context.TODO())
	r.Require().NoError(err)
	r.Equal([]string{"docker-hub"}, types)
	infos, err := r.ctl.ListRegistryProviderInfos(context.TODO())
	r.Require().NoError(err)
	r.NotContains(infos, "huggingface")

	r.setHuggingFace(true)
	types, err = r.ctl.ListRegistryProviderTypes(context.TODO())
	r.Require().NoError(err)
	r.Equal([]string{"docker-hub", "huggingface"}, types)
	infos, err = r.ctl.ListRegistryProviderInfos(context.TODO())
	r.Require().NoError(err)
	r.Contains(infos, "huggingface")
}

func (r *registryTestSuite) TestEnvironmentOverridesStoredSetting() {
	r.setHuggingFace(false)
	r.T().Setenv(huggingface.EnvKey, "true")
	r.NoError(CheckTypeEnabled(context.TODO(), model.RegistryTypeHuggingFace))

	r.setHuggingFace(true)
	r.T().Setenv(huggingface.EnvKey, "false")
	r.True(errors.IsErr(CheckTypeEnabled(context.TODO(), model.RegistryTypeHuggingFace), errors.BadRequestCode))
}

func (r *registryTestSuite) TestCreateAndUpdateRejectDisabledType() {
	r.setHuggingFace(false)
	registry := &model.Registry{Name: "hf", Type: model.RegistryTypeHuggingFace, URL: "https://huggingface.co"}

	_, err := r.ctl.Create(context.TODO(), registry)
	r.True(errors.IsErr(err, errors.BadRequestCode))
	r.Contains(err.Error(), "registry type huggingface is disabled")
	r.True(errors.IsErr(r.ctl.Update(context.TODO(), registry), errors.BadRequestCode))
	r.regMgr.AssertNotCalled(r.T(), "CreateAdapter", mock.Anything, mock.Anything)
	r.regMgr.AssertNotCalled(r.T(), "Create", mock.Anything, mock.Anything)
}

func (r *registryTestSuite) TestPingAndInfoRejectDisabledType() {
	r.setHuggingFace(false)
	registry := &model.Registry{ID: 3, Name: "hf", Type: model.RegistryTypeHuggingFace}

	healthy, err := r.ctl.IsHealthy(context.TODO(), registry)
	r.False(healthy)
	r.True(errors.IsErr(err, errors.BadRequestCode))

	mock.OnAnything(r.regMgr, "Get").Return(registry, nil).Once()
	_, err = r.ctl.GetInfo(context.TODO(), 3)
	r.True(errors.IsErr(err, errors.BadRequestCode))
	r.regMgr.AssertNotCalled(r.T(), "CreateAdapter", mock.Anything, mock.Anything)
}

func (r *registryTestSuite) TestPreviewRejectsDisabledType() {
	r.setHuggingFace(false)
	mock.OnAnything(r.regMgr, "Get").Return(&model.Registry{ID: 3, Name: "hf", Type: model.RegistryTypeHuggingFace}, nil).Once()

	_, err := r.ctl.Preview(context.TODO(), 3, "org/model", "main", nil)
	r.True(errors.IsErr(err, errors.BadRequestCode))
	r.regMgr.AssertNotCalled(r.T(), "CreateAdapter", mock.Anything, mock.Anything)
}

func (r *registryTestSuite) TestDisableGuardRefusesWhileRegistriesExist() {
	r.setHuggingFace(true)
	guard := registriesOfTypeGuard(r.regMgr, model.RegistryTypeHuggingFace)

	mock.OnAnything(r.regMgr, "Count").Return(int64(2), nil).Once()
	err := guard(context.TODO())
	r.Require().Error(err)
	r.Contains(err.Error(), "still used by 2 huggingface registry endpoint(s)")

	mock.OnAnything(r.regMgr, "Count").Return(int64(0), nil).Once()
	r.NoError(guard(context.TODO()))
}

// A switch-off another instance committed while Create waited for the
// feature lock must win, even when the local configuration cache still
// reports the feature on.
func (r *registryTestSuite) TestCreateRechecksTypeUnderFeatureLock() {
	r.setHuggingFace(true)
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Healthy, nil)
	var locked feature.Feature
	r.ctl.lockFeature = func(_ context.Context, f feature.Feature) (bool, error) {
		locked = f
		return false, nil
	}

	ctx := orm.NewContext(context.TODO(), &ormtesting.FakeOrmer{})
	_, err := r.ctl.Create(ctx, &model.Registry{Name: "hf", Type: model.RegistryTypeHuggingFace, URL: "https://huggingface.co"})
	r.True(errors.IsErr(err, errors.BadRequestCode), "error = %v", err)
	r.Equal(huggingface.Feature, locked)
	r.regMgr.AssertNotCalled(r.T(), "Create", mock.Anything, mock.Anything)
}
