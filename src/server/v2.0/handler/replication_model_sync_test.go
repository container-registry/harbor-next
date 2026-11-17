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

package handler

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	repctlmodel "github.com/goharbor/harbor/src/controller/replication/model"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	replicationtesting "github.com/goharbor/harbor/src/testing/controller/replication"
	"github.com/goharbor/harbor/src/testing/mock"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

type replicationModelSyncSuite struct {
	htesting.Suite
	ctl *replicationtesting.Controller
}

func (s *replicationModelSyncSuite) SetupSuite() {
	s.ctl = &replicationtesting.Controller{}
	s.Config = &restapi.Config{ReplicationAPI: &replicationAPI{ctl: s.ctl}}
	s.Suite.SetupSuite()
}

func (s *replicationModelSyncSuite) SetupTest() {
	s.ctl.ExpectedCalls = nil
	s.ctl.Calls = nil
	s.Security.ExpectedCalls = nil
	s.Security.Calls = nil
	s.Security.On("IsAuthenticated").Return(true).Maybe()
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Maybe()
	s.Security.On("GetUsername").Return("admin").Maybe()
}

func (s *replicationModelSyncSuite) policyBody() *models.ReplicationPolicy {
	revisionTag := true
	return &models.ReplicationPolicy{
		Name:           "smol",
		SrcRegistry:    &models.Registry{ID: 2},
		DestNamespace:  "library",
		DestRepository: "smol",
		RevisionTag:    &revisionTag,
		Trigger:        &models.ReplicationTrigger{Type: model.TriggerTypeManual},
	}
}

func (s *replicationModelSyncSuite) TestCreateAndGetCarryModelSyncFields() {
	var created *repctlmodel.Policy
	s.ctl.On("CreatePolicy", mock.Anything, mock.Anything).Return(int64(5), nil).Once().
		Run(func(args testifymock.Arguments) { created = args.Get(1).(*repctlmodel.Policy) })

	res, err := s.PostJSON("/replication/policies", s.policyBody())
	s.Require().NoError(err)
	s.Equal(201, res.StatusCode)
	s.Require().NotNil(created)
	s.True(created.RevisionTag)
	s.Equal("smol", created.DestRepository)

	created.ID = 5
	mock.OnAnything(s.ctl, "GetPolicy").Return(created, nil).Once()
	got := &models.ReplicationPolicy{}
	res, err = s.GetJSON("/replication/policies/5", got)
	s.Require().NoError(err)
	s.Equal(200, res.StatusCode)
	s.Require().NotNil(got.RevisionTag)
	s.True(*got.RevisionTag)
	s.Equal("smol", got.DestRepository)
}

func (s *replicationModelSyncSuite) TestUpdateCarriesModelSyncFields() {
	mock.OnAnything(s.ctl, "GetPolicy").Return(&repctlmodel.Policy{ID: 5, Name: "smol"}, nil).Maybe()
	var updated *repctlmodel.Policy
	s.ctl.On("UpdatePolicy", mock.Anything, mock.Anything).Return(nil).Once().
		Run(func(args testifymock.Arguments) { updated = args.Get(1).(*repctlmodel.Policy) })

	res, err := s.PutJSON("/replication/policies/5", s.policyBody())
	s.Require().NoError(err)
	s.Equal(200, res.StatusCode)
	s.Require().NotNil(updated)
	s.True(updated.RevisionTag)
	s.Equal("smol", updated.DestRepository)
}

func TestReplicationModelSyncSuite(t *testing.T) {
	suite.Run(t, &replicationModelSyncSuite{})
}
