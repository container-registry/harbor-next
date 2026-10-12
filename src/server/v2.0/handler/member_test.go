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

func memberWriteAccess() []*types.Policy {
	return []*types.Policy{
		{Resource: rbac.ResourceMember, Action: rbac.ActionCreate},
		{Resource: rbac.ResourceMember, Action: rbac.ActionUpdate},
	}
}

// Whoever may create and update members of a project may give them any
// role, the project admin role included. There is no extra check for that
// role, so provisioning robots keep working (see #1137 and its revert).
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
					Robot: robotmodel.Robot{Name: "project-robot"},
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
					Robot: robotmodel.Robot{Name: "system-robot"},
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

			controller := &recordingMemberController{}
			api := newMemberAPI()
			api.ctl = controller
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
