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

package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkg "github.com/goharbor/harbor/src/pkg/federatedidp/model"
)

func TestFederatedIdpToSwaggerOptionalCSVFields(t *testing.T) {
	tests := []struct {
		name                string
		supportedAlgorithms string
		claimsSupported     string
		wantAlgorithms      []string
		wantClaims          []string
	}{
		{
			name:           "empty optional CSV fields serialize as empty arrays",
			wantAlgorithms: []string{},
			wantClaims:     []string{},
		},
		{
			name:                "whitespace-only optional CSV fields serialize as empty arrays",
			supportedAlgorithms: "  ",
			claimsSupported:     "  ",
			wantAlgorithms:      []string{},
			wantClaims:          []string{},
		},
		{
			name:                "CSV fields are trimmed and empty entries are ignored",
			supportedAlgorithms: "RS256, ES256,,",
			claimsSupported:     "iss, aud, sub,,",
			wantAlgorithms:      []string{"RS256", "ES256"},
			wantClaims:          []string{"iss", "aud", "sub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idp := NewFederatedIdp(&pkg.FederatedIdp{
				Name:                "kubernetes",
				Issuer:              "https://kubernetes.example.test",
				SupportedAlgorithms: tt.supportedAlgorithms,
				ClaimsSupported:     tt.claimsSupported,
			})

			got := idp.ToSwagger()

			require.Equal(t, tt.wantAlgorithms, got.SupportedAlgorithms)
			require.Equal(t, tt.wantClaims, got.ClaimsSupported)
		})
	}
}
