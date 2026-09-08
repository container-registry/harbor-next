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
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/systemquota"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	systemquotatesting "github.com/goharbor/harbor/src/testing/controller/systemquota"
	"github.com/goharbor/harbor/src/testing/mock"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

type SystemQuotaTestSuite struct {
	htesting.Suite
	ctl *systemquotatesting.Controller
}

func (s *SystemQuotaTestSuite) SetupSuite() {
	s.ctl = &systemquotatesting.Controller{}
	s.Config = &restapi.Config{SystemquotaAPI: &systemQuotaAPI{ctl: s.ctl}}
	s.Suite.SetupSuite()
}

func (s *SystemQuotaTestSuite) SetupTest() {
	s.ctl.ExpectedCalls = nil
	s.ctl.Calls = nil
	s.Security.ExpectedCalls = nil
	s.Security.Calls = nil
}

func (s *SystemQuotaTestSuite) asAdmin() {
	s.Security.On("IsAuthenticated").Return(true)
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)
	s.Security.On("GetUsername").Return("admin")
}

func (s *SystemQuotaTestSuite) asUser() {
	s.Security.On("IsAuthenticated").Return(true)
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(false)
	s.Security.On("GetUsername").Return("user")
}

func (s *SystemQuotaTestSuite) TestGetAnonymous() {
	s.Security.On("IsAuthenticated").Return(false)
	res, err := s.Get("/system/quota")
	s.NoError(err)
	s.Equal(401, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestGetForbiddenForNonAdmin() {
	s.asUser()
	res, err := s.Get("/system/quota")
	s.NoError(err)
	s.Equal(403, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestGetNotSet() {
	s.asAdmin()
	s.ctl.On("Get", mock.Anything).Return(nil, errors.NotFoundError(nil))
	res, err := s.Get("/system/quota")
	s.NoError(err)
	s.Equal(404, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestGet() {
	s.asAdmin()
	at := time.Now()
	s.ctl.On("Get", mock.Anything).Return(&systemquota.Status{
		Hard: 1000, Used: 400, Free: 600, UsedSource: systemquota.UsedSourceMeasured, MeasuredAt: &at,
		Allocated: 900, UnlimitedProjects: 3, Enforce: true, UpdateTime: at,
	}, nil)

	var payload models.SystemQuota
	res, err := s.GetJSON("/system/quota", &payload)
	s.NoError(err)
	s.Equal(200, res.StatusCode)
	s.Equal(int64(1000), payload.Hard["storage"])
	s.Equal(int64(400), payload.Used["storage"])
	s.Equal(int64(600), payload.Free)
	s.Equal("measured", payload.UsedSource)
	s.Equal(int64(900), payload.Allocated)
	s.Equal(int64(3), payload.UnlimitedProjects)
	s.True(payload.Enforce)
	s.False(time.Time(payload.MeasuredAt).IsZero())
}

func (s *SystemQuotaTestSuite) TestUpdate() {
	s.asAdmin()
	s.ctl.On("Update", mock.Anything, int64(2048), true).Return(nil)
	res, err := s.PutJSON("/system/quota", &models.SystemQuotaUpdateReq{Hard: models.ResourceList{"storage": 2048}, Enforce: true})
	s.NoError(err)
	s.Equal(200, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestUpdateRejectsOtherResources() {
	s.asAdmin()
	res, err := s.PutJSON("/system/quota", &models.SystemQuotaUpdateReq{Hard: models.ResourceList{"count": 1}})
	s.NoError(err)
	s.Equal(400, res.StatusCode)
	s.ctl.AssertNotCalled(s.T(), "Update", mock.Anything, mock.Anything, mock.Anything)
}

func (s *SystemQuotaTestSuite) TestUpdateRequiresHard() {
	s.asAdmin()
	res, err := s.PutJSON("/system/quota", map[string]any{"enforce": true})
	s.NoError(err)
	// go-swagger body validation is reported as 422 by lib_http.SendError, like every other endpoint
	s.Equal(422, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestUpdateForbiddenForNonAdmin() {
	s.asUser()
	res, err := s.PutJSON("/system/quota", &models.SystemQuotaUpdateReq{Hard: models.ResourceList{"storage": 1}})
	s.NoError(err)
	s.Equal(403, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestDelete() {
	s.asAdmin()
	s.ctl.On("Delete", mock.Anything).Return(nil)
	res, err := s.Delete("/system/quota")
	s.NoError(err)
	s.Equal(200, res.StatusCode)
}

func (s *SystemQuotaTestSuite) TestDeleteForbiddenForNonAdmin() {
	s.asUser()
	res, err := s.Delete("/system/quota")
	s.NoError(err)
	s.Equal(403, res.StatusCode)
}

func TestSystemQuotaTestSuite(t *testing.T) {
	suite.Run(t, &SystemQuotaTestSuite{})
}
