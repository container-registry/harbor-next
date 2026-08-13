//go:build db

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

package dao

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
	robotModel "github.com/goharbor/harbor/src/pkg/robot/model"
	htesting "github.com/goharbor/harbor/src/testing"
)

type DaoTestSuite struct {
	htesting.Suite
	dao DAO

	idpID1 int64
	idpID2 int64
}

func (suite *DaoTestSuite) SetupSuite() {
	suite.Suite.SetupSuite()
	suite.dao = New()
	suite.Suite.ClearTables = []string{"identity_providers", "claim_rules", "robot_identity_providers"}
	suite.createIdps()
}

// createTestRobot creates a test robot and returns its ID
func (suite *DaoTestSuite) createTestRobot(name string) int64 {
	ormer, err := orm.FromContext(orm.Context())
	suite.Nil(err)

	robot := &robotModel.Robot{
		Name:        name,
		Description: "test robot for federatedidp tests",
		Secret:      "test-secret",
		Salt:        "test-salt",
		ProjectID:   0,
		Visible:     true,
	}
	id, err := ormer.Insert(robot)
	suite.Nil(err)
	return id
}

// deleteTestRobot deletes a test robot by ID
func (suite *DaoTestSuite) deleteTestRobot(robotID int64) {
	ormer, err := orm.FromContext(orm.Context())
	suite.Nil(err)
	_, _ = ormer.Delete(&robotModel.Robot{ID: robotID})
}

func (suite *DaoTestSuite) createIdps() {
	var err error
	suite.idpID1, err = suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:              "test-idp-1",
		Description:       "test idp 1 description",
		Issuer:            "https://test-issuer-1.example.com",
		JWKSURI:           "https://test-issuer-1.example.com/.well-known/jwks.json",
		OfflineValidation: false,
		ProjectID:         0,
	})
	suite.Nil(err)

	suite.idpID2, err = suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:              "test-idp-2",
		Description:       "test idp 2 description",
		Issuer:            "https://test-issuer-2.example.com",
		OfflineValidation: true,
		JWKSKeys:          `{"keys":[{"kty":"RSA","kid":"test-key"}]}`,
		ProjectID:         1,
	})
	suite.Nil(err)
}

func TestDaoTestSuite(t *testing.T) {
	suite.Run(t, &DaoTestSuite{})
}

func TestFlattenClaimValues(t *testing.T) {
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   map[string][]string
	}{
		{
			name: "scalar claims stay at top level",
			claims: jwt.MapClaims{
				"iss": "https://issuer.example.com",
				"sub": "system:serviceaccount:default:builder",
			},
			want: map[string][]string{
				"iss": {"https://issuer.example.com"},
				"sub": {"system:serviceaccount:default:builder"},
			},
		},
		{
			name: "nested Kubernetes claims flatten to dot paths",
			claims: jwt.MapClaims{
				"kubernetes.io": map[string]any{
					"namespace": "default",
					"serviceaccount": map[string]any{
						"name": "builder",
					},
				},
			},
			want: map[string][]string{
				"kubernetes.io.namespace":           {"default"},
				"kubernetes.io.serviceaccount.name": {"builder"},
			},
		},
		{
			name: "array-valued claims keep all scalar values",
			claims: jwt.MapClaims{
				"aud": []any{"kubernetes.default.svc", "harbor.local"},
			},
			want: map[string][]string{
				"aud": {"kubernetes.default.svc", "harbor.local"},
			},
		},
		{
			name: "array objects are ignored instead of recursively matched",
			claims: jwt.MapClaims{
				"groups": []any{map[string]any{"name": "builders"}},
			},
			want: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, flattenClaimValues(tt.claims))
		})
	}
}

// =============================================================================
// FederatedIdP CRUD Tests
// =============================================================================

func (suite *DaoTestSuite) TestCreate() {
	// Test duplicate issuer fails
	r := &model.FederatedIdp{
		Name:   "duplicate-issuer-test",
		Issuer: "https://test-issuer-1.example.com", // Same as idpID1
	}
	_, err := suite.dao.Create(orm.Context(), r)
	suite.NotNil(err)
	suite.True(errors.IsErr(err, errors.ConflictCode))
}

func (suite *DaoTestSuite) TestDelete() {
	// Delete non-existent IdP
	err := suite.dao.Delete(orm.Context(), 99999)
	suite.Require().NotNil(err)
	suite.True(errors.IsErr(err, errors.NotFoundCode))

	// Create and delete an IdP
	id, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "to-delete",
		Issuer: "https://to-delete.example.com",
	})
	suite.Nil(err)

	err = suite.dao.Delete(orm.Context(), id)
	suite.Nil(err)

	// Verify deletion
	_, err = suite.dao.Get(orm.Context(), id)
	suite.True(errors.IsErr(err, errors.NotFoundCode))
}

func (suite *DaoTestSuite) TestGet() {
	// Get non-existent
	_, err := suite.dao.Get(orm.Context(), 99999)
	suite.Require().NotNil(err)
	suite.True(errors.IsErr(err, errors.NotFoundCode))

	// Get existing
	idp, err := suite.dao.Get(orm.Context(), suite.idpID1)
	suite.Nil(err)
	suite.Equal("test-idp-1", idp.Name)
	suite.Equal("https://test-issuer-1.example.com", idp.Issuer)
}

func (suite *DaoTestSuite) TestGetIdpByIssuer() {
	idp, err := suite.dao.GetIdpByIssuer(orm.Context(), "https://test-issuer-1.example.com")
	suite.Nil(err)
	suite.Equal(suite.idpID1, idp.ID)

	// Non-existent issuer
	_, err = suite.dao.GetIdpByIssuer(orm.Context(), "https://nonexistent.example.com")
	suite.NotNil(err)
}

func (suite *DaoTestSuite) TestList() {
	idps, err := suite.dao.List(orm.Context(), &q.Query{
		Keywords: map[string]any{
			"name": "test-idp-1",
		},
	})
	suite.Require().Nil(err)
	suite.Equal(1, len(idps))
	suite.Equal(suite.idpID1, idps[0].ID)
}

func (suite *DaoTestSuite) TestCount() {
	total, err := suite.dao.Count(orm.Context(), nil)
	suite.Nil(err)
	suite.True(total >= 2)

	total, err = suite.dao.Count(orm.Context(), &q.Query{
		Keywords: map[string]any{
			"project_id": 0,
		},
	})
	suite.Nil(err)
	suite.True(total >= 1)
}

func (suite *DaoTestSuite) TestUpdate() {
	idp, err := suite.dao.Get(orm.Context(), suite.idpID1)
	suite.Nil(err)

	idp.Description = "updated description"
	err = suite.dao.Update(orm.Context(), idp)
	suite.Nil(err)

	updated, err := suite.dao.Get(orm.Context(), suite.idpID1)
	suite.Nil(err)
	suite.Equal("updated description", updated.Description)
}

// =============================================================================
// Claim Rules Tests
// =============================================================================

func (suite *DaoTestSuite) TestCreateClaimsSuccess() {
	claims := []model.ClaimRule{
		{
			IdentityProviderID: suite.idpID1,
			RobotID:            0,
			ClaimPath:          "org",
			Value:              "harbor",
		},
	}
	err := suite.dao.CreateClaims(orm.Context(), suite.idpID1, claims)
	suite.Nil(err)

	// Verify claims were created
	fetched, err := suite.dao.ListClaims(orm.Context(), suite.idpID1, "")
	suite.Nil(err)
	suite.True(len(fetched) >= 1)
}

func (suite *DaoTestSuite) TestCreateClaimsDuplicateInBatchFails() {
	// Create a new IdP for this test to avoid interference
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "dup-batch-test",
		Issuer: "https://dup-batch-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	claims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            100,
			ClaimPath:          "sub",
			Value:              "user1",
		},
		{
			IdentityProviderID: idpID,
			RobotID:            100,
			ClaimPath:          "sub",
			Value:              "user1", // Exact duplicate
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, claims)
	suite.NotNil(err)
	suite.Contains(err.Error(), "duplicate")
}

func (suite *DaoTestSuite) TestCreateClaimsRobotCannotOverrideIdpLevel() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "override-test",
		Issuer: "https://override-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create IdP-level claim
	idpClaims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            0, // IdP-level
			ClaimPath:          "issuer-claim",
			Value:              "harbor",
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, idpClaims)
	suite.Nil(err)

	// Try to create robot claim with same path - should fail
	robotClaims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            100, // Robot-level
			ClaimPath:          "issuer-claim",
			Value:              "different",
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, robotClaims)
	suite.NotNil(err)
	suite.Contains(err.Error(), "already owned by identity provider")
}

func (suite *DaoTestSuite) TestCreateClaimsCrossRobotOverlapAllowed() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "cross-robot-test",
		Issuer: "https://cross-robot-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create claim for robot 100
	robot100Claims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            100,
			ClaimPath:          "sub",
			Value:              "shared-user",
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, robot100Claims)
	suite.Nil(err)

	// Same (path, value) can appear on another robot; exact full claim-set
	// duplicates are rejected by the controller-level matcher validation.
	robot101Claims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            101,
			ClaimPath:          "sub",
			Value:              "shared-user", // Same value as robot 100
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, robot101Claims)
	suite.Nil(err)
}

func (suite *DaoTestSuite) TestCreateClaimsSamePathDifferentValueAllowed() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "diff-value-test",
		Issuer: "https://diff-value-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create claim for robot 100
	robot100Claims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            100,
			ClaimPath:          "sub",
			Value:              "user-100",
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, robot100Claims)
	suite.Nil(err)

	// Create same path but different value for robot 101 - should succeed
	robot101Claims := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            101,
			ClaimPath:          "sub",
			Value:              "user-101", // Different value
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, robot101Claims)
	suite.Nil(err)

	// Verify both exist
	claims, err := suite.dao.ListClaims(orm.Context(), idpID, "")
	suite.Nil(err)
	suite.Equal(2, len(claims))
}

func (suite *DaoTestSuite) TestCreateClaimsSameRobotDuplicatePathFails() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "same-robot-dup-test",
		Issuer: "https://same-robot-dup-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create first claim for robot 100
	firstClaim := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            100,
			ClaimPath:          "groups",
			Value:              "admins",
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, firstClaim)
	suite.Nil(err)

	// Try to add another claim with same path for same robot - should fail
	secondClaim := []model.ClaimRule{
		{
			IdentityProviderID: idpID,
			RobotID:            100,
			ClaimPath:          "groups",
			Value:              "developers", // Different value but same path
		},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, secondClaim)
	suite.NotNil(err)
	suite.Contains(err.Error(), "already has claim_path")
}

// =============================================================================
// List Claims with Filters Tests
// =============================================================================

func (suite *DaoTestSuite) TestListClaimsWithFilters() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "list-filter-test",
		Issuer: "https://list-filter-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create various claims one by one
	claimsToCreate := []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "org", Value: "harbor"},      // IdP-level
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "user-100"},  // Robot 100
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "email", Value: "a@b.com"}, // Robot 100
		{IdentityProviderID: idpID, RobotID: 101, ClaimPath: "sub", Value: "user-101"},  // Robot 101
	}

	for _, c := range claimsToCreate {
		err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{c})
		suite.Nil(err)
	}

	// Test: No filters - should return all 4
	all, err := suite.dao.ListClaimsWithFilters(orm.Context(), idpID, "", nil, false)
	suite.Nil(err)
	suite.Equal(4, len(all))

	// Test: IdP only - should return 1
	idpOnly, err := suite.dao.ListClaimsWithFilters(orm.Context(), idpID, "", nil, true)
	suite.Nil(err)
	suite.Equal(1, len(idpOnly))
	suite.Equal(int64(0), idpOnly[0].RobotID)

	// Test: Robot 100 - should return 2
	robot100ID := int64(100)
	robot100Claims, err := suite.dao.ListClaimsWithFilters(orm.Context(), idpID, "", &robot100ID, false)
	suite.Nil(err)
	suite.Equal(2, len(robot100Claims))

	// Test: Filter by claim_path
	subClaims, err := suite.dao.ListClaimsWithFilters(orm.Context(), idpID, "sub", nil, false)
	suite.Nil(err)
	suite.Equal(2, len(subClaims)) // user-100 and user-101
}

func (suite *DaoTestSuite) TestListClaimsIdpOnly() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "idp-only-test",
		Issuer: "https://idp-only-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create IdP-level and robot-level claims
	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "org", Value: "harbor"},
	})
	suite.Nil(err)

	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "user"},
	})
	suite.Nil(err)

	// ListClaimsIdpOnly should only return IdP-level claims
	idpOnlyClaims, err := suite.dao.ListClaimsIdpOnly(orm.Context(), idpID, "")
	suite.Nil(err)
	suite.Equal(1, len(idpOnlyClaims))
	suite.Equal("org", idpOnlyClaims[0].ClaimPath)
	suite.Equal(int64(0), idpOnlyClaims[0].RobotID)
}

// =============================================================================
// Delete Claims Tests
// =============================================================================

func (suite *DaoTestSuite) TestDeleteClaimRulesByRobotID() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "delete-robot-claims-test",
		Issuer: "https://delete-robot-claims-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create claims for IdP level and robot 100
	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "org", Value: "harbor"},
	})
	suite.Nil(err)

	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "user-100"},
	})
	suite.Nil(err)

	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 101, ClaimPath: "sub", Value: "user-101"},
	})
	suite.Nil(err)

	// Delete claims for robot 100
	err = suite.dao.DeleteClaimRulesByRobotID(orm.Context(), 100)
	suite.Nil(err)

	// Verify robot 100's claims are gone but others remain
	remaining, err := suite.dao.ListClaims(orm.Context(), idpID, "")
	suite.Nil(err)
	suite.Equal(2, len(remaining)) // IdP-level and robot 101's claims

	for _, c := range remaining {
		suite.NotEqual(int64(100), c.RobotID)
	}
}

func (suite *DaoTestSuite) TestDeleteClaimsSelective() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "selective-delete-test",
		Issuer: "https://selective-delete-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create multiple claims
	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "user1"},
	})
	suite.Nil(err)

	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "email", Value: "user1@example.com"},
	})
	suite.Nil(err)

	// Delete just one specific claim
	err = suite.dao.DeleteClaims(orm.Context(), []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "user1"},
	})
	suite.Nil(err)

	// Verify only email claim remains
	remaining, err := suite.dao.ListClaims(orm.Context(), idpID, "")
	suite.Nil(err)
	suite.Equal(1, len(remaining))
	suite.Equal("email", remaining[0].ClaimPath)
}

// =============================================================================
// Robot Identity Provider Tests
// =============================================================================

func (suite *DaoTestSuite) TestRobotIdpCRUD() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "robot-idp-crud-test",
		Issuer: "https://robot-idp-crud-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create a real robot to satisfy FK constraint
	robotID := suite.createTestRobot("test-robot-crud")
	defer suite.deleteTestRobot(robotID)

	// Create robot-idp association
	robotIdp := &model.RobotIdentityProvider{
		IdentityProviderID: idpID,
		RobotID:            robotID,
	}
	rid, err := suite.dao.CreateRobotIdp(orm.Context(), robotIdp)
	suite.Nil(err)
	suite.NotEqual(int64(0), rid)

	// Check HasRobotIdpByRobotID
	hasIdp, err := suite.dao.HasRobotIdpByRobotID(orm.Context(), robotID)
	suite.Nil(err)
	suite.True(hasIdp)

	// Check for non-existent robot
	hasIdp, err = suite.dao.HasRobotIdpByRobotID(orm.Context(), 999999)
	suite.Nil(err)
	suite.False(hasIdp)

	// Get IdP ID by robot ID
	fetchedIdpID, err := suite.dao.GetIdpIDByRobotID(orm.Context(), robotID)
	suite.Nil(err)
	suite.Equal(idpID, fetchedIdpID)

	// List robot IDPs by IdP ID
	robotIdps, err := suite.dao.ListRobotIdpByIdpID(orm.Context(), idpID)
	suite.Nil(err)
	suite.Equal(1, len(robotIdps))
	suite.Equal(robotID, robotIdps[0].RobotID)

	// Delete by robot ID
	err = suite.dao.DeleteRobotIdpByRobotID(orm.Context(), robotID)
	suite.Nil(err)

	// Verify deletion
	hasIdp, err = suite.dao.HasRobotIdpByRobotID(orm.Context(), robotID)
	suite.Nil(err)
	suite.False(hasIdp)
}

func (suite *DaoTestSuite) TestDeleteRobotIdpByIdpID() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "delete-idp-robots-test",
		Issuer: "https://delete-idp-robots-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create real robots to satisfy FK constraints
	var robotIDs []int64
	for i := 0; i < 3; i++ {
		robotID := suite.createTestRobot("test-robot-delete-" + string(rune('A'+i)))
		robotIDs = append(robotIDs, robotID)
		defer suite.deleteTestRobot(robotID)
	}

	// Create multiple robot associations
	for _, robotID := range robotIDs {
		_, err := suite.dao.CreateRobotIdp(orm.Context(), &model.RobotIdentityProvider{
			IdentityProviderID: idpID,
			RobotID:            robotID,
		})
		suite.Nil(err)
	}

	// Verify they exist
	robotIdps, err := suite.dao.ListRobotIdpByIdpID(orm.Context(), idpID)
	suite.Nil(err)
	suite.Equal(3, len(robotIdps))

	// Delete all by IdP ID
	err = suite.dao.DeleteRobotIdpByIdpID(orm.Context(), idpID)
	suite.Nil(err)

	// Verify all are deleted
	robotIdps, err = suite.dao.ListRobotIdpByIdpID(orm.Context(), idpID)
	suite.Nil(err)
	suite.Equal(0, len(robotIdps))
}

// =============================================================================
// IdP-Level Claim Duplication Tests
// =============================================================================

func (suite *DaoTestSuite) TestIdpLevelClaimDuplicateValueAllowed() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "idp-dup-value-test",
		Issuer: "https://idp-dup-value-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// IdP-level claims can have same path but different values
	claims := []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "groups", Value: "admins"},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, claims)
	suite.Nil(err)

	// Add another IdP-level claim with same path but different value
	claims2 := []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "groups", Value: "operators"},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, claims2)
	suite.Nil(err)

	// Verify both exist
	allClaims, err := suite.dao.ListClaimsIdpOnly(orm.Context(), idpID, "groups")
	suite.Nil(err)
	suite.Equal(2, len(allClaims))
}

func (suite *DaoTestSuite) TestIdpLevelExactDuplicateFails() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "idp-exact-dup-test",
		Issuer: "https://idp-exact-dup-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Create IdP-level claim
	claims := []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "org", Value: "harbor"},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, claims)
	suite.Nil(err)

	// Try to add exact duplicate - should fail
	duplicateClaim := []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "org", Value: "harbor"},
	}
	err = suite.dao.CreateClaims(orm.Context(), idpID, duplicateClaim)
	suite.NotNil(err)
	suite.Contains(err.Error(), "duplicate IDP-level claim")
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func (suite *DaoTestSuite) TestEmptyClaimsBatch() {
	// Creating empty claims should succeed (no-op)
	err := suite.dao.CreateClaims(orm.Context(), suite.idpID1, []model.ClaimRule{})
	suite.Nil(err)
}

func (suite *DaoTestSuite) TestDeleteNonExistentClaims() {
	// Deleting non-existent claims should succeed (classic behavior)
	err := suite.dao.DeleteClaims(orm.Context(), []model.ClaimRule{
		{IdentityProviderID: suite.idpID1, RobotID: 100, ClaimPath: "nonexistent", Value: "value"},
	})
	suite.Nil(err)
}

func (suite *DaoTestSuite) TestDeleteByProjectID() {
	// Create IdP in project 999
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:      "project-delete-test",
		Issuer:    "https://project-delete-test.example.com",
		ProjectID: 999,
	})
	suite.Nil(err)

	// Delete by project ID
	err = suite.dao.DeleteByProjectID(orm.Context(), 999)
	suite.Nil(err)

	// Verify deletion
	_, err = suite.dao.Get(orm.Context(), idpID)
	suite.True(errors.IsErr(err, errors.NotFoundCode))
}

// =============================================================================
// Comprehensive Validation Logic Tests
// =============================================================================

func (suite *DaoTestSuite) TestValidateUniqueClaimsComprehensive() {
	idpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:   "validate-comprehensive-test",
		Issuer: "https://validate-comprehensive-test.example.com",
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), idpID)

	// Setup: Create initial claims
	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 0, ClaimPath: "issuer-claim", Value: "harbor-issuer"},
	})
	suite.Nil(err)

	err = suite.dao.CreateClaims(orm.Context(), idpID, []model.ClaimRule{
		{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "service-account"},
	})
	suite.Nil(err)

	suite.T().Run("Cannot override IdP-level claim with robot claim", func(t *testing.T) {
		robotClaim := []model.ClaimRule{
			{IdentityProviderID: idpID, RobotID: 200, ClaimPath: "issuer-claim", Value: "different"},
		}
		err := suite.dao.CreateClaims(orm.Context(), idpID, robotClaim)
		suite.NotNil(err)
		suite.Contains(err.Error(), "already owned by identity provider")
	})

	suite.T().Run("Can overlap path+value across robots", func(t *testing.T) {
		robotClaim := []model.ClaimRule{
			{IdentityProviderID: idpID, RobotID: 200, ClaimPath: "sub", Value: "service-account"}, // Same as robot 100
		}
		err := suite.dao.CreateClaims(orm.Context(), idpID, robotClaim)
		suite.Nil(err)
	})

	suite.T().Run("Same robot cannot have duplicate path", func(t *testing.T) {
		robotClaim := []model.ClaimRule{
			{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "sub", Value: "another-value"}, // Same robot, same path
		}
		err := suite.dao.CreateClaims(orm.Context(), idpID, robotClaim)
		suite.NotNil(err)
		suite.Contains(err.Error(), "already has claim_path")
	})

	suite.T().Run("Different path for same robot OK", func(t *testing.T) {
		robotClaim := []model.ClaimRule{
			{IdentityProviderID: idpID, RobotID: 100, ClaimPath: "email", Value: "robot@harbor.io"},
		}
		err := suite.dao.CreateClaims(orm.Context(), idpID, robotClaim)
		suite.Nil(err)
	})

	suite.T().Run("Same path different value for different robot OK", func(t *testing.T) {
		robotClaim := []model.ClaimRule{
			{IdentityProviderID: idpID, RobotID: 300, ClaimPath: "sub", Value: "different-service"}, // Different value
		}
		err := suite.dao.CreateClaims(orm.Context(), idpID, robotClaim)
		suite.Nil(err)
	})
}

// =============================================================================
// FindMatchingRobot Tests - Based on testTopMatchrobot.md scenarios
// =============================================================================

// TestFindMatchingRobotScenarios tests the 6 scenarios from testTopMatchrobot.md
// The key requirement: ALL robot claims must be present in the token.
// Token may have extra claims, but robot cannot have claims missing from token.
func (suite *DaoTestSuite) TestFindMatchingRobotScenarios() {
	// Create System-level Federated IDP (fedidp1) - project_id = 0
	fedidp1, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:      "fedidp1-system",
		Issuer:    "https://fedidp1.example.com",
		ProjectID: 0, // System level
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), fedidp1)

	// Create Project-level Federated IDP (fedidp2) - project_id = 1
	fedidp2, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:      "fedidp2-project",
		Issuer:    "https://fedidp2.example.com",
		ProjectID: 1, // Project level
	})
	suite.Nil(err)
	defer suite.dao.Delete(orm.Context(), fedidp2)

	// Setup fedidp1:
	// - IDP Claims (robot_id=0): 4 claims (iss, aud, azp, scope)
	// - robot1 (ID=101): 1 claim (org=harbor)
	// - robot2 (ID=102): 2 claims (org=harbor, team=development) - overlaps with robot1
	err = suite.dao.CreateClaims(orm.Context(), fedidp1, []model.ClaimRule{
		// IDP-level claims (robot_id = 0)
		{IdentityProviderID: fedidp1, RobotID: 0, ClaimPath: "iss", Value: "https://fedidp1.example.com"},
		{IdentityProviderID: fedidp1, RobotID: 0, ClaimPath: "aud", Value: "harbor"},
		{IdentityProviderID: fedidp1, RobotID: 0, ClaimPath: "azp", Value: "harbor-client"},
		{IdentityProviderID: fedidp1, RobotID: 0, ClaimPath: "scope", Value: "openid"},
		// robot1 claims
		{IdentityProviderID: fedidp1, RobotID: 101, ClaimPath: "org", Value: "harbor"},
		// robot2 claims (overlaps with robot1 on org=harbor)
		{IdentityProviderID: fedidp1, RobotID: 102, ClaimPath: "org", Value: "harbor"},
		{IdentityProviderID: fedidp1, RobotID: 102, ClaimPath: "team", Value: "development"},
	})
	suite.Nil(err)

	// Setup fedidp2:
	// - IDP Claims (robot_id=0): 3 claims (iss, aud, tenant)
	// - robot3 (ID=103): 1 claim (env=production)
	// - robot4 (ID=104): 1 claim (env=staging) - different value than robot3
	err = suite.dao.CreateClaims(orm.Context(), fedidp2, []model.ClaimRule{
		// IDP-level claims
		{IdentityProviderID: fedidp2, RobotID: 0, ClaimPath: "iss", Value: "https://fedidp2.example.com"},
		{IdentityProviderID: fedidp2, RobotID: 0, ClaimPath: "aud", Value: "harbor-project"},
		{IdentityProviderID: fedidp2, RobotID: 0, ClaimPath: "tenant", Value: "acme-corp"},
		// robot3 claims
		{IdentityProviderID: fedidp2, RobotID: 103, ClaimPath: "env", Value: "production"},
		// robot4 claims (different value)
		{IdentityProviderID: fedidp2, RobotID: 104, ClaimPath: "env", Value: "staging"},
	})
	suite.Nil(err)

	// =========================================================================
	// Scenario 1: JWT matches all IDP claims + all robot claims → robot1 selected
	// =========================================================================
	suite.T().Run("Scenario1_AllClaimsMatch_Robot1Selected", func(t *testing.T) {
		// JWT with 10 claims, matching all IDP claims + robot1's single claim
		tokenClaims := jwt.MapClaims{
			"iss":    "https://fedidp1.example.com",
			"aud":    "harbor",
			"azp":    "harbor-client",
			"scope":  "openid",
			"org":    "harbor", // robot1's claim
			"sub":    "user@example.com",
			"email":  "user@example.com",
			"name":   "Test User",
			"groups": "developers",
			"exp":    1234567890,
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidp1, tokenClaims)
		suite.Nil(err)
		// robot1 (101) has 1 claim: org=harbor - fully matched
		// robot2 (102) has 2 claims: org=harbor, team=development - team is MISSING from token
		// Only robot1 should match since ALL its claims are in the token
		suite.Equal(int64(101), robotID, "robot1 should be selected - all its claims match")
	})

	// =========================================================================
	// Scenario 2: Partial robot claim match → no robot selected
	// =========================================================================
	suite.T().Run("Scenario2_PartialRobotClaimMatch_NoRobotSelected", func(t *testing.T) {
		// JWT matches only 1 claim of robot4, but robot4 needs env=staging
		// This JWT has env=production (matches robot3) but wrong tenant
		tokenClaims := jwt.MapClaims{
			"iss":    "https://fedidp2.example.com",
			"aud":    "harbor-project",
			"tenant": "wrong-tenant", // Doesn't match IDP claim - but we're testing robot matching only
			"env":    "wrong-env",    // Doesn't match robot3 OR robot4
			"sub":    "user@example.com",
			"email":  "user@example.com",
			"name":   "Test User",
			"groups": "developers",
			"role":   "admin",
			"exp":    1234567890,
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidp2, tokenClaims)
		// No robot should match because env doesn't match any robot's required value
		suite.Nil(err) // No error, just no match
		suite.Equal(int64(0), robotID, "No robot should be selected")
	})

	// =========================================================================
	// Scenario 3: Issuer does not match any IDP → reject (tested at controller level)
	// Note: This is handled by GetIdpByIssuer, not FindMatchingRobot
	// =========================================================================
	suite.T().Run("Scenario3_UnknownIssuer_NotFoundAtDAOLevel", func(t *testing.T) {
		// This tests GetIdpByIssuer, not FindMatchingRobot
		_, err := suite.dao.GetIdpByIssuer(orm.Context(), "https://unknown-issuer.example.com")
		suite.NotNil(err, "Unknown issuer should return error")
	})

	// =========================================================================
	// Scenario 4: IDP claims partially matched but robot claims fully matched → reject
	// Note: IDP claims validation happens in robotjwt.go (validateIDPClaims), not here
	// FindMatchingRobot only handles robot_id > 0 claims
	// =========================================================================
	suite.T().Run("Scenario4_IDPClaimsNotValidatedHere", func(t *testing.T) {
		// This scenario tests IDP-level claim validation which happens in robotjwt.go
		// FindMatchingRobot excludes robot_id=0 claims, so it only matches robot claims
		// The IDP claim validation is separate (done by ListClaimsIdpOnly + validateIDPClaims)

		// Token with all robot2 claims but we'll verify IDP claims are ignored by FindMatchingRobot
		tokenClaims := jwt.MapClaims{
			"org":  "harbor",
			"team": "development",
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidp1, tokenClaims)
		suite.Nil(err)
		// robot2 has org=harbor + team=development, both present
		suite.Equal(int64(102), robotID, "robot2 should match - IDP claims are validated separately")
	})

	// =========================================================================
	// Scenario 5: Multiple robots match, pick the one with full match → robot3
	// =========================================================================
	suite.T().Run("Scenario5_MultipleRobotsOnlyOneFullyMatches_Robot3Selected", func(t *testing.T) {
		// JWT matches all claims for robot3, but only some for robot4
		tokenClaims := jwt.MapClaims{
			"iss":    "https://fedidp2.example.com",
			"aud":    "harbor-project",
			"tenant": "acme-corp",
			"env":    "production", // Matches robot3, NOT robot4 (which needs staging)
			"sub":    "user@example.com",
			"email":  "user@example.com",
			"role":   "deployer",
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidp2, tokenClaims)
		suite.Nil(err)
		// robot3 (103): env=production - FULLY MATCHED
		// robot4 (104): env=staging - NOT MATCHED (token has production)
		suite.Equal(int64(103), robotID, "robot3 should be selected - it's the only one with all claims matched")
	})

	// =========================================================================
	// Scenario 6: Robot account with zero claims → invalid candidate
	// =========================================================================
	suite.T().Run("Scenario6_RobotWithZeroClaims_NotSelected", func(t *testing.T) {
		// Create a new IDP with a robot that has no claims
		emptyClaimIDP, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
			Name:      "empty-claim-test",
			Issuer:    "https://empty-claim.example.com",
			ProjectID: 0,
		})
		suite.Nil(err)
		defer suite.dao.Delete(orm.Context(), emptyClaimIDP)

		// Create only IDP-level claims, no robot claims
		// robot_id = 105 exists in robot_identity_providers but has no claims
		err = suite.dao.CreateClaims(orm.Context(), emptyClaimIDP, []model.ClaimRule{
			{IdentityProviderID: emptyClaimIDP, RobotID: 0, ClaimPath: "iss", Value: "https://empty-claim.example.com"},
		})
		suite.Nil(err)

		// Token with valid claims
		tokenClaims := jwt.MapClaims{
			"iss":  "https://empty-claim.example.com",
			"sub":  "user@example.com",
			"org":  "harbor",
			"team": "development",
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), emptyClaimIDP, tokenClaims)
		suite.Nil(err)
		// No robots have claims defined, so no robot can match
		suite.Equal(int64(0), robotID, "No robot should be selected when no robots have claims")
	})

	// =========================================================================
	// Additional: Robot with most claims wins among fully matching robots
	// =========================================================================
	suite.T().Run("MostSpecificRobotWins", func(t *testing.T) {
		// Token has all claims for both robot1 AND robot2
		tokenClaims := jwt.MapClaims{
			"iss":   "https://fedidp1.example.com",
			"aud":   "harbor",
			"azp":   "harbor-client",
			"scope": "openid",
			"org":   "harbor",      // Matches robot1 AND robot2
			"team":  "development", // Matches robot2 only
			"sub":   "user@example.com",
			"email": "user@example.com",
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidp1, tokenClaims)
		suite.Nil(err)
		// robot1 (101): 1 claim (org=harbor) - FULLY MATCHED
		// robot2 (102): 2 claims (org=harbor, team=development) - FULLY MATCHED
		// robot2 should win because it has more claims (more specific)
		suite.Equal(int64(102), robotID, "robot2 should be selected - it has more claims (more specific)")
	})

	// =========================================================================
	// Additional: Empty token returns error
	// =========================================================================
	suite.T().Run("EmptyTokenClaims_ReturnsError", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidp1, tokenClaims)
		suite.NotNil(err, "Empty token should return error")
		suite.Equal(int64(0), robotID)
	})
}

func (suite *DaoTestSuite) TestFindMatchingRobotClaimPathBehaviors() {
	fedidpID, err := suite.dao.Create(orm.Context(), &model.FederatedIdp{
		Name:      "claim-path-behavior-idp",
		Issuer:    "https://claim-path-behavior.example.com",
		ProjectID: 0,
	})
	suite.Require().Nil(err)
	defer suite.dao.Delete(orm.Context(), fedidpID)

	err = suite.dao.CreateClaims(orm.Context(), fedidpID, []model.ClaimRule{
		{IdentityProviderID: fedidpID, RobotID: 201, ClaimPath: "kubernetes.io.namespace", Value: "default"},
		{IdentityProviderID: fedidpID, RobotID: 202, ClaimPath: "aud", Value: "harbor.local"},
		{IdentityProviderID: fedidpID, RobotID: 203, ClaimPath: "kubernetes.io.namespace", Value: "default"},
		{IdentityProviderID: fedidpID, RobotID: 203, ClaimPath: "kubernetes.io.serviceaccount.name", Value: "builder"},
	})
	suite.Require().Nil(err)

	suite.T().Run("nested Kubernetes claims match by dot path", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{
			"kubernetes.io": map[string]any{
				"namespace": "default",
			},
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidpID, tokenClaims)

		suite.Require().NoError(err)
		suite.Equal(int64(201), robotID)
	})

	suite.T().Run("array-valued JWT claims match when they contain the configured value", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{
			"aud": []any{"kubernetes.default.svc", "harbor.local"},
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidpID, tokenClaims)

		suite.Require().NoError(err)
		suite.Equal(int64(202), robotID)
	})

	suite.T().Run("array-valued JWT claims do not match when the configured value is absent", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{
			"aud": []any{"kubernetes.default.svc", "other-registry.local"},
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidpID, tokenClaims)

		suite.Require().NoError(err)
		suite.Equal(int64(0), robotID)
	})

	suite.T().Run("nested objects inside arrays are ignored instead of matched smartly", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{
			"kubernetes.io": []any{
				map[string]any{"namespace": "default"},
			},
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidpID, tokenClaims)

		suite.Require().NoError(err)
		suite.Equal(int64(0), robotID)
	})

	suite.T().Run("most specific robot wins when nested token satisfies multiple claim sets", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{
			"kubernetes.io": map[string]any{
				"namespace": "default",
				"serviceaccount": map[string]any{
					"name": "builder",
				},
			},
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidpID, tokenClaims)

		suite.Require().NoError(err)
		suite.Equal(int64(203), robotID)
	})

	suite.T().Run("literal dotted top-level claim names still match directly", func(t *testing.T) {
		tokenClaims := jwt.MapClaims{
			"kubernetes.io.namespace": "default",
		}

		robotID, err := suite.dao.FindMatchingRobot(orm.Context(), fedidpID, tokenClaims)

		suite.Require().NoError(err)
		suite.Equal(int64(201), robotID)
	})
}
