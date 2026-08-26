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

	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	registrytesting "github.com/goharbor/harbor/src/testing/controller/registry"
	"github.com/goharbor/harbor/src/testing/mock"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

type RegistryTestSuite struct {
	htesting.Suite
	regCtl *registrytesting.Controller
}

func (suite *RegistryTestSuite) SetupSuite() {
	suite.regCtl = &registrytesting.Controller{}
	suite.Config = &restapi.Config{
		RegistryAPI: &registryAPI{ctl: suite.regCtl},
	}
	suite.Suite.SetupSuite()
}

func (suite *RegistryTestSuite) SetupTest() {
	suite.regCtl.ExpectedCalls = nil
	suite.regCtl.Calls = nil
	suite.Security.ExpectedCalls = nil
	suite.Security.Calls = nil
}

func (suite *RegistryTestSuite) ptrStr(s string) *string { return &s }

// TestPingRegistryByIDIgnoresOverrides guards against CVE-class credential
// exfiltration: a caller referencing an existing registry by id must not be able
// to override its saved connection settings (url, insecure, ca certificate) and so
// redirect the health check (and the saved credentials) to an untrusted endpoint.
func (suite *RegistryTestSuite) TestPingRegistryByIDIgnoresOverrides() {
	suite.Security.On("IsAuthenticated").Return(true).Once()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

	saved := &model.Registry{ID: 1, Type: "harbor", URL: "https://registry.example.com", Insecure: false}
	mock.OnAnything(suite.regCtl, "Get").Return(saved, nil).Once()

	var pinged *model.Registry
	suite.regCtl.On("IsHealthy", mock.Anything, mock.Anything).Return(true, nil).Once().
		Run(func(args testifymock.Arguments) { pinged = args.Get(1).(*model.Registry) })

	id := int64(1)
	insecure := true
	res, err := suite.PostJSON("/registries/ping", &models.RegistryPing{
		ID:            &id,
		URL:           suite.ptrStr("https://attacker.example.com"),
		Insecure:      &insecure,
		CaCertificate: suite.ptrStr("-----BEGIN CERTIFICATE-----\nattacker\n-----END CERTIFICATE-----"),
	})
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)
	suite.Require().NotNil(pinged)
	// every supplied override is ignored; the saved settings are used
	suite.Equal("https://registry.example.com", pinged.URL)
	suite.False(pinged.Insecure)
	suite.Empty(pinged.CACertificate)
}

// TestPingRegistryInlineUsesSuppliedURL confirms inline pings (no id) still honor
// the supplied URL, so the fix does not regress the normal ad-hoc ping flow.
func (suite *RegistryTestSuite) TestPingRegistryInlineUsesSuppliedURL() {
	suite.Security.On("IsAuthenticated").Return(true).Once()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

	var pinged *model.Registry
	suite.regCtl.On("IsHealthy", mock.Anything, mock.Anything).Return(true, nil).Once().
		Run(func(args testifymock.Arguments) { pinged = args.Get(1).(*model.Registry) })

	res, err := suite.PostJSON("/registries/ping", &models.RegistryPing{
		Type: suite.ptrStr("harbor"),
		URL:  suite.ptrStr("https://inline.example.com"),
	})
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)
	suite.Require().NotNil(pinged)
	suite.Equal("https://inline.example.com", pinged.URL)
}

// TestPingRegistryInlineInvalidSchemeRejected guards the inline ping path against
// URLs outside the scheme allowlist reaching the health check.
func (suite *RegistryTestSuite) TestPingRegistryInlineInvalidSchemeRejected() {
	suite.Security.On("IsAuthenticated").Return(true).Once()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

	res, err := suite.PostJSON("/registries/ping", &models.RegistryPing{
		Type: suite.ptrStr("harbor"),
		URL:  suite.ptrStr("gopher://attacker.example.com"),
	})
	suite.NoError(err)
	suite.Equal(400, res.StatusCode)
	suite.regCtl.AssertNotCalled(suite.T(), "IsHealthy", testifymock.Anything, testifymock.Anything)
}

// TestUpdateRegistryID0ReturnsNotFound guards against updating synthetic local registry ID 0,
// asserting the NotFound response and that controller's Get and Update methods are not called.
func (suite *RegistryTestSuite) TestUpdateRegistryID0ReturnsNotFound() {
	suite.Security.On("IsAuthenticated").Return(true).Once()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

	res, err := suite.PutJSON("/registries/0", &models.RegistryUpdate{
		URL: suite.ptrStr("https://attacker.example.com"),
	})
	suite.NoError(err)
	suite.Equal(404, res.StatusCode)

	suite.regCtl.AssertNotCalled(suite.T(), "Get", testifymock.Anything, testifymock.Anything)
	suite.regCtl.AssertNotCalled(suite.T(), "Update", testifymock.Anything, testifymock.Anything)
}

// TestUpdateRegistryClearsAccessSecretOnURLChange tests that updating an existing registry's
// URL without providing a new AccessSecret clears the stored AccessSecret.
func (suite *RegistryTestSuite) TestUpdateRegistryClearsAccessSecretOnURLChange() {
	suite.Security.On("IsAuthenticated").Return(true).Once()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

	saved := &model.Registry{
		ID:         1,
		Type:       "harbor",
		URL:        "https://registry.example.com",
		Credential: &model.Credential{Type: "basic", AccessKey: "admin", AccessSecret: "secret123"},
	}
	mock.OnAnything(suite.regCtl, "Get").Return(saved, nil).Once()

	var updated *model.Registry
	suite.regCtl.On("Update", mock.Anything, mock.Anything).Return(nil).Once().
		Run(func(args testifymock.Arguments) { updated = args.Get(1).(*model.Registry) })

	res, err := suite.PutJSON("/registries/1", &models.RegistryUpdate{
		URL: suite.ptrStr("https://new.example.com"),
	})
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)
	suite.Require().NotNil(updated)
	suite.Equal("https://new.example.com", updated.URL)
	suite.Empty(updated.Credential.AccessSecret)
}

// TestUpdateRegistryInvalidURLReturnsError tests that updating a registry with an invalid
// URL returns BadRequest error and does not update the registry.
func (suite *RegistryTestSuite) TestUpdateRegistryInvalidURLReturnsError() {
	for _, invalidURL := range []string{
		"gopher://invalid.example.com",
		"file:///etc/passwd",
		"http://127.0.0.%31/",
	} {
		suite.SetupTest()
		suite.Security.On("IsAuthenticated").Return(true).Once()
		suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

		saved := &model.Registry{
			ID:         1,
			Type:       "harbor",
			URL:        "https://registry.example.com",
			Credential: &model.Credential{Type: "basic", AccessKey: "admin", AccessSecret: "secret123"},
		}
		mock.OnAnything(suite.regCtl, "Get").Return(saved, nil).Once()

		res, err := suite.PutJSON("/registries/1", &models.RegistryUpdate{
			URL: suite.ptrStr(invalidURL),
		})
		suite.NoError(err)
		suite.Equal(400, res.StatusCode, "URL %q must be rejected", invalidURL)
		suite.regCtl.AssertNotCalled(suite.T(), "Update", testifymock.Anything, testifymock.Anything)
	}
}

// TestUpdateRegistryStorageSchemeURLAccepted documents that schema-aware validation
// accepts storage-backed registry URLs (sftp/s3) on update, and that changing to one
// still clears the stored AccessSecret like any other URL change.
func (suite *RegistryTestSuite) TestUpdateRegistryStorageSchemeURLAccepted() {
	for _, newURL := range []string{
		"sftp://storage.example.com",
		"s3://bucket.example.com",
	} {
		suite.SetupTest()
		suite.Security.On("IsAuthenticated").Return(true).Once()
		suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()

		saved := &model.Registry{
			ID:         1,
			Type:       "harbor",
			URL:        "https://registry.example.com",
			Credential: &model.Credential{Type: "basic", AccessKey: "admin", AccessSecret: "secret123"},
		}
		mock.OnAnything(suite.regCtl, "Get").Return(saved, nil).Once()

		var updated *model.Registry
		suite.regCtl.On("Update", mock.Anything, mock.Anything).Return(nil).Once().
			Run(func(args testifymock.Arguments) { updated = args.Get(1).(*model.Registry) })

		res, err := suite.PutJSON("/registries/1", &models.RegistryUpdate{
			URL: suite.ptrStr(newURL),
		})
		suite.NoError(err)
		suite.Equal(200, res.StatusCode, "URL %q must be accepted", newURL)
		suite.Require().NotNil(updated)
		suite.Equal(newURL, updated.URL)
		suite.Empty(updated.Credential.AccessSecret)
	}
}

func TestRegistryTestSuite(t *testing.T) {
	suite.Run(t, &RegistryTestSuite{})
}
