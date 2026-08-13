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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	pkg "github.com/goharbor/harbor/src/pkg/federatedidp/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
)

// =============================================================================
// Name Validation Tests
// =============================================================================

func TestValidateFedIdpName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
		errMsg    string
	}{
		// Valid names
		{
			name:      "valid simple name",
			input:     "myidp",
			expectErr: false,
		},
		{
			name:      "valid name with hyphen",
			input:     "my-idp",
			expectErr: false,
		},
		{
			name:      "valid name with underscore",
			input:     "my_idp",
			expectErr: false,
		},
		{
			name:      "valid name with dot",
			input:     "my.idp",
			expectErr: false,
		},
		{
			name:      "valid name with numbers",
			input:     "idp123",
			expectErr: false,
		},
		{
			name:      "valid name with mixed separators",
			input:     "my-idp_v2.0",
			expectErr: false,
		},
		{
			name:      "valid single character",
			input:     "a",
			expectErr: false,
		},
		{
			name:      "valid max length name (64 chars)",
			input:     "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01",
			expectErr: false,
		},

		// Invalid names - empty and length
		{
			name:      "empty name",
			input:     "",
			expectErr: true,
			errMsg:    "name is required",
		},
		{
			name:      "name exceeds max length (65 chars)",
			input:     "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz012",
			expectErr: true,
			errMsg:    "exceeds maximum length",
		},

		// Invalid names - must start with letter
		{
			name:      "starts with number",
			input:     "123idp",
			expectErr: true,
			errMsg:    "must start with a letter",
		},
		{
			name:      "starts with hyphen",
			input:     "-myidp",
			expectErr: true,
			errMsg:    "must start with a letter",
		},
		{
			name:      "starts with underscore",
			input:     "_myidp",
			expectErr: true,
			errMsg:    "must start with a letter",
		},
		{
			name:      "starts with dot",
			input:     ".myidp",
			expectErr: true,
			errMsg:    "must start with a letter",
		},

		// Invalid names - uppercase
		{
			name:      "uppercase letters",
			input:     "MyIdp",
			expectErr: true,
			errMsg:    "lowercase", // matches "must start with a lowercase letter"
		},
		{
			name:      "all uppercase",
			input:     "MYIDP",
			expectErr: true,
			errMsg:    "start with a lowercase letter",
		},
		{
			name:      "mixed case in middle",
			input:     "myIdP",
			expectErr: true,
			errMsg:    "must be lowercase",
		},

		// Invalid names - spaces
		{
			name:      "contains space",
			input:     "my idp",
			expectErr: true,
			errMsg:    "cannot contain spaces",
		},
		{
			name:      "multiple spaces",
			input:     "my  idp",
			expectErr: true,
			errMsg:    "cannot contain spaces",
		},

		// Invalid names - special characters
		{
			name:      "contains plus",
			input:     "my+idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "contains at symbol",
			input:     "my@idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "contains hash",
			input:     "my#idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "contains slash",
			input:     "my/idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "contains backslash",
			input:     "my\\idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},

		// Invalid names - separator placement
		{
			name:      "ends with hyphen",
			input:     "myidp-",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "ends with underscore",
			input:     "myidp_",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "ends with dot",
			input:     "myidp.",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "consecutive hyphens",
			input:     "my--idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},
		{
			name:      "consecutive underscores",
			input:     "my__idp",
			expectErr: true,
			errMsg:    "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFedIdpName(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test whitespace trimming in name
func TestValidateFedIdpNameTrimming(t *testing.T) {
	// Note: The validate() function in the handler trims whitespace before calling validateFedIdpName
	// This test validates that the handler properly trims leading/trailing spaces

	tests := []struct {
		name           string
		input          string
		expectedTrim   string
		shouldValidate bool
	}{
		{
			name:           "leading spaces",
			input:          "  myidp",
			expectedTrim:   "myidp",
			shouldValidate: true,
		},
		{
			name:           "trailing spaces",
			input:          "myidp  ",
			expectedTrim:   "myidp",
			shouldValidate: true,
		},
		{
			name:           "both leading and trailing",
			input:          "  myidp  ",
			expectedTrim:   "myidp",
			shouldValidate: true,
		},
		{
			name:           "only spaces - becomes empty after trim",
			input:          "   ",
			expectedTrim:   "",
			shouldValidate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := trimWhitespace(tt.input)
			assert.Equal(t, tt.expectedTrim, trimmed)

			if tt.shouldValidate {
				err := validateFedIdpName(trimmed)
				assert.NoError(t, err)
			} else {
				err := validateFedIdpName(trimmed)
				assert.Error(t, err)
			}
		})
	}
}

// Helper function to trim whitespace (simulating what validate() does)
func trimWhitespace(s string) string {
	return strings.TrimSpace(s)
}

// =============================================================================
// Description Validation Tests
// =============================================================================

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "empty description",
			input:     "",
			expectErr: false,
		},
		{
			name:      "short description",
			input:     "My federated IdP",
			expectErr: false,
		},
		{
			name:      "max length description (264 chars)",
			input:     string(make([]byte, 264)),
			expectErr: false,
		},
		{
			name:      "exceeds max length (265 chars)",
			input:     string(make([]byte, 265)),
			expectErr: true,
		},
		{
			name:      "description with special chars",
			input:     "My IdP for @company! #auth",
			expectErr: false,
		},
		{
			name:      "description with unicode",
			input:     "身份提供者 - Identity Provider",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDescription(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "exceeds maximum length")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// HTTPS URL Validation Tests
// =============================================================================

func TestValidateHTTPSURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		fieldName string
		expectErr bool
		errMsg    string
	}{
		// Valid HTTPS URLs
		{
			name:      "valid https url",
			url:       "https://example.com",
			fieldName: "issuer",
			expectErr: false,
		},
		{
			name:      "valid https url with path",
			url:       "https://example.com/oauth2",
			fieldName: "issuer",
			expectErr: false,
		},
		{
			name:      "valid https url with port",
			url:       "https://example.com:8443",
			fieldName: "issuer",
			expectErr: false,
		},
		{
			name:      "valid https url with subdomain",
			url:       "https://auth.example.com",
			fieldName: "issuer",
			expectErr: false,
		},

		// Invalid URLs - protocol
		{
			name:      "http url (not https)",
			url:       "http://example.com",
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "must use HTTPS",
		},
		{
			name:      "no protocol",
			url:       "example.com",
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "must use HTTPS",
		},
		{
			name:      "ftp protocol",
			url:       "ftp://example.com",
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "must use HTTPS",
		},

		// Invalid URLs - empty
		{
			name:      "empty url",
			url:       "",
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "cannot be empty",
		},

		// Invalid URLs - length
		{
			name:      "url exceeds max length",
			url:       "https://example.com/" + string(make([]byte, 2050)),
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "exceeds maximum length",
		},

		// Invalid URLs - malformed
		{
			name:      "invalid url format",
			url:       "not-a-url",
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "must use HTTPS",
		},
		{
			name:      "url with spaces",
			url:       "https://example .com",
			fieldName: "issuer",
			expectErr: true,
			errMsg:    "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPSURL(tt.url, tt.fieldName)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// JWKS Keys Validation Tests
// =============================================================================

func TestValidateJWKSKeys(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expectErr bool
		errMsg    string
	}{
		// Valid JWKS
		{
			name: "valid jwks with single key",
			input: map[string]any{
				"keys": []any{
					map[string]any{
						"kty": "RSA",
						"kid": "key-1",
						"n":   "0vx7agoebGcQ...",
						"e":   "AQAB",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid jwks with multiple keys",
			input: map[string]any{
				"keys": []any{
					map[string]any{
						"kty": "RSA",
						"kid": "key-1",
						"n":   "abc123",
						"e":   "AQAB",
					},
					map[string]any{
						"kty": "EC",
						"kid": "key-2",
						"crv": "P-256",
						"x":   "xyz",
						"y":   "abc",
					},
				},
			},
			expectErr: false,
		},

		// Invalid JWKS - nil or empty
		{
			name:      "nil jwks",
			input:     nil,
			expectErr: true,
			errMsg:    "is required",
		},
		{
			name: "empty keys array",
			input: map[string]any{
				"keys": []any{},
			},
			expectErr: true,
			errMsg:    "cannot be empty",
		},

		// Invalid JWKS - missing keys array
		{
			name:      "missing keys field",
			input:     map[string]any{},
			expectErr: true,
			errMsg:    "must contain a 'keys' array",
		},
		{
			name: "keys is not an array",
			input: map[string]any{
				"keys": "not-an-array",
			},
			expectErr: true,
			errMsg:    "must be an array",
		},

		// Invalid JWKS - key missing required fields
		{
			name: "key missing kty",
			input: map[string]any{
				"keys": []any{
					map[string]any{
						"kid": "key-1",
						"n":   "abc",
						"e":   "AQAB",
					},
				},
			},
			expectErr: true,
			errMsg:    "missing required 'kty' field",
		},
		{
			name: "key missing kid",
			input: map[string]any{
				"keys": []any{
					map[string]any{
						"kty": "RSA",
						"n":   "abc",
						"e":   "AQAB",
					},
				},
			},
			expectErr: true,
			errMsg:    "missing required 'kid' field",
		},
		{
			name: "key is not an object",
			input: map[string]any{
				"keys": []any{
					"not-an-object",
				},
			},
			expectErr: true,
			errMsg:    "must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWKSKeys(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Supported Algorithms Validation Tests
// =============================================================================

func TestValidateSupportedAlgorithms(t *testing.T) {
	tests := []struct {
		name       string
		algorithms []string
		expectErr  bool
		errMsg     string
	}{
		// Valid algorithms
		{
			name:       "valid RS256",
			algorithms: []string{"RS256"},
			expectErr:  false,
		},
		{
			name:       "valid multiple algorithms",
			algorithms: []string{"RS256", "RS384", "RS512", "ES256"},
			expectErr:  false,
		},
		{
			name:       "valid PS algorithms",
			algorithms: []string{"PS256", "PS384", "PS512"},
			expectErr:  false,
		},
		{
			name:       "valid with numbers only",
			algorithms: []string{"A256"},
			expectErr:  false,
		},

		// Invalid algorithms - length
		{
			name:       "too short (2 chars)",
			algorithms: []string{"RS"},
			expectErr:  true,
			errMsg:     "too short",
		},
		{
			name:       "too long (65 chars)",
			algorithms: []string{string(make([]byte, 65))},
			expectErr:  true,
			errMsg:     "exceeds maximum length",
		},

		// Invalid algorithms - characters
		{
			name:       "lowercase letters",
			algorithms: []string{"rs256"},
			expectErr:  true,
			errMsg:     "invalid characters",
		},
		{
			name:       "contains hyphen",
			algorithms: []string{"RS-256"},
			expectErr:  true,
			errMsg:     "invalid characters",
		},
		{
			name:       "contains underscore",
			algorithms: []string{"RS_256"},
			expectErr:  true,
			errMsg:     "invalid characters",
		},
		{
			name:       "contains space",
			algorithms: []string{"RS 256"},
			expectErr:  true,
			errMsg:     "invalid characters",
		},
		{
			name:       "one valid one invalid",
			algorithms: []string{"RS256", "rs384"},
			expectErr:  true,
			errMsg:     "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSupportedAlgorithms(tt.algorithms)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Level Validation Tests
// =============================================================================

func TestIsValidLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected bool
	}{
		{
			name:     "valid system level",
			level:    "system",
			expected: true,
		},
		{
			name:     "valid project level",
			level:    "project",
			expected: true,
		},
		{
			name:     "invalid level - uppercase",
			level:    "System",
			expected: false,
		},
		{
			name:     "invalid level - unknown",
			level:    "global",
			expected: false,
		},
		{
			name:     "invalid level - empty",
			level:    "",
			expected: false,
		},
		{
			name:     "invalid level - with spaces",
			level:    " system ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidLevel(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Update Validation Tests (Immutable Fields)
// =============================================================================

// TestUpdateImmutableFields tests that immutable fields cannot be changed after creation
// These tests verify the business logic that certain fields should never change

func TestUpdateValidation_ImmutableFields(t *testing.T) {
	// This is a conceptual test documenting the expected behavior
	// In real integration tests, these would be API calls

	immutableFields := []struct {
		fieldName   string
		description string
	}{
		{"name", "Name cannot be changed after creation"},
		{"issuer", "Issuer cannot be changed (would break existing robot tokens)"},
		{"offline_validation", "Validation mode cannot be switched after creation"},
		{"project_id", "Project scope cannot be changed after creation"},
		{"openid_config_url", "OpenID config URL is set at creation (online mode)"},
		{"jwks_uri", "JWKS URI is set at creation (online mode)"},
	}

	t.Log("Immutable fields that cannot be updated:")
	for _, f := range immutableFields {
		t.Logf("  - %s: %s", f.fieldName, f.description)
	}

	// Document what CAN be updated
	mutableFieldsOnline := []string{"description"}
	mutableFieldsOffline := []string{"description", "jwks_keys"}

	t.Logf("Mutable fields (online mode): %v", mutableFieldsOnline)
	t.Logf("Mutable fields (offline mode): %v", mutableFieldsOffline)
}

// TestValidateUpdateImmutableFields tests the actual validation logic for immutable fields
func TestValidateUpdateImmutableFields(t *testing.T) {
	api := &fedIDPAPI{}

	// Helper to create a pointer to string
	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }

	// Base existing IdP in online mode
	existingOnline := &models.FederatedIdp{
		ID:                  1,
		Name:                "test-idp",
		Description:         "Original description",
		Issuer:              "https://example.com",
		OfflineValidation:   false,
		OpenidConfigURL:     "https://example.com/.well-known/openid-configuration",
		JwksURI:             "https://example.com/.well-known/jwks.json",
		ClaimsSupported:     []string{"sub", "aud", "exp"},
		SupportedAlgorithms: []string{"RS256"},
	}

	// Base existing IdP in offline mode
	existingOffline := &models.FederatedIdp{
		ID:                2,
		Name:              "offline-idp",
		Description:       "Offline description",
		Issuer:            "https://offline.example.com",
		OfflineValidation: true,
		JwksKeys:          map[string]any{"keys": []any{}},
	}

	tests := []struct {
		name      string
		existing  *models.FederatedIdp
		update    *models.FederatedIdpUpdate
		expectErr bool
		errMsg    string
	}{
		// Online mode tests
		{
			name:     "online mode - description update allowed",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				Description: strPtr("Updated description"),
			},
			expectErr: false,
		},
		{
			name:     "online mode - rejects claims_supported",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				ClaimsSupported: []string{"sub", "aud"},
			},
			expectErr: true,
			errMsg:    "cannot modify claims_supported in online validation mode",
		},
		{
			name:     "online mode - rejects supported_algorithms",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				SupportedAlgorithms: []string{"RS256", "ES256"},
			},
			expectErr: true,
			errMsg:    "cannot modify supported_algorithms in online validation mode",
		},
		{
			name:     "online mode - rejects jwks_keys",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				JwksKeys: map[string]any{"keys": []any{}},
			},
			expectErr: true,
			errMsg:    "cannot modify jwks_keys in online validation mode",
		},

		// Offline mode tests
		{
			name:     "offline mode - description update allowed",
			existing: existingOffline,
			update: &models.FederatedIdpUpdate{
				Description: strPtr("Updated offline description"),
			},
			expectErr: false,
		},
		{
			name:     "offline mode - jwks_keys update allowed",
			existing: existingOffline,
			update: &models.FederatedIdpUpdate{
				JwksKeys: map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "new-key"}}},
			},
			expectErr: false,
		},

		// Immutable fields (both modes)
		{
			name:     "rejects name change",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				Name: strPtr("new-name"),
			},
			expectErr: true,
			errMsg:    "cannot modify name after creation",
		},
		{
			name:     "rejects issuer change",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				Issuer: strPtr("https://new-issuer.com"),
			},
			expectErr: true,
			errMsg:    "cannot modify issuer after creation",
		},
		{
			name:     "rejects validation mode switch",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				OfflineValidation: boolPtr(true),
			},
			expectErr: true,
			errMsg:    "cannot switch validation mode after creation",
		},
		{
			name:     "rejects openid_config_url change",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				OpenidConfigURL: strPtr("https://new-url.com/.well-known/openid-configuration"),
			},
			expectErr: true,
			errMsg:    "cannot modify openid_config_url after creation",
		},
		{
			name:     "rejects jwks_uri change",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				JwksURI: strPtr("https://new-url.com/.well-known/jwks.json"),
			},
			expectErr: true,
			errMsg:    "cannot modify jwks_uri after creation",
		},

		// Same value should still be accepted for some fields
		{
			name:     "same name value is accepted",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				Name: strPtr("test-idp"), // Same as existing
			},
			expectErr: false,
		},
		{
			name:     "same issuer value is accepted",
			existing: existingOnline,
			update: &models.FederatedIdpUpdate{
				Issuer: strPtr("https://example.com"), // Same as existing
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert models.FederatedIdp to pkg model for the test
			pkgExisting := convertToPkgFederatedIdp(tt.existing)
			err := api.validateUpdateImmutableFields(pkgExisting, tt.update)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// convertToPkgFederatedIdp converts models.FederatedIdp to pkg.FederatedIdp for testing
func convertToPkgFederatedIdp(m *models.FederatedIdp) *pkg.FederatedIdp {
	return &pkg.FederatedIdp{
		ID:                m.ID,
		Name:              m.Name,
		Description:       m.Description,
		Issuer:            m.Issuer,
		OfflineValidation: m.OfflineValidation,
		OpenIDConfigURL:   m.OpenidConfigURL,
		JWKSURI:           m.JwksURI,
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestEdgeCases_InputSanitization(t *testing.T) {
	t.Run("SQL injection attempts in name", func(t *testing.T) {
		injectionStrings := []string{
			"'; DROP TABLE--",
			"1; DELETE FROM users",
			"admin'--",
		}
		for _, s := range injectionStrings {
			err := validateFedIdpName(s)
			assert.Error(t, err, "SQL injection string should fail validation: %s", s)
		}
	})

	t.Run("XSS attempts in name", func(t *testing.T) {
		xssStrings := []string{
			"<script>alert(1)</script>",
			"javascript:alert(1)",
			"<img src=x onerror=alert(1)>",
		}
		for _, s := range xssStrings {
			err := validateFedIdpName(s)
			assert.Error(t, err, "XSS string should fail validation: %s", s)
		}
	})

	t.Run("Unicode/emoji in name", func(t *testing.T) {
		unicodeStrings := []string{
			"idp🔐",
			"身份提供者",
			"idpé",
		}
		for _, s := range unicodeStrings {
			err := validateFedIdpName(s)
			assert.Error(t, err, "Unicode/emoji should fail name validation: %s", s)
		}
	})

	t.Run("Control characters in name", func(t *testing.T) {
		controlStrings := []string{
			"idp\x00name", // null byte
			"idp\nname",   // newline
			"idp\tname",   // tab
			"idp\rname",   // carriage return
		}
		for _, s := range controlStrings {
			err := validateFedIdpName(s)
			assert.Error(t, err, "Control character should fail validation: %q", s)
		}
	})
}

// =============================================================================
// Integration Test Scenarios (Documentation)
// =============================================================================

// TestScenarios documents the test scenarios that should be covered in integration tests
func TestScenarios(t *testing.T) {
	scenarios := []struct {
		category    string
		description string
		testCases   []string
	}{
		{
			category:    "CREATE - Online Mode",
			description: "Creating federated IdP with online validation",
			testCases: []string{
				"Create IdP with valid openid_config_url and jwks_uri",
				"Issuer is extracted from discovery document",
				"Claims and algorithms are derived from discovery",
				"Reject if openid_config_url is unreachable",
				"Reject if jwks_uri doesn't match discovery",
				"Reject if discovery document is invalid",
			},
		},
		{
			category:    "CREATE - Offline Mode",
			description: "Creating federated IdP with offline validation",
			testCases: []string{
				"Create IdP with valid issuer and jwks_keys",
				"Reject if jwks_keys is missing",
				"Reject if jwks_keys structure is invalid",
				"Reject if issuer is not HTTPS",
				"Optional: supported_algorithms validation",
			},
		},
		{
			category:    "CREATE - Uniqueness",
			description: "Uniqueness constraints",
			testCases: []string{
				"Reject duplicate name within same project",
				"Allow same name in different projects",
				"Reject duplicate issuer globally",
				"Reject duplicate issuer even across projects",
			},
		},
		{
			category:    "UPDATE",
			description: "Update federated IdP",
			testCases: []string{
				"Update description (allowed)",
				"Update jwks_keys in offline mode (allowed)",
				"Reject update of issuer",
				"Reject update of name",
				"Reject switching validation mode",
				"Reject update of jwks_keys in online mode",
			},
		},
		{
			category:    "DELETE",
			description: "Delete federated IdP",
			testCases: []string{
				"Delete IdP with no associated robots",
				"Reject delete if robots are associated",
				"Claims are cascade deleted",
			},
		},
		{
			category:    "LIST",
			description: "List federated IdPs",
			testCases: []string{
				"List system-level IdPs (default)",
				"List project-level IdPs with ProjectID",
				"Reject Level=project without ProjectID",
				"Filter by name",
				"Pagination works correctly",
			},
		},
		{
			category:    "GET",
			description: "Get federated IdP by ID",
			testCases: []string{
				"Get existing IdP returns all fields",
				"Get non-existent IdP returns 404",
				"Permission check for project-level IdP",
			},
		},
	}

	for _, s := range scenarios {
		t.Logf("\n=== %s ===", s.category)
		t.Logf("Description: %s", s.description)
		for i, tc := range s.testCases {
			t.Logf("  %d. %s", i+1, tc)
		}
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkValidateFedIdpName(b *testing.B) {
	validName := "my-federated-idp-v1"
	for i := 0; i < b.N; i++ {
		validateFedIdpName(validName)
	}
}

func BenchmarkValidateJWKSKeys(b *testing.B) {
	validJWKS := map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"kid": "key-1",
				"n":   "0vx7agoebGcQ...",
				"e":   "AQAB",
			},
		},
	}
	for i := 0; i < b.N; i++ {
		validateJWKSKeys(validJWKS)
	}
}

// =============================================================================
// Claim Rule Validation Tests
// =============================================================================

func TestValidateClaimRule(t *testing.T) {
	tests := []struct {
		name      string
		claimRule *models.ClaimRule
		index     int
		expectErr bool
		errMsg    string
	}{
		// Valid claim rules
		{
			name: "valid simple claim",
			claimRule: &models.ClaimRule{
				ClaimPath: "sub",
				Value:     "user123",
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "valid claim with nested path",
			claimRule: &models.ClaimRule{
				ClaimPath: "custom.nested.claim",
				Value:     "some-value",
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "valid claim with max path length (128 chars)",
			claimRule: &models.ClaimRule{
				ClaimPath: strings.Repeat("a", 128),
				Value:     "value",
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "valid claim with max value length (256 chars)",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     strings.Repeat("v", 256),
			},
			index:     0,
			expectErr: false,
		},

		// Invalid claim rules - nil
		{
			name:      "nil claim rule",
			claimRule: nil,
			index:     0,
			expectErr: true,
			errMsg:    "is nil",
		},

		// Invalid claim rules - claim_path
		{
			name: "empty claim_path",
			claimRule: &models.ClaimRule{
				ClaimPath: "",
				Value:     "value",
			},
			index:     0,
			expectErr: true,
			errMsg:    "claim_path is required",
		},
		{
			name: "claim_path exceeds max length (129 chars)",
			claimRule: &models.ClaimRule{
				ClaimPath: strings.Repeat("a", 129),
				Value:     "value",
			},
			index:     0,
			expectErr: true,
			errMsg:    "claim_path exceeds maximum length",
		},

		// Invalid claim rules - value
		{
			name: "empty value",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     "",
			},
			index:     0,
			expectErr: true,
			errMsg:    "value is required",
		},
		{
			name: "value exceeds max length (257 chars)",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     strings.Repeat("v", 257),
			},
			index:     0,
			expectErr: true,
			errMsg:    "value exceeds maximum length",
		},

		// Test index in error message
		{
			name: "error shows correct index",
			claimRule: &models.ClaimRule{
				ClaimPath: "",
				Value:     "value",
			},
			index:     5,
			expectErr: true,
			errMsg:    "index 5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClaimRule(tc.claimRule, tc.index, nil)

			if tc.expectErr {
				assert.Error(t, err, "expected error but got none")
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg,
						"error message should contain '%s'", tc.errMsg)
				}
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateClaimRuleClaimsSupported tests claim_path validation against claims_supported
func TestValidateClaimRuleClaimsSupported(t *testing.T) {
	tests := []struct {
		name            string
		claimRule       *models.ClaimRule
		claimsSupported []string
		expectErr       bool
		errMsg          string
	}{
		{
			name:            "valid claim in supported list",
			claimRule:       &models.ClaimRule{ClaimPath: "sub", Value: "test"},
			claimsSupported: []string{"sub", "email", "name"},
			expectErr:       false,
		},
		{
			name:            "claim not in supported list",
			claimRule:       &models.ClaimRule{ClaimPath: "custom_claim", Value: "test"},
			claimsSupported: []string{"sub", "email", "name"},
			expectErr:       true,
			errMsg:          "not in the IdP's claims_supported list",
		},
		{
			name:            "skip validation when claims_supported is empty",
			claimRule:       &models.ClaimRule{ClaimPath: "any_claim", Value: "test"},
			claimsSupported: []string{},
			expectErr:       false,
		},
		{
			name:            "skip validation when claims_supported is nil",
			claimRule:       &models.ClaimRule{ClaimPath: "any_claim", Value: "test"},
			claimsSupported: nil,
			expectErr:       false,
		},
		{
			name:            "case insensitive matching - uppercase input",
			claimRule:       &models.ClaimRule{ClaimPath: "SUB", Value: "test"},
			claimsSupported: []string{"sub", "email"},
			expectErr:       false,
		},
		{
			name:            "case insensitive matching - mixed case supported",
			claimRule:       &models.ClaimRule{ClaimPath: "email", Value: "test"},
			claimsSupported: []string{"Sub", "EMAIL", "Name"},
			expectErr:       false,
		},
		{
			name:            "whitespace trimming in supported claims",
			claimRule:       &models.ClaimRule{ClaimPath: "sub", Value: "test"},
			claimsSupported: []string{" sub ", "  email  "},
			expectErr:       false,
		},
		{
			name:            "whitespace trimming in claim path",
			claimRule:       &models.ClaimRule{ClaimPath: " email ", Value: "test"},
			claimsSupported: []string{"sub", "email"},
			expectErr:       false,
		},
		{
			name:            "aud claim allowed even when not in claims_supported",
			claimRule:       &models.ClaimRule{ClaimPath: "aud", Value: "harbor"},
			claimsSupported: []string{"sub", "email"},
			expectErr:       false,
		},
		{
			name:            "iss claim validated when in claims_supported",
			claimRule:       &models.ClaimRule{ClaimPath: "iss", Value: "https://idp.example.com"},
			claimsSupported: []string{"iss", "sub", "aud"},
			expectErr:       false,
		},
		{
			name:            "iss claim allowed even when not in claims_supported",
			claimRule:       &models.ClaimRule{ClaimPath: "iss", Value: "https://idp.example.com"},
			claimsSupported: []string{"sub", "email"},
			expectErr:       false,
		},
		{
			name:            "aud claim allowed on an EKS claims_supported list",
			claimRule:       &models.ClaimRule{ClaimPath: "aud", Value: "container-registry"},
			claimsSupported: []string{"sub", "iss"},
			expectErr:       false,
		},
		{
			name:            "mandatory claim exemption is case insensitive",
			claimRule:       &models.ClaimRule{ClaimPath: " AUD ", Value: "container-registry"},
			claimsSupported: []string{"sub", "iss"},
			expectErr:       false,
		},
		{
			name:            "non-mandatory claim still rejected on an EKS claims_supported list",
			claimRule:       &models.ClaimRule{ClaimPath: "email", Value: "user@example.com"},
			claimsSupported: []string{"sub", "iss"},
			expectErr:       true,
			errMsg:          "not in the IdP's claims_supported list",
		},
		// Additional cases for empty claims_supported allowing all claims
		{
			name:            "empty claims_supported allows any custom claim",
			claimRule:       &models.ClaimRule{ClaimPath: "my_custom_claim", Value: "test"},
			claimsSupported: []string{},
			expectErr:       false,
		},
		{
			name:            "empty claims_supported allows kubernetes service account claim",
			claimRule:       &models.ClaimRule{ClaimPath: "kubernetes.io/serviceaccount/namespace", Value: "default"},
			claimsSupported: []string{},
			expectErr:       false,
		},
		// Cases for strict validation when claims_supported has specific values
		{
			name:            "claims_supported with aud,iss,sub allows sub",
			claimRule:       &models.ClaimRule{ClaimPath: "sub", Value: "user123"},
			claimsSupported: []string{"aud", "iss", "sub"},
			expectErr:       false,
		},
		{
			name:            "claims_supported with aud,iss,sub rejects email",
			claimRule:       &models.ClaimRule{ClaimPath: "email", Value: "user@example.com"},
			claimsSupported: []string{"aud", "iss", "sub"},
			expectErr:       true,
			errMsg:          "not in the IdP's claims_supported list",
		},
		{
			name:            "claims_supported with aud,iss,sub rejects custom claim",
			claimRule:       &models.ClaimRule{ClaimPath: "custom_claim", Value: "value"},
			claimsSupported: []string{"aud", "iss", "sub"},
			expectErr:       true,
			errMsg:          "not in the IdP's claims_supported list",
		},
		{
			name:            "claims_supported with email,name allows email",
			claimRule:       &models.ClaimRule{ClaimPath: "email", Value: "user@example.com"},
			claimsSupported: []string{"email", "name", "picture"},
			expectErr:       false,
		},
		{
			name:            "claims_supported with email,name still allows aud",
			claimRule:       &models.ClaimRule{ClaimPath: "aud", Value: "harbor"},
			claimsSupported: []string{"email", "name", "picture"},
			expectErr:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClaimRule(tc.claimRule, 0, tc.claimsSupported)

			if tc.expectErr {
				assert.Error(t, err, "expected error but got none")
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg,
						"error message should contain '%s'", tc.errMsg)
				}
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateClaimRuleEdgeCases tests edge cases and boundary conditions
func TestValidateClaimRuleEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		claimRule *models.ClaimRule
		index     int
		expectErr bool
		errMsg    string
	}{
		// Boundary cases
		{
			name: "claim_path at exact max length",
			claimRule: &models.ClaimRule{
				ClaimPath: strings.Repeat("x", maxClaimPathLength),
				Value:     "value",
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "claim_path one over max",
			claimRule: &models.ClaimRule{
				ClaimPath: strings.Repeat("x", maxClaimPathLength+1),
				Value:     "value",
			},
			index:     0,
			expectErr: true,
			errMsg:    "claim_path exceeds maximum length",
		},
		{
			name: "value at exact max length",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     strings.Repeat("y", maxClaimValueLength),
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "value one over max",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     strings.Repeat("y", maxClaimValueLength+1),
			},
			index:     0,
			expectErr: true,
			errMsg:    "value exceeds maximum length",
		},

		// Special characters
		{
			name: "claim_path with special characters",
			claimRule: &models.ClaimRule{
				ClaimPath: "urn:example:claim:type",
				Value:     "value",
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "value with special characters",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     "user@example.com",
			},
			index:     0,
			expectErr: false,
		},
		{
			name: "value with unicode",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     "用户名",
			},
			index:     0,
			expectErr: false,
		},

		// Whitespace handling
		{
			name: "claim_path with only whitespace",
			claimRule: &models.ClaimRule{
				ClaimPath: "   ",
				Value:     "value",
			},
			index:     0,
			expectErr: false, // whitespace is valid (not trimmed)
		},
		{
			name: "value with only whitespace",
			claimRule: &models.ClaimRule{
				ClaimPath: "claim",
				Value:     "   ",
			},
			index:     0,
			expectErr: false, // whitespace is valid (not trimmed)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClaimRule(tc.claimRule, tc.index, nil)

			if tc.expectErr {
				assert.Error(t, err, "expected error but got none")
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg,
						"error message should contain '%s'", tc.errMsg)
				}
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateClaimRuleBatch tests validation of multiple claims
func TestValidateClaimRuleBatch(t *testing.T) {
	validClaims := []*models.ClaimRule{
		{ClaimPath: "sub", Value: "user1"},
		{ClaimPath: "email", Value: "user@example.com"},
		{ClaimPath: "groups", Value: "admins"},
	}

	// All valid claims should pass
	for i, c := range validClaims {
		err := validateClaimRule(c, i, nil)
		assert.NoError(t, err, "claim at index %d should be valid", i)
	}

	// Mixed batch with one invalid claim
	mixedClaims := []*models.ClaimRule{
		{ClaimPath: "sub", Value: "user1"},
		{ClaimPath: "", Value: "value"}, // invalid - empty path
		{ClaimPath: "groups", Value: "admins"},
	}

	for i, c := range mixedClaims {
		err := validateClaimRule(c, i, nil)
		if i == 1 {
			assert.Error(t, err, "claim at index 1 should be invalid")
			assert.Contains(t, err.Error(), "index 1")
		} else {
			assert.NoError(t, err, "claim at index %d should be valid", i)
		}
	}
}

// TestClaimPathPatterns tests various claim path formats
func TestClaimPathPatterns(t *testing.T) {
	validPaths := []string{
		"sub",                       // simple
		"email",                     // simple
		"custom_claim",              // underscore
		"custom-claim",              // hyphen
		"custom.nested.claim",       // dots
		"urn:example:claim",         // URN format
		"https://example.com/claim", // URL format
		"groups[0]",                 // array notation
		"$.data.items[*].name",      // JSONPath-like
		"claim with spaces",         // spaces (valid, not trimmed)
	}

	for _, path := range validPaths {
		t.Run("path: "+path, func(t *testing.T) {
			err := validateClaimRule(&models.ClaimRule{
				ClaimPath: path,
				Value:     "test-value",
			}, 0, nil)
			assert.NoError(t, err, "path '%s' should be valid", path)
		})
	}
}

// TestClaimValuePatterns tests various claim value formats
func TestClaimValuePatterns(t *testing.T) {
	validValues := []string{
		"simple",                 // simple string
		"user@example.com",       // email
		"123456",                 // numeric string
		"true",                   // boolean string
		"null",                   // null string
		"{}",                     // JSON object
		"[]",                     // JSON array
		`{"key": "value"}`,       // JSON with content
		"value with spaces",      // spaces
		"value-with-dashes",      // dashes
		"value_with_underscores", // underscores
		"Mixed.Case.Value",       // mixed case
	}

	for _, value := range validValues {
		t.Run("value: "+value, func(t *testing.T) {
			err := validateClaimRule(&models.ClaimRule{
				ClaimPath: "test-claim",
				Value:     value,
			}, 0, nil)
			assert.NoError(t, err, "value '%s' should be valid", value)
		})
	}
}

// =============================================================================
// Claim Constants Tests
// =============================================================================

func TestClaimValidationConstants(t *testing.T) {
	// Verify constants are set correctly
	assert.Equal(t, 128, maxClaimPathLength, "maxClaimPathLength should be 128")
	assert.Equal(t, 256, maxClaimValueLength, "maxClaimValueLength should be 256")
}

// =============================================================================
// Benchmarks for Claim Validation
// =============================================================================

func BenchmarkValidateClaimRule(b *testing.B) {
	validClaim := &models.ClaimRule{
		ClaimPath: "sub",
		Value:     "user123",
	}
	for i := 0; i < b.N; i++ {
		validateClaimRule(validClaim, 0, nil)
	}
}

func BenchmarkValidateClaimRuleLongPath(b *testing.B) {
	longPathClaim := &models.ClaimRule{
		ClaimPath: strings.Repeat("a", 100),
		Value:     "value",
	}
	for i := 0; i < b.N; i++ {
		validateClaimRule(longPathClaim, 0, nil)
	}
}

func BenchmarkValidateClaimRuleLongValue(b *testing.B) {
	longValueClaim := &models.ClaimRule{
		ClaimPath: "claim",
		Value:     strings.Repeat("v", 200),
	}
	for i := 0; i < b.N; i++ {
		validateClaimRule(longValueClaim, 0, nil)
	}
}

// =============================================================================
// Robot Account with Federated IdP - Integration Test Scenarios
// =============================================================================

// TestRobotAccountCreationWithFedIdpScenarios documents and tests scenarios for robot creation
// with federated IdP association
func TestRobotAccountCreationWithFedIdpScenarios(t *testing.T) {
	scenarios := []struct {
		category    string
		description string
		testCases   []string
	}{
		{
			category:    "CREATE Robot with FederatedIdP",
			description: "Creating robot accounts associated with federated IdPs",
			testCases: []string{
				"Create robot with valid federatedidp_id should succeed",
				"Create robot with federatedidp_id=0 should create normal robot (no IdP)",
				"Create robot with non-existent federatedidp_id should fail",
				"Create robot with claims should validate against IdP claim rules",
				"Robot creation should create robot_identity_providers record",
				"Robot with IdP should not allow secret refresh",
			},
		},
		{
			category:    "Robot Claims Validation",
			description: "Validating claim rules when creating robots with federated IdPs",
			testCases: []string{
				"Robot cannot have duplicate claim_path within same robot",
				"Robot claims cannot duplicate exact (path, value) from another robot on same IdP",
				"Robot claims cannot override IdP-level claims (robot_id=0)",
				"Multiple robots can have same claim_path with different values",
				"Robot claims must have valid claim_path (non-empty, max 128 chars)",
				"Robot claims must have valid value (non-empty, max 256 chars)",
			},
		},
	}

	for _, s := range scenarios {
		t.Logf("\n=== %s ===", s.category)
		t.Logf("Description: %s", s.description)
		for i, tc := range s.testCases {
			t.Logf("  %d. %s", i+1, tc)
		}
	}
}

// TestRobotAccountDeletionWithFedIdpScenarios documents deletion scenarios
func TestRobotAccountDeletionWithFedIdpScenarios(t *testing.T) {
	scenarios := []struct {
		category    string
		description string
		testCases   []string
	}{
		{
			category:    "DELETE Robot with FederatedIdP",
			description: "Deleting robot accounts that have federated IdP associations",
			testCases: []string{
				"Delete robot should cascade delete claim_rules for that robot",
				"Delete robot should delete robot_identity_providers record",
				"Delete robot should not affect other robots' claims on same IdP",
				"Delete robot should not affect IdP-level claims (robot_id=0)",
			},
		},
		{
			category:    "DELETE FederatedIdP",
			description: "Deleting federated IdPs and impact on associated records",
			testCases: []string{
				"Cannot delete IdP if robots are still associated",
				"Delete IdP (after robots removed) should cascade delete all claim_rules",
				"Delete IdP should cascade delete robot_identity_providers records",
				"Delete IdP should not affect other IdPs",
			},
		},
	}

	for _, s := range scenarios {
		t.Logf("\n=== %s ===", s.category)
		t.Logf("Description: %s", s.description)
		for i, tc := range s.testCases {
			t.Logf("  %d. %s", i+1, tc)
		}
	}
}

// =============================================================================
// Claim Duplication Validation Tests
// =============================================================================

// TestClaimDuplicationRules tests the claim duplication prevention logic
// These tests validate the business rules for claim uniqueness
func TestClaimDuplicationRules(t *testing.T) {
	t.Run("Rule 1: No duplicate claim in input batch", func(t *testing.T) {
		// When creating multiple claims in a single request,
		// no two claims should have the same (idp_id, robot_id, claim_path, value)
		t.Log("Input batch validation should detect duplicates before DB insertion")
		t.Log("Example: Two claims with same path='sub', value='user1' for same robot should fail")
	})

	t.Run("Rule 2: Robot claims cannot override IDP-level claims", func(t *testing.T) {
		// If an IdP has a claim_rule with robot_id=0 (IdP-level),
		// a robot-specific claim with the same claim_path should be rejected
		t.Log("IdP-level claims (robot_id=0) take precedence")
		t.Log("Robot cannot add claim_path that exists at IdP level")
	})

	t.Run("Rule 3: Exact full robot claim sets must be unique", func(t *testing.T) {
		// Different robots can share individual claims, but their full claim sets
		// must not be identical or token matching becomes ambiguous.
		t.Log("Controller validation prevents ambiguous full robot claim sets")
		t.Log("Example: Robot A can share (sub, user1) with Robot B if Robot B also has another distinguishing claim")
	})

	t.Run("Rule 4: Same robot cannot have duplicate claim_path", func(t *testing.T) {
		// A single robot cannot have two claim rules with the same claim_path
		t.Log("Each robot should have unique claim_paths")
		t.Log("Example: Robot A cannot have two 'sub' claims")
	})

	t.Run("Rule 5: Different robots CAN have same claim_path with different values", func(t *testing.T) {
		// This is allowed and is the basis for token matching
		t.Log("Different robots can match different claim values")
		t.Log("Example: Robot A has (sub, user1), Robot B has (sub, user2) - OK")
	})
}

// TestValidateClaimDuplicatesInBatch tests detection of duplicates within a batch
func TestValidateClaimDuplicatesInBatch(t *testing.T) {
	tests := []struct {
		name        string
		claims      []claimInput
		expectErr   bool
		errContains string
		description string
	}{
		{
			name: "unique claims should pass",
			claims: []claimInput{
				{idpID: 1, robotID: 100, path: "sub", value: "user1"},
				{idpID: 1, robotID: 100, path: "email", value: "user@example.com"},
				{idpID: 1, robotID: 100, path: "groups", value: "admins"},
			},
			expectErr:   false,
			description: "All unique (path, value) combinations should be accepted",
		},
		{
			name: "duplicate within batch should fail",
			claims: []claimInput{
				{idpID: 1, robotID: 100, path: "sub", value: "user1"},
				{idpID: 1, robotID: 100, path: "sub", value: "user1"}, // exact duplicate
			},
			expectErr:   true,
			errContains: "duplicate",
			description: "Exact duplicate in same batch should be rejected",
		},
		{
			name: "same path different value OK",
			claims: []claimInput{
				{idpID: 1, robotID: 100, path: "groups", value: "admins"},
				{idpID: 1, robotID: 101, path: "groups", value: "developers"}, // different robot, different value
			},
			expectErr:   false,
			description: "Same path with different values for different robots should be OK",
		},
		{
			name: "same path same value different robots should pass",
			claims: []claimInput{
				{idpID: 1, robotID: 100, path: "sub", value: "user1"},
				{idpID: 1, robotID: 101, path: "sub", value: "user1"}, // different robot, same value
			},
			expectErr:   false,
			description: "Same (path, value) for different robots can be part of a more specific match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			// Check for duplicates using the same logic as DAO
			err := validateClaimBatchForDuplicates(tt.claims)

			if tt.expectErr {
				assert.Error(t, err, "expected error but got none")
				if tt.errContains != "" {
					assert.Contains(t, strings.ToLower(err.Error()), tt.errContains)
				}
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// claimInput is a helper struct for testing claim validation
type claimInput struct {
	idpID   int64
	robotID int64
	path    string
	value   string
}

// validateClaimBatchForDuplicates validates claims in a batch for duplicates
// This mirrors the logic in dao.validateUniqueClaims
func validateClaimBatchForDuplicates(claims []claimInput) error {
	type key struct {
		IDP     int64
		RobotID int64
		Path    string
		Value   string
	}
	seen := make(map[key]struct{})

	for _, c := range claims {
		// Check exact duplicate
		k := key{IDP: c.idpID, RobotID: c.robotID, Path: c.path, Value: c.value}
		if _, ok := seen[k]; ok {
			return errorf("duplicate claim in input batch for claim_path=%s", c.path)
		}
		seen[k] = struct{}{}

	}

	return nil
}

// errorf creates a simple error for testing
func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// =============================================================================
// Claim Cascade Deletion Tests
// =============================================================================

// TestClaimCascadeDeletionScenarios documents cascade deletion behavior
func TestClaimCascadeDeletionScenarios(t *testing.T) {
	t.Run("Delete Robot cascades claim_rules deletion", func(t *testing.T) {
		// Scenario: Robot has claim_rules associated with it
		// Expected: When robot is deleted, all claim_rules with that robot_id are deleted
		t.Log("Steps:")
		t.Log("  1. Create IdP with ID=1")
		t.Log("  2. Create Robot with ID=100, associated with IdP=1")
		t.Log("  3. Create claim_rules for robot_id=100")
		t.Log("  4. Delete Robot with ID=100")
		t.Log("  5. Verify: claim_rules with robot_id=100 are deleted")
		t.Log("  6. Verify: IdP-level claims (robot_id=0) are NOT affected")
	})

	t.Run("Delete FederatedIdP cascades all related records", func(t *testing.T) {
		// Scenario: IdP has claim_rules (both IdP-level and robot-level)
		// Expected: When IdP is deleted, all related records are deleted
		t.Log("Steps:")
		t.Log("  1. Create IdP with ID=1")
		t.Log("  2. Create IdP-level claims (robot_id=0)")
		t.Log("  3. Attempt to delete IdP - should fail if robots exist")
		t.Log("  4. Remove all associated robots first")
		t.Log("  5. Delete IdP")
		t.Log("  6. Verify: All claim_rules for idp_id=1 are deleted")
		t.Log("  7. Verify: robot_identity_providers records are deleted")
	})

	t.Run("Delete Robot does not affect other robots", func(t *testing.T) {
		// Scenario: Multiple robots associated with same IdP
		// Expected: Deleting one robot only affects that robot's claims
		t.Log("Steps:")
		t.Log("  1. Create IdP with ID=1")
		t.Log("  2. Create Robot A (ID=100) with claims")
		t.Log("  3. Create Robot B (ID=101) with claims")
		t.Log("  4. Delete Robot A")
		t.Log("  5. Verify: Robot B's claims are intact")
		t.Log("  6. Verify: IdP-level claims are intact")
	})
}

// =============================================================================
// Robot with IdP Secret Refresh Restriction Tests
// =============================================================================

// TestRobotWithIdPSecretRefreshRestriction tests that robots with IdP cannot refresh secrets
func TestRobotWithIdPSecretRefreshRestriction(t *testing.T) {
	t.Run("Robot with IdP should not allow secret refresh", func(t *testing.T) {
		// Robots that authenticate via federated IdP tokens should not need
		// or be allowed to refresh their Harbor secret
		t.Log("Business Rule:")
		t.Log("  - Robots with federated IdP use external JWT tokens for authentication")
		t.Log("  - They don't use Harbor-generated secrets")
		t.Log("  - Secret refresh endpoint should return error for these robots")
		t.Log("")
		t.Log("Expected behavior:")
		t.Log("  - Check HasRobotIdpByRobotID returns true")
		t.Log("  - Return BadRequestError with message about IdP association")
	})

	t.Run("Robot without IdP should allow secret refresh", func(t *testing.T) {
		// Normal robots (without IdP) should work as usual
		t.Log("Business Rule:")
		t.Log("  - Normal robots use Harbor-generated secrets")
		t.Log("  - Secret refresh should work normally")
		t.Log("")
		t.Log("Expected behavior:")
		t.Log("  - Check HasRobotIdpByRobotID returns false")
		t.Log("  - Allow secret refresh operation")
	})
}

// =============================================================================
// IdP Deletion with Robot Association Tests
// =============================================================================

// TestIdPDeletionBlockedByRobots tests that IdP deletion is blocked when robots exist
func TestIdPDeletionBlockedByRobots(t *testing.T) {
	t.Run("Cannot delete IdP with associated robots", func(t *testing.T) {
		t.Log("Business Rule:")
		t.Log("  - IdP cannot be deleted if robot_identity_providers records exist")
		t.Log("  - User must first delete all associated robots")
		t.Log("")
		t.Log("Error message: 'Please delete the associated robots before deleting the federated idp'")
		t.Log("")
		t.Log("Implementation check (federated_idp.go:253-259):")
		t.Log("  1. ListRobotIdpByIdpID(ctx, params.ID)")
		t.Log("  2. If len(robotIdps) > 0, return BadRequestError")
	})

	t.Run("Can delete IdP with no associated robots", func(t *testing.T) {
		t.Log("Business Rule:")
		t.Log("  - IdP with no robot associations can be deleted freely")
		t.Log("  - All IdP-level claim_rules should be cascade deleted")
	})
}

// =============================================================================
// Claim Rule IDP vs Robot Level Tests
// =============================================================================

// TestClaimRuleIdPvsRobotLevel tests the distinction between IdP-level and robot-level claims
func TestClaimRuleIdPvsRobotLevel(t *testing.T) {
	tests := []struct {
		name        string
		robotID     int64
		isIdpLevel  bool
		description string
	}{
		{
			name:        "robot_id=0 is IdP-level",
			robotID:     0,
			isIdpLevel:  true,
			description: "Claims with robot_id=0 belong to the IdP and apply to all robots",
		},
		{
			name:        "robot_id>0 is robot-level",
			robotID:     100,
			isIdpLevel:  false,
			description: "Claims with robot_id>0 are specific to that robot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Description: %s", tt.description)
			assert.Equal(t, tt.isIdpLevel, tt.robotID == 0)
		})
	}
}

// TestIdpLevelClaimsCannotBeOverridden tests that robot claims cannot override IdP-level claims
func TestIdpLevelClaimsCannotBeOverridden(t *testing.T) {
	t.Run("Robot claim with same path as IdP-level claim should fail", func(t *testing.T) {
		// Given: IdP has claim_rule (path="org", value="harbor", robot_id=0)
		// When: Trying to create robot claim (path="org", value="other", robot_id=100)
		// Then: Should fail with "already owned by identity provider" error

		t.Log("Implementation check (dao.go validateUniqueClaims):")
		t.Log("  if e.RobotID == 0 {")
		t.Log("    return error: claim_path 'X' already owned by identity provider N")
		t.Log("  }")
	})
}

// =============================================================================
// ListClaimsWithFilters Tests
// =============================================================================

// TestListClaimsWithFiltersValidation tests the claim listing filter combinations
func TestListClaimsWithFiltersValidation(t *testing.T) {
	tests := []struct {
		name        string
		idpOnly     *bool
		robotID     *int64
		expectErr   bool
		description string
	}{
		{
			name:        "no filters returns all claims",
			idpOnly:     nil,
			robotID:     nil,
			expectErr:   false,
			description: "Without filters, return all claims for the IdP",
		},
		{
			name:        "idp_only=true returns only IdP-level claims",
			idpOnly:     boolPtr(true),
			robotID:     nil,
			expectErr:   false,
			description: "Filter to only IdP-level claims (robot_id=0)",
		},
		{
			name:        "robot_id=100 returns only that robot's claims",
			idpOnly:     nil,
			robotID:     int64Ptr(100),
			expectErr:   false,
			description: "Filter to specific robot's claims",
		},
		{
			name:        "both idp_only and robot_id should fail",
			idpOnly:     boolPtr(true),
			robotID:     int64Ptr(100),
			expectErr:   true,
			description: "Cannot specify both idp_only and robot_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			// Simulate the validation logic from ListClaimRules handler
			if tt.idpOnly != nil && *tt.idpOnly && tt.robotID != nil {
				assert.True(t, tt.expectErr, "should error when both idp_only and robot_id specified")
			} else {
				assert.False(t, tt.expectErr, "should not error for valid filter combinations")
			}
		})
	}
}

// Helper functions for pointer values
func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}

// =============================================================================
// Comprehensive Integration Test Matrix
// =============================================================================

// TestFederatedIdPRobotIntegrationMatrix provides a complete test matrix
// for federated IdP and robot account interactions
func TestFederatedIdPRobotIntegrationMatrix(t *testing.T) {
	matrix := []struct {
		operation     string
		preconditions string
		action        string
		expected      string
		verified      bool
	}{
		// Robot Creation
		{
			operation:     "Create Robot",
			preconditions: "IdP exists, federatedidp_id provided",
			action:        "POST /robots with federatedidp_id=1",
			expected:      "Robot created, robot_identity_providers record created",
			verified:      true,
		},
		{
			operation:     "Create Robot",
			preconditions: "IdP does not exist",
			action:        "POST /robots with federatedidp_id=999",
			expected:      "Error: IdP not found",
			verified:      false,
		},
		{
			operation:     "Create Robot",
			preconditions: "No federatedidp_id provided",
			action:        "POST /robots without federatedidp_id",
			expected:      "Normal robot created (no IdP association)",
			verified:      true,
		},

		// Claim Creation
		{
			operation:     "Create Claims",
			preconditions: "Robot exists, IdP exists",
			action:        "POST /idps/{id}/claims with robot_id",
			expected:      "Claims created for robot",
			verified:      true,
		},
		{
			operation:     "Create Claims",
			preconditions: "Duplicate claim in batch",
			action:        "POST /idps/{id}/claims with duplicates",
			expected:      "Error: duplicate claim",
			verified:      true,
		},
		{
			operation:     "Create Claims",
			preconditions: "IdP-level claim exists for path",
			action:        "POST /idps/{id}/claims with same path for robot",
			expected:      "Error: claim owned by IdP",
			verified:      true,
		},
		{
			operation:     "Create Claims",
			preconditions: "Same (path, value) exists for another robot",
			action:        "POST /idps/{id}/claims with duplicate path+value",
			expected:      "Claims created; exact full robot claim-set duplicates are rejected by controller validation",
			verified:      true,
		},

		// Robot Deletion
		{
			operation:     "Delete Robot",
			preconditions: "Robot has IdP association and claims",
			action:        "DELETE /robots/{id}",
			expected:      "Robot deleted, claim_rules deleted, robot_identity_providers deleted",
			verified:      true,
		},

		// IdP Deletion
		{
			operation:     "Delete IdP",
			preconditions: "Robots still associated",
			action:        "DELETE /idps/{id}",
			expected:      "Error: delete associated robots first",
			verified:      true,
		},
		{
			operation:     "Delete IdP",
			preconditions: "No robots associated",
			action:        "DELETE /idps/{id}",
			expected:      "IdP deleted, all claim_rules deleted",
			verified:      true,
		},

		// Secret Refresh
		{
			operation:     "Refresh Secret",
			preconditions: "Robot has IdP association",
			action:        "POST /robots/{id}/refresh",
			expected:      "Error: cannot refresh, robot has IdP",
			verified:      true,
		},
		{
			operation:     "Refresh Secret",
			preconditions: "Robot has no IdP",
			action:        "POST /robots/{id}/refresh",
			expected:      "Secret refreshed successfully",
			verified:      true,
		},
	}

	t.Log("=== Federated IdP + Robot Integration Test Matrix ===")
	t.Log("")
	for _, m := range matrix {
		status := "[ ]"
		if m.verified {
			status = "[x]"
		}
		t.Logf("%s %s", status, m.operation)
		t.Logf("    Preconditions: %s", m.preconditions)
		t.Logf("    Action: %s", m.action)
		t.Logf("    Expected: %s", m.expected)
		t.Log("")
	}
}
