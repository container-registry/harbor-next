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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/testing/mock"
	testingproject "github.com/goharbor/harbor/src/testing/pkg/project"
	testingreg "github.com/goharbor/harbor/src/testing/pkg/reg"
	testingrep "github.com/goharbor/harbor/src/testing/pkg/replication"
)

func TestPackageRegistryRejectedWhenMultiFormatDisabled(t *testing.T) {
	t.Setenv("HARBOR_ENABLE_COMMERCIAL_MULTI_FORMAT_ARTIFACTS", "false")
	regMgr := &testingreg.Manager{}
	controller := &controller{
		regMgr: regMgr,
		repMgr: &testingrep.Manager{},
		proMgr: &testingproject.Manager{},
	}

	err := controller.validate(context.Background(), &model.Registry{
		Name: "npmjs",
		Type: model.RegistryTypeNPMJS,
		URL:  "https://registry.npmjs.org",
	})

	assert.ErrorContains(t, err, "commercial feature multi_format_artifacts is not enabled")
	regMgr.AssertNotCalled(t, "CreateAdapter")
}

func TestPackageProvidersFollowMultiFormatFeature(t *testing.T) {
	config.InitWithSettings(map[string]any{
		common.ReplicationAdapterWhiteList: "harbor,npmjs,maven-central,go-registry",
	})
	providerTypes := []string{"harbor", "npmjs", "maven-central", "go-registry"}
	regMgr := &testingreg.Manager{}
	mock.OnAnything(regMgr, "ListRegistryProviderTypes").Return(providerTypes, nil)
	controller := &controller{regMgr: regMgr}

	t.Setenv("HARBOR_ENABLE_COMMERCIAL_MULTI_FORMAT_ARTIFACTS", "false")
	types, err := controller.ListRegistryProviderTypes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"harbor"}, types)

	t.Setenv("HARBOR_ENABLE_COMMERCIAL_MULTI_FORMAT_ARTIFACTS", "true")
	types, err = controller.ListRegistryProviderTypes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, providerTypes, types)
}

func TestMultiFormatDisableGuardRejectsExistingEndpoints(t *testing.T) {
	regMgr := &testingreg.Manager{}
	mock.OnAnything(regMgr, "List").Return([]*model.Registry{
		{Type: model.RegistryTypeDockerHub},
		{Type: model.RegistryTypeNPMJS},
		{Type: model.RegistryTypeGoRegistry},
	}, nil)

	err := (multiFormatDisableGuard{regMgr: regMgr}).Validate(context.Background())

	assert.ErrorContains(t, err, "cannot be disabled while 2 package registry endpoint(s) exist")
}
