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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	cfgModels "github.com/goharbor/harbor/src/lib/config/models"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	configtesting "github.com/goharbor/harbor/src/testing/controller/config"
	"github.com/goharbor/harbor/src/testing/mock"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

// commercialStyleKey stands in for a key a commercial patch registers.
const commercialStyleKey = "enable_commercial_handler_test"

func init() {
	feature.Register(feature.Definition{Feature: "commercial_handler_test", ConfigKey: commercialStyleKey, EnvKey: "HARBOR_ENABLE_COMMERCIAL_HANDLER_TEST"})
}

type ConfigAPITestSuite struct {
	htesting.Suite
	ctl *configtesting.Controller
}

func (s *ConfigAPITestSuite) SetupSuite() {
	s.ctl = &configtesting.Controller{}
	s.Config = &restapi.Config{ConfigureAPI: &configAPI{controller: s.ctl}}
	s.Middlewares = map[string]func(http.Handler) http.Handler{"UpdateConfigurations": captureFeatureKeys}
	s.Suite.SetupSuite()
}

func (s *ConfigAPITestSuite) put(body string) (int, string) {
	res, err := s.DoReq(http.MethodPut, "/configurations", bytes.NewBufferString(body))
	s.Require().NoError(err)
	defer res.Body.Close()
	msg, err := io.ReadAll(res.Body)
	s.Require().NoError(err)
	return res.StatusCode, string(msg)
}

func (s *ConfigAPITestSuite) TestRejectsFeatureKeys() {
	s.Security.On("IsAuthenticated").Return(true)
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

	for _, key := range []string{feature.ModelSyncConfigKey, commercialStyleKey} {
		code, msg := s.put(`{"read_only":false,"` + key + `":true}`)
		s.Equal(http.StatusBadRequest, code, key)
		s.Contains(msg, key+" is a feature toggle: use PUT /api/v2.0/features/"+key)
	}
	s.ctl.AssertNotCalled(s.T(), "UpdateUserConfigs", mock.Anything, mock.Anything)
}

func (s *ConfigAPITestSuite) TestRejectsOversizedBody() {
	code, _ := s.put(`{"read_only":"` + strings.Repeat("x", maxConfigBodySize) + `"}`)
	s.Equal(http.StatusRequestEntityTooLarge, code)
}

func (s *ConfigAPITestSuite) TestFeatureKeyNeedsAuthorizationFirst() {
	s.Security.On("IsAuthenticated").Return(false).Once()
	code, _ := s.put(`{"` + commercialStyleKey + `":true}`)
	s.Equal(http.StatusUnauthorized, code)
}

func (s *ConfigAPITestSuite) TestRoundTripAndUnknownKeys() {
	s.Security.On("IsAuthenticated").Return(true)
	s.Security.On("IsSolutionUser").Return(false)
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)
	// the controller still reports feature items; the response model drops them
	s.ctl.On("UserConfigs", mock.Anything).Return(map[string]*cfgModels.Value{
		"read_only":                {Val: false, Editable: true},
		feature.ModelSyncConfigKey: {Val: true, Editable: true},
		commercialStyleKey:         {Val: true, Editable: true},
	}, nil).Once()

	res, err := s.Get("/configurations")
	s.Require().NoError(err)
	defer res.Body.Close()
	s.Require().Equal(http.StatusOK, res.StatusCode)
	var got map[string]struct {
		Value any `json:"value"`
	}
	s.Require().NoError(json.NewDecoder(res.Body).Decode(&got))
	s.Contains(got, "read_only")
	s.NotContains(got, feature.ModelSyncConfigKey)
	s.NotContains(got, commercialStyleKey)

	back := map[string]any{"not_a_known_key": "x"}
	for key, item := range got {
		back[key] = item.Value
	}
	body, err := json.Marshal(back)
	s.Require().NoError(err)
	s.ctl.On("UpdateUserConfigs", mock.Anything, map[string]any{"read_only": false}).Return(nil).Once()
	code, msg := s.put(string(body))
	s.Equal(http.StatusOK, code, msg)
	s.ctl.AssertExpectations(s.T())
}

func TestConfigAPITestSuite(t *testing.T) {
	suite.Run(t, &ConfigAPITestSuite{})
}
