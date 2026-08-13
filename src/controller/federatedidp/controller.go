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
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg"
	"github.com/goharbor/harbor/src/pkg/commercial"
	"github.com/goharbor/harbor/src/pkg/federatedidp"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
	"github.com/goharbor/harbor/src/pkg/project"
	"github.com/goharbor/harbor/src/pkg/replication"
	"github.com/goharbor/harbor/src/server/middleware/security/jwkscache"
)

// Ctl is a global registry controller instance
var Ctl = NewController()

func init() {
	commercial.RegisterDisableGuard(commercial.IdentityProviders, identityProviderDisableGuard{counter: federatedidp.Mgr}.Validate)
}

type identityProviderCounter interface {
	Count(context.Context, *q.Query) (int64, error)
}

type identityProviderDisableGuard struct {
	counter identityProviderCounter
}

func (g identityProviderDisableGuard) Validate(ctx context.Context) error {
	count, err := g.counter.Count(ctx, nil)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("identity_providers cannot be disabled while %d identity provider endpoints exist", count)
	}
	return nil
}

type Controller interface {
	// Create the federated idp
	Create(ctx context.Context, federatedIdp *model.FederatedIdp) (id int64, err error)
	// Count returns the count of federated idps according to the query
	Count(ctx context.Context, query *q.Query) (count int64, err error)
	// List federated idps according to the query
	List(ctx context.Context, query *q.Query) (federatedIdps []*model.FederatedIdp, err error)
	// Get the federated idp specified by ID
	Get(ctx context.Context, id int64) (federatedIdp *model.FederatedIdp, err error)
	// Get the federated idp specified by IDP Name
	GetIdpByIssuer(ctx context.Context, issuer string) (federatedIdp *model.FederatedIdp, err error)
	// GetTopMatchedRobot ...
	GetTopMatchedRobot(ctx context.Context, issuerID int64, tokenClaims jwt.MapClaims) (int64, error)
	// Update the specified federated idp
	Update(ctx context.Context, federatedIdp *model.FederatedIdp, props ...string) (err error)
	// Delete the federated idp specified by ID
	Delete(ctx context.Context, id int64) (err error)
	// DeleteByProjectID deletes all federated idps associated with the specified project
	DeleteByProjectID(ctx context.Context, projectID int64) (err error)
	// ListClaims returns all the claims associated with federated idp specified by ID
	ListClaims(ctx context.Context, id int64, claimPath string) (claims []model.ClaimRule, err error)
	// ListClaimsIdpOnly returns the claims of the federated idp only specified by ID
	ListClaimsIdpOnly(ctx context.Context, id int64, claimPath string) (claims []model.ClaimRule, err error)
	// ListClaimsWithFilters lists claims with flexible filtering options
	ListClaimsWithFilters(ctx context.Context, idpID int64, claimPath string, robotID *int64, idpOnly bool) (claims []model.ClaimRule, err error)
	// CreateClaims creates the claims
	CreateClaims(ctx context.Context, idpID int64, claims []model.ClaimRule) (err error)
	// DeleteClaims deletes the claims according to the query
	DeleteClaims(ctx context.Context, claims []model.ClaimRule) (err error)
	// CreateRobotIdp creates a new RobotIdentityProvider record
	CreateRobotIdp(ctx context.Context, idpID, robotID int64) (int64, error)
	// DeleteRobotIdpByIdpID deletes a RobotIdentityProvider record
	DeleteRobotIdpByIdpID(ctx context.Context, idpID int64) error
	// DeleteRobotIdpByRobotID deletes a RobotIdentityProvider record
	DeleteRobotIdpByRobotID(ctx context.Context, robotID int64) error
	// DeleteClaimRulesByRobotID deletes all claim_rules records associated with a given robot ID
	DeleteClaimRulesByRobotID(ctx context.Context, robotID int64) error
	// HasRobotIdpByRobotID checks if a given robot has at least one associated identity provider.
	HasRobotIdpByRobotID(ctx context.Context, robotID int64) (bool, error)
	// ListRobotIdpByIdpID lists all robot_identity_providers associated with the given IDP ID
	ListRobotIdpByIdpID(ctx context.Context, idpID int64) ([]model.RobotIdentityProvider, error)
	// GetIdpIDByRobotID returns the identity provider ID for a given robot ID
	GetIdpIDByRobotID(ctx context.Context, robotID int64) (int64, error)
}

// NewController creates an instance of the federated idp controller
func NewController() Controller {
	return &controller{
		fidpMgr: federatedidp.Mgr,
		proMgr:  pkg.ProjectMgr,
	}
}

type controller struct {
	fidpMgr federatedidp.Manager
	repMgr  replication.Manager
	proMgr  project.Manager
}

func (c *controller) Create(ctx context.Context, federatedidp *model.FederatedIdp) (int64, error) {
	if err := c.validate(ctx, federatedidp); err != nil {
		return 0, err
	}
	// Check uniqueness of name and issuer across all IdPs
	if err := c.validateUniqueness(ctx, federatedidp, false); err != nil {
		return 0, err
	}
	id, err := c.fidpMgr.Create(ctx, federatedidp)
	if err != nil {
		return 0, err
	}

	// For online IdPs, fetch and cache JWKS immediately
	if !federatedidp.OfflineValidation && federatedidp.JWKSURI != "" {
		if refreshErr := jwkscache.NewManager().ForceRefresh(ctx, id, federatedidp.JWKSURI); refreshErr != nil {
			log.Warningf("Failed to cache JWKS for new IdP %d: %v", id, refreshErr)
			// Don't fail the create - JWKS will be fetched on first use
		}
	}

	return id, nil
}

// validateUniqueness checks that the IdP name and issuer are unique across all IdPs
// (both system-level and project-level)
func (c *controller) validateUniqueness(ctx context.Context, fed *model.FederatedIdp, isUpdate bool) error {
	// Query all IdPs to check for name and issuer conflicts
	allIdps, err := c.fidpMgr.List(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to list IdPs for uniqueness validation")
	}

	for _, existingIdp := range allIdps {
		// Skip self when updating
		if isUpdate && existingIdp.ID == fed.ID {
			continue
		}

		// Check name uniqueness across all IdPs
		if existingIdp.Name == fed.Name {
			return errors.New(nil).WithCode(errors.ConflictCode).
				WithMessagef("identity provider with name '%s' already exists", fed.Name)
		}

		// Check issuer uniqueness across all IdPs
		if existingIdp.Issuer == fed.Issuer {
			return errors.New(nil).WithCode(errors.ConflictCode).
				WithMessagef("identity provider with issuer '%s' already exists", fed.Issuer)
		}
	}

	return nil
}

func (c *controller) validate(_ context.Context, fed *model.FederatedIdp) error {
	if len(fed.Name) == 0 {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("name cannot be empty")
	}
	if len(fed.Name) > 64 {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("the max length of name is 64")
	}
	if len(fed.Issuer) == 0 {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("issuer cannot be empty")
	}
	issuerURL, err := lib.ValidateURL(fed.Issuer)
	if err != nil {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("invalid issuer URL")
	}
	fed.Issuer = issuerURL

	// Validate JWKS URI
	if len(fed.JWKSURI) == 0 && !fed.OfflineValidation {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("invalid jwks_uri")
	} else if !fed.OfflineValidation {
		url, err := lib.ValidateURL(fed.JWKSURI)
		if err != nil {
			return errors.New(nil).WithCode(errors.BadRequestCode).
				WithMessage("invalid jwks_uri")
		}
		fed.JWKSURI = url
	}

	if len(fed.OpenIDConfigURL) > 0 && !fed.OfflineValidation {
		url, err := lib.ValidateURL(fed.OpenIDConfigURL)
		if err != nil {
			return errors.New(nil).WithCode(errors.BadRequestCode).
				WithMessage("invalid openid_config_url")
		}
		fed.OpenIDConfigURL = url
	}

	if fed.OfflineValidation {
		// check for jwks_keys
		if len(fed.JWKSKeys) == 0 {
			return errors.New(nil).WithCode(errors.BadRequestCode).
				WithMessage("invalid jwks_keys")
		}
	}
	// Validate Project ID
	if fed.ProjectID < 0 {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("project_id must be set (use 0 for system level)")
	}

	return nil
}

func (c *controller) Count(ctx context.Context, query *q.Query) (int64, error) {
	return c.fidpMgr.Count(ctx, query)
}

func (c *controller) List(ctx context.Context, query *q.Query) ([]*model.FederatedIdp, error) {
	return c.fidpMgr.List(ctx, query)
}

func (c *controller) Get(ctx context.Context, id int64) (*model.FederatedIdp, error) {
	return c.fidpMgr.Get(ctx, id)
}

func (c *controller) GetIdpByIssuer(ctx context.Context, issuer string) (*model.FederatedIdp, error) {
	if !commercial.Enabled(ctx, commercial.IdentityProviders) {
		return nil, errors.ForbiddenError(nil).WithMessage("commercial feature identity_providers is not enabled")
	}
	// Issuers are stored normalized without a trailing slash (lib.ValidateURL),
	// but some providers (e.g. AKS) mint tokens whose iss claim keeps it.
	return c.fidpMgr.GetIdpByIssuer(ctx, strings.TrimRight(issuer, "/"))
}

func (c *controller) GetTopMatchedRobot(ctx context.Context, issuerID int64, tokenClaims jwt.MapClaims) (int64, error) {
	if !commercial.Enabled(ctx, commercial.IdentityProviders) {
		return 0, errors.ForbiddenError(nil).WithMessage("commercial feature identity_providers is not enabled")
	}
	return c.fidpMgr.GetTopMatchedRobot(ctx, issuerID, tokenClaims)
}

func (c *controller) Update(ctx context.Context, registry *model.FederatedIdp, props ...string) error {
	if err := c.validate(ctx, registry); err != nil {
		return err
	}
	// Check uniqueness of name and issuer across all IdPs (excluding self)
	if err := c.validateUniqueness(ctx, registry, true); err != nil {
		return err
	}
	if err := c.fidpMgr.Update(ctx, registry, props...); err != nil {
		return err
	}

	// For online IdPs, force refresh JWKS cache on any edit
	// This ensures key rotations are picked up immediately when users update IdP config
	if !registry.OfflineValidation && registry.JWKSURI != "" {
		if refreshErr := jwkscache.NewManager().ForceRefresh(ctx, registry.ID, registry.JWKSURI); refreshErr != nil {
			log.Warningf("Failed to refresh JWKS cache for IdP %d: %v", registry.ID, refreshErr)
			// Don't fail the update - JWKS will be refreshed on next validation
		}
	}

	return nil
}

func (c *controller) Delete(ctx context.Context, id int64) error {
	return c.fidpMgr.Delete(ctx, id)
}

func (c *controller) DeleteByProjectID(ctx context.Context, projectID int64) error {
	return c.fidpMgr.DeleteByProjectID(ctx, projectID)
}

func (c *controller) ListClaims(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error) {
	return c.fidpMgr.ListClaims(ctx, id, claimPath)
}

func (c *controller) ListClaimsIdpOnly(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error) {
	if !commercial.Enabled(ctx, commercial.IdentityProviders) {
		return nil, errors.ForbiddenError(nil).WithMessage("commercial feature identity_providers is not enabled")
	}
	return c.fidpMgr.ListClaimsIdpOnly(ctx, id, claimPath)
}

func (c *controller) ListClaimsWithFilters(ctx context.Context, idpID int64, claimPath string, robotID *int64, idpOnly bool) ([]model.ClaimRule, error) {
	return c.fidpMgr.ListClaimsWithFilters(ctx, idpID, claimPath, robotID, idpOnly)
}

func (c *controller) CreateClaims(ctx context.Context, idpID int64, claims []model.ClaimRule) error {
	if err := validateIdentityClaims(claims); err != nil {
		return err
	}
	// Validate that robot claims don't exactly match another robot's claims
	if err := c.validateRobotClaims(ctx, idpID, claims); err != nil {
		return err
	}
	return c.fidpMgr.CreateClaims(ctx, idpID, claims)
}

// validateIdentityClaims ensures that each immutable identity claim is set at
// most once in a request.
func validateIdentityClaims(claims []model.ClaimRule) error {
	seen := make(map[string]struct{}, 2)
	for _, claim := range claims {
		if claim.RobotID > 0 || (claim.ClaimPath != "iss" && claim.ClaimPath != "aud") {
			continue
		}
		if _, ok := seen[claim.ClaimPath]; ok {
			return errors.New(nil).WithCode(errors.ConflictCode).
				WithMessagef("identity claim %q may only be specified once", claim.ClaimPath)
		}
		seen[claim.ClaimPath] = struct{}{}
	}
	return nil
}

// validateRobotClaims ensures that the new robot's claims don't exactly match
// any existing robot's claims within the same IdP. At least one claim must be different.
func (c *controller) validateRobotClaims(ctx context.Context, idpID int64, newClaims []model.ClaimRule) error {
	// If no claims or all claims are IdP-level (robot_id = 0), skip validation
	var robotClaims []model.ClaimRule
	var newRobotID int64 = -1
	for _, claim := range newClaims {
		if claim.RobotID > 0 {
			robotClaims = append(robotClaims, claim)
			if newRobotID == -1 {
				newRobotID = claim.RobotID
			}
		}
	}
	if len(robotClaims) == 0 {
		return nil // No robot-specific claims to validate
	}

	// Get all existing claims for this IdP
	existingClaims, err := c.fidpMgr.ListClaims(ctx, idpID, "")
	if err != nil {
		return errors.Wrap(err, "failed to list claims for validation")
	}

	// Group existing claims by robot_id and include the target robot's current
	// claims when comparing the final claim set after this request.
	robotClaimSets := make(map[int64]map[string]string) // robot_id -> map[claim_path]value
	newClaimSet := make(map[string]string)
	for _, claim := range existingClaims {
		if claim.RobotID <= 0 {
			continue
		}
		if claim.RobotID == newRobotID {
			newClaimSet[claim.ClaimPath] = claim.Value
			continue
		}
		if _, ok := robotClaimSets[claim.RobotID]; !ok {
			robotClaimSets[claim.RobotID] = make(map[string]string)
		}
		robotClaimSets[claim.RobotID][claim.ClaimPath] = claim.Value
	}

	for _, claim := range robotClaims {
		newClaimSet[claim.ClaimPath] = claim.Value
	}

	// Check against each existing robot's claims
	for existingRobotID, existingClaimSet := range robotClaimSets {
		if claimSetsMatch(newClaimSet, existingClaimSet) {
			return errors.New(nil).WithCode(errors.ConflictCode).
				WithMessagef("robot claims exactly match existing robot (ID: %d). At least one claim must be different to uniquely identify this robot.", existingRobotID)
		}
	}

	return nil
}

// claimSetsMatch returns true if both claim sets contain exactly the same claim paths and values
func claimSetsMatch(set1, set2 map[string]string) bool {
	if len(set1) != len(set2) {
		return false
	}
	for path, value := range set1 {
		if otherValue, ok := set2[path]; !ok || otherValue != value {
			return false
		}
	}
	return true
}

func (c *controller) DeleteClaims(ctx context.Context, claims []model.ClaimRule) error {
	return c.fidpMgr.DeleteClaims(ctx, claims)
}

func (c *controller) CreateRobotIdp(ctx context.Context, idpID, robotID int64) (int64, error) {
	idp := &model.RobotIdentityProvider{
		IdentityProviderID: idpID,
		RobotID:            robotID,
	}
	return c.fidpMgr.CreateRobotIdp(ctx, idp)
}

func (c *controller) DeleteRobotIdpByIdpID(ctx context.Context, idpID int64) error {
	return c.fidpMgr.DeleteRobotIdpByIdpID(ctx, idpID)
}

func (c *controller) DeleteRobotIdpByRobotID(ctx context.Context, robotID int64) error {
	return c.fidpMgr.DeleteRobotIdpByRobotID(ctx, robotID)
}

func (c *controller) HasRobotIdpByRobotID(ctx context.Context, robotID int64) (bool, error) {
	return c.fidpMgr.HasRobotIdpByRobotID(ctx, robotID)
}

func (c *controller) ListRobotIdpByIdpID(ctx context.Context, idpID int64) ([]model.RobotIdentityProvider, error) {
	return c.fidpMgr.ListRobotIdpByIdpID(ctx, idpID)
}

func (c *controller) DeleteClaimRulesByRobotID(ctx context.Context, robotID int64) error {
	return c.fidpMgr.DeleteClaimRulesByRobotID(ctx, robotID)
}

func (c *controller) GetIdpIDByRobotID(ctx context.Context, robotID int64) (int64, error) {
	return c.fidpMgr.GetIdpIDByRobotID(ctx, robotID)
}
