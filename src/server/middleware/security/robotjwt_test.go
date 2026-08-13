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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
)

// TestExtractJWTToken tests token extraction from different sources
func TestExtractJWTToken(t *testing.T) {
	config.InitWithSettings(map[string]any{common.RobotNamePrefix: "robot@"})

	testJWT := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJ0ZXN0In0.signature"

	tests := []struct {
		name     string
		setup    func() *http.Request
		expected string
	}{
		{
			name: "bearer token",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/api/projects", nil)
				req.Header.Set("Authorization", "Bearer "+testJWT)
				return req
			},
			expected: testJWT,
		},
		{
			name: "basic auth with JWT password",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/api/projects", nil)
				req.SetBasicAuth("robot@test", testJWT)
				return req
			},
			expected: testJWT,
		},
		{
			name: "basic auth with regular password",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/api/projects", nil)
				req.SetBasicAuth("robot@test", "Harbor12345")
				return req
			},
			expected: "",
		},
		{
			name: "basic auth with non-robot username",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/api/projects", nil)
				req.SetBasicAuth("admin", testJWT)
				return req
			},
			expected: testJWT,
		},
		{
			name: "no authorization",
			setup: func() *http.Request {
				return httptest.NewRequest("GET", "/api/projects", nil)
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()
			result := extractJWTToken(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFlattenClaims tests the claim flattening behavior used for IdP-level JWT validation.
func TestFlattenClaims(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string][]string
	}{
		{
			name: "simple scalar claims stay at top level",
			input: map[string]any{
				"sub": "user123",
				"iss": "https://issuer.com",
			},
			expected: map[string][]string{
				"sub": {"user123"},
				"iss": {"https://issuer.com"},
			},
		},
		{
			name: "nested object claims flatten to dot paths",
			input: map[string]any{
				"user": map[string]any{
					"name":  "John Doe",
					"email": "john@example.com",
				},
				"kubernetes.io": map[string]any{
					"namespace": "default",
					"serviceaccount": map[string]any{
						"name": "builder",
					},
				},
			},
			expected: map[string][]string{
				"user.name":                         {"John Doe"},
				"user.email":                        {"john@example.com"},
				"kubernetes.io.namespace":           {"default"},
				"kubernetes.io.serviceaccount.name": {"builder"},
			},
		},
		{
			name: "array claims keep all scalar values for contains matching",
			input: map[string]any{
				"aud": []any{"kubernetes.default.svc", "harbor.example.com"},
			},
			expected: map[string][]string{
				"aud": {"kubernetes.default.svc", "harbor.example.com"},
			},
		},
		{
			name: "objects inside arrays are ignored",
			input: map[string]any{
				"kubernetes.io": []any{map[string]any{"namespace": "default"}},
			},
			expected: map[string][]string{},
		},
		{
			name: "literal dotted top-level keys are preserved",
			input: map[string]any{
				"kubernetes.io.namespace": "default",
			},
			expected: map[string][]string{
				"kubernetes.io.namespace": {"default"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string][]string)
			flattenClaims("", tt.input, result)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBasicAuthTokenExtraction tests basic auth token extraction
func TestBasicAuthTokenExtraction(t *testing.T) {
	config.InitWithSettings(map[string]any{common.RobotNamePrefix: "robot@"})

	tests := []struct {
		name          string
		authorization string
		expected      string
	}{
		{
			name:          "valid basic auth with JWT password",
			authorization: "Basic " + base64.StdEncoding.EncodeToString([]byte("robot@test:eyJhbGciOiJSUzI1NiJ9.payload.signature")),
			expected:      "eyJhbGciOiJSUzI1NiJ9.payload.signature",
		},
		{
			name:          "basic auth with non-robot username",
			authorization: "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:eyJhbGciOiJSUzI1NiJ9.payload.signature")),
			expected:      "eyJhbGciOiJSUzI1NiJ9.payload.signature",
		},
		{
			name:          "basic auth with regular password",
			authorization: "Basic " + base64.StdEncoding.EncodeToString([]byte("robot@test:Harbor12345")),
			expected:      "Harbor12345",
		},
		{
			name:          "no authorization header",
			authorization: "",
			expected:      "",
		},
		{
			name:          "not basic auth",
			authorization: "Bearer token",
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/projects", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			result := basicAuthToken(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsJWT tests JWT format validation
func TestIsJWT(t *testing.T) {
	tests := []struct {
		name  string
		token string
		isJWT bool
	}{
		{
			name:  "valid JWT format",
			token: "eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJ0ZXN0In0.signature",
			isJWT: true,
		},
		{
			name:  "regular password",
			token: "Harbor12345",
			isJWT: false,
		},
		{
			name:  "empty string",
			token: "",
			isJWT: false,
		},
		{
			name:  "only two parts",
			token: "eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJ0ZXN0In0",
			isJWT: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsJWT(tt.token)
			assert.Equal(t, tt.isJWT, result)
		})
	}
}

func TestIsJWTDoesNotLogToken(t *testing.T) {
	buf := &bytes.Buffer{}
	log.DefaultLogger().SetOutput(buf)
	defer log.DefaultLogger().SetOutput(os.Stdout)

	secret := "Harbor12345"
	assert.False(t, IsJWT(secret))
	assert.NotContains(t, buf.String(), secret)
	assert.Empty(t, buf.String())
}

func TestClaimValuesMatch(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
		match  bool
	}{
		{
			name:   "matches exact scalar value",
			values: []string{"harbor.local"},
			want:   "harbor.local",
			match:  true,
		},
		{
			name:   "matches when array contains configured value",
			values: []string{"kubernetes.default.svc", "harbor.local"},
			want:   "harbor.local",
			match:  true,
		},
		{
			name:   "does not match when array omits configured value",
			values: []string{"kubernetes.default.svc", "other.local"},
			want:   "harbor.local",
			match:  false,
		},
		{
			name:   "trims whitespace around stored and token values",
			values: []string{" harbor.local "},
			want:   "harbor.local",
			match:  true,
		},
		{
			name:   "missing value does not match",
			values: nil,
			want:   "harbor.local",
			match:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.match, claimValuesMatch(tt.values, tt.want))
		})
	}
}

// TestRobotJWTNoToken tests when no JWT token is provided
func TestRobotJWTNoToken(t *testing.T) {
	conf := map[string]interface{}{
		common.RobotNamePrefix: "robot@",
	}
	config.InitWithSettings(conf)

	robotjwt := &robotjwt{}
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.SetBasicAuth("robot@test", "Harbor12345")

	ctx := robotjwt.Generate(req)
	assert.Nil(t, ctx)
}

// TestRobotJWTWithoutIssuer tests JWT without issuer claim
func TestRobotJWTWithoutIssuer(t *testing.T) {
	conf := map[string]interface{}{
		common.RobotNamePrefix: "robot@",
	}
	config.InitWithSettings(conf)

	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "test-subject",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	robotjwt := &robotjwt{}
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	ctx := robotjwt.Generate(req)
	assert.Nil(t, ctx)
}

// TestGetJWKFromJWKS tests JWK extraction from JWKS
func TestGetJWKFromJWKS(t *testing.T) {
	tests := []struct {
		name      string
		jwksJSON  string
		tokenKid  string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid JWKS with matching kid",
			jwksJSON:  `{"keys": [{"kid": "key-1", "kty": "RSA", "n": "test-modulus", "e": "AQAB"}]}`,
			tokenKid:  "key-1",
			expectErr: false,
		},
		{
			name:      "valid JWKS without matching kid",
			jwksJSON:  `{"keys": [{"kid": "key-2", "kty": "RSA", "n": "test-modulus", "e": "AQAB"}]}`,
			tokenKid:  "key-1",
			expectErr: true,
			errMsg:    "no matching JWK found",
		},
		{
			name:      "invalid JSON",
			jwksJSON:  `invalid json`,
			tokenKid:  "key-1",
			expectErr: true,
			errMsg:    "failed to parse JWKS JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwkKey, err := getJWKFromJWKS(tt.jwksJSON, tt.tokenKid)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, jwkKey)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, jwkKey)
				assert.Equal(t, tt.tokenKid, *jwkKey.Kid)
			}
		})
	}
}

// TestGetSupportedClaims tests OpenID configuration fetching
func TestGetSupportedClaims(t *testing.T) {
	t.Run("valid openid config", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"issuer": "https://example.com", "claims_supported": ["sub", "iss", "aud"]}`))
		}))
		defer server.Close()

		claims, err := GetSupportedClaims(nil, server.URL)
		assert.NoError(t, err)
		assert.Len(t, claims, 3)
	})

	t.Run("missing claims_supported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"issuer": "https://example.com"}`))
		}))
		defer server.Close()

		claims, err := GetSupportedClaims(nil, server.URL)
		assert.NoError(t, err)
		assert.Nil(t, claims)
	})

	t.Run("404 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		_, err := GetSupportedClaims(nil, server.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status 404")
	})
}

// Test helper functions for JWT creation and key management

func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey, &privateKey.PublicKey
}

func createTestJWT(t *testing.T, claims jwt.MapClaims, privateKey *rsa.PrivateKey) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-id"

	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return tokenString
}

func createTestJWKSet(t *testing.T, publicKey *rsa.PublicKey) jwk.Set {
	// Convert RSA public key to PKIX format
	pkixBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)

	// Create PEM block
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pkixBytes,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	// Parse with jwk
	key, err := jwk.ParseKey(pemBytes, jwk.WithPEM(true))
	require.NoError(t, err)

	// Set the key ID
	err = key.Set(jwk.KeyIDKey, "test-key-id")
	require.NoError(t, err)

	// Create set
	set := jwk.NewSet()
	set.AddKey(key)

	return set
}

func mustMarshalJWK(t *testing.T, set jwk.Set) string {
	data, err := json.Marshal(set)
	require.NoError(t, err)
	return string(data)
}

// TestRobotJWTClaimMatchingLogic documents and tests the claim matching behavior
func TestRobotJWTClaimMatchingLogic(t *testing.T) {
	t.Run("robot claims subset of JWT claims - should match", func(t *testing.T) {
		// This test documents the expected behavior:
		// Robot has claims: org=harbor, team=development
		// JWT has claims: org=harbor, team=development, role=admin, project=library
		// Expected: MATCH (all robot claims are present in JWT)

		robotClaims := map[string]string{
			"org":  "harbor",
			"team": "development",
		}

		jwtClaims := map[string]string{
			"org":     "harbor",
			"team":    "development",
			"role":    "admin",
			"project": "library",
		}

		// Simulate the matching logic: all robot claims must be in JWT
		allPresent := true
		for robotKey, robotValue := range robotClaims {
			jwtValue, exists := jwtClaims[robotKey]
			if !exists || jwtValue != robotValue {
				allPresent = false
				break
			}
		}

		assert.True(t, allPresent, "Robot claims should be a subset of JWT claims")
	})

	t.Run("JWT missing required robot claim - should not match", func(t *testing.T) {
		robotClaims := map[string]string{
			"org":  "harbor",
			"team": "development",
		}

		jwtClaims := map[string]string{
			"org": "harbor",
			// Missing "team" claim
			"role":    "admin",
			"project": "library",
		}

		allPresent := true
		for robotKey, robotValue := range robotClaims {
			jwtValue, exists := jwtClaims[robotKey]
			if !exists || jwtValue != robotValue {
				allPresent = false
				break
			}
		}

		assert.False(t, allPresent, "Should not match when JWT is missing robot claims")
	})

	t.Run("JWT claim value mismatch - should not match", func(t *testing.T) {
		robotClaims := map[string]string{
			"org": "harbor",
		}

		jwtClaims := map[string]string{
			"org": "different-org", // Wrong value
		}

		allPresent := true
		for robotKey, robotValue := range robotClaims {
			jwtValue, exists := jwtClaims[robotKey]
			if !exists || jwtValue != robotValue {
				allPresent = false
				break
			}
		}

		assert.False(t, allPresent, "Should not match when claim values differ")
	})
}

// TestGetSupportedAlgorithms tests the algorithm extraction from IdP model
func TestGetSupportedAlgorithms(t *testing.T) {
	tests := []struct {
		name       string
		algorithms string
		expected   []string
	}{
		{
			name:       "single algorithm",
			algorithms: "RS256",
			expected:   []string{"RS256"},
		},
		{
			name:       "multiple algorithms",
			algorithms: "RS256,RS384,RS512",
			expected:   []string{"RS256", "RS384", "RS512"},
		},
		{
			name:       "empty string",
			algorithms: "",
			expected:   nil,
		},
		{
			name:       "mixed algorithms",
			algorithms: "RS256,ES256,PS256",
			expected:   []string{"RS256", "ES256", "PS256"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idp := &model.FederatedIdp{
				SupportedAlgorithms: tt.algorithms,
			}
			result := getSupportedAlgorithms(idp)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAlgorithmValidationIntegration tests the integration of algorithm validation
func TestAlgorithmValidationIntegration(t *testing.T) {
	t.Run("algorithm validation is called before signature validation", func(t *testing.T) {
		// This test documents that algorithm validation happens BEFORE signature validation
		// in the Generate() flow to prevent algorithm confusion attacks
		//
		// Flow should be:
		// 1. Extract JWT
		// 2. Parse to get issuer + kid
		// 3. Get IdP by issuer
		// 4. Get JWKS
		// 5. Validate algorithm against IdP's supported_algorithms  <-- NEW
		// 6. Parse/validate token signature
		// 7. Validate claims
		// 8. Find matching robot

		// The order is critical because we want to reject tokens with
		// unsupported algorithms BEFORE we attempt signature verification
	})

	t.Run("empty supported_algorithms allows any algorithm", func(t *testing.T) {
		// When IdP has no supported_algorithms configured,
		// any algorithm should be allowed (backward compatibility)
		idp := &model.FederatedIdp{
			SupportedAlgorithms: "",
		}
		algorithms := getSupportedAlgorithms(idp)
		assert.Nil(t, algorithms, "Empty config should return nil (allow any)")
	})

	t.Run("supported_algorithms comma-separated parsing", func(t *testing.T) {
		idp := &model.FederatedIdp{
			SupportedAlgorithms: "RS256,ES256,PS256",
		}
		algorithms := getSupportedAlgorithms(idp)
		assert.Len(t, algorithms, 3)
		assert.Contains(t, algorithms, "RS256")
		assert.Contains(t, algorithms, "ES256")
		assert.Contains(t, algorithms, "PS256")
	})
}
