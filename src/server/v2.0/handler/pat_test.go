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
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/errors"
	patmodel "github.com/goharbor/harbor/src/pkg/pat/model"
	apimodels "github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	pattesting "github.com/goharbor/harbor/src/testing/controller/pat"
	usertesting "github.com/goharbor/harbor/src/testing/controller/user"
	"github.com/goharbor/harbor/src/testing/mock"
	audittesting "github.com/goharbor/harbor/src/testing/pkg/auditext"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

// decodeJSONBody reads and JSON-decodes an *http.Response body into target,
// leaving the response otherwise usable (status code etc already read by
// the caller).
func decodeJSONBody(res *http.Response, target any) error {
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// readBody reads an *http.Response body as a string.
func readBody(res *http.Response) (string, error) {
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// jsonBodyFunc returns a factory producing a fresh io.Reader on each call,
// since a request body reader is drained after being sent once.
func jsonBodyFunc(v any) func() io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return func() io.Reader {
		return bytes.NewReader(data)
	}
}

type authzReq struct {
	method  string
	url     string
	bodyGen func() io.Reader
}

func (r authzReq) body() io.Reader {
	if r.bodyGen == nil {
		return nil
	}
	return r.bodyGen()
}

// PATHandlerTestSuite exercises the actual HTTP handlers for the PAT
// endpoints (routing, parameter binding, status codes, and response JSON
// mapping — in particular that the internal Secret/Salt fields never reach
// the serialized response), rather than calling patctl.Controller directly.
type PATHandlerTestSuite struct {
	htesting.Suite

	patCtl   *pattesting.Controller
	userCtl  *usertesting.Controller
	auditMgr *audittesting.Manager
}

func (suite *PATHandlerTestSuite) SetupSuite() {
	suite.patCtl = &pattesting.Controller{}
	suite.userCtl = &usertesting.Controller{}
	suite.auditMgr = &audittesting.Manager{}

	suite.Config = &restapi.Config{
		UserAPI: &usersAPI{
			ctl:      suite.userCtl,
			patCtl:   suite.patCtl,
			auditMgr: suite.auditMgr,
			getAuth: func(ctx context.Context) (string, error) {
				return common.DBAuth, nil
			},
		},
	}

	suite.Suite.SetupSuite()
	suite.auditMgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
}

func (suite *PATHandlerTestSuite) TestAuthorization() {
	name := "authz-test"
	reqs := []authzReq{
		{"GET", "/users/1/personal_access_tokens", nil},
		{"POST", "/users/1/personal_access_tokens", jsonBodyFunc(&apimodels.PersonalAccessTokenCreateRequest{Name: &name})},
		{"GET", "/users/1/personal_access_tokens/1", nil},
		{"PUT", "/users/1/personal_access_tokens/1", jsonBodyFunc(&apimodels.PersonalAccessTokenUpdateRequest{})},
		{"DELETE", "/users/1/personal_access_tokens/1", nil},
		{"PATCH", "/users/1/personal_access_tokens/1", jsonBodyFunc(&apimodels.PersonalAccessTokenRefreshRequest{})},
	}

	for _, req := range reqs {
		{
			// not authenticated
			suite.Security.On("IsAuthenticated").Return(false).Once()
			res, err := suite.DoReq(req.method, req.url, req.body())
			suite.NoError(err)
			suite.Equal(401, res.StatusCode, "%s %s", req.method, req.url)
		}
		{
			// authenticated but not permitted, and not the user themselves
			suite.Security.On("IsAuthenticated").Return(true).Twice()
			suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(false).Once()
			suite.Security.On("GetUsername").Return("someone-else").Once()
			res, err := suite.DoReq(req.method, req.url, req.body())
			suite.NoError(err)
			suite.Equal(403, res.StatusCode, "%s %s", req.method, req.url)
		}
	}
}

func (suite *PATHandlerTestSuite) TestCreatePersonalAccessToken() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.Security.On("GetUsername").Return("self").Once()
	suite.patCtl.On("Create", mock.Anything, mock.Anything).Return(int64(1), "hbr_pat_abcdef", nil).Once()

	name := "test-token"
	res, err := suite.PostJSON("/users/1/personal_access_tokens", &apimodels.PersonalAccessTokenCreateRequest{
		Name: &name,
	})
	suite.NoError(err)
	suite.Equal(201, res.StatusCode)

	var body apimodels.PersonalAccessTokenCreatedResponse
	suite.NoError(decodeJSONBody(res, &body))
	suite.Equal("test-token", body.Name)
	suite.Equal("hbr_pat_abcdef", body.Secret)
}

func (suite *PATHandlerTestSuite) TestCreatePersonalAccessTokenNameConflict() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.Security.On("GetUsername").Return("self").Once()
	suite.patCtl.On("Create", mock.Anything, mock.Anything).
		Return(int64(0), "", errors.ConflictError(nil)).Once()

	name := "dup-token"
	res, err := suite.PostJSON("/users/1/personal_access_tokens", &apimodels.PersonalAccessTokenCreateRequest{
		Name: &name,
	})
	suite.NoError(err)
	suite.Equal(409, res.StatusCode)
}

func (suite *PATHandlerTestSuite) TestListPersonalAccessTokens() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.patCtl.On("Count", mock.Anything, mock.Anything).Return(int64(2), nil).Once()
	suite.patCtl.On("List", mock.Anything, mock.Anything).Return([]*patmodel.PersonalAccessToken{
		{ID: 1, UserID: 1, Name: "token-1", Secret: "hashed-secret-1", Salt: "salt-1", ExpiresAt: -1},
		{ID: 2, UserID: 1, Name: "token-2", Secret: "hashed-secret-2", Salt: "salt-2", ExpiresAt: -1},
	}, nil).Once()

	res, err := suite.Get("/users/1/personal_access_tokens")
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)
	suite.Equal("2", res.Header.Get("X-Total-Count"))

	body, err := readBody(res)
	suite.NoError(err)
	// The internal controller-layer model carries the hashed secret/salt;
	// the whole point of this test is to prove they never reach the wire.
	suite.NotContains(strings.ToLower(body), `"secret"`)
	suite.NotContains(strings.ToLower(body), `"salt"`)
	suite.Contains(body, "token-1")
	suite.Contains(body, "token-2")
}

func (suite *PATHandlerTestSuite) TestGetPersonalAccessToken() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.patCtl.On("Get", mock.Anything, int64(1)).Return(&patmodel.PersonalAccessToken{
		ID: 1, UserID: 1, Name: "get-token", Description: "Get test token", ExpiresAt: -1,
	}, nil).Once()

	var body apimodels.PersonalAccessToken
	res, err := suite.GetJSON("/users/1/personal_access_tokens/1", &body)
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)
	suite.Equal(int64(1), body.ID)
	suite.Equal("get-token", body.Name)
	suite.Equal("Get test token", body.Description)
}

// TestGetPersonalAccessTokenIDOR verifies that a token belonging to a
// different user, even if its numeric ID is guessed correctly, is reported
// as not found rather than returned.
func (suite *PATHandlerTestSuite) TestGetPersonalAccessTokenIDOR() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.patCtl.On("Get", mock.Anything, int64(1)).Return(&patmodel.PersonalAccessToken{
		ID: 1, UserID: 42, Name: "someone-elses-token", ExpiresAt: -1,
	}, nil).Once()

	res, err := suite.Get("/users/1/personal_access_tokens/1")
	suite.NoError(err)
	suite.Equal(404, res.StatusCode)
}

func (suite *PATHandlerTestSuite) TestUpdatePersonalAccessToken() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.Security.On("GetUsername").Return("self").Once()
	suite.patCtl.On("Get", mock.Anything, int64(1)).Return(&patmodel.PersonalAccessToken{
		ID: 1, UserID: 1, Name: "update-token", Disabled: false, ExpiresAt: -1,
	}, nil).Once()
	suite.patCtl.On("Update", mock.Anything, mock.Anything).Return(nil).Once()

	res, err := suite.PutJSON("/users/1/personal_access_tokens/1", &apimodels.PersonalAccessTokenUpdateRequest{
		Disabled: true,
	})
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)
}

func (suite *PATHandlerTestSuite) TestDeletePersonalAccessToken() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.Security.On("GetUsername").Return("self").Once()
	suite.patCtl.On("Get", mock.Anything, int64(1)).Return(&patmodel.PersonalAccessToken{
		ID: 1, UserID: 1, Name: "delete-token", ExpiresAt: -1,
	}, nil).Once()
	suite.patCtl.On("Delete", mock.Anything, int64(1)).Return(nil).Once()

	res, err := suite.Delete("/users/1/personal_access_tokens/1")
	suite.NoError(err)
	suite.Equal(204, res.StatusCode)
}

func (suite *PATHandlerTestSuite) TestRefreshPersonalAccessTokenSecret() {
	suite.Security.On("IsAuthenticated").Return(true).Twice()
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Once()
	suite.Security.On("GetUsername").Return("self").Once()
	suite.patCtl.On("Get", mock.Anything, int64(1)).Return(&patmodel.PersonalAccessToken{
		ID: 1, UserID: 1, Name: "refresh-token", ExpiresAt: -1,
	}, nil).Once()
	suite.patCtl.On("RefreshSecret", mock.Anything, int64(1), "").Return("hbr_pat_newsecret", nil).Once()

	res, err := suite.PatchJSON("/users/1/personal_access_tokens/1", &apimodels.PersonalAccessTokenRefreshRequest{})
	suite.NoError(err)
	suite.Equal(200, res.StatusCode)

	var body apimodels.PersonalAccessTokenCreatedResponse
	suite.NoError(decodeJSONBody(res, &body))
	suite.Equal("hbr_pat_newsecret", body.Secret)
}

func TestPATHandlerTestSuite(t *testing.T) {
	suite.Run(t, &PATHandlerTestSuite{})
}
