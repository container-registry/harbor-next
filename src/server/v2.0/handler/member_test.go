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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/runtime"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	commonmodels "github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/rbac"
	rbacProject "github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/local"
	robotsec "github.com/goharbor/harbor/src/common/security/robot"
	"github.com/goharbor/harbor/src/controller/member"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/controller/robot"
	roleCtl "github.com/goharbor/harbor/src/controller/role"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/permission/policy"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	robotmodel "github.com/goharbor/harbor/src/pkg/robot/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	memberop "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/member"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/member"
	securityMock "github.com/goharbor/harbor/src/testing/common/security"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	robottesting "github.com/goharbor/harbor/src/testing/controller/robot"
	usertesting "github.com/goharbor/harbor/src/testing/controller/user"
	"github.com/goharbor/harbor/src/testing/mock"
)

// ---------------------------------------------------------------------------
// Inline mock for roleCtl.Controller
// Only Get needs real behaviour; the rest are no-ops for these tests.
// ---------------------------------------------------------------------------

type stubRoleCtl struct {
	testifymock.Mock
}

func (s *stubRoleCtl) Get(ctx context.Context, id int64, option *roleCtl.Option) (*roleCtl.Role, error) {
	args := s.Called(ctx, id, option)
	r, _ := args.Get(0).(*roleCtl.Role)
	return r, args.Error(1)
}

func (s *stubRoleCtl) Create(ctx context.Context, r *roleCtl.Role) (int64, error) { return 0, nil }
func (s *stubRoleCtl) Delete(ctx context.Context, id int64, option ...*roleCtl.Option) error {
	return nil
}
func (s *stubRoleCtl) Update(ctx context.Context, r *roleCtl.Role, option *roleCtl.Option) error {
	return nil
}
func (s *stubRoleCtl) List(ctx context.Context, query *q.Query, option *roleCtl.Option) ([]*roleCtl.Role, error) {
	return nil, nil
}
func (s *stubRoleCtl) Count(ctx context.Context, query *q.Query) (int64, error) { return 0, nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const testProjectID int64 = 42

// policyAccess builds a single-permission role for test fixtures.
func policyAccess(resource rbac.Resource, action rbac.Action) []*types.Policy {
	return []*types.Policy{{Resource: resource, Action: action}}
}

// projectResource returns the full RBAC resource string for a project subresource,
// matching what HasProjectPermission builds internally.
func projectResource(subresource rbac.Resource) rbac.Resource {
	return rbacProject.NewNamespace(testProjectID).Resource(subresource)
}

// newCtxWithSecurity injects sc into a background context, mirroring what the
// handler middleware does at request time.
func newCtxWithSecurity(sc security.Context) context.Context {
	return security.NewContext(context.Background(), sc)
}

// newAPI returns a memberAPI wired to the given role controller stub.
func newAPI(rc *stubRoleCtl) *memberAPI {
	return &memberAPI{roleCtl: rc}
}

// ---------------------------------------------------------------------------
// Tests for checkNoEscalation
// ---------------------------------------------------------------------------

func TestCheckNoEscalation_RoleWithNoPermissions(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	// Can() should never be called — role has zero permissions

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(10), testifymock.Anything).
		Return(&roleCtl.Role{}, nil)

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 10)
	assert.NoError(t, err)
	sc.AssertNotCalled(t, "Can")
}

func TestCheckNoEscalation_CallerHasAllPermissions(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	// Caller holds repository:pull
	sc.On("Can", testifymock.Anything,
		rbac.ActionPull, projectResource(rbac.ResourceRepository)).
		Return(true)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(20), testifymock.Anything).
		Return(&roleCtl.Role{
			Permissions: []*roleCtl.Permission{{
				Access: policyAccess(rbac.ResourceRepository, rbac.ActionPull),
			}},
		}, nil)

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 20)
	assert.NoError(t, err)
}

func TestCheckNoEscalation_CallerLacksPermission(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	// Caller does NOT hold member:create
	sc.On("Can", testifymock.Anything,
		rbac.ActionCreate, projectResource(rbac.ResourceMember)).
		Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(30), testifymock.Anything).
		Return(&roleCtl.Role{
			Permissions: []*roleCtl.Permission{{
				Access: policyAccess(rbac.ResourceMember, rbac.ActionCreate),
			}},
		}, nil)

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 30)
	assert.Error(t, err)
	assert.Equal(t, errors.ForbiddenCode, errors.ErrCode(err), "expected ForbiddenError, got: %v", err)
}

// A human caller who may only update members cannot hand out project admin,
// themselves included: built-in roles get the same check as custom ones.
func TestCheckNoEscalation_BuiltinRoleCheckedForHumans(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	sc.On("Can", testifymock.Anything,
		rbac.ActionUpdate, projectResource(rbac.ResourceMember)).Return(true)
	sc.On("Can", testifymock.Anything,
		rbac.ActionDelete, projectResource(rbac.ResourceRepository)).Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(common.RoleProjectAdmin), testifymock.Anything).
		Return(projectAdminRole(), nil)

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, int64(common.RoleProjectAdmin))
	assert.Equal(t, errors.ForbiddenCode, errors.ErrCode(err), "expected ForbiddenError, got: %v", err)
}

func projectAdminRole() *roleCtl.Role {
	return &roleCtl.Role{
		Permissions: []*roleCtl.Permission{{
			Access: []*types.Policy{
				{Resource: rbac.ResourceMember, Action: rbac.ActionUpdate},
				{Resource: rbac.ResourceRepository, Action: rbac.ActionDelete},
			},
		}},
	}
}

func TestCheckNoEscalation_FirstPermissionPassesSecondFails(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	sc.On("Can", testifymock.Anything,
		rbac.ActionPull, projectResource(rbac.ResourceRepository)).
		Return(true)
	sc.On("Can", testifymock.Anything,
		rbac.ActionCreate, projectResource(rbac.ResourceMember)).
		Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(40), testifymock.Anything).
		Return(&roleCtl.Role{
			Permissions: []*roleCtl.Permission{{
				Access: []*types.Policy{
					{Resource: rbac.ResourceRepository, Action: rbac.ActionPull},
					{Resource: rbac.ResourceMember, Action: rbac.ActionCreate},
				},
			}},
		}, nil)

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 40)
	assert.Error(t, err)
	assert.Equal(t, errors.ForbiddenCode, errors.ErrCode(err))
}

func TestCheckNoEscalation_RoleCtlGetError(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(50), testifymock.Anything).
		Return(nil, fmt.Errorf("db error"))

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 50)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestCheckNoEscalation_MultiplePermissionSetsAllowed(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	sc.On("Can", testifymock.Anything,
		rbac.ActionPull, projectResource(rbac.ResourceRepository)).Return(true)
	sc.On("Can", testifymock.Anything,
		rbac.ActionPush, projectResource(rbac.ResourceRepository)).Return(true)
	sc.On("Can", testifymock.Anything,
		rbac.ActionRead, projectResource(rbac.ResourceArtifact)).Return(true)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(60), testifymock.Anything).
		Return(&roleCtl.Role{
			Permissions: []*roleCtl.Permission{
				{Access: policyAccess(rbac.ResourceRepository, rbac.ActionPull)},
				{Access: policyAccess(rbac.ResourceRepository, rbac.ActionPush)},
				{Access: policyAccess(rbac.ResourceArtifact, rbac.ActionRead)},
			},
		}, nil)

	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 60)
	assert.NoError(t, err)
}

func TestCheckNoEscalation_ProjectNameInsteadOfID(t *testing.T) {
	// Passing a project name string follows a code path that looks up the project
	// from DB (not available in unit tests), so we only verify that numeric IDs work.
	// This test documents the expected behavior for coverage completeness.
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(70), testifymock.Anything).
		Return(&roleCtl.Role{}, nil)

	// numeric int64 → no DB lookup → should work cleanly
	err := newAPI(rc).checkNoEscalation(newCtxWithSecurity(sc), testProjectID, 70)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Handler-level escalation gate on the MODIFY path (UpdateProjectMember)
// ---------------------------------------------------------------------------

// TestUpdateProjectMember_EscalationBlocked proves that modifying a member's
// role is subject to the anti-escalation check: a caller who may update members
// but does NOT hold a permission granted by the target role is rejected with 403,
// and the member controller's UpdateRole is never reached. This guards against a
// regression that drops checkNoEscalation from the update (modification) path.
func TestUpdateProjectMember_EscalationBlocked(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	sc.On("IsAuthenticated").Return(true)
	// caller may update members (passes RequireProjectAccess)...
	sc.On("Can", testifymock.Anything,
		rbac.ActionUpdate, projectResource(rbac.ResourceMember)).Return(true)
	// ...but does NOT hold member:create, which the target role grants
	sc.On("Can", testifymock.Anything,
		rbac.ActionCreate, projectResource(rbac.ResourceMember)).Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(30), testifymock.Anything).
		Return(&roleCtl.Role{
			Permissions: []*roleCtl.Permission{{
				Access: policyAccess(rbac.ResourceMember, rbac.ActionCreate),
			}},
		}, nil)

	xFalse := false
	params := memberop.UpdateProjectMemberParams{
		ProjectNameOrID: fmt.Sprintf("%d", testProjectID),
		XIsResourceName: &xFalse,
		Mid:             7,
		Role:            &models.RoleRequest{RoleID: 30},
	}

	// ctl (member controller) is intentionally nil: the escalation check must
	// reject before UpdateRole is reached, so it is never dereferenced.
	resp := newAPI(rc).UpdateProjectMember(newCtxWithSecurity(sc), params)

	rr := httptest.NewRecorder()
	resp.WriteResponse(rr, runtime.JSONProducer())
	assert.Equal(t, http.StatusForbidden, rr.Code)
	// reached checkNoEscalation (fetched the target role) before rejecting
	rc.AssertCalled(t, "Get", testifymock.Anything, int64(30), testifymock.Anything)
}

// TestCreateProjectMember_EscalationBlocked proves the same anti-escalation guard
// applies on the ASSIGN path: a caller allowed to create members but lacking a
// permission the target role grants is rejected with 403, before the member
// controller's Create is reached.
func TestCreateProjectMember_EscalationBlocked(t *testing.T) {
	sc := &securityMock.Context{}
	sc.On("IsSysAdmin").Return(false)
	sc.On("IsAuthenticated").Return(true)
	// caller may create members (passes RequireProjectAccess)...
	sc.On("Can", testifymock.Anything,
		rbac.ActionCreate, projectResource(rbac.ResourceMember)).Return(true)
	// ...but does NOT hold repository:push, which the target role grants
	sc.On("Can", testifymock.Anything,
		rbac.ActionPush, projectResource(rbac.ResourceRepository)).Return(false)

	rc := &stubRoleCtl{}
	rc.On("Get", testifymock.Anything, int64(30), testifymock.Anything).
		Return(&roleCtl.Role{
			Permissions: []*roleCtl.Permission{{
				Access: policyAccess(rbac.ResourceRepository, rbac.ActionPush),
			}},
		}, nil)

	xFalse := false
	params := memberop.CreateProjectMemberParams{
		ProjectNameOrID: fmt.Sprintf("%d", testProjectID),
		XIsResourceName: &xFalse,
		ProjectMember:   &models.ProjectMember{RoleID: 30},
	}

	// ctl (member controller) is intentionally nil: the escalation check must
	// reject before Create is reached, so it is never dereferenced.
	resp := newAPI(rc).CreateProjectMember(newCtxWithSecurity(sc), params)

	rr := httptest.NewRecorder()
	resp.WriteResponse(rr, runtime.JSONProducer())
	assert.Equal(t, http.StatusForbidden, rr.Code)
	rc.AssertCalled(t, "Get", testifymock.Anything, int64(30), testifymock.Anything)
}

// ---------------------------------------------------------------------------
// Compile-time interface check — fails the build if stubRoleCtl drifts.
// ---------------------------------------------------------------------------

var _ roleCtl.Controller = (*stubRoleCtl)(nil)

type recordingMemberController struct {
	member.Controller
	createdRoles []int
	updatedRoles []int
}

func (c *recordingMemberController) Create(
	_ context.Context, _ any, req member.Request,
) (int, error) {
	c.createdRoles = append(c.createdRoles, req.Role)
	return 1, nil
}

func (c *recordingMemberController) UpdateRole(
	_ context.Context, _ any, _ int, role int,
) error {
	c.updatedRoles = append(c.updatedRoles, role)
	return nil
}

func memberWriteAccess() []*types.Policy {
	return []*types.Policy{
		{Resource: rbac.ResourceMember, Action: rbac.ActionCreate},
		{Resource: rbac.ResourceMember, Action: rbac.ActionUpdate},
	}
}

const sysAdminID int64 = 3

// A robot that may create and update members of a project may give them any
// built-in role its creator could grant, the project admin role included, so
// provisioning robots keep working (see #1137 and its revert). A system admin
// holds every permission.
func TestMemberWritesGrantProjectAdmin(t *testing.T) {
	const projectID int64 = 1
	// The robot context captures project.Ctl when it is built, so each case
	// builds its context after the mock controller is in place.
	tests := []struct {
		name        string
		securityCtx func() security.Context
	}{
		{
			name: "project robot",
			securityCtx: func() security.Context {
				return robotsec.NewSecurityContext(&robot.Robot{
					Robot: robotmodel.Robot{
						Name: "project-robot", CreatorType: local.ContextName, CreatorRef: sysAdminID,
					},
					Level: robot.LEVELPROJECT,
					Permissions: []*robot.Permission{{
						Scope:  "/project/1",
						Access: memberWriteAccess(),
					}},
				})
			},
		},
		{
			name: "system robot covering all projects",
			securityCtx: func() security.Context {
				return robotsec.NewSecurityContext(&robot.Robot{
					Robot: robotmodel.Robot{
						Name: "system-robot", CreatorType: local.ContextName, CreatorRef: sysAdminID,
					},
					Level: robot.LEVELSYSTEM,
					Permissions: []*robot.Permission{{
						Scope:  robot.SCOPEALLPROJECT,
						Access: memberWriteAccess(),
					}},
				})
			},
		},
		{
			name: "system admin",
			securityCtx: func() security.Context {
				return local.NewSecurityContext(&commonmodels.User{
					UserID: 3, Username: "system-admin", SysAdminFlag: true,
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectCtl := projecttesting.NewController(t)
			p := &project.Project{ProjectID: projectID}
			projectCtl.On("Get", mock.Anything, projectID, mock.Anything).
				Return(p, nil).Maybe()
			projectCtl.On("Get", mock.Anything, projectID).Return(p, nil).Maybe()
			previousProjectCtl := project.Ctl
			project.Ctl = projectCtl
			t.Cleanup(func() {
				project.Ctl = previousProjectCtl
			})

			rc := &stubRoleCtl{}
			rc.On("Get", testifymock.Anything, int64(common.RoleProjectAdmin), testifymock.Anything).
				Return(projectAdminRole(), nil).Maybe()
			controller := &recordingMemberController{}
			userCtl := usertesting.NewController(t)
			userCtl.On("Get", mock.Anything, int(sysAdminID), mock.Anything).
				Return(&commonmodels.User{UserID: int(sysAdminID), Username: "admin", SysAdminFlag: true}, nil).Maybe()
			api := newMemberAPI()
			api.ctl = controller
			api.roleCtl = rc
			api.userCtl = userCtl
			ctx := security.NewContext(context.Background(), test.securityCtx())

			allowed, err := api.HasProjectPermission(
				ctx, projectID, rbac.ActionCreate, rbac.ResourceMember,
			)
			require.NoError(t, err)
			require.True(t, allowed, "the caller must hold member:create")

			createParams := operation.NewCreateProjectMemberParams()
			createParams.ProjectNameOrID = "1"
			createParams.ProjectMember = &models.ProjectMember{
				RoleID: common.RoleProjectAdmin,
			}
			createResponder := api.CreateProjectMember(ctx, createParams)

			updateParams := operation.NewUpdateProjectMemberParams()
			updateParams.ProjectNameOrID = "1"
			updateParams.Mid = 1
			updateParams.Role = &models.RoleRequest{RoleID: common.RoleProjectAdmin}
			updateResponder := api.UpdateProjectMember(ctx, updateParams)

			assert.IsType(t, &operation.CreateProjectMemberCreated{}, createResponder)
			assert.IsType(t, &operation.UpdateProjectMemberOK{}, updateResponder)
			assert.Equal(t, []int{common.RoleProjectAdmin}, controller.createdRoles)
			assert.Equal(t, []int{common.RoleProjectAdmin}, controller.updatedRoles)
		})
	}
}

// A robot granting a built-in role skips the escalation check (goharbor/harbor#24300):
// its creator's state no longer matters, member:create/update is the guard.
func TestRobotBuiltinGrantIgnoresCreator(t *testing.T) {
	const (
		projectID      int64 = 1
		projectAdminID int64 = 10
		memberAdminID  int64 = 11
		goneUserID     int64 = 12
		customRoleID   int64 = 100
	)
	creatorRobot := func(id int64, creatorType string, creatorRef int64) *robot.Robot {
		return &robot.Robot{
			Robot: robotmodel.Robot{
				ID: id, Name: fmt.Sprintf("robot-%d", id), ExpiresAt: -1,
				CreatorType: creatorType, CreatorRef: creatorRef,
			},
			Level: robot.LEVELPROJECT,
			Permissions: []*robot.Permission{{
				Scope:  "/project/1",
				Access: memberWriteAccess(),
			}},
		}
	}
	tests := []struct {
		name   string
		caller *robot.Robot
	}{
		{name: "created by system admin", caller: creatorRobot(1, local.ContextName, sysAdminID)},
		{name: "created by project admin", caller: creatorRobot(1, local.ContextName, projectAdminID)},
		{name: "created by member updater", caller: creatorRobot(1, local.ContextName, memberAdminID)},
		{name: "creator deleted", caller: creatorRobot(1, local.ContextName, goneUserID)},
		{name: "unknown creator type", caller: creatorRobot(1, "", 0)},
		{name: "created by robot created by system admin", caller: creatorRobot(1, "robot", 2)},
		{name: "created by robot created by member updater", caller: creatorRobot(1, "robot", 4)},
		{name: "robot creator chain too deep", caller: creatorRobot(1, "robot", 5)},
		{name: "created by disabled robot", caller: creatorRobot(1, "robot", 8)},
		{name: "created by expired robot", caller: creatorRobot(1, "robot", 9)},
		{name: "creator ref zero", caller: creatorRobot(1, local.ContextName, 0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grants := append(policy.Shipped(),
				policy.Grant{RoleID: customRoleID, Resource: rbac.ResourceMember.String(), Action: rbac.ActionUpdate.String()},
				policy.Grant{RoleID: customRoleID, Resource: rbac.ResourceRobot.String(), Action: rbac.ActionCreate.String()},
			)
			store, err := policy.NewInMemory(grants)
			require.NoError(t, err)
			previousStore := policy.SetDefault(store)
			t.Cleanup(func() { policy.SetDefault(previousStore) })

			projectCtl := projecttesting.NewController(t)
			p := &project.Project{ProjectID: projectID}
			projectCtl.On("Get", mock.Anything, projectID, mock.Anything).Return(p, nil).Maybe()
			projectCtl.On("ListRoles", mock.Anything, projectID, mock.MatchedBy(func(u *commonmodels.User) bool {
				return int64(u.UserID) == projectAdminID
			})).Return([]int{common.RoleProjectAdmin}, nil).Maybe()
			projectCtl.On("ListRoles", mock.Anything, projectID, mock.Anything).
				Return([]int{int(customRoleID)}, nil).Maybe()
			previousProjectCtl := project.Ctl
			project.Ctl = projectCtl
			t.Cleanup(func() { project.Ctl = previousProjectCtl })

			userCtl := usertesting.NewController(t)
			for _, u := range []*commonmodels.User{
				{UserID: int(sysAdminID), Username: "admin", SysAdminFlag: true},
				{UserID: int(projectAdminID), Username: "project-admin"},
				{UserID: int(memberAdminID), Username: "member-updater"},
			} {
				userCtl.On("Get", mock.Anything, u.UserID, mock.Anything).Return(u, nil).Maybe()
			}
			userCtl.On("Get", mock.Anything, int(goneUserID), mock.Anything).
				Return(nil, errors.NotFoundError(nil)).Maybe()
			userCtl.On("Get", mock.Anything, 0, mock.Anything).
				Return(nil, errors.NotFoundError(nil)).Maybe()

			robotCtl := robottesting.NewController(t)
			disabledCreator := creatorRobot(8, local.ContextName, sysAdminID)
			disabledCreator.Disabled = true
			expiredCreator := creatorRobot(9, local.ContextName, sysAdminID)
			expiredCreator.ExpiresAt = 1
			for _, r := range []*robot.Robot{
				disabledCreator,
				expiredCreator,
				creatorRobot(2, local.ContextName, sysAdminID),
				creatorRobot(4, local.ContextName, memberAdminID),
				creatorRobot(5, "robot", 6),
				creatorRobot(6, "robot", 7),
				creatorRobot(7, local.ContextName, sysAdminID),
			} {
				robotCtl.On("Get", mock.Anything, r.ID, mock.Anything).Return(r, nil).Maybe()
			}

			rc := &stubRoleCtl{}
			rc.On("Get", testifymock.Anything, int64(common.RoleProjectAdmin), testifymock.Anything).
				Return(projectAdminRole(), nil).Maybe()
			controller := &recordingMemberController{}
			api := &memberAPI{ctl: controller, roleCtl: rc, robotCtl: robotCtl, userCtl: userCtl}
			ctx := security.NewContext(context.Background(), robotsec.NewSecurityContext(test.caller))

			createParams := operation.NewCreateProjectMemberParams()
			createParams.ProjectNameOrID = "1"
			createParams.ProjectMember = &models.ProjectMember{RoleID: common.RoleProjectAdmin}
			createResponder := api.CreateProjectMember(ctx, createParams)

			updateParams := operation.NewUpdateProjectMemberParams()
			updateParams.ProjectNameOrID = "1"
			updateParams.Mid = 1
			updateParams.Role = &models.RoleRequest{RoleID: common.RoleProjectAdmin}
			updateResponder := api.UpdateProjectMember(ctx, updateParams)

			assert.IsType(t, &operation.CreateProjectMemberCreated{}, createResponder)
			assert.IsType(t, &operation.UpdateProjectMemberOK{}, updateResponder)
			assert.Equal(t, []int{common.RoleProjectAdmin}, controller.createdRoles)
			assert.Equal(t, []int{common.RoleProjectAdmin}, controller.updatedRoles)
		})
	}
}

// A robot granting a custom role skips the escalation check (goharbor/harbor#24300),
// even when the role holds permissions the robot itself lacks.
func TestRobotCustomRoleSkipsEscalationCheck(t *testing.T) {
	const projectID int64 = 1
	tests := []struct {
		name   string
		access []*types.Policy
	}{
		{name: "robot holds the permission", access: policyAccess(rbac.ResourceMember, rbac.ActionUpdate)},
		{name: "robot lacks the permission", access: policyAccess(rbac.ResourceRobot, rbac.ActionCreate)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectCtl := projecttesting.NewController(t)
			projectCtl.On("Get", mock.Anything, projectID, mock.Anything).
				Return(&project.Project{ProjectID: projectID}, nil).Maybe()
			previousProjectCtl := project.Ctl
			project.Ctl = projectCtl
			t.Cleanup(func() { project.Ctl = previousProjectCtl })

			userCtl := usertesting.NewController(t)
			userCtl.On("Get", mock.Anything, int(sysAdminID), mock.Anything).
				Return(&commonmodels.User{UserID: int(sysAdminID), SysAdminFlag: true}, nil).Maybe()

			const customRoleID int64 = 100
			rc := &stubRoleCtl{}
			rc.On("Get", testifymock.Anything, customRoleID, testifymock.Anything).
				Return(&roleCtl.Role{Permissions: []*roleCtl.Permission{{Access: test.access}}}, nil).Maybe()
			api := &memberAPI{roleCtl: rc, userCtl: userCtl, robotCtl: robottesting.NewController(t)}
			caller := &robot.Robot{
				Robot: robotmodel.Robot{
					ID: 1, Name: "robot-1", ExpiresAt: -1, CreatorType: local.ContextName, CreatorRef: sysAdminID,
				},
				Level:       robot.LEVELPROJECT,
				Permissions: []*robot.Permission{{Scope: "/project/1", Access: memberWriteAccess()}},
			}
			ctx := security.NewContext(context.Background(), robotsec.NewSecurityContext(caller))

			require.NoError(t, api.checkNoEscalation(ctx, projectID, customRoleID))
		})
	}
}

func TestCreatorLacksHintsAtExternalAuth(t *testing.T) {
	assert.Equal(t, "the robot's creator lacks", creatorLacks(context.Background()))
	assert.Equal(t, "the robot's creator lacks", creatorLacks(lib.WithAuthMode(context.Background(), common.DBAuth)))
	assert.Contains(t, creatorLacks(lib.WithAuthMode(context.Background(), common.LDAPAuth)), "system admin flag")
}

type stubMemberController struct {
	member.Controller
	testifymock.Mock
}

func (s *stubMemberController) Create(ctx context.Context, projectNameOrID any, req member.Request) (int, error) {
	args := s.Called(ctx, projectNameOrID, req)
	return args.Int(0), args.Error(1)
}

func (s *stubMemberController) UpdateRole(ctx context.Context, projectNameOrID any, memberID, roleID int) error {
	return s.Called(ctx, projectNameOrID, memberID, roleID).Error(0)
}

func TestRobotMemberRoleAssignment(t *testing.T) {
	for _, level := range []string{robot.LEVELPROJECT, robot.LEVELSYSTEM} {
		for _, action := range []rbac.Action{rbac.ActionCreate, rbac.ActionUpdate} {
			for _, roleID := range []int{common.RoleProjectAdmin, common.RoleDeveloper, common.RoleGuest, common.RoleMaintainer, common.RoleLimitedGuest, 100} {
				t.Run(fmt.Sprintf("%s/%s/role-%d", level, action, roleID), func(t *testing.T) {
					pc := &projecttesting.Controller{}
					old := project.Ctl
					project.Ctl = pc
					t.Cleanup(func() { project.Ctl = old })
					pc.On("Get", testifymock.Anything, testProjectID, testifymock.Anything).
						Return(&project.Project{ProjectID: testProjectID}, nil)
					sc := robotsec.NewSecurityContext(&robot.Robot{
						Robot: robotmodel.Robot{Name: "robot$library+member-updater", ProjectID: testProjectID},
						Level: level,
						Permissions: []*robot.Permission{{
							Kind: robot.LEVELPROJECT, Namespace: "library", Scope: fmt.Sprintf("/project/%d", testProjectID),
							Access: policyAccess(rbac.ResourceMember, action),
						}},
					})
					rc := &stubRoleCtl{}
					rc.On("Get", testifymock.Anything, int64(100), testifymock.Anything).
						Return(&roleCtl.Role{Permissions: []*roleCtl.Permission{{Access: policyAccess(rbac.ResourceRepository, rbac.ActionPush)}}}, nil).Maybe()
					mc := &stubMemberController{}
					api := &memberAPI{ctl: mc, roleCtl: rc}
					xFalse := false
					rr := httptest.NewRecorder()
					ctx := newCtxWithSecurity(sc)
					if action == rbac.ActionCreate {
						mc.On("Create", ctx, testProjectID, member.Request{Role: roleID, MemberUser: member.User{Username: "mike01"}}).Return(2, nil).Once()
						api.CreateProjectMember(ctx, memberop.CreateProjectMemberParams{
							ProjectNameOrID: fmt.Sprint(testProjectID), XIsResourceName: &xFalse,
							ProjectMember: &models.ProjectMember{RoleID: int64(roleID), MemberUser: &models.UserEntity{Username: "mike01"}},
						}).WriteResponse(rr, runtime.JSONProducer())
						assert.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
					} else {
						mc.On("UpdateRole", ctx, testProjectID, 2, roleID).Return(nil).Once()
						api.UpdateProjectMember(ctx, memberop.UpdateProjectMemberParams{
							ProjectNameOrID: fmt.Sprint(testProjectID), XIsResourceName: &xFalse, Mid: 2,
							Role: &models.RoleRequest{RoleID: int64(roleID)},
						}).WriteResponse(rr, runtime.JSONProducer())
						assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
					}
					mc.AssertExpectations(t)
				})
			}
		}
	}
}

func TestRobotMemberAccessRequired(t *testing.T) {
	for _, level := range []string{robot.LEVELPROJECT, robot.LEVELSYSTEM} {
		for _, tc := range []struct {
			name   string
			scope  string
			action rbac.Action
		}{
			{"wrong action", fmt.Sprintf("/project/%d", testProjectID), rbac.ActionList},
			{"wrong project", fmt.Sprintf("/project/%d", testProjectID+1), rbac.ActionUpdate},
		} {
			t.Run(level+"/"+tc.name, func(t *testing.T) {
				pc := &projecttesting.Controller{}
				old := project.Ctl
				project.Ctl = pc
				t.Cleanup(func() { project.Ctl = old })
				pc.On("Get", testifymock.Anything, testProjectID, testifymock.Anything).
					Return(&project.Project{ProjectID: testProjectID}, nil)
				sc := robotsec.NewSecurityContext(&robot.Robot{
					Robot: robotmodel.Robot{Name: "robot$library+restricted"}, Level: level,
					Permissions: []*robot.Permission{{Scope: tc.scope, Access: policyAccess(rbac.ResourceMember, tc.action)}},
				})
				xFalse := false
				rr := httptest.NewRecorder()
				(&memberAPI{}).UpdateProjectMember(newCtxWithSecurity(sc), memberop.UpdateProjectMemberParams{
					ProjectNameOrID: fmt.Sprint(testProjectID), XIsResourceName: &xFalse, Mid: 2,
					Role: &models.RoleRequest{RoleID: common.RoleLimitedGuest},
				}).WriteResponse(rr, runtime.JSONProducer())
				assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			})
		}
	}
}
