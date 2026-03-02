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
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	registry_token "github.com/docker/distribution/registry/auth/token"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/v2token"
	"github.com/goharbor/harbor/src/common/utils"
	project_ctl "github.com/goharbor/harbor/src/controller/project"
	svc_token "github.com/goharbor/harbor/src/core/service/token"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/token"
	v2 "github.com/goharbor/harbor/src/pkg/token/claims/v2"
)

type v2TokenClaims struct {
	v2.Claims
	Access []*registry_token.ResourceActions `json:"access"`
}

type v2Token struct{}

func (vt *v2Token) Generate(req *http.Request) security.Context {
	logger := log.G(req.Context())
	if !strings.HasPrefix(req.URL.Path, "/v2") {
		return nil
	}
	tokenStr := bearerToken(req)
	if len(tokenStr) == 0 {
		return nil
	}

	defaultOpt := token.DefaultTokenOptions()
	if defaultOpt == nil {
		logger.Warningf("failed to get default options")
		return nil
	}
	cl := &v2TokenClaims{}
	t, err := token.Parse(defaultOpt, tokenStr, cl)
	if err != nil {
		logger.Warningf("failed to decode bearer token: %v", err)
		return nil
	}
	var v = jwt.NewValidator(jwt.WithLeeway(common.JwtLeeway), jwt.WithAudience(svc_token.Registry))
	if err := v.Validate(t.Claims); err != nil {
		logger.Warningf("failed to decode bearer token: %v", err)
		return nil
	}
	claims, ok := t.Claims.(*v2TokenClaims)
	if !ok {
		logger.Warningf("invalid token claims.")
		return nil
	}
	if !tokenIssuedAfterProjectCreation(req.Context(), logger, claims) {
		return nil
	}
	return v2token.New(req.Context(), claims.Subject, claims.Access)
}

// tokenIssuedAfterProjectCreation checks that the token was not issued before
// the target project was created. This prevents tokens from a deleted project
// being reused against a new project with the same name.
func tokenIssuedAfterProjectCreation(ctx context.Context, logger *log.Logger, claims *v2TokenClaims) bool {
	if claims.IssuedAt == nil {
		logger.Warningf("bearer token has no issued-at claim, rejecting")
		return false
	}
	iat := claims.IssuedAt.Time

	for _, access := range claims.Access {
		if access.Type != "repository" {
			continue
		}
		projectName, _ := utils.ParseRepository(access.Name)
		if projectName == "" {
			continue
		}
		p, err := project_ctl.Ctl.GetByName(ctx, projectName)
		if err != nil {
			logger.Warningf("failed to get project %q for token validation: %v", projectName, err)
			return false
		}
		if iat.Before(p.CreationTime) {
			logger.Warningf("bearer token issued at %v is before project %q creation time %v, rejecting",
				iat, projectName, p.CreationTime)
			return false
		}
	}
	return true
}
