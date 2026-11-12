//  Copyright Project Harbor Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package member

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	comModels "github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/member"
	"github.com/goharbor/harbor/src/pkg/project"
	"github.com/goharbor/harbor/src/pkg/project/models"
	roleModel "github.com/goharbor/harbor/src/pkg/role/model"
	"github.com/goharbor/harbor/src/pkg/user"
	"github.com/goharbor/harbor/src/pkg/usergroup"
	modelGroup "github.com/goharbor/harbor/src/pkg/usergroup/model"
	"github.com/goharbor/harbor/src/testing/mock"
	mockMember "github.com/goharbor/harbor/src/testing/pkg/member"
	mockProject "github.com/goharbor/harbor/src/testing/pkg/project"
	mockRole "github.com/goharbor/harbor/src/testing/pkg/role"
	mockUser "github.com/goharbor/harbor/src/testing/pkg/user"
	mockUsergroup "github.com/goharbor/harbor/src/testing/pkg/usergroup"
)

type MemberControllerTestSuite struct {
	suite.Suite
	userManager   user.Manager
	memberManager member.Manager
	projectMgr    project.Manager
	groupManager  usergroup.Manager
	roleManager   *mockRole.Manager
	controller    *controller
}

func (suite *MemberControllerTestSuite) SetupSuite() {
	suite.userManager = &mockUser.Manager{}
	suite.memberManager = &mockMember.Manager{}
	suite.projectMgr = &mockProject.Manager{}
	suite.groupManager = &mockUsergroup.Manager{}
	suite.roleManager = &mockRole.Manager{}
	// Role 1 exists; role 4 (a deleted built-in) and role 100 do not.
	suite.roleManager.On("Get", mock.Anything, int64(1)).Return(&roleModel.Role{ID: 1, Name: "projectAdmin", IsBuiltin: true}, nil)
	suite.roleManager.On("Get", mock.Anything, int64(4)).Return(nil, errors.NotFoundError(nil).WithMessage("role 4 not found"))
	suite.roleManager.On("Get", mock.Anything, int64(100)).Return(nil, errors.NotFoundError(nil).WithMessage("role 100 not found"))
	suite.controller = &controller{
		userManager:  suite.userManager,
		mgr:          suite.memberManager,
		projectMgr:   suite.projectMgr,
		groupManager: suite.groupManager,
		roleMgr:      suite.roleManager,
	}
}

// Every role id is looked up: a built-in that was deleted is as invalid as a
// custom role that never existed, and both fail before the project_member
// foreign key could.
func (suite *MemberControllerTestSuite) TestValidateRole() {
	ctx := context.Background()
	suite.NoError(suite.controller.validateRole(ctx, 1))
	for _, id := range []int{0, -1, 4, 100} {
		err := suite.controller.validateRole(ctx, id)
		suite.Require().Error(err, "role %d", id)
		suite.True(errors.IsErr(err, errors.BadRequestCode), "role %d", id)
	}
}

func (suite *MemberControllerTestSuite) TestUpdateRoleToDeletedBuiltin() {
	mock.OnAnything(suite.projectMgr, "Get").Return(&models.Project{ProjectID: 1}, nil)
	err := suite.controller.UpdateRole(context.Background(), 1, 7, 4)
	suite.Require().Error(err)
	suite.True(errors.IsErr(err, errors.BadRequestCode))
	suite.memberManager.(*mockMember.Manager).AssertNotCalled(suite.T(), "UpdateRole", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (suite *MemberControllerTestSuite) TearDownSuite() {

}

func (suite *MemberControllerTestSuite) TestAddProjectMemberWithUser() {
	mock.OnAnything(suite.projectMgr, "Get").Return(&models.Project{
		ProjectID: 1,
	}, nil)
	suite.userManager.(*mockUser.Manager).On("Get", mock.Anything, 1).Return(nil, fmt.Errorf("user not found"))
	_, err := suite.controller.Create(nil, 1, Request{MemberUser: User{UserID: 1}})
	suite.Error(err)
	_, err = suite.controller.Create(nil, 1, Request{MemberUser: User{UserID: 1}})
	suite.Error(err)
	suite.userManager.(*mockUser.Manager).On("Get", mock.Anything, 2).Return(&comModels.User{UserID: 2, Username: "mike"}, nil)
	suite.memberManager.(*mockMember.Manager).On("Add", mock.Anything, 1, 2).Return(nil)
	mock.OnAnything(suite.memberManager, "List").Return(nil, nil)
	mock.OnAnything(suite.memberManager, "AddProjectMember").Return(0, nil)
	_, err = suite.controller.Create(nil, 1, Request{MemberUser: User{UserID: 2}, Role: 1})
	suite.NoError(err)
}

func (suite *MemberControllerTestSuite) TestAddProjectMemberWithUserGroup() {
	mock.OnAnything(suite.projectMgr, "Get").Return(&models.Project{
		ProjectID: 1,
	}, nil)
	suite.groupManager.(*mockUsergroup.Manager).On("Get", mock.Anything, 1).Return(nil, fmt.Errorf("user group not found"))
	_, err := suite.controller.Create(nil, 1, Request{MemberGroup: UserGroup{ID: 1}})
	suite.Error(err)
	suite.groupManager.(*mockUsergroup.Manager).On("Get", mock.Anything, 1).Return(nil, fmt.Errorf("group not found"))
	_, err = suite.controller.Create(nil, 1, Request{MemberGroup: UserGroup{ID: 1}})
	suite.Error(err)
	suite.groupManager.(*mockUsergroup.Manager).On("Get", mock.Anything, 2).Return(&modelGroup.UserGroup{ID: 2, GroupName: "group1"}, nil)
	suite.memberManager.(*mockMember.Manager).On("Add", mock.Anything, 1, 2).Return(nil)
	mock.OnAnything(suite.memberManager, "List").Return(nil, nil)
	mock.OnAnything(suite.memberManager, "AddProjectMember").Return(0, nil)
	_, err = suite.controller.Create(nil, 1, Request{MemberGroup: UserGroup{ID: 2}, Role: 1})
	suite.NoError(err)
}

func (suite *MemberControllerTestSuite) TestAddProjectMemberWithEmptyRequest() {
	_, err := suite.controller.Create(context.Background(), 1, Request{})
	suite.Error(err)
	suite.True(errors.IsErr(err, errors.BadRequestCode))

	_, err = suite.controller.Create(context.Background(), 1, Request{Role: 1, MemberUser: User{UserID: -1}})
	suite.Error(err)
	suite.True(errors.IsErr(err, errors.BadRequestCode))
}

func (suite *MemberControllerTestSuite) TestIsProjectAdmin() {
	mock.OnAnything(suite.projectMgr, "ListAdminRolesOfUser").Return([]models.Member{models.Member{ID: 2, ProjectID: 2}}, nil)
	ok, err := suite.controller.IsProjectAdmin(context.Background(), comModels.User{UserID: 1})
	suite.NoError(err)
	suite.True(ok)
}

func TestMemberControllerTestSuite(t *testing.T) {
	suite.Run(t, &MemberControllerTestSuite{})
}
