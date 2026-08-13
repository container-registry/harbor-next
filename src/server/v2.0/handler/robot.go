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
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/security/local"
	robotSc "github.com/goharbor/harbor/src/common/security/robot"
	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/controller/federatedidp"
	"github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/commercial"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	pkg "github.com/goharbor/harbor/src/pkg/robot/model"
	"github.com/goharbor/harbor/src/server/v2.0/handler/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/robot"
)

func newRobotAPI() *robotAPI {
	return &robotAPI{
		robotCtl:  robot.Ctl,
		fedidpCtl: federatedidp.Ctl,
	}
}

type robotAPI struct {
	BaseAPI
	robotCtl  robot.Controller
	fedidpCtl federatedidp.Controller
}

func (rAPI *robotAPI) CreateRobot(ctx context.Context, params operation.CreateRobotParams) middleware.Responder {
	if err := validateName(params.Robot.Name); err != nil {
		return rAPI.SendError(ctx, err)
	}

	if hasFederatedIDP(params.Robot.FederatedidpID) {
		sanitizeFederatedRobotPermissions(params.Robot.Permissions)
	}

	if err := rAPI.validate(params.Robot.Duration, params.Robot.Level, params.Robot.Permissions); err != nil {
		return rAPI.SendError(ctx, err)
	}

	sc, err := rAPI.GetSecurityContext(ctx)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	r := &robot.Robot{
		Robot: pkg.Robot{
			Name:        params.Robot.Name,
			Description: params.Robot.Description,
			Duration:    params.Robot.Duration,
			Visible:     true,
		},
		Level:           params.Robot.Level,
		ProjectNameOrID: params.Robot.Permissions[0].Namespace,
	}

	if err := rAPI.requireAccess(ctx, r, rbac.ActionCreate); err != nil {
		return rAPI.SendError(ctx, err)
	}

	var creatorRef int64
	switch s := sc.(type) {
	case *local.SecurityContext:
		creatorRef = int64(s.User().UserID)
	case *robotSc.SecurityContext:
		if s.User() == nil {
			return rAPI.SendError(ctx, errors.New(nil).WithMessage("invalid security context: empty robot account"))
		}
		if !isValidPermissionScope(params.Robot.Permissions, s.User().Permissions) {
			return rAPI.SendError(ctx, errors.New(nil).WithMessagef("permission scope is invalid. It must be equal to or more restrictive than the creator robot's permissions: %s", s.User().Name).WithCode(errors.DENIED))
		}
		creatorRef = s.User().ID
	default:
		return rAPI.SendError(ctx, errors.New(nil).WithMessage("invalid security context"))
	}
	r.CreatorType = sc.Name()
	r.CreatorRef = creatorRef

	if err := lib.JSONCopy(&r.Permissions, params.Robot.Permissions); err != nil {
		log.Warningf("failed to call JSONCopy on robot permission when CreateRobot, error: %v", err)
	}

	if err := robot.SetProject(ctx, r); err != nil {
		return rAPI.SendError(ctx, err)
	}
	if hasFederatedIDP(params.Robot.FederatedidpID) && !commercial.Enabled(ctx, commercial.IdentityProviders) {
		return rAPI.SendError(ctx, errors.ForbiddenError(nil).WithMessage("commercial feature identity_providers is not enabled"))
	}

	rid, pwd, err := rAPI.robotCtl.Create(ctx, r)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	if params.Robot.FederatedidpID != nil {
		idpID := *params.Robot.FederatedidpID
		if idpID > 0 {
			// Fetch the federated IDP to validate project scope and permissions
			idp, err := rAPI.fedidpCtl.Get(ctx, idpID)
			if err != nil {
				return rAPI.SendError(ctx, err)
			}

			// Validate project scope:
			// - System-level robot (project_id=0) can only link to system-level IDP (project_id=0)
			// - Project-level robot can link to same-project IDP OR system-level IDP
			if r.ProjectID == 0 {
				// System-level robot can only use system-level IDP
				if idp.ProjectID != 0 {
					return rAPI.SendError(ctx, errors.ForbiddenError(nil).
						WithMessage("system-level robot can only be linked to system-level federated IDP"))
				}
			} else {
				// Project-level robot can use same-project IDP or system-level IDP
				if idp.ProjectID != 0 && idp.ProjectID != r.ProjectID {
					return rAPI.SendError(ctx, errors.ForbiddenError(nil).
						WithMessage("robot can only be linked to federated IDP from the same project or system level"))
				}
			}

			// Check user has permission to access the federated IDP
			if idp.ProjectID > 0 {
				// Project-level IDP: check project access
				if err := rAPI.RequireProjectAccess(ctx, idp.ProjectID, rbac.ActionRead, rbac.ResourceFederatedIdp); err != nil {
					return rAPI.SendError(ctx, err)
				}
			} else {
				// System-level IDP: check system access
				if err := rAPI.RequireSystemAccess(ctx, rbac.ActionRead, rbac.ResourceFederatedIdp); err != nil {
					return rAPI.SendError(ctx, err)
				}
			}

			// For project-level robots, check if project federated IDP feature is enabled
			if r.Level == robot.LEVELPROJECT && !config.EnableProjectFederatedIDP(ctx) {
				return rAPI.SendError(ctx, errors.ForbiddenError(nil).WithMessage("project-level federated identity provider feature is not enabled"))
			}
			_, err = rAPI.fedidpCtl.CreateRobotIdp(ctx, idpID, rid)
			if err != nil {
				return rAPI.SendError(ctx, err)
			}
		}
	}

	created, err := rAPI.robotCtl.Get(ctx, rid, nil)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	var fedIdpID int64
	if params.Robot.FederatedidpID != nil {
		fedIdpID = *params.Robot.FederatedidpID
	}

	location := fmt.Sprintf("%s/%d", strings.TrimSuffix(params.HTTPRequest.URL.Path, "/"), created.ID)
	return operation.NewCreateRobotCreated().WithLocation(location).WithPayload(&models.RobotCreated{
		ID:             created.ID,
		Name:           created.Name,
		FederatedidpID: fedIdpID,
		Secret:         robotCreatedSecret(fedIdpID, pwd),
		CreationTime:   strfmt.DateTime(created.CreationTime),
		ExpiresAt:      created.ExpiresAt,
	})
}

func robotCreatedSecret(fedIdpID int64, secret string) string {
	if fedIdpID > 0 {
		return ""
	}
	return secret
}

func hasFederatedIDP(id *int64) bool {
	return id != nil && *id > 0
}

func (rAPI *robotAPI) DeleteRobot(ctx context.Context, params operation.DeleteRobotParams) middleware.Responder {
	if err := rAPI.RequireAuthenticated(ctx); err != nil {
		return rAPI.SendError(ctx, err)
	}

	r, err := rAPI.robotCtl.Get(ctx, params.RobotID, nil)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	if err := rAPI.requireAccess(ctx, r, rbac.ActionDelete); err != nil {
		return rAPI.SendError(ctx, err)
	}

	if err := rAPI.robotCtl.Delete(ctx, params.RobotID); err != nil {
		// for the version 1 robot account, has to ignore the no permission error.
		if !r.Editable && errors.IsNotFoundErr(err) {
			return operation.NewDeleteRobotOK()
		}
		return rAPI.SendError(ctx, err)
	}

	// delete all claim_rules records associated with the given robot_id
	if err := rAPI.fedidpCtl.DeleteClaimRulesByRobotID(ctx, params.RobotID); err != nil {
		if errors.IsNotFoundErr(err) {
			return operation.NewDeleteRobotOK()
		}
		return rAPI.SendError(ctx, err)
	}
	// check if robotidp record exists if yes, delete it
	if err := rAPI.fedidpCtl.DeleteRobotIdpByRobotID(ctx, params.RobotID); err != nil {
		if errors.IsNotFoundErr(err) {
			return operation.NewDeleteRobotOK()
		}
		return rAPI.SendError(ctx, err)
	}

	return operation.NewDeleteRobotOK()
}

func (rAPI *robotAPI) ListRobot(ctx context.Context, params operation.ListRobotParams) middleware.Responder {
	if err := rAPI.RequireAuthenticated(ctx); err != nil {
		return rAPI.SendError(ctx, err)
	}

	query, err := rAPI.BuildQuery(ctx, params.Q, params.Sort, params.Page, params.PageSize)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	var projectID int64
	var level string
	// GET /api/v2.0/robots or GET /api/v2.0/robots?q=Level=system to get all of system level robots.
	// GET /api/v2.0/robots?q=Level=project,ProjectID=1
	if _, ok := query.Keywords["Level"]; ok {
		if !isValidLevel(query.Keywords["Level"].(string)) {
			return rAPI.SendError(ctx, errors.New(nil).WithMessage("bad request error level input").WithCode(errors.BadRequestCode))
		}
		level = query.Keywords["Level"].(string)
		if level == robot.LEVELPROJECT {
			if _, ok := query.Keywords["ProjectID"]; !ok {
				return rAPI.SendError(ctx, errors.BadRequestError(nil).WithMessage("must with project ID when to query project robots"))
			}
			pid, err := strconv.ParseInt(query.Keywords["ProjectID"].(string), 10, 64)
			if err != nil || pid <= 0 {
				return rAPI.SendError(ctx, errors.BadRequestError(nil).WithMessage("ProjectID must be a positive integer"))
			}
			projectID = pid
		} else if level == robot.LEVELSYSTEM {
			query.Keywords["ProjectID"] = 0
		}
	} else {
		level = robot.LEVELSYSTEM
		query.Keywords["ProjectID"] = 0
	}
	query.Keywords["Visible"] = true

	r := &robot.Robot{
		ProjectNameOrID: projectID,
		Level:           level,
	}
	if err := rAPI.requireAccess(ctx, r, rbac.ActionList); err != nil {
		return rAPI.SendError(ctx, err)
	}

	total, err := rAPI.robotCtl.Count(ctx, query)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	robots, err := rAPI.robotCtl.List(ctx, query, &robot.Option{
		WithPermission: true,
	})
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	var results []*models.Robot
	for _, r := range robots {
		// Populate federatedidp_id for each robot
		idpID, err := rAPI.fedidpCtl.GetIdpIDByRobotID(ctx, r.ID)
		if err != nil {
			log.Warningf("failed to get federated idp id for robot %d: %v", r.ID, err)
		} else {
			r.FederatedIdpID = idpID
		}
		results = append(results, model.NewRobot(r).ToSwagger())
	}

	return operation.NewListRobotOK().
		WithXTotalCount(total).
		WithLink(rAPI.Links(ctx, params.HTTPRequest.URL, total, query.PageNumber, query.PageSize).String()).
		WithPayload(results)
}

func (rAPI *robotAPI) GetRobotByID(ctx context.Context, params operation.GetRobotByIDParams) middleware.Responder {
	if err := rAPI.RequireAuthenticated(ctx); err != nil {
		return rAPI.SendError(ctx, err)
	}

	r, err := rAPI.robotCtl.Get(ctx, params.RobotID, &robot.Option{
		WithPermission: true,
	})
	if err != nil {
		return rAPI.SendError(ctx, err)
	}
	if err := rAPI.requireAccess(ctx, r, rbac.ActionRead); err != nil {
		return rAPI.SendError(ctx, err)
	}

	// Populate federatedidp_id for the robot
	idpID, err := rAPI.fedidpCtl.GetIdpIDByRobotID(ctx, r.ID)
	if err != nil {
		log.Warningf("failed to get federated idp id for robot %d: %v", r.ID, err)
	} else {
		r.FederatedIdpID = idpID
	}

	return operation.NewGetRobotByIDOK().WithPayload(model.NewRobot(r).ToSwagger())
}

func (rAPI *robotAPI) UpdateRobot(ctx context.Context, params operation.UpdateRobotParams) middleware.Responder {
	var err error
	if err := rAPI.RequireAuthenticated(ctx); err != nil {
		return rAPI.SendError(ctx, err)
	}
	r, err := rAPI.robotCtl.Get(ctx, params.RobotID, &robot.Option{
		WithPermission: true,
	})
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	if !r.Editable {
		err = errors.DeniedError(nil).WithMessage("editing of legacy robot is not allowed")
	} else {
		err = rAPI.updateV2Robot(ctx, params, r)
	}
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	return operation.NewUpdateRobotOK()
}

func (rAPI *robotAPI) RefreshSec(ctx context.Context, params operation.RefreshSecParams) middleware.Responder {
	if err := rAPI.RequireAuthenticated(ctx); err != nil {
		return rAPI.SendError(ctx, err)
	}

	r, err := rAPI.robotCtl.Get(ctx, params.RobotID, nil)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}

	hasIdp, err := rAPI.fedidpCtl.HasRobotIdpByRobotID(ctx, r.ID)
	if err != nil {
		return rAPI.SendError(ctx, err)
	}
	if hasIdp {
		return rAPI.SendError(ctx, errors.New(nil).WithMessage("cannot refresh secret, robot has identity provider associated").WithCode(errors.BadRequestCode))
	}

	if err := rAPI.requireAccess(ctx, r, rbac.ActionUpdate); err != nil {
		return rAPI.SendError(ctx, err)
	}

	var secret string
	robotSec := &models.RobotSec{}
	if params.RobotSec.Secret != "" {
		if !robot.IsValidSec(params.RobotSec.Secret) {
			return rAPI.SendError(ctx, errors.New("the secret must be 8-128, inclusively, characters long with at least 1 uppercase letter, 1 lowercase letter and 1 number").WithCode(errors.BadRequestCode))
		}
		secret = utils.Encrypt(params.RobotSec.Secret, r.Salt, utils.SHA256)
		robotSec.Secret = ""
	} else {
		sec, pwd, _, err := robot.CreateSec(r.Salt)
		if err != nil {
			return rAPI.SendError(ctx, err)
		}
		secret = sec
		robotSec.Secret = pwd
	}

	r.Secret = secret
	if err := rAPI.robotCtl.Update(ctx, r, nil); err != nil {
		return rAPI.SendError(ctx, err)
	}

	return operation.NewRefreshSecOK().WithPayload(robotSec)
}

func (rAPI *robotAPI) requireAccess(ctx context.Context, r *robot.Robot, action rbac.Action) error {
	if r.Level == robot.LEVELSYSTEM {
		return rAPI.RequireSystemAccess(ctx, action, rbac.ResourceRobot)
	} else if r.Level == robot.LEVELPROJECT {
		var ns any
		if r.ProjectNameOrID != nil {
			ns = r.ProjectNameOrID
		} else if r.ProjectID > 0 {
			ns = r.ProjectID
		} else if r.ProjectName != "" {
			ns = r.ProjectName
		}
		return rAPI.RequireProjectAccess(ctx, ns, action, rbac.ResourceRobot)
	}

	return errors.ForbiddenError(nil)
}

// more validation
func (rAPI *robotAPI) validate(d int64, level string, permissions []*models.RobotPermission) error {
	if !isValidDuration(d) {
		return errors.New(nil).WithMessagef("bad request error duration input: %d, duration must be either -1(Never) or a positive integer", d).WithCode(errors.BadRequestCode)
	}

	if !isValidLevel(level) {
		return errors.New(nil).WithMessagef("bad request error level input: %s", level).WithCode(errors.BadRequestCode)
	}

	if len(permissions) == 0 {
		return errors.New(nil).WithMessage("bad request empty permission").WithCode(errors.BadRequestCode)
	}

	for _, perm := range permissions {
		if len(perm.Access) == 0 {
			return errors.New(nil).WithMessage("bad request empty access").WithCode(errors.BadRequestCode)
		}
	}

	// to create a project robot, the permission must be only one project scope.
	if level == robot.LEVELPROJECT && len(permissions) > 1 {
		return errors.New(nil).WithMessage("bad request permission").WithCode(errors.BadRequestCode)
	}

	provider := rbac.GetPermissionProvider()
	// to validate the access scope
	for _, perm := range permissions {
		if perm.Kind == robot.LEVELSYSTEM {
			polices := provider.GetPermissions(rbac.ScopeSystem)
			for _, acc := range perm.Access {
				if !containsAccess(polices, acc) {
					return errors.New(nil).WithMessagef("bad request permission: %s:%s", acc.Resource, acc.Action).WithCode(errors.BadRequestCode)
				}
			}
		} else if perm.Kind == robot.LEVELPROJECT {
			polices := provider.GetPermissions(rbac.ScopeProject)
			for _, acc := range perm.Access {
				if !containsAccess(polices, acc) {
					return errors.New(nil).WithMessagef("bad request permission: %s:%s", acc.Resource, acc.Action).WithCode(errors.BadRequestCode)
				}
			}
		} else {
			return errors.New(nil).WithMessagef("bad request permission level: %s", perm.Kind).WithCode(errors.BadRequestCode)
		}
	}

	return nil
}

func (rAPI *robotAPI) updateV2Robot(ctx context.Context, params operation.UpdateRobotParams, r *robot.Robot) error {
	if params.Robot.Duration == nil {
		params.Robot.Duration = &r.Duration
	}
	hasIdp, err := rAPI.fedidpCtl.HasRobotIdpByRobotID(ctx, r.ID)
	if err != nil {
		return err
	}
	if hasIdp {
		sanitizeFederatedRobotPermissions(params.Robot.Permissions)
	}
	if err := rAPI.validate(*params.Robot.Duration, params.Robot.Level, params.Robot.Permissions); err != nil {
		return err
	}
	if r.Level != robot.LEVELSYSTEM {
		projectID, err := getProjectID(ctx, params.Robot.Permissions[0].Namespace)
		if err != nil {
			return err
		}
		if r.ProjectID != projectID {
			return errors.BadRequestError(nil).WithMessage("cannot update the project id of robot")
		}
	}
	r.ProjectNameOrID = params.Robot.Permissions[0].Namespace
	if err := rAPI.requireAccess(ctx, r, rbac.ActionUpdate); err != nil {
		return err
	}
	if params.Robot.Level != r.Level || params.Robot.Name != r.Name {
		return errors.BadRequestError(nil).WithMessage("cannot update the level or name of robot")
	}

	if r.Duration != *params.Robot.Duration {
		r.Duration = *params.Robot.Duration
		if *params.Robot.Duration == -1 {
			r.ExpiresAt = -1
		} else {
			r.ExpiresAt = r.CreationTime.AddDate(0, 0, int(*params.Robot.Duration)).Unix()
		}
	}

	r.Description = params.Robot.Description
	r.Disabled = params.Robot.Disable
	if len(params.Robot.Permissions) != 0 {
		if err := lib.JSONCopy(&r.Permissions, params.Robot.Permissions); err != nil {
			log.Warningf("failed to call JSONCopy on robot permission when updateV2Robot, error: %v", err)
		}
	}

	if err := rAPI.robotCtl.Update(ctx, r, &robot.Option{
		WithPermission: true,
	}); err != nil {
		return err
	}
	return nil
}

func isValidLevel(l string) bool {
	return l == robot.LEVELSYSTEM || l == robot.LEVELPROJECT
}

func isValidDuration(d int64) bool {
	return d == -1 || (d > 0 && d < math.MaxInt32)
}

// validateName validates the robot name, especially '+' cannot be a valid character
func validateName(name string) error {
	robotNameReg := `^[a-z0-9]+(?:[._-][a-z0-9]+)*$`
	legal := regexp.MustCompile(robotNameReg).MatchString(name)
	if !legal {
		return errors.BadRequestError(nil).WithMessage("robot name is not in lower case or contains illegal characters")
	}
	return nil
}

func containsAccess(policies []*types.Policy, item *models.Access) bool {
	for _, po := range policies {
		if po.Resource.String() == item.Resource && po.Action.String() == item.Action {
			return true
		}
	}
	return false
}

func sanitizeFederatedRobotPermissions(permissions []*models.RobotPermission) {
	for _, perm := range permissions {
		if perm == nil {
			continue
		}

		accesses := perm.Access[:0]
		for _, access := range perm.Access {
			if !isFederatedRobotBlockedAccess(access) {
				accesses = append(accesses, access)
			}
		}
		perm.Access = accesses
	}
}

func isFederatedRobotBlockedAccess(access *models.Access) bool {
	if access == nil || access.Resource != rbac.ResourceRobot.String() {
		return false
	}
	return access.Action == rbac.ActionCreate.String() || access.Action == rbac.ActionUpdate.String()
}

// isValidPermissionScope checks if permission slice A is a subset of permission slice B
func isValidPermissionScope(creating []*models.RobotPermission, creator []*robot.Permission) bool {
	creatorMap := make(map[string]*robot.Permission)
	for _, creatorPerm := range creator {
		key := fmt.Sprintf("%s:%s", creatorPerm.Kind, creatorPerm.Namespace)
		creatorMap[key] = creatorPerm
	}

	hasLessThanOrEqualAccess := func(creating []*models.Access, creator []*types.Policy) bool {
		creatorMap := make(map[string]*types.Policy)
		for _, creatorP := range creator {
			key := fmt.Sprintf("%s:%s:%s", creatorP.Resource, creatorP.Action, creatorP.Effect)
			creatorMap[key] = creatorP
		}
		for _, creatingP := range creating {
			key := fmt.Sprintf("%s:%s:%s", creatingP.Resource, creatingP.Action, creatingP.Effect)
			if _, found := creatorMap[key]; !found {
				return false
			}
		}
		return true
	}

	for _, pCreating := range creating {
		key := fmt.Sprintf("%s:%s", pCreating.Kind, pCreating.Namespace)
		creatorPerm, found := creatorMap[key]
		if !found {
			allProjects := fmt.Sprintf("%s:*", pCreating.Kind)
			if creatorPerm, found = creatorMap[allProjects]; !found {
				return false
			}
		}
		if !hasLessThanOrEqualAccess(pCreating.Access, creatorPerm.Access) {
			return false
		}
	}
	return true
}
