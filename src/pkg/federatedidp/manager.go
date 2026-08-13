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

package federatedidp

import (
	"context"

	"github.com/golang-jwt/jwt/v5"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/federatedidp/dao"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
)

var (
	// Mgr is a global variable for the default federatedidp manager implementation
	Mgr = NewManager()
)

// separate claims type
// claim path -> claim value.
type Claims map[string]string

// Manager ...
type Manager interface {
	// Get ...
	Get(ctx context.Context, id int64) (*model.FederatedIdp, error)

	// GetIdpByIssuer ...
	GetIdpByIssuer(ctx context.Context, issuer string) (*model.FederatedIdp, error)

	// GetTopMatchedRobot ...
	GetTopMatchedRobot(ctx context.Context, issuerID int64, tokenClaims jwt.MapClaims) (int64, error)

	// Count returns the total count of robots according to the query
	Count(ctx context.Context, query *q.Query) (total int64, err error)

	// Create ...
	Create(ctx context.Context, m *model.FederatedIdp) (int64, error)

	// Delete ...
	Delete(ctx context.Context, id int64) error

	// DeleteByProjectID ...
	DeleteByProjectID(ctx context.Context, projectID int64) error

	// Update ...
	Update(ctx context.Context, m *model.FederatedIdp, props ...string) error

	// List ...
	List(ctx context.Context, query *q.Query) ([]*model.FederatedIdp, error)

	// ListClaims ...
	ListClaims(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error)

	// ListClaimsIdpOnly ...
	ListClaimsIdpOnly(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error)

	// ListClaimsWithFilters lists claims with flexible filtering options
	ListClaimsWithFilters(ctx context.Context, idpID int64, claimPath string, robotID *int64, idpOnly bool) ([]model.ClaimRule, error)

	// CreateClaims ...
	CreateClaims(ctx context.Context, idpID int64, claims []model.ClaimRule) error

	// DeleteClaims ...
	DeleteClaims(ctx context.Context, claims []model.ClaimRule) error

	// CreateRobotIdp ...
	CreateRobotIdp(ctx context.Context, r *model.RobotIdentityProvider) (int64, error)

	// DeleteRobotIdpByIdpID ...
	DeleteRobotIdpByIdpID(ctx context.Context, idpID int64) error

	// DeleteRobotIdpByRobotID ...
	DeleteRobotIdpByRobotID(ctx context.Context, robotID int64) error

	// DeleteClaimRulesByRobotID deletes all claim_rules records associated with a given robot ID
	DeleteClaimRulesByRobotID(ctx context.Context, robotID int64) error

	// HasRobotIdpByRobotID ...
	HasRobotIdpByRobotID(ctx context.Context, robotID int64) (bool, error)

	// ListRobotIdpByIdpID ...
	ListRobotIdpByIdpID(ctx context.Context, idpID int64) ([]model.RobotIdentityProvider, error)

	// GetIdpIDByRobotID returns the identity provider ID for a given robot ID
	GetIdpIDByRobotID(ctx context.Context, robotID int64) (int64, error)
}

var _ Manager = &manager{}

type manager struct {
	dao dao.DAO
}

// NewManager return a new instance of defaultRobotManager
func NewManager() Manager {
	return &manager{
		dao: dao.New(),
	}
}

// Get ...
func (m *manager) Get(ctx context.Context, id int64) (*model.FederatedIdp, error) {
	return m.dao.Get(ctx, id)
}

// GetIdpByName ...
func (m *manager) GetIdpByIssuer(ctx context.Context, issuer string) (*model.FederatedIdp, error) {
	return m.dao.GetIdpByIssuer(ctx, issuer)
}

// GetTopMatchedRobot ...
func (m *manager) GetTopMatchedRobot(ctx context.Context, issuerID int64, tokenClaims jwt.MapClaims) (int64, error) {
	return m.dao.FindMatchingRobot(ctx, issuerID, tokenClaims)
}

// Count ...
func (m *manager) Count(ctx context.Context, query *q.Query) (total int64, err error) {
	return m.dao.Count(ctx, query)
}

// Create ...
func (m *manager) Create(ctx context.Context, f *model.FederatedIdp) (int64, error) {
	return m.dao.Create(ctx, f)
}

// Delete ...
func (m *manager) Delete(ctx context.Context, id int64) error {
	return m.dao.Delete(ctx, id)
}

// DeleteByProjectID ...
func (m *manager) DeleteByProjectID(ctx context.Context, projectID int64) error {
	return m.dao.DeleteByProjectID(ctx, projectID)
}

// Update ...
func (m *manager) Update(ctx context.Context, f *model.FederatedIdp, props ...string) error {
	return m.dao.Update(ctx, f, props...)
}

// List ...
func (m *manager) List(ctx context.Context, query *q.Query) ([]*model.FederatedIdp, error) {
	return m.dao.List(ctx, query)
}

// ListClaims ...
func (m *manager) ListClaims(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error) {
	return m.dao.ListClaims(ctx, id, claimPath)
}

// ListClaimsIdpOnly ...
func (m *manager) ListClaimsIdpOnly(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error) {
	return m.dao.ListClaimsIdpOnly(ctx, id, claimPath)
}

// ListClaimsWithFilters ...
func (m *manager) ListClaimsWithFilters(ctx context.Context, idpID int64, claimPath string, robotID *int64, idpOnly bool) ([]model.ClaimRule, error) {
	return m.dao.ListClaimsWithFilters(ctx, idpID, claimPath, robotID, idpOnly)
}

// CreateClaims ...
func (m *manager) CreateClaims(ctx context.Context, idpID int64, claims []model.ClaimRule) error {
	return m.dao.CreateClaims(ctx, idpID, claims)
}

// DeleteClaims ...
func (m *manager) DeleteClaims(ctx context.Context, claims []model.ClaimRule) error {
	return m.dao.DeleteClaims(ctx, claims)
}

// CreateRobotIdp ...
func (m *manager) CreateRobotIdp(ctx context.Context, r *model.RobotIdentityProvider) (int64, error) {
	return m.dao.CreateRobotIdp(ctx, r)
}

// DeleteRobotIdpByIdpID ...
func (m *manager) DeleteRobotIdpByIdpID(ctx context.Context, idpID int64) error {
	return m.dao.DeleteRobotIdpByIdpID(ctx, idpID)
}

// DeleteRobotIdpByRobotID ...
func (m *manager) DeleteRobotIdpByRobotID(ctx context.Context, robotID int64) error {
	return m.dao.DeleteRobotIdpByRobotID(ctx, robotID)
}

// DeleteClaimRulesByRobotID deletes all claim_rules records associated with a given robot ID
func (m *manager) DeleteClaimRulesByRobotID(ctx context.Context, robotID int64) error {
	return m.dao.DeleteClaimRulesByRobotID(ctx, robotID)
}

// HasRobotIdpByRobotID ...
func (m *manager) HasRobotIdpByRobotID(ctx context.Context, robotID int64) (bool, error) {
	return m.dao.HasRobotIdpByRobotID(ctx, robotID)
}

// ListRobotIdpByIdpID ...
func (m *manager) ListRobotIdpByIdpID(ctx context.Context, idpID int64) ([]model.RobotIdentityProvider, error) {
	return m.dao.ListRobotIdpByIdpID(ctx, idpID)
}

// GetIdpIDByRobotID returns the identity provider ID for a given robot ID
func (m *manager) GetIdpIDByRobotID(ctx context.Context, robotID int64) (int64, error) {
	return m.dao.GetIdpIDByRobotID(ctx, robotID)
}
