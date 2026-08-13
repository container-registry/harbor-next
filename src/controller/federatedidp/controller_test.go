// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package federatedidp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	pkgfederatedidp "github.com/goharbor/harbor/src/pkg/federatedidp"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
)

type claimManager struct {
	pkgfederatedidp.Manager
	count int64
}

func (m *claimManager) Count(context.Context, *q.Query) (int64, error) {
	return m.count, nil
}

func (m *claimManager) ListClaimsIdpOnly(context.Context, int64, string) ([]model.ClaimRule, error) {
	return nil, nil
}

func (m *claimManager) CreateClaims(context.Context, int64, []model.ClaimRule) error {
	return nil
}

func TestCreateClaimsAllowsEachIdentityClaimOnlyOncePerRequest(t *testing.T) {
	tests := []struct {
		name    string
		claims  []model.ClaimRule
		wantErr bool
	}{
		{
			name: "single issuer succeeds",
			claims: []model.ClaimRule{
				{ClaimPath: "iss", Value: "https://issuer.example.com"},
			},
		},
		{
			name: "one issuer and one audience succeed",
			claims: []model.ClaimRule{
				{ClaimPath: "iss", Value: "https://issuer.example.com"},
				{ClaimPath: "aud", Value: "harbor"},
			},
		},
		{
			name: "multiple issuers fail",
			claims: []model.ClaimRule{
				{ClaimPath: "iss", Value: "https://issuer-a.example.com"},
				{ClaimPath: "iss", Value: "https://issuer-b.example.com"},
			},
			wantErr: true,
		},
		{
			name: "multiple audiences fail",
			claims: []model.ClaimRule{
				{ClaimPath: "aud", Value: "harbor-a"},
				{ClaimPath: "aud", Value: "harbor-b"},
			},
			wantErr: true,
		},
	}

	controller := &controller{fidpMgr: &claimManager{}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := controller.CreateClaims(t.Context(), 1, tt.claims)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, errors.IsErr(err, errors.ConflictCode))
		})
	}
}

func TestIdentityProviderDisableGuard(t *testing.T) {
	guard := identityProviderDisableGuard{counter: &claimManager{count: 1}}
	require.ErrorContains(t, guard.Validate(t.Context()), "cannot be disabled")

	guard = identityProviderDisableGuard{counter: &claimManager{}}
	require.NoError(t, guard.Validate(t.Context()))
}
