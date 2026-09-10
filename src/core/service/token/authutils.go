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

package token //nolint:revive

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/docker/distribution/registry/auth/token"
	"github.com/golang-jwt/jwt/v5"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	tokenpkg "github.com/goharbor/harbor/src/pkg/token"
	v2 "github.com/goharbor/harbor/src/pkg/token/claims/v2"
)

var (
	privateKey string
)

func init() {
	privateKey = config.TokenPrivateKeyPath()
}

// GetResourceActions ...
func GetResourceActions(scopes []string) []*token.ResourceActions {
	log.Debugf("scopes: %+v", scopes)
	var res []*token.ResourceActions
	for _, s := range scopes {
		if s == "" {
			continue
		}
		items := strings.Split(s, ":")
		length := len(items)

		typee := ""
		name := ""
		actions := make([]string, 0)

		if length == 1 {
			typee = items[0]
		} else if length == 2 {
			typee = items[0]
			name = items[1]
		} else {
			typee = items[0]
			name = strings.Join(items[1:length-1], ":")
			if len(items[length-1]) > 0 {
				actions = strings.Split(items[length-1], ",")
			}
		}

		res = append(res, &token.ResourceActions{
			Type:    typee,
			Name:    name,
			Actions: actions,
		})
	}
	return res
}

// filterAccess iterate a list of resource actions and try to use the filter that matches the resource type to filter the actions.
func filterAccess(ctx context.Context, access []*token.ResourceActions,
	ctl project.Controller, filters map[string]accessFilter) error {
	secCtx, ok := security.FromContext(ctx)
	if !ok {
		return fmt.Errorf("failed to  get security context from request")
	}
	var err error
	for _, a := range access {
		f, ok := filters[a.Type]
		if !ok {
			a.Actions = []string{}
			log.Warningf("No filter found for access type: %s, skip filter, the access of resource '%s' will be set empty.", a.Type, a.Name)
			continue
		}
		err = f.filter(ctx, ctl, a)
		log.Debugf("user: %s, access: %v", secCtx.GetUsername(), a)
		if err != nil {
			log.Errorf("Failed to handle the resource %s:%s, due to error %v, returning empty access for it.",
				a.Type, a.Name, err)
			a.Actions = []string{}
		}
	}
	return nil
}

// MakeToken makes a valid jwt token based on parms.
func MakeToken(ctx context.Context, username, service string, access []*token.ResourceActions) (*models.Token, error) {
	options, err := tokenpkg.NewOptions("", v2.Issuer, privateKey)
	if err != nil {
		return nil, err
	}
	expiration, err := config.TokenExpiration(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	claims := &v2.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    options.Issuer,
			Subject:   username,
			Audience:  jwt.ClaimStrings([]string{service}),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expiration) * time.Minute)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        utils.GenerateRandomStringWithLen(16),
		},
		Access: access,
	}
	tok, err := tokenpkg.New(options, claims)
	if err != nil {
		return nil, err
	}
	// Add kid to token header for compatibility with docker distribution's code
	// see https://github.com/docker/distribution/blob/release/2.7/registry/auth/token/token.go#L197
	// Use the key from options.GetKey() to derive the kid, supporting both PKCS8 and traditional PEM formats
	key, err := options.GetKey()
	if err != nil {
		return nil, err
	}
	kid, err := generateKeyID(key)
	if err != nil {
		return nil, err
	}
	tok.Header["kid"] = kid

	rawToken, err := tok.Raw()
	if err != nil {
		return nil, err
	}
	return &models.Token{
		Token:     rawToken,
		ExpiresIn: expiration * 60,
		IssuedAt:  now.Format(time.RFC3339),
	}, nil
}

// generateKeyID derives a key ID from a crypto key.
// This approach supports RSA, ECDSA, and Ed25519 keys without relying on libtrust.
func generateKeyID(key any) (string, error) {
	var pubBytes []byte
	var err error

	switch k := key.(type) {
	case *rsa.PrivateKey:
		pubBytes, err = x509.MarshalPKIXPublicKey(&k.PublicKey)
	case *ecdsa.PrivateKey:
		pubBytes, err = x509.MarshalPKIXPublicKey(&k.PublicKey)
	case ed25519.PrivateKey:
		pubBytes = []byte(k)
	case *rsa.PublicKey:
		pubBytes, err = x509.MarshalPKIXPublicKey(k)
	case *ecdsa.PublicKey:
		pubBytes, err = x509.MarshalPKIXPublicKey(k)
	case ed25519.PublicKey:
		pubBytes = []byte(k)
	default:
		return "", fmt.Errorf("unsupported key type: %T", key)
	}

	if err != nil {
		return "", err
	}

	// Use SHA256 hash of the key bytes as the key ID (first 8 hex chars)
	hash := sha256.Sum256(pubBytes)
	return fmt.Sprintf("%x", hash[:8]), nil
}
