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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/runtime/middleware"
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
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	robotmodel "github.com/goharbor/harbor/src/pkg/robot/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	memberop "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/member"
	"github.com/goharbor/harbor/src/server/v2.0/restapi/operations/permissions"
	robotop "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/robot"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	robottesting "github.com/goharbor/harbor/src/testing/controller/robot"
	"github.com/goharbor/harbor/src/testing/mock"
)

const gaProjectID int64 = 1

var gaLowerRoles = []int{common.RoleDeveloper, common.RoleGuest, common.RoleMaintainer, common.RoleLimitedGuest}

// gaMembers records what reaches the member controller.
type gaMembers struct {
	member.Controller
	created   []member.Request
	updated   []int
	createErr error
	isAdmin   bool
}

func (c *gaMembers) Create(_ context.Context, _ any, req member.Request) (int, error) {
	if c.createErr != nil {
		return 0, c.createErr
	}
	c.created = append(c.created, req)
	return len(c.created), nil
}

func (c *gaMembers) UpdateRole(_ context.Context, _ any, _ int, role int) error {
	c.updated = append(c.updated, role)
	return nil
}

func (c *gaMembers) IsProjectAdmin(context.Context, commonmodels.User) (bool, error) {
	return c.isAdmin, nil
}

// gaInstallProjectCtl replaces project.Ctl, which security contexts capture at
// construction, and baseProjectCtl with a mock that knows project "pN" as ID N.
func gaInstallProjectCtl(t *testing.T, roles func(projectID int64, u *commonmodels.User) []int) {
	ctl := projecttesting.NewController(t)
	byName := func(name string) (*project.Project, error) {
		var id int64
		if _, err := fmt.Sscanf(name, "p%d", &id); err != nil || fmt.Sprintf("p%d", id) != name {
			return nil, errors.NotFoundError(nil)
		}
		return &project.Project{ProjectID: id, Name: name}, nil
	}
	get := func(_ context.Context, idOrName any, _ ...project.Option) (*project.Project, error) {
		switch v := idOrName.(type) {
		case int64:
			return &project.Project{ProjectID: v, Name: fmt.Sprintf("p%d", v)}, nil
		case string:
			return byName(v)
		}
		return nil, errors.NotFoundError(nil)
	}
	ctl.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(get).Maybe()
	ctl.On("Get", mock.Anything, mock.Anything).Return(get).Maybe()
	ctl.On("GetByName", mock.Anything, mock.Anything).
		Return(func(_ context.Context, name string, _ ...project.Option) (*project.Project, error) {
			return byName(name)
		}).Maybe()
	ctl.On("ListRoles", mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, projectID int64, u *commonmodels.User) ([]int, error) {
			if roles == nil {
				return nil, nil
			}
			return roles(projectID, u), nil
		}).Maybe()
	previous, previousBase := project.Ctl, baseProjectCtl
	project.Ctl, baseProjectCtl = ctl, ctl
	t.Cleanup(func() { project.Ctl, baseProjectCtl = previous, previousBase })
}

func gaAllow(resource rbac.Resource, action rbac.Action) *types.Policy {
	return &types.Policy{Resource: resource, Action: action, Effect: types.EffectAllow}
}

func gaDeny(resource rbac.Resource, action rbac.Action) *types.Policy {
	return &types.Policy{Resource: resource, Action: action, Effect: types.EffectDeny}
}

func gaMember(actions ...rbac.Action) []*types.Policy {
	var access []*types.Policy
	for _, action := range actions {
		access = append(access, gaAllow(rbac.ResourceMember, action))
	}
	return access
}

func gaCoverAll(access ...*types.Policy) *robot.Permission {
	return &robot.Permission{Kind: robot.LEVELPROJECT, Namespace: "*", Scope: robot.SCOPEALLPROJECT, Access: access}
}

func gaOnProject(projectID int64, access ...*types.Policy) *robot.Permission {
	return &robot.Permission{
		Kind:      robot.LEVELPROJECT,
		Namespace: fmt.Sprintf("p%d", projectID),
		Scope:     fmt.Sprintf("%s/%d", robot.SCOPEPROJECT, projectID),
		Access:    access,
	}
}

func gaProjectRobot(access ...*types.Policy) *robot.Robot {
	return &robot.Robot{
		Robot:       robotmodel.Robot{ID: 10, Name: "robot$p1+provisioner", ProjectID: gaProjectID},
		Level:       robot.LEVELPROJECT,
		Permissions: []*robot.Permission{gaOnProject(gaProjectID, access...)},
	}
}

func gaSystemRobot(permissions ...*robot.Permission) *robot.Robot {
	return &robot.Robot{
		Robot:       robotmodel.Robot{ID: 20, Name: "robot$provisioner"},
		Level:       robot.LEVELSYSTEM,
		Permissions: permissions,
	}
}

var gaBodies = map[string]*models.ProjectMember{
	"user by name":  {MemberUser: &models.UserEntity{Username: "alice"}},
	"user by id":    {MemberUser: &models.UserEntity{UserID: 7}},
	"group by id":   {MemberGroup: &models.UserGroup{ID: 11, GroupType: common.OIDCGroupType}},
	"ldap group dn": {MemberGroup: &models.UserGroup{LdapGroupDn: "cn=ops,dc=example,dc=com"}},
}

func gaCreate(ctx context.Context, api *memberAPI, body *models.ProjectMember, role int) middleware.Responder {
	params := memberop.NewCreateProjectMemberParams()
	params.ProjectNameOrID = fmt.Sprint(gaProjectID)
	m := *body
	m.RoleID = int64(role)
	params.ProjectMember = &m
	return api.CreateProjectMember(ctx, params)
}

func gaUpdate(ctx context.Context, api *memberAPI, role int) middleware.Responder {
	params := memberop.NewUpdateProjectMemberParams()
	params.ProjectNameOrID = fmt.Sprint(gaProjectID)
	params.Mid = 3
	params.Role = &models.RoleRequest{RoleID: int64(role)}
	return api.UpdateProjectMember(ctx, params)
}

func gaForbidden(t *testing.T, responder middleware.Responder) string {
	t.Helper()
	errResponder, ok := responder.(*ErrResponder)
	require.True(t, ok, "expected an error responder, got %T", responder)
	require.True(t, errors.IsErr(errResponder.err, errors.ForbiddenCode), "expected 403, got %v", errResponder.err)
	return errResponder.err.Error()
}

// gaOutcome says whether project admin (role 1) and the lower built-in roles
// can be granted.
type gaOutcome struct {
	admin bool
	lower bool
	// noMemberWrite marks callers stopped by the member permission check
	// before the escalation check runs.
	noMemberWrite bool
}

func gaAssertGrants(t *testing.T, newCtx func() context.Context, want gaOutcome) {
	t.Helper()
	check := func(t *testing.T, act func(*memberAPI) middleware.Responder, role int, allowed bool, ctl *gaMembers) {
		api := newMemberAPI()
		api.ctl = ctl
		responder := act(api)
		if !allowed {
			msg := gaForbidden(t, responder)
			if role == common.RoleProjectAdmin && !want.noMemberWrite {
				assert.Contains(t, msg, "member:grant-admin", "role %d", role)
			}
			assert.Empty(t, ctl.created)
			assert.Empty(t, ctl.updated)
			return
		}
		_, failed := responder.(*ErrResponder)
		require.False(t, failed, "role %d: %v", role, responder)
	}
	for name, body := range gaBodies {
		t.Run("create "+name, func(t *testing.T) {
			check(t, func(api *memberAPI) middleware.Responder {
				return gaCreate(newCtx(), api, body, common.RoleProjectAdmin)
			},
				common.RoleProjectAdmin, want.admin, &gaMembers{})
			for _, role := range gaLowerRoles {
				check(t, func(api *memberAPI) middleware.Responder { return gaCreate(newCtx(), api, body, role) },
					role, want.lower, &gaMembers{})
			}
		})
	}
	t.Run("update", func(t *testing.T) {
		check(t, func(api *memberAPI) middleware.Responder { return gaUpdate(newCtx(), api, common.RoleProjectAdmin) },
			common.RoleProjectAdmin, want.admin, &gaMembers{})
		for _, role := range gaLowerRoles {
			check(t, func(api *memberAPI) middleware.Responder { return gaUpdate(newCtx(), api, role) },
				role, want.lower, &gaMembers{})
		}
	})
}

func gaRobotCtx(r *robot.Robot) func() context.Context {
	return func() context.Context {
		return security.NewContext(context.Background(), robotsec.NewSecurityContext(r))
	}
}

// member:grant-admin lets a robot give the built-in project admin role, which
// requireProjectAdminGrant otherwise refuses to every robot. The lower roles
// only need member:create or member:update, and a deny for it wins over an allow.
func TestGrantAdminPermissionForRobots(t *testing.T) {
	writes := gaMember(rbac.ActionCreate, rbac.ActionUpdate)
	withGrant := gaMember(rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionGrantAdmin)
	tests := []struct {
		name  string
		robot *robot.Robot
		want  gaOutcome
	}{
		{"project robot with member:create and member:update", gaProjectRobot(writes...), gaOutcome{lower: true}},
		{"project robot with member:grant-admin", gaProjectRobot(withGrant...), gaOutcome{admin: true, lower: true}},
		{"system robot with member:grant-admin on all projects", gaSystemRobot(gaCoverAll(withGrant...)), gaOutcome{admin: true, lower: true}},
		{"system robot with member:grant-admin on the project", gaSystemRobot(gaOnProject(gaProjectID, withGrant...)), gaOutcome{admin: true, lower: true}},
		{
			"member:grant-admin on all projects, denied on the project",
			gaSystemRobot(gaCoverAll(withGrant...), gaOnProject(gaProjectID, gaDeny(rbac.ResourceMember, rbac.ActionGrantAdmin))),
			gaOutcome{lower: true},
		},
		{
			"member:grant-admin allowed and denied on all projects",
			gaSystemRobot(gaCoverAll(append(withGrant, gaDeny(rbac.ResourceMember, rbac.ActionGrantAdmin))...)),
			gaOutcome{lower: true},
		},
		{
			"member:grant-admin on another project only",
			gaSystemRobot(gaOnProject(gaProjectID, writes...), gaOnProject(2, gaMember(rbac.ActionGrantAdmin)...)),
			gaOutcome{lower: true},
		},
		{"member:grant-admin without member:create or member:update", gaProjectRobot(gaMember(rbac.ActionGrantAdmin)...), gaOutcome{noMemberWrite: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gaInstallProjectCtl(t, nil)
			gaAssertGrants(t, gaRobotCtx(test.robot), test.want)
		})
	}
}

func TestGrantAdminPermissionForLocalUsers(t *testing.T) {
	const adminGroup = 42
	rolesOf := func(_ int64, u *commonmodels.User) []int {
		switch {
		case u == nil:
			return nil
		case u.Username == "project-admin":
			return []int{common.RoleProjectAdmin}
		case u.Username == "group-admin" && len(u.GroupIDs) > 0 && u.GroupIDs[0] == adminGroup:
			return []int{common.RoleProjectAdmin}
		case u.Username == "maintainer":
			return []int{common.RoleMaintainer}
		}
		return nil
	}
	tests := []struct {
		name string
		user *commonmodels.User
		want gaOutcome
	}{
		{"system admin", &commonmodels.User{UserID: 2, Username: "admin", SysAdminFlag: true}, gaOutcome{admin: true, lower: true}},
		{"project admin", &commonmodels.User{UserID: 4, Username: "project-admin"}, gaOutcome{admin: true, lower: true}},
		{"project admin through a group", &commonmodels.User{UserID: 5, Username: "group-admin", GroupIDs: []int{adminGroup}}, gaOutcome{admin: true, lower: true}},
		{"maintainer", &commonmodels.User{UserID: 7, Username: "maintainer"}, gaOutcome{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gaInstallProjectCtl(t, rolesOf)
			newCtx := func() context.Context {
				return security.NewContext(context.Background(), local.NewSecurityContext(test.user))
			}
			if test.want.admin {
				gaAssertGrants(t, newCtx, test.want)
				return
			}
			api := newMemberAPI()
			api.ctl = &gaMembers{}
			gaForbidden(t, gaCreate(newCtx(), api, gaBodies["user by id"], common.RoleProjectAdmin))
		})
	}
}

// A grant through member:grant-admin is logged once it has succeeded, naming the
// caller, the grantee, the project and the permission.
func TestGrantAdminIsLogged(t *testing.T) {
	grantRobot := gaSystemRobot(gaCoverAll(gaMember(rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionGrantAdmin)...))
	tests := []struct {
		name     string
		secCtx   func() security.Context
		act      func(context.Context, *memberAPI) middleware.Responder
		failWith error
		wantLog  []string
	}{
		{
			name:   "robot adds a user",
			secCtx: func() security.Context { return robotsec.NewSecurityContext(grantRobot) },
			act: func(ctx context.Context, api *memberAPI) middleware.Responder {
				return gaCreate(ctx, api, gaBodies["user by name"], common.RoleProjectAdmin)
			},
			wantLog: []string{`"robot$provisioner"`, `user "alice"`, "member 1", "project 1", "member:grant-admin"},
		},
		{
			name:   "robot adds an LDAP group",
			secCtx: func() security.Context { return robotsec.NewSecurityContext(grantRobot) },
			act: func(ctx context.Context, api *memberAPI) middleware.Responder {
				return gaCreate(ctx, api, gaBodies["ldap group dn"], common.RoleProjectAdmin)
			},
			wantLog: []string{`"robot$provisioner"`, `LDAP group "cn=ops,dc=example,dc=com"`, "member:grant-admin"},
		},
		{
			name:   "robot promotes a member",
			secCtx: func() security.Context { return robotsec.NewSecurityContext(grantRobot) },
			act: func(ctx context.Context, api *memberAPI) middleware.Responder {
				return gaUpdate(ctx, api, common.RoleProjectAdmin)
			},
			wantLog: []string{`"robot$provisioner"`, "member 3", "project 1", "member:grant-admin"},
		},
		{
			name:   "robot grant the member controller rejects",
			secCtx: func() security.Context { return robotsec.NewSecurityContext(grantRobot) },
			act: func(ctx context.Context, api *memberAPI) middleware.Responder {
				return gaCreate(ctx, api, gaBodies["user by id"], common.RoleProjectAdmin)
			},
			failWith: errors.BadRequestError(nil).WithMessage("User 7 not found"),
		},
		{
			name: "system admin",
			secCtx: func() security.Context {
				return local.NewSecurityContext(&commonmodels.User{UserID: 2, Username: "admin", SysAdminFlag: true})
			},
			act: func(ctx context.Context, api *memberAPI) middleware.Responder {
				return gaCreate(ctx, api, gaBodies["user by id"], common.RoleProjectAdmin)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gaInstallProjectCtl(t, nil)
			var buf bytes.Buffer
			ctx := log.WithLogger(context.Background(), log.New(&buf, log.NewTextFormatter(), log.InfoLevel))
			ctx = security.NewContext(ctx, test.secCtx())
			api := newMemberAPI()
			api.ctl = &gaMembers{createErr: test.failWith}

			_, failed := test.act(ctx, api).(*ErrResponder)
			require.Equal(t, test.failWith != nil, failed)
			if len(test.wantLog) == 0 {
				assert.NotContains(t, buf.String(), "project admin role")
				return
			}
			assert.Contains(t, buf.String(), "[INFO]")
			for _, want := range test.wantLog {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

// member:grant-admin is a project permission of the robot catalog only.
func TestRobotCatalogOffersGrantAdmin(t *testing.T) {
	grant := func() []*models.Access {
		return []*models.Access{{Resource: rbac.ResourceMember.String(), Action: rbac.ActionGrantAdmin.String()}}
	}
	rAPI := &robotAPI{}
	assert.NoError(t, rAPI.validate(-1, robot.LEVELPROJECT, []*models.RobotPermission{
		{Kind: robot.LEVELPROJECT, Namespace: "p1", Access: grant()},
	}))
	assert.NoError(t, rAPI.validate(-1, robot.LEVELSYSTEM, []*models.RobotPermission{
		{Kind: robot.LEVELPROJECT, Namespace: "*", Access: grant()},
	}))
	err := rAPI.validate(-1, robot.LEVELSYSTEM, []*models.RobotPermission{
		{Kind: robot.LEVELSYSTEM, Namespace: "/", Access: grant()},
	})
	assert.Equal(t, errors.BadRequestCode, errors.ErrCode(err), "got %v", err)
}

// A robot gives member:grant-admin to a robot it creates only when it holds it
// on the same or a wider namespace. Robot creation checks no role of a local
// user, so a project admin can mint it for a project robot of their own project,
// where they may grant project admin anyway; only a system admin can mint it for
// all projects. The project admin case needs the database and is not covered here.
func TestMintingGrantAdminRobots(t *testing.T) {
	createSystemRobots := &robot.Permission{
		Kind: robot.LEVELSYSTEM, Namespace: "/", Scope: robot.SCOPESYSTEM,
		Access: []*types.Policy{gaAllow(rbac.ResourceRobot, rbac.ActionCreate)},
	}
	grantOn := func(namespace string) []*models.RobotPermission {
		return []*models.RobotPermission{{
			Kind: robot.LEVELPROJECT, Namespace: namespace,
			Access: []*models.Access{{
				Resource: rbac.ResourceMember.String(), Action: rbac.ActionGrantAdmin.String(), Effect: types.EffectAllow.String(),
			}},
		}}
	}
	rolesOf := func(_ int64, u *commonmodels.User) []int {
		if u != nil && u.Username == "project-admin" {
			return []int{common.RoleProjectAdmin}
		}
		return nil
	}
	tests := []struct {
		name     string
		secCtx   func() security.Context
		level    string
		perms    []*models.RobotPermission
		wantCode string
	}{
		{
			name: "system robot with member writes on all projects",
			secCtx: func() security.Context {
				return robotsec.NewSecurityContext(gaSystemRobot(createSystemRobots, gaCoverAll(gaMember(rbac.ActionCreate, rbac.ActionUpdate)...)))
			},
			level: robot.LEVELSYSTEM, perms: grantOn("*"), wantCode: errors.DENIED,
		},
		{
			name: "system robot with member:grant-admin on one project minting it for all projects",
			secCtx: func() security.Context {
				return robotsec.NewSecurityContext(gaSystemRobot(createSystemRobots, gaOnProject(gaProjectID, gaMember(rbac.ActionGrantAdmin)...)))
			},
			level: robot.LEVELSYSTEM, perms: grantOn("*"), wantCode: errors.DENIED,
		},
		{
			name: "project robot with member writes and robot:create",
			secCtx: func() security.Context {
				return robotsec.NewSecurityContext(gaProjectRobot(append(gaMember(rbac.ActionCreate, rbac.ActionUpdate), gaAllow(rbac.ResourceRobot, rbac.ActionCreate))...))
			},
			level: robot.LEVELPROJECT, perms: grantOn("p1"), wantCode: errors.DENIED,
		},
		{
			name: "system robot holding member:grant-admin on all projects",
			secCtx: func() security.Context {
				return robotsec.NewSecurityContext(gaSystemRobot(createSystemRobots, gaCoverAll(gaMember(rbac.ActionGrantAdmin)...)))
			},
			level: robot.LEVELSYSTEM, perms: grantOn("*"),
		},
		{
			name: "system admin",
			secCtx: func() security.Context {
				return local.NewSecurityContext(&commonmodels.User{UserID: 2, Username: "admin", SysAdminFlag: true})
			},
			level: robot.LEVELSYSTEM, perms: grantOn("*"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gaInstallProjectCtl(t, rolesOf)
			robotCtl := robottesting.NewController(t)
			if test.wantCode == "" {
				robotCtl.On("Create", mock.Anything, mock.Anything).Return(int64(30), "secret", nil).Once()
				robotCtl.On("Get", mock.Anything, int64(30), mock.Anything).
					Return(&robot.Robot{Robot: robotmodel.Robot{ID: 30, Name: "minted"}}, nil).Once()
			}
			params := robotop.CreateRobotParams{
				HTTPRequest: httptest.NewRequest(http.MethodPost, "/api/v2.0/robots", nil),
				Robot:       &models.RobotCreate{Name: "minted", Duration: -1, Level: test.level, Permissions: test.perms},
			}
			ctx := security.NewContext(context.Background(), test.secCtx())
			responder := (&robotAPI{robotCtl: robotCtl}).CreateRobot(ctx, params)
			if test.wantCode == "" {
				assert.IsType(t, &robotop.CreateRobotCreated{}, responder)
				return
			}
			errResponder, ok := responder.(*ErrResponder)
			require.True(t, ok, "got %T", responder)
			assert.True(t, errors.IsErr(errResponder.err, test.wantCode), "got %v", errResponder.err)
		})
	}
}

// Only system admins can give a robot member:grant-admin, so only they are
// offered it.
func TestPermissionsCatalogGrantAdminForAdmins(t *testing.T) {
	has := func(list []*models.Permission) bool {
		for _, p := range list {
			if p.Resource == rbac.ResourceMember.String() && p.Action == rbac.ActionGrantAdmin.String() {
				return true
			}
		}
		return false
	}
	tests := []struct {
		name        string
		user        *commonmodels.User
		projectAdm  bool
		wantProject bool
	}{
		{"system admin", &commonmodels.User{UserID: 2, Username: "admin", SysAdminFlag: true}, false, true},
		{"project admin", &commonmodels.User{UserID: 4, Username: "project-admin"}, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &permissionsAPI{mc: &gaMembers{isAdmin: test.projectAdm}}
			ctx := security.NewContext(context.Background(), local.NewSecurityContext(test.user))
			ok, isOK := api.GetPermissions(ctx, permissions.GetPermissionsParams{}).(*permissions.GetPermissionsOK)
			require.True(t, isOK)
			assert.Equal(t, test.wantProject, has(ok.Payload.Project))
			assert.False(t, has(ok.Payload.System))
		})
	}
}
