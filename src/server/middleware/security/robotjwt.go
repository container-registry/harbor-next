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
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v3/jwk"
	jwxjwt "github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/security"
	robotCtx "github.com/goharbor/harbor/src/common/security/robot"
	"github.com/goharbor/harbor/src/controller/federatedidp"
	robot_ctl "github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
	"github.com/goharbor/harbor/src/pkg/token"
	"github.com/goharbor/harbor/src/server/middleware/security/jwkscache"
	"github.com/goharbor/harbor/src/server/middleware/security/jwthandler"
)

// JWK represents a single JSON Web Key
type JWK struct {
	Kid *string  `json:"kid,omitempty"`
	Kty *string  `json:"kty,omitempty"`
	Use *string  `json:"use,omitempty"`
	N   *string  `json:"n,omitempty"`
	E   *string  `json:"e,omitempty"`
	X5c []string `json:"x5c,omitempty"`
}

// JWKS represents a set of JSON Web Keys
type JWKS struct {
	Keys []JWK `json:"keys,omitempty"`
}

// Global JWKS cache manager (initialized once, thread-safe)
var (
	jwksCacheManager *jwkscache.Manager
	jwksCacheOnce    sync.Once
)

// getJWKSCacheManager returns the singleton JWKS cache manager
func getJWKSCacheManager() *jwkscache.Manager {
	jwksCacheOnce.Do(func() {
		jwksCacheManager = jwkscache.NewManager()
	})
	return jwksCacheManager
}

type robotjwt struct{}

// Generate creates a security context for robot accounts authenticated via JWT tokens
// from federated identity providers.
//
// The flow is:
//  1. Extract JWT from Bearer token or Basic Auth password field
//  2. Parse the token to get issuer and key ID (kid)
//  3. Look up the federated IdP by issuer
//  4. Validate the JWT signature using IdP's JWKS (online or offline)
//  5. Validate IdP-level claims (claims with robot_id=0)
//  6. Find the robot account whose claims are ALL present in the token
//     (token may have more claims than the robot requires)
//  7. Return security context for the matched robot
func (r *robotjwt) Generate(req *http.Request) security.Context {
	logger := log.G(req.Context())

	// Step 1: Extract JWT token from request
	tokenStr := extractJWTToken(req)
	if tokenStr == "" {
		return nil
	}

	// Step 2: Parse token to get issuer and kid (without signature validation)
	jwtToken, err := jwt.Parse(tokenStr, nil)
	if jwtToken == nil {
		logger.Debug("failed to parse JWT token structure")
		return nil
	}

	issuer, err := jwtToken.Claims.GetIssuer()
	if err != nil || issuer == "" {
		logger.Debugf("failed to get issuer from token: %v", err)
		return nil
	}

	kid, ok := jwtToken.Header["kid"].(string)
	if !ok || kid == "" {
		logger.Debugf("token header missing kid")
		return nil
	}

	// Step 3: Get federated IdP by issuer
	idp, err := federatedidp.Ctl.GetIdpByIssuer(req.Context(), issuer)
	if err != nil {
		logger.Debugf("no IdP found for issuer %s: %v", issuer, err)
		return nil
	}

	// Step 4: Get JWKS and validate the token
	jwkSet, err := getJWKSet(req.Context(), idp, kid)
	if err != nil {
		logger.Debugf("failed to get JWK set: %v", err)
		return nil
	}

	// Step 4.5: Validate JWT algorithm against IdP's supported algorithms
	supportedAlgorithms := getSupportedAlgorithms(idp)
	if _, err := jwthandler.ValidateAlgorithm(tokenStr, supportedAlgorithms); err != nil {
		logger.Debugf("JWT algorithm validation failed: %v", err)
		return nil
	}

	parsedToken, err := jwthandler.ParseToken(tokenStr, jwkSet)
	if err != nil {
		logger.Debugf("JWT signature validation failed: %v", err)
		return nil
	}

	// Step 5: Validate IdP-level claims
	if err := validateIDPClaims(req.Context(), idp.ID, parsedToken); err != nil {
		logger.Debugf("IdP claims validation failed: %v", err)
		return nil
	}

	// Step 6: Find matching robot account
	tokenClaims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		logger.Debugf("failed to get token claims as MapClaims")
		return nil
	}

	robot, err := findMatchingRobot(req.Context(), idp.ID, tokenClaims)
	if err != nil {
		logger.Debugf("no matching robot found: %v", err)
		return nil
	}

	// Step 7: Return security context
	logger.Debugf("robot JWT auth successful for %s, request: %s %s", robot.Name, req.Method, req.URL.Path)
	return robotCtx.NewSecurityContext(robot)
}

// extractJWTToken extracts JWT from Bearer header or Basic Auth password
func extractJWTToken(req *http.Request) string {
	// Try Bearer token first
	tokenStr := bearerToken(req)
	if tokenStr != "" {
		return tokenStr
	}

	// Try Basic Auth password
	tokenStr = basicAuthToken(req)
	if tokenStr == "" {
		return ""
	}

	if !IsJWT(tokenStr) {
		return ""
	}

	return tokenStr
}

// getJWKSet retrieves JWKS based on IdP configuration (online or offline)
// Online mode uses two-tier caching (in-memory + DB) for performance
func getJWKSet(ctx context.Context, idp *model.FederatedIdp, kid string) (jwk.Set, error) {
	if idp.OfflineValidation {
		// Offline: use stored JWKS keys (no caching needed)
		if idp.JWKSKeys == "" || idp.JWKSKeys == "{}" {
			return nil, fmt.Errorf("IdP %s has no JWKS keys configured", idp.Name)
		}

		jwkSet, err := jwk.Parse([]byte(idp.JWKSKeys))
		if err != nil {
			return nil, fmt.Errorf("failed to parse stored JWKS: %w", err)
		}

		if _, found := jwkSet.LookupKeyID(kid); !found {
			return nil, fmt.Errorf("key %s not found in stored JWKS", kid)
		}

		return jwkSet, nil
	}

	// Online: use two-tier cache (in-memory + DB)
	manager := getJWKSCacheManager()
	jwkSet, err := manager.GetJWKS(ctx, idp)
	if err != nil {
		return nil, fmt.Errorf("failed to get JWKS: %w", err)
	}

	if _, found := jwkSet.LookupKeyID(kid); !found {
		return nil, fmt.Errorf("key %s not found in JWKS", kid)
	}

	return jwkSet, nil
}

// validateIDPClaims checks that the token contains all required IdP-level claims
// IdP-level claims are those with robot_id=0
func validateIDPClaims(ctx context.Context, idpID int64, parsedToken jwxjwt.Token) error {
	idpClaims, err := federatedidp.Ctl.ListClaimsIdpOnly(ctx, idpID, "")
	if err != nil {
		return fmt.Errorf("failed to list IdP claims: %w", err)
	}

	tokenValues := tokenClaimValues(parsedToken)
	for _, claim := range idpClaims {
		if !claimValuesMatch(tokenValues[claim.ClaimPath], claim.Value) {
			return fmt.Errorf("claim %s mismatch: want %q", claim.ClaimPath, claim.Value)
		}
	}

	return nil
}

// findMatchingRobot finds the robot whose claims are all present in the token.
// The robot with the most matching claims wins. All robot claims must be in the token,
// but the token may have additional claims not required by the robot.
func findMatchingRobot(ctx context.Context, idpID int64, tokenClaims jwt.MapClaims) (*robot_ctl.Robot, error) {
	robotID, err := federatedidp.Ctl.GetTopMatchedRobot(ctx, idpID, tokenClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to query matching robot: %w", err)
	}

	if robotID == 0 {
		return nil, fmt.Errorf("no robot matched the token claims")
	}

	robot, err := robot_ctl.Ctl.Get(ctx, robotID, &robot_ctl.Option{
		WithPermission: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get robot %d: %w", robotID, err)
	}

	if robot == nil {
		return nil, fmt.Errorf("robot %d not found", robotID)
	}

	if robot.Disabled {
		return nil, fmt.Errorf("robot %s is disabled", robot.Name)
	}

	if robot.ExpiresAt != -1 && robot.ExpiresAt <= time.Now().Unix() {
		return nil, fmt.Errorf("robot %s is expired", robot.Name)
	}

	return robot, nil
}

// tokenClaimValues flattens a parsed JWT into dot-path keys with one or more scalar values.
func tokenClaimValues(token jwxjwt.Token) map[string][]string {
	values := make(map[string][]string)
	for _, key := range token.Keys() {
		var value any
		if err := token.Get(key, &value); err != nil {
			continue
		}
		flattenClaims(key, value, values)
	}
	return values
}

// flattenClaims recursively flattens nested JWT objects into dot-separated claim paths.
func flattenClaims(prefix string, data any, out map[string][]string) {
	switch v := data.(type) {
	case jwt.MapClaims:
		for key, value := range v {
			flattenClaims(joinClaimPath(prefix, key), value, out)
		}
	case map[string]any:
		for key, value := range v {
			flattenClaims(joinClaimPath(prefix, key), value, out)
		}
	case []string:
		for _, item := range v {
			out[prefix] = append(out[prefix], fmt.Sprintf("%v", item))
		}
	case []any:
		for _, item := range v {
			switch item.(type) {
			case map[string]any, []any, []string:
				continue
			default:
				out[prefix] = append(out[prefix], fmt.Sprintf("%v", item))
			}
		}
	default:
		out[prefix] = append(out[prefix], fmt.Sprintf("%v", v))
	}
}

func joinClaimPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func claimValuesMatch(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

// GetSupportedClaims fetches OpenID configuration and returns claims_supported
func GetSupportedClaims(ctx context.Context, openIDConfigURL string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openIDConfigURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OpenID config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("unexpected status %d from OpenID config: %s", resp.StatusCode, string(body))
	}

	var config map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode OpenID config: %w", err)
	}

	rawClaims, ok := config["claims_supported"]
	if !ok {
		return nil, nil
	}

	claimsArray, ok := rawClaims.([]any)
	if !ok {
		return nil, fmt.Errorf("claims_supported has unexpected type: %T", rawClaims)
	}

	var claims []string
	for _, c := range claimsArray {
		if s, ok := c.(string); ok {
			claims = append(claims, s)
		}
	}

	return claims, nil
}

// GetAndParseJWK fetches JWKS from a URI
func GetAndParseJWK(ctx context.Context, jwksURI string) (jwk.Set, error) {
	set, err := jwk.Fetch(ctx, jwksURI)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	return set, nil
}

// getJWKFromJWKS finds a JWK by key ID from a JWKS JSON string
func getJWKFromJWKS(jwksJSON string, tokenKid string) (*JWK, error) {
	var jwks JWKS
	if err := json.Unmarshal([]byte(jwksJSON), &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS JSON: %w", err)
	}

	for i := range jwks.Keys {
		if jwks.Keys[i].Kid != nil && *jwks.Keys[i].Kid == tokenKid {
			return &jwks.Keys[i], nil
		}
	}

	return nil, fmt.Errorf("no matching JWK found for kid: %s", tokenKid)
}

// jwkToPublicKey converts a JWK to PKIX-encoded public key bytes
func jwkToPublicKey(jwkKey JWK) ([]byte, error) {
	// Try x5c certificate chain first
	if len(jwkKey.X5c) > 0 {
		certBytes, err := base64.StdEncoding.DecodeString(jwkKey.X5c[0])
		if err != nil {
			return nil, fmt.Errorf("failed to decode x5c certificate: %w", err)
		}

		cert, err := x509.ParseCertificate(certBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate: %w", err)
		}

		return x509.MarshalPKIXPublicKey(cert.PublicKey)
	}

	// Fall back to RSA n and e
	if jwkKey.N == nil || jwkKey.E == nil {
		return nil, fmt.Errorf("JWK does not contain 'n', 'e', or 'x5c' fields")
	}

	modulusBytes, err := base64.RawURLEncoding.DecodeString(*jwkKey.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	exponentBytes, err := base64.RawURLEncoding.DecodeString(*jwkKey.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	pubKey := &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulusBytes),
		E: int(new(big.Int).SetBytes(exponentBytes).Int64()),
	}

	return x509.MarshalPKIXPublicKey(pubKey)
}

// getRSAPublicKeyFromJWK converts a JWK to an RSA public key
func getRSAPublicKeyFromJWK(jwkKey *JWK) (*rsa.PublicKey, error) {
	if jwkKey.Kty == nil || *jwkKey.Kty != "RSA" {
		return nil, fmt.Errorf("unsupported key type: %v", jwkKey.Kty)
	}
	if jwkKey.N == nil || jwkKey.E == nil {
		return nil, fmt.Errorf("missing modulus or exponent")
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(*jwkKey.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(*jwkKey.E)
	if err != nil {
		eBytes, err = base64.URLEncoding.DecodeString(*jwkKey.E)
		if err != nil {
			return nil, fmt.Errorf("failed to decode exponent: %w", err)
		}
	}

	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("invalid exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}

// publicKeyToPEMBytes converts an RSA public key to PEM format
func publicKeyToPEMBytes(pubKey *rsa.PublicKey) ([]byte, error) {
	derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}), nil
}

// ParseToken parses and validates a JWT token with the given signing method
func ParseToken(signMethod jwt.SigningMethod, publicKey any, rawToken string, claims jwt.Claims) (*token.Token, error) {
	parser := jwt.NewParser(jwt.WithLeeway(common.JwtLeeway), jwt.WithValidMethods([]string{signMethod.Alg()}))
	tokn, err := parser.ParseWithClaims(rawToken, claims, func(_ *jwt.Token) (any, error) {
		switch signMethod.Alg() {
		case "RS256":
			pk, ok := publicKey.(rsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("invalid public key type for RS256")
			}
			return &pk, nil
		case "ES256":
			pk, ok := publicKey.(ecdsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("invalid public key type for ES256")
			}
			return &pk, nil
		default:
			return publicKey, nil
		}
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	if !tokn.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return &token.Token{Token: *tokn}, nil
}

// getSupportedAlgorithms returns the list of supported algorithms for an IdP
// Returns nil if no algorithms are configured (allows any algorithm)
func getSupportedAlgorithms(idp *model.FederatedIdp) []string {
	if idp.SupportedAlgorithms == "" {
		return nil
	}
	return strings.Split(idp.SupportedAlgorithms, ",")
}

// basicAuthToken extracts the password from a Basic Auth header.
func basicAuthToken(req *http.Request) string {
	if req == nil {
		return ""
	}

	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Basic ") {
		return ""
	}

	encoded := strings.TrimSpace(strings.TrimPrefix(h, "Basic "))
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return ""
	}

	return parts[1]
}
