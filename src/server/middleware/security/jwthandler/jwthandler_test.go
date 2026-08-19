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
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestJWT creates a signed JWT token with the specified algorithm for testing
func createTestJWT(t *testing.T, alg jwa.SignatureAlgorithm) (string, jwk.Set) {
	t.Helper()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create JWK from private key
	jwkKey, err := jwk.Import(privateKey)
	require.NoError(t, err)

	// Set key ID and algorithm
	err = jwkKey.Set(jwk.KeyIDKey, "test-key-id")
	require.NoError(t, err)
	err = jwkKey.Set(jwk.AlgorithmKey, alg)
	require.NoError(t, err)

	// Create public key JWK for verification
	pubKey, err := jwk.Import(privateKey.Public())
	require.NoError(t, err)
	err = pubKey.Set(jwk.KeyIDKey, "test-key-id")
	require.NoError(t, err)
	err = pubKey.Set(jwk.AlgorithmKey, alg)
	require.NoError(t, err)

	// Create JWKS
	jwkSet := jwk.NewSet()
	err = jwkSet.AddKey(pubKey)
	require.NoError(t, err)

	// Create JWT token
	token, err := jwt.NewBuilder().
		Issuer("https://test-issuer.example.com").
		Subject("test-subject").
		Build()
	require.NoError(t, err)

	// Sign the token
	signed, err := jwt.Sign(token, jwt.WithKey(alg, jwkKey))
	require.NoError(t, err)

	return string(signed), jwkSet
}

func TestValidateAlgorithm(t *testing.T) {
	// Create test tokens with different algorithms
	rs256Token, _ := createTestJWT(t, jwa.RS256())
	rs384Token, _ := createTestJWT(t, jwa.RS384())

	tests := []struct {
		name       string
		token      string
		algorithms []string
		wantAlg    string
		wantErr    bool
	}{
		{
			name:       "valid algorithm - RS256 in allowed list",
			token:      rs256Token,
			algorithms: []string{"RS256", "RS384", "RS512"},
			wantAlg:    "RS256",
			wantErr:    false,
		},
		{
			name:       "valid algorithm - RS384 in allowed list",
			token:      rs384Token,
			algorithms: []string{"RS256", "RS384", "RS512"},
			wantAlg:    "RS384",
			wantErr:    false,
		},
		{
			name:       "algorithm not in allowed list",
			token:      rs256Token,
			algorithms: []string{"ES256", "ES384"},
			wantErr:    true,
		},
		{
			name:       "empty algorithms list - allow any",
			token:      rs256Token,
			algorithms: []string{},
			wantAlg:    "RS256",
			wantErr:    false,
		},
		{
			name:       "nil algorithms list - allow any",
			token:      rs256Token,
			algorithms: nil,
			wantAlg:    "RS256",
			wantErr:    false,
		},
		{
			name:       "case insensitive matching - lowercase in list",
			token:      rs256Token,
			algorithms: []string{"rs256"},
			wantAlg:    "RS256",
			wantErr:    false,
		},
		{
			name:       "case insensitive matching - mixed case in list",
			token:      rs256Token,
			algorithms: []string{"Rs256"},
			wantAlg:    "RS256",
			wantErr:    false,
		},
		{
			name:       "invalid token format - not a JWT",
			token:      "not-a-jwt",
			algorithms: []string{"RS256"},
			wantErr:    true,
		},
		{
			name:       "invalid token format - empty string",
			token:      "",
			algorithms: []string{"RS256"},
			wantErr:    true,
		},
		{
			name:       "single algorithm in list - matching",
			token:      rs256Token,
			algorithms: []string{"RS256"},
			wantAlg:    "RS256",
			wantErr:    false,
		},
		{
			name:       "single algorithm in list - not matching",
			token:      rs256Token,
			algorithms: []string{"RS384"},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alg, err := ValidateAlgorithm(tt.token, tt.algorithms)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, "invalid token", err.Error(), "error should be generic for security")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantAlg, alg)
			}
		})
	}
}

func TestValidateAlgorithm_GenericErrorMessage(t *testing.T) {
	// Verify that error messages don't leak information about allowed algorithms
	rs256Token, _ := createTestJWT(t, jwa.RS256())

	// Algorithm not in list should return generic error
	_, err := ValidateAlgorithm(rs256Token, []string{"ES256", "ES384"})
	assert.Error(t, err)
	assert.Equal(t, "invalid token", err.Error())
	assert.NotContains(t, err.Error(), "RS256")
	assert.NotContains(t, err.Error(), "ES256")
	assert.NotContains(t, err.Error(), "algorithm")
}

func TestParseToken(t *testing.T) {
	// Create a valid test token
	token, jwkSet := createTestJWT(t, jwa.RS256())

	// Parse should succeed
	parsedToken, err := ParseToken(token, jwkSet)
	assert.NoError(t, err)
	assert.NotNil(t, parsedToken)

	// Verify claims
	issuer, ok := parsedToken.Issuer()
	assert.True(t, ok)
	assert.Equal(t, "https://test-issuer.example.com", issuer)

	subject, ok := parsedToken.Subject()
	assert.True(t, ok)
	assert.Equal(t, "test-subject", subject)
}

func TestParseToken_InvalidSignature(t *testing.T) {
	// Create a token with one key
	token, _ := createTestJWT(t, jwa.RS256())

	// Create a different JWKS (different key)
	differentToken, differentJwkSet := createTestJWT(t, jwa.RS256())
	_ = differentToken

	// Parsing with wrong JWKS should fail
	_, err := ParseToken(token, differentJwkSet)
	assert.Error(t, err)
}
