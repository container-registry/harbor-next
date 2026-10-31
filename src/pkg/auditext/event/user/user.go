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

package user //nolint:revive

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	"github.com/goharbor/harbor/src/pkg/auditext/event"
	pkgUser "github.com/goharbor/harbor/src/pkg/user"
)

const urlPattern = `^/api/v2.0/users/(\d+)(/password|/sysadmin)?$`

var urlRe = regexp.MustCompile(urlPattern)

// getUserFn reads a user on the request's own connection; overridable for tests
var getUserFn = func(ctx context.Context, id int) (*models.User, error) {
	return pkgUser.Mgr.Get(orm.ReuseContext(ctx), id)
}

func init() {
	var userResolver = &userEventResolver{
		Resolver: event.Resolver{
			ResourceType:        rbac.ResourceUser.String(),
			SucceedCodes:        []int{http.StatusCreated, http.StatusOK},
			SensitiveAttributes: []string{"password"},
			ShouldResolveName:   true,
			IDToNameFunc:        userIDToName,
			ResourceIDPattern:   urlPattern,
		},
	}
	userResolver.Details = userResolver.details
	commonevent.RegisterResolver(`^/api/v2\.0/users$`, userResolver)
	commonevent.RegisterResolver(urlPattern, userResolver)
}

type userEventResolver struct {
	event.Resolver
}

// userIDToName convert user id to user name
func userIDToName(ctx context.Context, userID string) string {
	id, err := strconv.ParseInt(userID, 10, 32)
	if err != nil {
		log.Errorf("failed to parse userID: %v to int", userID)
		return ""
	}
	// Runs inside the request transaction before a delete, so it must not take a
	// second pool connection (#850).
	user, err := getUserFn(ctx, int(id))
	if err != nil {
		log.Errorf("failed to get user %v, err %v", userID, err)
		return ""
	}
	return user.Username
}

// Snapshot loads the user before an update or delete so the description can
// carry the old values.
func (u *userEventResolver) Snapshot(c *commonevent.Metadata) any {
	if c.RequestMethod != http.MethodPut && c.RequestMethod != http.MethodDelete {
		return nil
	}
	id, ok := userIDFrom(c.RequestURL)
	if !ok {
		return nil
	}
	user, err := getUserFn(c.Ctx, id)
	if err != nil {
		log.Errorf("failed to snapshot user %d before %s: %v", id, c.RequestMethod, err)
		return nil
	}
	return user
}

func (u *userEventResolver) details(ce *commonevent.Metadata, evt *model.CommonEvent, d *desc.Description) {
	before, _ := ce.Before.(*models.User)
	switch ce.RequestMethod {
	case http.MethodPost:
		if !evt.IsSuccessful {
			// no Location header on failure: the requested name is only in the body
			var req struct {
				Username string `json:"username"`
			}
			if json.Unmarshal([]byte(ce.RequestPayload), &req) == nil {
				d.Name(req.Username)
			}
			return
		}
		if id, ok := userIDFrom(ce.ResponseLocation); ok {
			if user, err := getUserFn(ce.Ctx, id); err == nil {
				d.Set("id", user.UserID).Set("email", user.Email).Set("realname", user.Realname).Set("sysadmin", user.SysAdminFlag)
			}
		}
	case http.MethodDelete:
		if before != nil {
			d.Set("id", before.UserID).Set("email", before.Email).Set("sysadmin", before.SysAdminFlag)
		}
	case http.MethodPut:
		switch {
		case strings.HasSuffix(ce.RequestURL, "/password"):
			// never the value, and nothing at all when the change was rejected
			if evt.IsSuccessful {
				d.Secret("password")
			}
		case strings.HasSuffix(ce.RequestURL, "/sysadmin"):
			var req struct {
				SysadminFlag bool `json:"sysadmin_flag"`
			}
			if json.Unmarshal([]byte(ce.RequestPayload), &req) != nil {
				return
			}
			if before != nil {
				d.Changed("sysadmin", before.SysAdminFlag, req.SysadminFlag)
			} else {
				d.Set("sysadmin", req.SysadminFlag)
			}
		default:
			u.profileChanges(ce, evt, d, before)
		}
	}
}

// profileChanges diffs email, realname and comment. The new values come from
// the stored user on success and from the request body on failure.
func (u *userEventResolver) profileChanges(ce *commonevent.Metadata, evt *model.CommonEvent, d *desc.Description, before *models.User) {
	var req map[string]any
	if json.Unmarshal([]byte(ce.RequestPayload), &req) != nil {
		return
	}
	to := map[string]any{}
	for _, k := range []string{"email", "realname", "comment"} {
		if v, ok := req[k]; ok {
			to[k] = v
		}
	}
	if evt.IsSuccessful {
		if id, ok := userIDFrom(ce.RequestURL); ok {
			if after, err := getUserFn(ce.Ctx, id); err == nil {
				to = map[string]any{"email": after.Email, "realname": after.Realname, "comment": after.Comment}
			}
		}
	}
	from := map[string]any{}
	if before != nil {
		from = map[string]any{"email": before.Email, "realname": before.Realname, "comment": before.Comment}
	}
	for _, k := range []string{"email", "realname", "comment"} {
		v, ok := to[k]
		if !ok {
			continue
		}
		if before != nil {
			d.Changed(k, from[k], v)
		} else {
			d.Set(k, v)
		}
	}
}

func userIDFrom(url string) (int, bool) {
	m := urlRe.FindStringSubmatch(url)
	if len(m) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return id, true
}
