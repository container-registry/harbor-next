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

package security

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/utils"
	robot_ctl "github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/core/auth"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	robotmodel "github.com/goharbor/harbor/src/pkg/robot/model"
	robottesting "github.com/goharbor/harbor/src/testing/controller/robot"
	testingUser "github.com/goharbor/harbor/src/testing/controller/user"
	"github.com/goharbor/harbor/src/testing/mock"
)

const (
	robotName     = "robot$proj+ci"
	robotSecret   = "Robot-Secret-1"
	userName      = "alice"
	userPassword  = "User-Password-1"
	dbErrorDetail = "failed to connect to `user=harbor database=registry`: connection refused"
)

var errDB = fmt.Errorf("%s", dbErrorDetail)

func mustGenerate(t *testing.T, g generator, req *http.Request) security.Context {
	t.Helper()
	ctx, err := g.Generate(req)
	require.NoError(t, err)
	return ctx
}

func initAuthConfig(t *testing.T) {
	t.Helper()
	origMgr := config.DefaultCfgManager
	t.Cleanup(func() { config.DefaultCfgManager = origMgr })
	config.InitWithSettings(map[string]any{
		common.AUTHMode:        common.DBAuth,
		common.RobotNamePrefix: "robot$",
	})
}

// mockRobots makes the robot controller return robots and err for every lookup.
func mockRobots(t *testing.T, robots []*robot_ctl.Robot, err error) *robottesting.Controller {
	t.Helper()
	ctl := &robottesting.Controller{}
	mock.OnAnything(ctl, "List").Return(robots, err)
	orig := robot_ctl.Ctl
	robot_ctl.Ctl = ctl
	t.Cleanup(func() { robot_ctl.Ctl = orig })
	return ctl
}

func validRobot() *robot_ctl.Robot {
	salt := utils.GenerateRandomString()
	return &robot_ctl.Robot{Robot: robotmodel.Robot{
		Name:      "proj+ci",
		Salt:      salt,
		Secret:    utils.Encrypt(robotSecret, salt, utils.SHA256),
		ExpiresAt: -1,
	}}
}

// mockLogin replaces auth.Login, counting the calls.
func mockLogin(t *testing.T, user *models.User, err error) *int {
	t.Helper()
	calls := 0
	orig := login
	login = func(_ context.Context, _ models.AuthModel) (*models.User, error) {
		calls++
		return user, err
	}
	t.Cleanup(func() { login = orig })
	return &calls
}

func mockUsers(t *testing.T, user *models.User, err error) {
	t.Helper()
	ctl := &testingUser.Controller{}
	mock.OnAnything(ctl, "GetByName").Return(user, err)
	orig := uctl
	uctl = ctl
	t.Cleanup(func() { uctl = orig })
}

func basicAuthRequest(t *testing.T, path, username, password string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetBasicAuth(username, password)
	return req
}

func TestRobotGenerateBackendError(t *testing.T) {
	initAuthConfig(t)

	cases := []struct {
		name      string
		username  string
		password  string
		robots    []*robot_ctl.Robot
		listErr   error
		wantErr   bool
		wantCtx   bool
		wantLists int
	}{
		{"database unreachable", robotName, robotSecret, nil, errDB, true, false, 1},
		{"unknown robot", robotName, robotSecret, []*robot_ctl.Robot{}, nil, false, false, 1},
		{"wrong secret", robotName, "wrong", []*robot_ctl.Robot{validRobot()}, nil, false, false, 1},
		{"valid secret", robotName, robotSecret, []*robot_ctl.Robot{validRobot()}, nil, false, true, 1},
		{"NUL in name is not looked up", "robot$proj\x00", robotSecret, nil, errDB, false, false, 0},
		{"invalid UTF-8 in name is not looked up", "robot$\xff", robotSecret, nil, errDB, false, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctl := mockRobots(t, tc.robots, tc.listErr)
			ctx, err := (&robot{}).Generate(basicAuthRequest(t, "/v2/", tc.username, tc.password))
			if tc.wantErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), tc.password)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantCtx, ctx != nil)
			ctl.AssertNumberOfCalls(t, "List", tc.wantLists)
		})
	}
}

func TestBasicAuthGenerateBackendError(t *testing.T) {
	initAuthConfig(t)

	cases := []struct {
		name       string
		username   string
		user       *models.User
		loginErr   error
		wantErr    bool
		wantCtx    bool
		wantLogins int
	}{
		{"database unreachable", userName, nil, errDB, true, false, 1},
		{"wrong password", userName, nil, auth.NewErrAuth("Invalid credentials"), false, false, 1},
		{"locked user", userName, nil, nil, false, false, 1},
		{"valid password", userName, &models.User{UserID: 3, Username: userName}, nil, false, true, 1},
		{"NUL in username is not looked up", "ali\x00ce", nil, errDB, false, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := mockLogin(t, tc.user, tc.loginErr)
			ctx, err := (&basicAuth{}).Generate(basicAuthRequest(t, "/v2/", tc.username, userPassword))
			if tc.wantErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), userPassword)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantCtx, ctx != nil)
			assert.Equal(t, tc.wantLogins, *calls)
		})
	}
}

// In non-DB auth modes basic auth is only for the admin, and checking for the
// admin is itself a database lookup.
func TestBasicAuthGenerateSuperUserLookupError(t *testing.T) {
	initAuthConfig(t)

	cases := []struct {
		name       string
		user       *models.User
		lookupErr  error
		wantErr    bool
		wantLogins int
	}{
		{"database unreachable", nil, errDB, true, 0},
		{"unknown user", nil, errors.NotFoundError(nil), false, 0},
		{"not the admin", &models.User{UserID: 3, Username: userName}, nil, false, 0},
		{"the admin", &models.User{UserID: 1, Username: userName}, nil, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockUsers(t, tc.user, tc.lookupErr)
			calls := mockLogin(t, &models.User{UserID: 1, Username: userName}, nil)
			req := basicAuthRequest(t, "/api/v2.0/projects", userName, userPassword)
			req = req.WithContext(lib.WithAuthMode(req.Context(), common.OIDCAuth))
			_, err := (&basicAuth{}).Generate(req)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.wantLogins, *calls)
		})
	}
}

// TestMiddlewareBackendErrorIsNotUnauthorized drives the security middleware the
// way the core chain does: a backend failure while verifying credentials must be
// answered with 503, while wrong and absent credentials still reach the handler
// as anonymous, where the registry and API answer 401.
func TestMiddlewareBackendErrorIsNotUnauthorized(t *testing.T) {
	initAuthConfig(t)
	origGens := generators
	generators = []generator{&robot{}, &basicAuth{}}
	t.Cleanup(func() { generators = origGens })

	cases := []struct {
		name          string
		username      string
		password      string
		robotErr      error
		loginErr      error
		loginUser     *models.User
		wantStatus    int
		wantHandler   bool
		wantAnonymous bool
	}{
		{"robot, database unreachable", robotName, robotSecret, errDB, nil, nil, http.StatusServiceUnavailable, false, false},
		{"user, database unreachable", userName, userPassword, nil, errDB, nil, http.StatusServiceUnavailable, false, false},
		{"robot, wrong secret", robotName, "wrong", nil, nil, nil, http.StatusOK, true, true},
		{"user, wrong password", userName, "wrong", nil, auth.NewErrAuth("Invalid credentials"), nil, http.StatusOK, true, true},
		{"user, valid password", userName, userPassword, nil, nil, &models.User{UserID: 3, Username: userName}, http.StatusOK, true, false},
		{"no credentials", "", "", errDB, errDB, nil, http.StatusOK, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockRobots(t, []*robot_ctl.Robot{validRobot()}, tc.robotErr)
			mockLogin(t, tc.loginUser, tc.loginErr)

			req := httptest.NewRequest(http.MethodGet, "/v2/", nil)
			if tc.username != "" {
				req.SetBasicAuth(tc.username, tc.password)
			}
			var handlerCalled bool
			var sc security.Context
			handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				sc, _ = security.FromContext(r.Context())
			})
			rec := httptest.NewRecorder()
			Middleware()(UnauthorizedMiddleware()(handler)).ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantHandler, handlerCalled)
			if tc.wantHandler {
				require.NotNil(t, sc)
				assert.Equal(t, !tc.wantAnonymous, sc.IsAuthenticated())
				return
			}
			assert.Equal(t, "5", rec.Header().Get("Retry-After"))
			assert.JSONEq(t, `{"errors":[{"code":"UNAVAILABLE","message":"service unavailable"}]}`, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), dbErrorDetail)
			assert.NotContains(t, rec.Body.String(), tc.password)
		})
	}
}
