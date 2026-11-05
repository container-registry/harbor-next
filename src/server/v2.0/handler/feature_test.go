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
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	configtesting "github.com/goharbor/harbor/src/testing/controller/config"
	"github.com/goharbor/harbor/src/testing/mock"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

const (
	handlerFeatureKey      = "enable_handler_test_feature"
	handlerFeatureEnvKey   = "HARBOR_ENABLE_HANDLER_TEST_FEATURE"
	handlerLinkedKey       = "enable_handler_test_linked"
	handlerLinkedEnvKey    = "HARBOR_ENABLE_HANDLER_TEST_LINKED"
	handlerFeatureSection  = "handler_test_section"
	handlerFeatureLinkBase = "mailto:test@example.com?features="
)

func init() {
	feature.Register(
		feature.Definition{Feature: "handler_test_feature", ConfigKey: handlerFeatureKey, EnvKey: handlerFeatureEnvKey, Name: "Handler Test", Description: "d", Beta: true},
		feature.Definition{Feature: "handler_test_linked", ConfigKey: handlerLinkedKey, EnvKey: handlerLinkedEnvKey, Section: handlerFeatureSection, Name: "Linked", Default: true},
	)
	feature.RegisterSection(feature.Section{
		ID: handlerFeatureSection, Title: "Test Section", LinkText: "Ask", Order: 10,
		Link: func(_ context.Context, enabled []feature.Definition) string {
			names := make([]string, 0, len(enabled))
			for _, def := range enabled {
				names = append(names, def.Name)
			}
			return handlerFeatureLinkBase + strings.Join(names, ",")
		},
	})
}

func unsetHandlerFeatureEnv(t *testing.T) {
	for _, key := range []string{handlerFeatureEnvKey, handlerLinkedEnvKey} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func findFeature(list *models.FeatureList, key string) *models.Feature {
	for _, f := range list.Features {
		if f.Key == key {
			return f
		}
	}
	return nil
}

func TestFeatureListReportsStateLocksAndResolvedLinks(t *testing.T) {
	unsetHandlerFeatureEnv(t)
	list := featureList(t.Context(), false)

	plain := findFeature(list, handlerFeatureKey)
	if plain == nil || plain.Section != feature.OptionalSection || !plain.Beta || plain.Enabled || !plain.Editable || plain.LockedBy != "" {
		t.Fatalf("plain feature = %+v", plain)
	}
	linked := findFeature(list, handlerLinkedKey)
	if linked == nil || !linked.Enabled {
		t.Fatalf("default-on feature = %+v", linked)
	}
	var section *models.FeatureSection
	for _, s := range list.Sections {
		if s.ID == handlerFeatureSection {
			section = s
		}
	}
	if section == nil || section.Link != handlerFeatureLinkBase+"Linked" || section.LinkText != "Ask" {
		t.Fatalf("section = %+v", section)
	}

	t.Setenv(handlerFeatureEnvKey, "true")
	if got := findFeature(featureList(t.Context(), false), handlerFeatureKey); !got.Enabled || got.Editable || got.LockedBy != handlerFeatureEnvKey {
		t.Fatalf("env-locked feature = %+v", got)
	}

	unsetHandlerFeatureEnv(t)
	if got := findFeature(featureList(t.Context(), true), handlerFeatureKey); got.Editable || got.LockedBy != configOverwriteJSON {
		t.Fatalf("overwritten feature = %+v", got)
	}
}

type FeatureAPITestSuite struct {
	htesting.Suite
	ctl *configtesting.Controller
}

func (s *FeatureAPITestSuite) SetupSuite() {
	s.ctl = &configtesting.Controller{}
	s.Config = &restapi.Config{ConfigureAPI: &configAPI{controller: s.ctl}}
	s.Suite.SetupSuite()
}

func (s *FeatureAPITestSuite) put(key string, body string) int {
	res, err := s.DoReq(http.MethodPut, "/features/"+key, bytes.NewBufferString(body))
	s.Require().NoError(err)
	return res.StatusCode
}

func (s *FeatureAPITestSuite) TestAuthorization() {
	s.Security.On("IsAuthenticated").Return(false).Once()
	res, err := s.DoReq(http.MethodGet, "/features", nil)
	s.NoError(err)
	s.Equal(401, res.StatusCode)

	s.Security.On("IsAuthenticated").Return(true).Once()
	s.Security.On("GetUsername").Return("u").Once()
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(false).Once()
	s.Equal(403, s.put(handlerFeatureKey, `{"enabled":true}`))
}

func (s *FeatureAPITestSuite) TestListAndUpdate() {
	s.Security.On("IsAuthenticated").Return(true)
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

	res, err := s.DoReq(http.MethodGet, "/features", nil)
	s.Require().NoError(err)
	s.Equal(200, res.StatusCode)
	var raw struct {
		Features []map[string]any `json:"features"`
	}
	s.Require().NoError(json.NewDecoder(res.Body).Decode(&raw))
	found := false
	for _, f := range raw.Features {
		if f["key"] == handlerFeatureKey {
			found = true
			// false must be sent, not omitted, so API clients see the state
			s.Equal(false, f["enabled"])
			s.Contains(f, "editable")
		}
	}
	s.True(found)

	s.Equal(404, s.put("enable_not_a_feature", `{"enabled":true}`))
	s.Equal(422, s.put(handlerFeatureKey, `{}`))

	s.ctl.On("UpdateUserConfigs", mock.Anything, map[string]any{handlerFeatureKey: true}).Return(nil).Once()
	s.Equal(200, s.put(handlerFeatureKey, `{"enabled":true}`))

	s.ctl.On("UpdateUserConfigs", mock.Anything, map[string]any{handlerFeatureKey: false}).
		Return(errors.BadRequestError(nil).WithMessage("controlled by env")).Once()
	s.Equal(400, s.put(handlerFeatureKey, `{"enabled":false}`))
	s.ctl.AssertExpectations(s.T())
}

func TestFeatureAPITestSuite(t *testing.T) {
	suite.Run(t, &FeatureAPITestSuite{})
}
