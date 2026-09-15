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

package member

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/rbac"
	ctlevent "github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	ext "github.com/goharbor/harbor/src/pkg/auditext/event"
	pkgMember "github.com/goharbor/harbor/src/pkg/member"
	"github.com/goharbor/harbor/src/pkg/member/models"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

const (
	memberCreatePattern = `^/api/v2\.0/projects/[^/]+/members$`
	memberActionPattern = `^/api/v2\.0/projects/[^/]+/members/\d+$`
	extractPattern      = `^/api/v2\.0/projects/([^/]+)/members(?:/(\d+))?$`
)

var extractRe = regexp.MustCompile(extractPattern)

// overridable for testing
var (
	lookupMemberFn      = lookupMember
	getMemberFn         = getMember
	resolveProjectFn    = resolveProject
	auditEventEnabledFn = auditLogMemberEventEnabled
)

// roleNames mirrors projectRBACRole.GetRoleName so the description carries a
// name a reviewer recognises rather than a role id
var roleNames = map[int]string{
	common.RoleProjectAdmin: "projectAdmin",
	common.RoleMaintainer:   "maintainer",
	common.RoleDeveloper:    "developer",
	common.RoleGuest:        "guest",
	common.RoleLimitedGuest: "limitedGuest",
}

func roleName(id int) string {
	if n, ok := roleNames[id]; ok {
		return n
	}
	return strconv.Itoa(id)
}

func auditLogMemberEventEnabled(ctx context.Context, operation string) bool {
	if len(operation) == 0 {
		return false
	}
	ormCtx := orm.ReuseContext(ctx)
	return config.AuditLogEventEnabled(ormCtx, fmt.Sprintf("%v_%v", operation, rbac.ResourceMember.String()))
}

// custom resolver for project member events, extracts project id and user/group ids
func init() {
	r := &resolver{}
	commonevent.RegisterResolver(memberCreatePattern, r)
	commonevent.RegisterResolver(memberActionPattern, r)
}

type resolver struct{}

func (r *resolver) PreCheck(ctx context.Context, url string, method string) (bool, string) {
	operation := ext.MethodToOperation(method)
	if len(operation) == 0 {
		return false, ""
	}
	if !auditEventEnabledFn(ctx, operation) {
		return false, ""
	}
	m := extractRe.FindStringSubmatch(url)
	if len(m) < 2 {
		return false, ""
	}
	// for DELETE, resolve member info before the resource is deleted
	if method == http.MethodDelete {
		if len(m) >= 3 && len(m[2]) > 0 {
			name, typ := lookupMemberFn(ctx, m[1], m[2])
			if len(typ) > 0 {
				return true, typ + ":" + name
			}
			return true, name
		}
	}
	return true, ""
}

// Snapshot loads the member before an update or delete so the description
// can carry the old role.
func (r *resolver) Snapshot(c *commonevent.Metadata) any {
	if c.RequestMethod != http.MethodPut && c.RequestMethod != http.MethodDelete {
		return nil
	}
	m := extractRe.FindStringSubmatch(c.RequestURL)
	if len(m) < 3 || len(m[2]) == 0 {
		return nil
	}
	return getMemberFn(c.Ctx, m[1], m[2])
}

func (r *resolver) Resolve(ce *commonevent.Metadata, evt *event.Event) error {
	if ce == nil || evt == nil {
		return fmt.Errorf("metadata or event is nil")
	}
	operation := ext.MethodToOperation(ce.RequestMethod)
	if len(operation) == 0 {
		return nil
	}
	matches := extractRe.FindStringSubmatch(ce.RequestURL)
	if len(matches) < 2 {
		return nil
	}

	projectID, projectName := resolveProjectFn(ce.Ctx, matches[1])

	e := &model.CommonEvent{
		Operator:     ce.Username,
		ResourceType: rbac.ResourceMember.String(),
		Operation:    operation,
		ProjectID:    projectID,
		OccurAt:      time.Now(),
		IsSuccessful: true,
	}

	before, _ := ce.Before.(*models.Member)
	var req struct {
		RoleID     int `json:"role_id"`
		MemberUser struct {
			Username string `json:"username"`
		} `json:"member_user"`
		MemberGroup struct {
			GroupName string `json:"group_name"`
		} `json:"member_group"`
	}
	_ = json.Unmarshal([]byte(ce.RequestPayload), &req)

	var entityName, entityType string
	var after *models.Member
	switch operation {
	case "create":
		e.IsSuccessful = ce.ResponseCode == http.StatusCreated
		if m := extractRe.FindStringSubmatch(ce.ResponseLocation); len(m) >= 3 && len(m[2]) > 0 {
			entityName, entityType = lookupMemberFn(ce.Ctx, m[1], m[2])
			after = getMemberFn(ce.Ctx, m[1], m[2])
		}
		if entityName == "" {
			// rejected create: the requested member is only in the body
			if req.MemberUser.Username != "" {
				entityName, entityType = req.MemberUser.Username, common.UserMember
			} else if req.MemberGroup.GroupName != "" {
				entityName, entityType = req.MemberGroup.GroupName, common.GroupMember
			}
		}
	case "delete":
		e.IsSuccessful = ce.ResponseCode == http.StatusOK
		entityName, entityType = parsePreResolved(ce.ResourceName)
	case "update":
		e.IsSuccessful = ce.ResponseCode == http.StatusOK
		if e.IsSuccessful && len(matches) >= 3 && len(matches[2]) > 0 {
			entityName, entityType = lookupMemberFn(ce.Ctx, matches[1], matches[2])
			after = getMemberFn(ce.Ctx, matches[1], matches[2])
		} else if len(matches) >= 3 && len(matches[2]) > 0 {
			entityName = matches[2]
		}
		if before != nil && entityName == matches[2] {
			entityName, entityType = before.Entityname, before.EntityType
		}
	}

	e.ResourceName = entityName
	d := desc.New(ce.Username, operation, rbac.ResourceMember.String()).Name(entityName).Set("project", projectName)
	switch entityType {
	case common.GroupMember:
		d.Set("type", "group")
	case common.UserMember:
		d.Set("type", "user")
	}
	switch operation {
	case "create":
		if after != nil {
			d.Set("id", after.ID).Set("role", roleName(after.Role))
		} else if req.RoleID != 0 {
			d.Set("role", roleName(req.RoleID))
		}
	case "update":
		to := ""
		if after != nil {
			to = roleName(after.Role)
		} else if req.RoleID != 0 {
			to = roleName(req.RoleID)
		}
		if before != nil {
			d.Changed("role", roleName(before.Role), to)
		} else {
			d.Set("role", to)
		}
	case "delete":
		if before != nil {
			d.Set("id", before.ID).Set("role", roleName(before.Role))
		}
	}
	if !e.IsSuccessful {
		d.Error(ce.ResponseCode, ce.ResponseBody)
	}
	e.OperationDescription = d.String()

	evt.Topic = ctlevent.TopicCommonEvent
	evt.Data = e
	return nil
}

func lookupMember(ctx context.Context, projectNameOrID, memberIDStr string) (string, string) {
	m := getMemberFn(ctx, projectNameOrID, memberIDStr)
	if m == nil {
		return memberIDStr, ""
	}
	return m.Entityname, m.EntityType
}

// getMember loads a project member; nil when the project or member cannot be
// resolved.
func getMember(ctx context.Context, projectNameOrID, memberIDStr string) *models.Member {
	ormCtx := orm.ReuseContext(ctx)
	projectID, _ := resolveProjectFn(ormCtx, projectNameOrID)
	if projectID == 0 {
		return nil
	}
	memberID, err := strconv.Atoi(memberIDStr)
	if err != nil {
		return nil
	}
	m, err := pkgMember.Mgr.Get(ormCtx, projectID, memberID)
	if err != nil {
		log.Errorf("failed to get member %d in project %d: %v", memberID, projectID, err)
		return nil
	}
	return m
}

// resolveProject resolves a project name or ID string to (projectID, projectName).
func resolveProject(ctx context.Context, projectNameOrID string) (int64, string) {
	ormCtx := orm.ReuseContext(ctx)
	if id, err := strconv.ParseInt(projectNameOrID, 10, 64); err == nil {
		if p, err := pkg.ProjectMgr.Get(ormCtx, id); err == nil && p != nil {
			return p.ProjectID, p.Name
		} else if err != nil {
			log.Errorf("failed to resolve project %d: %v", id, err)
		}
	}
	p, err := pkg.ProjectMgr.Get(ormCtx, projectNameOrID)
	if err != nil {
		log.Errorf("failed to resolve project %s: %v", projectNameOrID, err)
		return 0, projectNameOrID
	}
	if p == nil {
		return 0, projectNameOrID
	}
	return p.ProjectID, p.Name
}

func parsePreResolved(info string) (string, string) {
	if parts := strings.SplitN(info, ":", 2); len(parts) == 2 {
		return parts[1], parts[0]
	}
	return info, ""
}
