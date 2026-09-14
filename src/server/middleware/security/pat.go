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
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/local"
	"github.com/goharbor/harbor/src/common/utils"
	pat_ctl "github.com/goharbor/harbor/src/controller/pat"
	"github.com/goharbor/harbor/src/controller/user"
	usergroup_ctl "github.com/goharbor/harbor/src/controller/usergroup"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/pat/model"
)

const patPrefix = "hbr_pat_"

type pat struct{}

func (p *pat) Generate(req *http.Request) security.Context {
	ctx := req.Context()
	log := log.G(ctx)

	username, secret, ok := req.BasicAuth()
	if !ok {
		return nil
	}

	// Skip robot accounts - they are handled by the robot middleware
	if strings.HasPrefix(username, config.RobotPrefix(ctx)) {
		return nil
	}

	// New tokens carry the prefix and are looked up via the secret-prefix
	// index; legacy tokens (migrated OIDC CLI secrets) have no prefix and
	// are verified against is_legacy PAT records via a full list instead,
	// so both keep working regardless of the current auth mode.
	isNewPAT := strings.HasPrefix(secret, patPrefix)

	// Lookup the user
	u, err := user.Ctl.GetByName(ctx, username)
	if err != nil {
		log.Debugf("failed to get user %s for PAT verification: %v", username, err)
		return nil
	}

	// PAT authorization is evaluated live against current project
	// membership/role (see PATSecurityContext.Can), which for LDAP/OIDC/
	// auth-proxy users also depends on group membership. There's no live
	// IdP session to ask on a PAT-authenticated request, so read back
	// whatever group membership was last synced at login time instead.
	if groupIDs, err := usergroup_ctl.Ctl.ListUserGroupIDs(ctx, u.UserID); err != nil {
		log.Debugf("failed to list group membership for user %d: %v", u.UserID, err)
	} else {
		u.GroupIDs = groupIDs
	}

	secretToVerify := strings.TrimPrefix(secret, patPrefix)

	// For new PATs, use prefix-based lookup to avoid expensive PBKDF2
	// verification on all tokens. Legacy tokens (no prefix) fall back to
	// full list query.
	var pats []*model.PersonalAccessToken
	if isNewPAT && len(secretToVerify) > 0 {
		// Compute the same prefix we stored at token creation
		prefix := fmt.Sprintf("%x", sha256.Sum256([]byte(secretToVerify)))[:8]
		pats, err = pat_ctl.Ctl.ListBySecretPrefix(ctx, u.UserID, prefix)
		if err != nil {
			log.Debugf("failed to list PATs by prefix for user %d: %v", u.UserID, err)
			return nil
		}
		// If no match by prefix, fall back to full list (backwards compat)
		if len(pats) == 0 {
			pats, err = pat_ctl.Ctl.List(ctx, q.New(q.KeyWords{"user_id": u.UserID, "disabled": false, "is_legacy": false}))
			if err != nil {
				log.Debugf("failed to list PATs for user %d: %v", u.UserID, err)
				return nil
			}
		}
	} else {
		// Legacy token or secret too short - use full list
		pats, err = pat_ctl.Ctl.List(ctx, q.New(q.KeyWords{"user_id": u.UserID, "disabled": false, "is_legacy": !isNewPAT}))
		if err != nil {
			log.Debugf("failed to list PATs for user %d: %v", u.UserID, err)
			return nil
		}
	}

	now := time.Now().Unix()

	// Try to find a matching PAT
	for _, token := range pats {
		// Check expiry
		if token.ExpiresAt != -1 && token.ExpiresAt <= now {
			continue
		}

		// Verify the secret using PAT-specific hash verification
		if !utils.VerifyPATSecret(secretToVerify, token.Secret) {
			continue
		}

		// Found a matching token - update last_used_at in the background
		// Detach from request context to avoid cancellation when request ends
		go func(t *model.PersonalAccessToken) {
			bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			t.LastUsedAt = time.Now().Unix()
			_ = pat_ctl.Ctl.Update(bgCtx, t, "last_used_at")
		}(token)

		log.Debugf("PAT authentication successful for user %s", username)
		return local.NewPATSecurityContext(u, token.Scope)
	}

	log.Debugf("failed to authenticate with PAT for user %s", username)
	return nil
}
