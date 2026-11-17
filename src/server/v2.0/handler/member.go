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
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-openapi/runtime/middleware"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/local"
	robotSec "github.com/goharbor/harbor/src/common/security/robot"
	"github.com/goharbor/harbor/src/controller/member"
	"github.com/goharbor/harbor/src/controller/robot"
	roleCtl "github.com/goharbor/harbor/src/controller/role"
	"github.com/goharbor/harbor/src/controller/user"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	memberModels "github.com/goharbor/harbor/src/pkg/member/models"
	"github.com/goharbor/harbor/src/pkg/permission/policy"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/member"
)

type memberAPI struct {
	BaseAPI
	ctl      member.Controller
	roleCtl  roleCtl.Controller
	robotCtl robot.Controller
	userCtl  user.Controller
}

// robotCreatorType is the creator type a robot-created robot records, the robot security context's name.
const robotCreatorType = "robot"

func newMemberAPI() *memberAPI {
	return &memberAPI{
		ctl:      member.NewController(),
		roleCtl:  roleCtl.Ctl,
		robotCtl: robot.Ctl,
		userCtl:  user.Ctl,
	}
}

func (m *memberAPI) CreateProjectMember(ctx context.Context, params operation.CreateProjectMemberParams) middleware.Responder {
	projectNameOrID := parseProjectNameOrID(params.ProjectNameOrID, params.XIsResourceName)
	if err := m.RequireProjectAccess(ctx, projectNameOrID, rbac.ActionCreate, rbac.ResourceMember); err != nil {
		return m.SendError(ctx, err)
	}
	if params.ProjectMember == nil {
		return m.SendError(ctx, errors.BadRequestError(nil).WithMessage("the project member should be provided"))
	}
	if err := m.checkNoEscalation(ctx, projectNameOrID, params.ProjectMember.RoleID); err != nil {
		return m.SendError(ctx, err)
	}
	req, err := toMemberReq(params.ProjectMember)
	if err != nil {
		return m.SendError(ctx, err)
	}
	id, err := m.ctl.Create(ctx, projectNameOrID, *req)
	if err != nil {
		return m.SendError(ctx, err)
	}
	return operation.NewCreateProjectMemberCreated().
		WithLocation(fmt.Sprintf("/api/v2.0/projects/%v/members/%d", projectNameOrID, id))
}

func toMemberReq(memberReq *models.ProjectMember) (*member.Request, error) {
	data, err := json.Marshal(memberReq)
	if err != nil {
		return nil, err
	}
	var result member.Request
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (m *memberAPI) DeleteProjectMember(ctx context.Context, params operation.DeleteProjectMemberParams) middleware.Responder {
	projectNameOrID := parseProjectNameOrID(params.ProjectNameOrID, params.XIsResourceName)
	if err := m.RequireProjectAccess(ctx, projectNameOrID, rbac.ActionDelete, rbac.ResourceMember); err != nil {
		return m.SendError(ctx, err)
	}
	if params.Mid == 0 {
		return m.SendError(ctx, errors.BadRequestError(nil).WithMessage("the project member id is required."))
	}
	err := m.ctl.Delete(ctx, projectNameOrID, int(params.Mid))
	if err != nil {
		return m.SendError(ctx, err)
	}
	return operation.NewDeleteProjectMemberOK()
}

func (m *memberAPI) GetProjectMember(ctx context.Context, params operation.GetProjectMemberParams) middleware.Responder {
	projectNameOrID := parseProjectNameOrID(params.ProjectNameOrID, params.XIsResourceName)
	if err := m.RequireProjectAccess(ctx, projectNameOrID, rbac.ActionRead, rbac.ResourceMember); err != nil {
		return m.SendError(ctx, err)
	}

	if params.Mid == 0 {
		return m.SendError(ctx, errors.BadRequestError(nil).WithMessage("Member ID cannot be empty"))
	}

	member, err := m.ctl.Get(ctx, projectNameOrID, int(params.Mid))
	if err != nil {
		return m.SendError(ctx, err)
	}
	return operation.NewGetProjectMemberOK().WithPayload(toProjectMemberResp(member))
}

func (m *memberAPI) ListProjectMembers(ctx context.Context, params operation.ListProjectMembersParams) middleware.Responder {
	projectNameOrID := parseProjectNameOrID(params.ProjectNameOrID, params.XIsResourceName)
	if err := m.RequireProjectAccess(ctx, projectNameOrID, rbac.ActionList, rbac.ResourceMember); err != nil {
		return m.SendError(ctx, err)
	}
	entityName := lib.StringValue(params.Entityname)
	query, err := m.BuildQuery(ctx, nil, nil, params.Page, params.PageSize)
	if err != nil {
		return m.SendError(ctx, err)
	}
	total, err := m.ctl.Count(ctx, projectNameOrID, query)
	if err != nil {
		return m.SendError(ctx, err)
	}
	if total == 0 {
		return operation.NewListProjectMembersOK().
			WithXTotalCount(0).
			WithPayload([]*models.ProjectMemberEntity{})
	}
	members, err := m.ctl.List(ctx, projectNameOrID, entityName, query)
	if err != nil {
		return m.SendError(ctx, err)
	}
	return operation.NewListProjectMembersOK().
		WithXTotalCount(int64(total)).
		WithLink(m.Links(ctx, params.HTTPRequest.URL, int64(total), query.PageNumber, query.PageSize).String()).
		WithPayload(toProjectMemberRespList(members))
}

func toProjectMemberRespList(members []*memberModels.Member) []*models.ProjectMemberEntity {
	result := make([]*models.ProjectMemberEntity, 0)
	for _, mem := range members {
		result = append(result, toProjectMemberResp(mem))
	}
	return result
}

func toProjectMemberResp(member *memberModels.Member) *models.ProjectMemberEntity {
	return &models.ProjectMemberEntity{
		ProjectID:  member.ProjectID,
		ID:         int64(member.ID),
		EntityName: member.Entityname,
		EntityID:   int64(member.EntityID),
		EntityType: member.EntityType,
		RoleID:     int64(member.Role),
		RoleName:   member.Rolename,
	}
}

func (m *memberAPI) UpdateProjectMember(ctx context.Context, params operation.UpdateProjectMemberParams) middleware.Responder {
	projectNameOrID := parseProjectNameOrID(params.ProjectNameOrID, params.XIsResourceName)
	if err := m.RequireProjectAccess(ctx, projectNameOrID, rbac.ActionUpdate, rbac.ResourceMember); err != nil {
		return m.SendError(ctx, err)
	}
	if params.Role == nil {
		return m.SendError(ctx, errors.BadRequestError(nil).WithMessage("role cannot be empty"))
	}
	if params.Mid == 0 {
		return m.SendError(ctx, errors.BadRequestError(nil).WithMessage("member id cannot be empty"))
	}
	if err := m.checkNoEscalation(ctx, projectNameOrID, params.Role.RoleID); err != nil {
		return m.SendError(ctx, err)
	}

	err := m.ctl.UpdateRole(ctx, projectNameOrID, int(params.Mid), int(params.Role.RoleID))
	if err != nil {
		return m.SendError(ctx, err)
	}
	return operation.NewUpdateProjectMemberOK()
}

// maxCreatorDepth bounds the robot-created-by-robot chain walked for built-in grants.
const maxCreatorDepth = 3

// checkNoEscalation rejects human role assignments that exceed the caller's permissions.
// Robots delegate roles through the member:create/update permission checked by the handler.
func (m *memberAPI) checkNoEscalation(ctx context.Context, projectNameOrID any, roleID int64) error {
	sc, err := m.GetSecurityContext(ctx)
	if err != nil {
		return err
	}
	if rc, ok := sc.(*robotSec.SecurityContext); ok && rc.IsAuthenticated() {
		// Robot permissions cannot express all human role permissions. Requiring
		// those permissions would break existing member-management robots. The
		// member controller still validates the target role before persisting it.
		return nil
	}

	r, err := m.roleCtl.Get(ctx, roleID, &roleCtl.Option{WithPermission: true})
	if err != nil {
		return err
	}
	var accesses []*types.Policy
	for _, perm := range r.Permissions {
		accesses = append(accesses, perm.Access...)
	}
	return m.checkGrant(ctx, projectNameOrID, roleID, accesses, "you lack", 0)
}

func (m *memberAPI) checkGrant(ctx context.Context, projectNameOrID any, roleID int64, accesses []*types.Policy, subject string, depth int) error {
	// Built-ins hold permissions no robot can, so the robot's creator must hold them (#1137, #1227).
	if r := robotCaller(ctx); r != nil && policy.IsBuiltinRole(roleID) {
		if depth >= maxCreatorDepth {
			return errors.ForbiddenError(nil).WithMessage("escalation not allowed: robot creator chain too deep")
		}
		creator, err := m.creatorContext(ctx, r)
		if err != nil {
			return err
		}
		return m.checkGrant(security.NewContext(ctx, creator), projectNameOrID, roleID, accesses, creatorLacks(ctx), depth+1)
	}
	for _, acc := range accesses {
		// Deny entries restrict access rather than grant it, so the caller need
		// not hold them; requiring them would wrongly block assigning a role that
		// merely denies something. (Custom roles cannot carry deny today —
		// validate() rejects any effect but "" / allow — this guards built-ins and
		// any future deny support.)
		if acc.Effect == types.EffectDeny {
			continue
		}
		has, err := m.HasProjectPermission(ctx, projectNameOrID, acc.Action, acc.Resource)
		if err != nil {
			return err
		}
		if !has {
			return errors.ForbiddenError(nil).WithMessagef(
				"escalation not allowed: %s %s:%s", subject, acc.Resource, acc.Action)
		}
	}
	return nil
}

func robotCaller(ctx context.Context) *robot.Robot {
	secCtx, ok := security.FromContext(ctx)
	if !ok {
		return nil
	}
	rc, ok := secCtx.(*robotSec.SecurityContext)
	if !ok {
		return nil
	}
	return rc.User()
}

// creatorContext builds a security context for whoever created r, from their current state.
func (m *memberAPI) creatorContext(ctx context.Context, r *robot.Robot) (security.Context, error) {
	switch r.CreatorType {
	case local.ContextName:
		u, err := m.userCtl.Get(ctx, int(r.CreatorRef), nil)
		if err != nil {
			return nil, creatorLookupError(err)
		}
		return local.NewSecurityContext(u), nil
	case robotCreatorType:
		c, err := m.robotCtl.Get(ctx, r.CreatorRef, &robot.Option{WithPermission: true})
		if err != nil {
			return nil, creatorLookupError(err)
		}
		if c.Disabled || (c.ExpiresAt != -1 && c.ExpiresAt <= time.Now().Unix()) {
			return nil, errors.ForbiddenError(nil).WithMessage("escalation not allowed: the robot's creator is inactive")
		}
		return robotSec.NewSecurityContext(c), nil
	default:
		return nil, errors.ForbiddenError(nil).WithMessage("escalation not allowed: the robot has no known creator")
	}
}

// creatorLacks words the denial; admin rights an external provider grants at login are not stored, so robots cannot inherit them.
func creatorLacks(ctx context.Context) string {
	if mode := lib.GetAuthMode(ctx); mode != "" && mode != common.DBAuth {
		return "the robot's creator lacks (admin rights granted by the external auth provider are not honoured for robots; set the system admin flag on the creator)"
	}
	return "the robot's creator lacks"
}

func creatorLookupError(err error) error {
	if errors.IsNotFoundErr(err) {
		return errors.ForbiddenError(nil).WithMessage("escalation not allowed: the robot's creator no longer exists")
	}
	return err
}
