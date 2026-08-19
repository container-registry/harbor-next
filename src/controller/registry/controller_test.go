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
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/testing/mock"
	testingproject "github.com/goharbor/harbor/src/testing/pkg/project"
	testingreg "github.com/goharbor/harbor/src/testing/pkg/reg"
	testingadapter "github.com/goharbor/harbor/src/testing/pkg/reg/adapter"
	testingrep "github.com/goharbor/harbor/src/testing/pkg/replication"
)

type registryTestSuite struct {
	suite.Suite
	ctl     *controller
	repMgr  *testingrep.Manager
	regMgr  *testingreg.Manager
	proMgr  *testingproject.Manager
	adapter *testingadapter.Adapter
}

func (r *registryTestSuite) SetupTest() {
	r.repMgr = &testingrep.Manager{}
	r.regMgr = &testingreg.Manager{}
	r.proMgr = &testingproject.Manager{}
	r.adapter = &testingadapter.Adapter{}
	r.ctl = &controller{
		repMgr: r.repMgr,
		regMgr: r.regMgr,
		proMgr: r.proMgr,
	}
}

func (r *registryTestSuite) TestValidate() {
	// empty name
	registry := &model.Registry{
		Name: "",
	}
	err := r.ctl.validate(nil, registry)
	r.NotNil(err)

	// empty URL
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "",
	}
	err = r.ctl.validate(nil, registry)
	r.NotNil(err)

	// URL with FTP scheme
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "ftp://example.com",
	}
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Healthy, nil)
	err = r.ctl.validate(nil, registry)
	r.Nil(err)

	// URL without scheme
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "example.com",
	}
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Healthy, nil)
	err = r.ctl.validate(nil, registry)
	r.Nil(err)
	r.Equal("http://example.com", registry.URL)
	r.regMgr.AssertExpectations(r.T())
	r.adapter.AssertExpectations(r.T())

	r.SetupTest()

	// URL with HTTP scheme
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "http://example.com",
	}
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Healthy, nil)
	err = r.ctl.validate(nil, registry)
	r.Nil(err)
	r.Equal("http://example.com", registry.URL)
	r.regMgr.AssertExpectations(r.T())
	r.adapter.AssertExpectations(r.T())

	r.SetupTest()

	// unhealthy
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "http://example.com",
	}
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Unhealthy, nil)
	err = r.ctl.validate(nil, registry)
	r.NotNil(err)
	r.regMgr.AssertExpectations(r.T())
	r.adapter.AssertExpectations(r.T())

	r.SetupTest()

	// URL with HTTPS scheme
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "https://example.com",
	}
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Healthy, nil)
	err = r.ctl.validate(nil, registry)
	r.Nil(err)
	r.Equal("https://example.com", registry.URL)
	r.regMgr.AssertExpectations(r.T())
	r.adapter.AssertExpectations(r.T())

	r.SetupTest()

	// URL with query string
	registry = &model.Registry{
		Name: "endpoint01",
		URL:  "http://example.com/redirect?key=value",
	}
	mock.OnAnything(r.regMgr, "CreateAdapter").Return(r.adapter, nil)
	mock.OnAnything(r.adapter, "HealthCheck").Return(model.Healthy, nil)
	err = r.ctl.validate(nil, registry)
	r.Nil(err)
	r.Equal("http://example.com/redirect", registry.URL)
	r.regMgr.AssertExpectations(r.T())
	r.adapter.AssertExpectations(r.T())
}

func (r *registryTestSuite) TestDelete() {
	// referenced by replication policy
	mock.OnAnything(r.repMgr, "Count").Return(int64(1), nil)
	err := r.ctl.Delete(nil, 1)
	r.NotNil(err)
	r.repMgr.AssertExpectations(r.T())

	r.SetupTest()

	// referenced by proxy cache project
	mock.OnAnything(r.repMgr, "Count").Return(int64(0), nil)
	mock.OnAnything(r.proMgr, "Count").Return(int64(1), nil)
	err = r.ctl.Delete(nil, 1)
	r.NotNil(err)
	r.repMgr.AssertExpectations(r.T())
	r.proMgr.AssertExpectations(r.T())

	r.SetupTest()

	// pass
	mock.OnAnything(r.repMgr, "Count").Return(int64(0), nil)
	mock.OnAnything(r.proMgr, "Count").Return(int64(0), nil)
	mock.OnAnything(r.regMgr, "Delete").Return(nil)
	err = r.ctl.Delete(nil, 1)
	r.Nil(err)
	r.repMgr.AssertExpectations(r.T())
	r.proMgr.AssertExpectations(r.T())
}

func (r *registryTestSuite) TestGetWhitelistedAdapters() {
	tests := []struct {
		name     string
		input    string
		expected map[string]struct{}
	}{
		{
			name:     "adapter empty",
			input:    "",
			expected: nil,
		},
		{
			name:  "adapters with spaces",
			input: "dockerhub, aws, gcr  ",
			expected: map[string]struct{}{
				"dockerhub": {},
				"aws":       {},
				"gcr":       {},
			},
		},
		{
			name:  "adapters with empty entries",
			input: "harbor, , quay,",
			expected: map[string]struct{}{
				"harbor": {},
				"quay":   {},
			},
		},
		{
			name:  "adapters all",
			input: "ali-acr,aws-ecr,azure-acr,docker-hub,google-gcr,harbor,huawei-SWR,jfrog-artifactory,tencent-tcr,volcengine-cr",
			expected: map[string]struct{}{
				"ali-acr":           {},
				"aws-ecr":           {},
				"azure-acr":         {},
				"docker-hub":        {},
				"google-gcr":        {},
				"harbor":            {},
				"huawei-SWR":        {},
				"jfrog-artifactory": {},
				"tencent-tcr":       {},
				"volcengine-cr":     {},
			},
		},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			conf := map[string]any{
				common.ReplicationAdapterWhiteList: tt.input,
			}
			config.InitWithSettings(conf)
			result := getWhitelistedAdapters(context.TODO())
			r.Equal(tt.expected, result)
		})
	}
}

func (r *registryTestSuite) TestSFTPRegistryRejectedWhenFeatureDisabled() {
	r.T().Setenv("HARBOR_ENABLE_COMMERCIAL_SFTP_REPLICATION", "false")

	err := r.ctl.validate(context.Background(), &model.Registry{
		Name: "sftp-endpoint",
		Type: model.RegistryTypeSFTP,
		URL:  "sftp://example.com",
	})

	r.Error(err)
	r.Contains(err.Error(), "commercial feature sftp_replication is not enabled")
	r.regMgr.AssertNotCalled(r.T(), "CreateAdapter")
}

func (r *registryTestSuite) TestSFTPProviderHiddenWhenFeatureDisabled() {
	r.T().Setenv("HARBOR_ENABLE_COMMERCIAL_SFTP_REPLICATION", "false")
	config.InitWithSettings(map[string]any{
		common.ReplicationAdapterWhiteList: "harbor,sftp",
	})
	mock.OnAnything(r.regMgr, "ListRegistryProviderTypes").Return([]string{"harbor", "sftp"}, nil)
	mock.OnAnything(r.regMgr, "ListRegistryProviderInfos").Return(map[string]*model.AdapterPattern{
		"harbor": {},
		"sftp":   {},
	}, nil)

	types, err := r.ctl.ListRegistryProviderTypes(context.Background())
	r.NoError(err)
	r.Equal([]string{"harbor"}, types)

	infos, err := r.ctl.ListRegistryProviderInfos(context.Background())
	r.NoError(err)
	r.Contains(infos, "harbor")
	r.NotContains(infos, "sftp")
}

func (r *registryTestSuite) TestSFTPProviderVisibleWhenFeatureEnabled() {
	r.T().Setenv("HARBOR_ENABLE_COMMERCIAL_SFTP_REPLICATION", "true")
	config.InitWithSettings(map[string]any{
		common.ReplicationAdapterWhiteList: "harbor,sftp",
	})
	mock.OnAnything(r.regMgr, "ListRegistryProviderTypes").Return([]string{"harbor", "sftp"}, nil)

	types, err := r.ctl.ListRegistryProviderTypes(context.Background())
	r.NoError(err)
	r.Equal([]string{"harbor", "sftp"}, types)
}

func (r *registryTestSuite) TestSFTPDisableGuardRejectsExistingEndpoints() {
	mock.OnAnything(r.regMgr, "Count").Return(int64(2), nil)

	err := (sftpReplicationDisableGuard{regMgr: r.regMgr}).Validate(context.Background())

	r.Error(err)
	r.Contains(err.Error(), "cannot be disabled while 2 SFTP registry endpoint(s) exist")
}

func (r *registryTestSuite) TestSFTPDisableGuardAllowsNoEndpoints() {
	mock.OnAnything(r.regMgr, "Count").Return(int64(0), nil)

	err := (sftpReplicationDisableGuard{regMgr: r.regMgr}).Validate(context.Background())

	r.NoError(err)
}

func TestRegistryTestSuite(t *testing.T) {
	suite.Run(t, &registryTestSuite{})
}
