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

package jwthandler

import (
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/log"
)

// ValidateAlgorithm extracts and validates the JWT's algorithm against the allowed list.
// Returns the algorithm string (normalized to uppercase) if valid, or a generic error if not.
// This must be called BEFORE signature validation to prevent algorithm confusion attacks.
//
// If supportedAlgorithms is empty or nil, any algorithm is allowed (for backward compatibility).
func ValidateAlgorithm(tokenStr string, supportedAlgorithms []string) (string, error) {
	// Parse the JWT without validation to extract the header
	msg, err := jws.Parse([]byte(tokenStr))
	if err != nil {
		log.Debug("failed to parse JWS structure")
		return "", fmt.Errorf("invalid token")
	}

	// Get the signatures (a compact JWT has exactly one)
	sigs := msg.Signatures()
	if len(sigs) == 0 {
		log.Debug("JWS has no signatures")
		return "", fmt.Errorf("invalid token")
	}

	// Extract algorithm from protected header
	alg, ok := sigs[0].ProtectedHeaders().Algorithm()
	if !ok {
		log.Debug("JWS signature has no algorithm")
		return "", fmt.Errorf("invalid token")
	}

	// Get string representation and normalize to uppercase
	algStr := strings.ToUpper(alg.String())
	if algStr == "" {
		log.Debug("JWS algorithm is empty")
		return "", fmt.Errorf("invalid token")
	}

	// If no supported algorithms configured, allow any (backward compatibility)
	if len(supportedAlgorithms) == 0 {
		log.Debugf("no supported algorithms configured, allowing %s", algStr)
		return algStr, nil
	}

	// Check if algorithm is in the supported list (case-insensitive)
	for _, supported := range supportedAlgorithms {
		if strings.EqualFold(algStr, supported) {
			return algStr, nil
		}
	}

	// Algorithm not in allowed list - return generic error (security: don't reveal allowed algorithms)
	log.Debugf("algorithm %s not in supported list %v", algStr, supportedAlgorithms)
	return "", fmt.Errorf("invalid token")
}

// ParseToken parses and validates a JWT token using the provided JWKS.
// It validates the signature and standard claims (exp, nbf, iat).
func ParseToken(token string, jwkSet jwk.Set) (jwt.Token, error) {
	parsedToken, err := jwt.Parse([]byte(token), jwt.WithKeySet(jwkSet), jwt.WithValidate(true), jwt.WithAcceptableSkew(common.JwtLeeway))
	if err != nil {
		log.Debug("failed to verify JWT signature")
		return nil, fmt.Errorf("invalid token")
	}

	log.Debug("JWT signature validation successful")
	return parsedToken, nil
}
