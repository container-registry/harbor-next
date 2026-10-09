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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	commonmodels "github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/local"
	robotsec "github.com/goharbor/harbor/src/common/security/robot"
	"github.com/goharbor/harbor/src/controller/member"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/controller/robot"
	liberrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	robotmodel "github.com/goharbor/harbor/src/pkg/robot/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/member"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	"github.com/goharbor/harbor/src/testing/mock"
)

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

func TestProjectAdminGrantRequiresAdminRole(t *testing.T) {
	const projectID int64 = 1
	tests := []struct {
		name          string
		user          *commonmodels.User
		robot         *robot.Robot
		roles         []int
		wantForbidden bool
	}{
		{
			name:          "project robot",
			robot:         memberRobot(robot.LEVELPROJECT, "/project/1", rbac.ActionCreate, rbac.ActionUpdate),
			wantForbidden: true,
		},
		{
			name:  "system robot covering all projects",
			robot: memberRobot(robot.LEVELSYSTEM, robot.SCOPEALLPROJECT, rbac.ActionCreate, rbac.ActionUpdate),
		},
		{
			name:          "system robot scoped to one project",
			robot:         memberRobot(robot.LEVELSYSTEM, "/project/1", rbac.ActionCreate, rbac.ActionUpdate),
			wantForbidden: true,
		},
		{
			name:          "maintainer",
			user:          &commonmodels.User{UserID: 4, Username: "maintainer"},
			roles:         []int{common.RoleMaintainer},
			wantForbidden: true,
		},
		{
			name:          "non-member",
			user:          &commonmodels.User{UserID: 5, Username: "non-member"},
			wantForbidden: true,
		},
		{
			name:  "project admin",
			user:  &commonmodels.User{UserID: 2, Username: "project-admin"},
			roles: []int{common.RoleProjectAdmin},
		},
		{
			name: "system admin",
			user: &commonmodels.User{
				UserID: 3, Username: "system-admin", SysAdminFlag: true,
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
			projectCtl.On("ListRoles", mock.Anything, projectID, mock.Anything).
				Return(test.roles, nil).Maybe()

			previousProjectCtl := project.Ctl
			project.Ctl = projectCtl
			t.Cleanup(func() {
				project.Ctl = previousProjectCtl
			})

			var securityCtx security.Context
			if test.robot != nil {
				securityCtx = robotsec.NewSecurityContext(test.robot)
			} else {
				securityCtx = local.NewSecurityContext(test.user)
			}

			controller := &recordingMemberController{}
			api := newMemberAPI()
			api.ctl = controller
			ctx := security.NewContext(context.Background(), securityCtx)
			if test.robot != nil {
				allowed, err := api.HasProjectPermission(
					ctx, projectID, rbac.ActionCreate, rbac.ResourceMember,
				)
				require.NoError(t, err)
				require.True(t, allowed)
			}

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

			if test.wantForbidden {
				createError, ok := createResponder.(*ErrResponder)
				require.True(t, ok)
				assert.True(t, liberrors.IsErr(createError.err, liberrors.ForbiddenCode))
				updateError, ok := updateResponder.(*ErrResponder)
				require.True(t, ok)
				assert.True(t, liberrors.IsErr(updateError.err, liberrors.ForbiddenCode))
				assert.Empty(t, controller.createdRoles)
				assert.Empty(t, controller.updatedRoles)
				return
			}

			assert.IsType(t, &operation.CreateProjectMemberCreated{}, createResponder)
			assert.IsType(t, &operation.UpdateProjectMemberOK{}, updateResponder)
			assert.Equal(t, []int{common.RoleProjectAdmin}, controller.createdRoles)
			assert.Equal(t, []int{common.RoleProjectAdmin}, controller.updatedRoles)
		})
	}
}

// The generic member permission check already stops local users without a
// project-admin role, so drive requireProjectAdminGrant directly to prove its
// own role check denies them too.
func TestRequireProjectAdminGrantDeniesLocalNonAdmin(t *testing.T) {
	const projectID int64 = 1
	for _, roles := range [][]int{nil, {common.RoleDeveloper}, {common.RoleMaintainer}} {
		projectCtl := projecttesting.NewController(t)
		projectCtl.On("Get", mock.Anything, projectID).
			Return(&project.Project{ProjectID: projectID}, nil)
		projectCtl.On("ListRoles", mock.Anything, projectID, mock.Anything).
			Return(roles, nil)

		api := &memberAPI{projectCtl: projectCtl}
		user := &commonmodels.User{UserID: 6, Username: "local-user"}
		ctx := security.NewContext(context.Background(), local.NewSecurityContext(user))

		err := api.requireProjectAdminGrant(ctx, projectID, common.RoleProjectAdmin, rbac.ActionCreate)
		assert.True(t, liberrors.IsErr(err, liberrors.ForbiddenCode), "roles %v", roles)
		assert.NoError(t, api.requireProjectAdminGrant(ctx, projectID, common.RoleDeveloper, rbac.ActionCreate))
	}
}

func memberRobot(level, scope string, actions ...rbac.Action) *robot.Robot {
	access := make([]*types.Policy, 0, len(actions))
	for _, action := range actions {
		access = append(access, &types.Policy{Resource: rbac.ResourceMember, Action: action})
	}
	return &robot.Robot{
		Robot:       robotmodel.Robot{Name: "member-robot"},
		Level:       level,
		Permissions: []*robot.Permission{{Scope: scope, Access: access}},
	}
}

// A system robot is exempt only for the member action its cover-all
// permission allows; anything narrower stays below project-admin.
func TestRequireProjectAdminGrantSystemRobot(t *testing.T) {
	const projectID int64 = 1
	tests := []struct {
		name          string
		robot         *robot.Robot
		action        rbac.Action
		wantForbidden bool
	}{
		{
			name:   "create allowed by cover-all member:create",
			robot:  memberRobot(robot.LEVELSYSTEM, robot.SCOPEALLPROJECT, rbac.ActionCreate),
			action: rbac.ActionCreate,
		},
		{
			name:          "update without cover-all member:update",
			robot:         memberRobot(robot.LEVELSYSTEM, robot.SCOPEALLPROJECT, rbac.ActionCreate),
			action:        rbac.ActionUpdate,
			wantForbidden: true,
		},
		{
			name:          "cover-all scope without member permission",
			robot:         memberRobot(robot.LEVELSYSTEM, robot.SCOPEALLPROJECT),
			action:        rbac.ActionCreate,
			wantForbidden: true,
		},
		{
			name: "cover-all member:create denied",
			robot: &robot.Robot{
				Level: robot.LEVELSYSTEM,
				Permissions: []*robot.Permission{{
					Scope: robot.SCOPEALLPROJECT,
					Access: []*types.Policy{{
						Resource: rbac.ResourceMember, Action: rbac.ActionCreate, Effect: types.EffectDeny,
					}},
				}},
			},
			action:        rbac.ActionCreate,
			wantForbidden: true,
		},
		{
			name: "cover-all member:create with an unknown effect",
			robot: &robot.Robot{
				Level: robot.LEVELSYSTEM,
				Permissions: []*robot.Permission{{
					Scope: robot.SCOPEALLPROJECT,
					Access: []*types.Policy{{
						Resource: rbac.ResourceMember, Action: rbac.ActionCreate, Effect: "bogus",
					}},
				}},
			},
			action:        rbac.ActionCreate,
			wantForbidden: true,
		},
		{
			name:          "project robot carrying a cover-all scope",
			robot:         memberRobot(robot.LEVELPROJECT, robot.SCOPEALLPROJECT, rbac.ActionCreate),
			action:        rbac.ActionCreate,
			wantForbidden: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &memberAPI{}
			ctx := security.NewContext(context.Background(), robotsec.NewSecurityContext(test.robot))
			err := api.requireProjectAdminGrant(ctx, projectID, common.RoleProjectAdmin, test.action)
			if test.wantForbidden {
				assert.True(t, liberrors.IsErr(err, liberrors.ForbiddenCode))
				return
			}
			assert.NoError(t, err)
		})
	}
}
