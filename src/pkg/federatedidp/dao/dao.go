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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
)

// DAO defines the interface to access the federatedidp data model
type DAO interface {
	// Create ...
	Create(ctx context.Context, r *model.FederatedIdp) (int64, error)

	// Update ...
	Update(ctx context.Context, r *model.FederatedIdp, props ...string) error

	// Get ...
	Get(ctx context.Context, id int64) (*model.FederatedIdp, error)

	// GetIdpByIssuer ...
	GetIdpByIssuer(ctx context.Context, issuer string) (*model.FederatedIdp, error)

	// FindMatchingRobot finds robots where ALL robot claims are present in the token.
	// Returns the robot with the most claims (most specific match).
	FindMatchingRobot(ctx context.Context, issuerID int64, tokenClaims jwt.MapClaims) (int64, error)

	// Count returns the total count of federatedidps according to the query
	Count(ctx context.Context, query *q.Query) (total int64, err error)

	// List ...
	List(ctx context.Context, query *q.Query) ([]*model.FederatedIdp, error)

	// Delete ...
	Delete(ctx context.Context, id int64) error

	// DeleteByProjectID ...
	DeleteByProjectID(ctx context.Context, projectID int64) error

	// ListClaims ...
	ListClaims(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error)

	// ListClaimsIdpOnly ...
	ListClaimsIdpOnly(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error)

	// ListClaimsWithFilters lists claims with flexible filtering options
	// - robotID: if >= 0, filter by robot_id; if nil, no robot filtering
	// - idpOnly: if true, only return IdP-level claims (robot_id = 0)
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

	// UpdateJWKSCache updates the cached JWKS keys and timestamps
	UpdateJWKSCache(ctx context.Context, id int64, jwksKeys string, cachedAt, expiresAt time.Time) error

	// UpdateJWKSFetchAttempt records when a fetch was attempted (for rate limiting)
	UpdateJWKSFetchAttempt(ctx context.Context, id int64, attemptTime time.Time) error
}

// New creates a default implementation for Dao
func New() DAO {
	return &dao{}
}

type dao struct{}

func (d *dao) Create(ctx context.Context, f *model.FederatedIdp) (int64, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}
	f.CreationTime = time.Now()
	id, err := ormer.Insert(f)
	if err != nil {
		// Check for unique constraint violation (issuer already exists)
		// log.L.Debug()
		log.Debugf("error in adding federated idp: %v", err)
		return 0, orm.WrapConflictError(err, "federated idp %d:%s already exists, error: %v", f.ProjectID, f.Name, err)
	}
	return id, err
}

func (d *dao) Update(ctx context.Context, f *model.FederatedIdp, props ...string) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}
	n, err := ormer.Update(f, props...)
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.NotFoundError(nil).WithMessagef("federatedidp %d not found", f.ID)
	}
	return nil
}

func (d *dao) Get(ctx context.Context, id int64) (*model.FederatedIdp, error) {
	f := &model.FederatedIdp{
		ID: id,
	}
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := ormer.Read(f); err != nil {
		return nil, orm.WrapNotFoundError(err, "federatedidp %d not found", id)
	}
	return f, nil
}

func (d *dao) GetIdpByIssuer(ctx context.Context, issuer string) (*model.FederatedIdp, error) {
	f := &model.FederatedIdp{
		Issuer: issuer,
	}
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := ormer.Read(f, "issuer"); err != nil {
		return nil, orm.WrapNotFoundError(err, "federatedidp with issuer: %s not found", issuer)
	}
	return f, nil
}

func (d *dao) Count(ctx context.Context, query *q.Query) (int64, error) {
	qs, err := orm.QuerySetterForCount(ctx, &model.FederatedIdp{}, query)
	if err != nil {
		return 0, err
	}
	return qs.Count()
}

func (d *dao) Delete(ctx context.Context, id int64) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}
	n, err := ormer.Delete(&model.FederatedIdp{
		ID: id,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.NotFoundError(nil).WithMessagef("federatedidp %d not found", id)
	}
	return nil
}

func (d *dao) List(ctx context.Context, query *q.Query) ([]*model.FederatedIdp, error) {
	federatedidps := []*model.FederatedIdp{}

	qs, err := orm.QuerySetter(ctx, &model.FederatedIdp{}, query)
	if err != nil {
		return nil, err
	}
	if _, err = qs.All(&federatedidps); err != nil {
		return nil, err
	}
	return federatedidps, nil
}

func (d *dao) DeleteByProjectID(ctx context.Context, projectID int64) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	_, err = ormer.Raw("DELETE FROM identity_providers WHERE project_id = ?", projectID).Exec()

	return err
}

func (d *dao) ListClaims(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	qs := ormer.QueryTable(new(model.ClaimRule)).Filter("identity_provider_id", id)

	if claimPath != "" {
		qs = qs.Filter("claim_path", claimPath)
	}

	var rules []model.ClaimRule
	_, err = qs.All(&rules)
	return rules, err
}

func (d *dao) ListClaimsIdpOnly(ctx context.Context, id int64, claimPath string) ([]model.ClaimRule, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	qs := ormer.QueryTable(new(model.ClaimRule)).Filter("identity_provider_id", id).Filter("robot_id", 0)

	if claimPath != "" {
		qs = qs.Filter("claim_path", claimPath)
	}

	var rules []model.ClaimRule
	_, err = qs.All(&rules)
	return rules, err
}

// ListClaimsWithFilters lists claims with flexible filtering options
func (d *dao) ListClaimsWithFilters(ctx context.Context, idpID int64, claimPath string, robotID *int64, idpOnly bool) ([]model.ClaimRule, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	qs := ormer.QueryTable(new(model.ClaimRule)).Filter("identity_provider_id", idpID)

	// Apply robot filtering
	if idpOnly {
		// Only IdP-level claims (robot_id = 0)
		qs = qs.Filter("robot_id", 0)
	} else if robotID != nil {
		// Filter by specific robot_id (0 = IdP-level, >0 = specific robot)
		qs = qs.Filter("robot_id", *robotID)
	}
	// If neither idpOnly nor robotID specified, return all claims

	// Apply claim_path filter
	if claimPath != "" {
		qs = qs.Filter("claim_path", claimPath)
	}

	var rules []model.ClaimRule
	_, err = qs.All(&rules)
	return rules, err
}

// CreateClaims inserts multiple ClaimRule records into the DB
func (d *dao) CreateClaims(ctx context.Context, idpID int64, claims []model.ClaimRule) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	if len(claims) == 0 {
		return nil // nothing to insert
	}

	// validate if the claims are unique
	if err := d.validateUniqueClaims(ctx, idpID, claims); err != nil {
		return err
	}

	// InsertMulti takes (bulkSize, slice)
	_, err = ormer.InsertMulti(len(claims), claims)
	return err
}

// DeleteClaims deletes claim rules using classic deletion behavior:
// - Deletes claims that exist
// - Silently ignores claims that don't exist
// - Returns error only on actual database errors
func (d *dao) DeleteClaims(ctx context.Context, claims []model.ClaimRule) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	for _, claim := range claims {
		// Validate required fields
		if claim.IdentityProviderID == 0 || claim.ClaimPath == "" {
			return errors.New(nil).WithCode(errors.BadRequestCode).
				WithMessage("invalid claim rule: missing identity_provider_id or claim_path")
		}

		qs := ormer.QueryTable(new(model.ClaimRule)).
			Filter("identity_provider_id", claim.IdentityProviderID).
			Filter("claim_path", claim.ClaimPath)

		// If robot_id is provided, filter by it too
		if claim.RobotID > 0 {
			qs = qs.Filter("robot_id", claim.RobotID)
		} else {
			// If robot_id is 0, we're deleting IdP-level claims
			qs = qs.Filter("robot_id", 0)
		}

		// If value is provided, also filter by value
		if claim.Value != "" {
			qs = qs.Filter("value", claim.Value)
		}

		// Delete matching claims - ignore if nothing found (classic behavior)
		_, err := qs.Delete()
		if err != nil {
			return err
		}
	}
	return nil
}

// FindMatchingRobot finds robots where ALL robot claims are present in the token.
// Returns the robot with the most claims (most specific match).
// Rules:
// 1. ALL robot claims must be present in the token (token may have extra claims)
// 2. IDP-level claims (robot_id = 0) are NOT considered - they're validated separately
// 3. Robots with zero claims are NOT eligible for matching
// 4. Among fully matching robots, the one with the most claims wins
func (d *dao) FindMatchingRobot(ctx context.Context, issuerID int64, tokenClaims jwt.MapClaims) (int64, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}

	if len(tokenClaims) == 0 {
		return 0, fmt.Errorf("token has no claims to match")
	}

	var claimRules []model.ClaimRule
	_, err = ormer.QueryTable(new(model.ClaimRule)).
		Filter("identity_provider_id", issuerID).
		Filter("robot_id__gt", 0).
		All(&claimRules)
	if err != nil {
		return 0, fmt.Errorf("failed to list robot claims: %w", err)
	}

	tokenValues := flattenClaimValues(tokenClaims)
	robotClaims := make(map[int64][]model.ClaimRule)
	for _, rule := range claimRules {
		robotClaims[rule.RobotID] = append(robotClaims[rule.RobotID], rule)
	}

	var matchedRobotID int64
	matchedClaimCount := 0
	for robotID, rules := range robotClaims {
		if len(rules) == 0 || !allClaimsMatch(tokenValues, rules) {
			continue
		}
		if len(rules) > matchedClaimCount || (len(rules) == matchedClaimCount && (matchedRobotID == 0 || robotID < matchedRobotID)) {
			matchedRobotID = robotID
			matchedClaimCount = len(rules)
		}
	}

	if matchedRobotID == 0 {
		log.Debugf("no robots matched your token for issuerID=%d", issuerID)
		return 0, nil
	}

	log.Debugf("FindMatchingRobot found robot_id=%d with %d claims matched", matchedRobotID, matchedClaimCount)
	return matchedRobotID, nil
}

func allClaimsMatch(tokenValues map[string][]string, rules []model.ClaimRule) bool {
	for _, rule := range rules {
		if !claimValueMatches(tokenValues[rule.ClaimPath], rule.Value) {
			return false
		}
	}
	return true
}

func claimValueMatches(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func flattenClaimValues(claims jwt.MapClaims) map[string][]string {
	values := make(map[string][]string)
	for key, value := range claims {
		flattenClaimValue(key, value, values)
	}
	return values
}

func flattenClaimValue(path string, value any, values map[string][]string) {
	switch v := value.(type) {
	case jwt.MapClaims:
		for key, nested := range v {
			flattenClaimValue(joinClaimPath(path, key), nested, values)
		}
	case map[string]any:
		for key, nested := range v {
			flattenClaimValue(joinClaimPath(path, key), nested, values)
		}
	case []string:
		for _, item := range v {
			values[path] = append(values[path], fmt.Sprintf("%v", item))
		}
	case []any:
		for _, item := range v {
			switch item.(type) {
			case jwt.MapClaims, map[string]any, []any, []string:
				continue
			default:
				values[path] = append(values[path], fmt.Sprintf("%v", item))
			}
		}
	default:
		values[path] = append(values[path], fmt.Sprintf("%v", v))
	}
}

func joinClaimPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// CreateRobotIdentityProvider creates a new RobotIdentityProvider record
func (d *dao) CreateRobotIdp(ctx context.Context, r *model.RobotIdentityProvider) (int64, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}
	r.CreationTime = time.Now()
	id, err := ormer.Insert(r)
	if err != nil {
		return 0, orm.WrapConflictError(err, "robot identity provider %d:%d already exists", r.RobotID, r.IdentityProviderID)
	}
	return id, err
}

// DeleteRobotIdentityProvider deletes a RobotIdentityProvider record
func (d *dao) DeleteRobotIdpByIdpID(ctx context.Context, idpID int64) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	_, err = ormer.Raw("DELETE FROM robot_identity_providers WHERE identity_provider_id = ?", idpID).Exec()

	return err
}

// DeleteRobotIdpByRobotID deletes a RobotIdentityProvider record
func (d *dao) DeleteRobotIdpByRobotID(ctx context.Context, robotID int64) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	_, err = ormer.Raw("DELETE FROM robot_identity_providers WHERE robot_id = ?", robotID).Exec()

	return err
}

// DeleteClaimRulesByRobotID deletes all claim_rules records associated with a given robot ID
func (d *dao) DeleteClaimRulesByRobotID(ctx context.Context, robotID int64) error {
	// Retrieve ORM instance from context
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err // return if ORM context retrieval fails
	}

	// Delete all claim rules linked to the given robot_id
	_, err = ormer.Raw("DELETE FROM claim_rules WHERE robot_id = ?", robotID).Exec()

	return err // return any SQL execution error
}

// HasRobotIdp checks if a given robot has at least one associated identity provider.
func (d *dao) HasRobotIdpByRobotID(ctx context.Context, robotID int64) (bool, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return false, err
	}

	// Only need to know if at least one record exists
	exists := ormer.QueryTable(new(model.RobotIdentityProvider)).
		Filter("robot_id", robotID).
		Exist()

	return exists, nil
}

// lists all robot_identity_providers associated with the given IDP ID
func (d *dao) ListRobotIdpByIdpID(ctx context.Context, idpID int64) ([]model.RobotIdentityProvider, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	var robotIDPs []model.RobotIdentityProvider

	// Fetch all robot IDP records matching the given IDP ID
	_, err = ormer.Raw(
		"SELECT * FROM robot_identity_providers WHERE identity_provider_id = ?",
		idpID,
	).QueryRows(&robotIDPs)

	if err != nil {
		return nil, err
	}

	return robotIDPs, nil
}

// GetIdpIDByRobotID returns the identity provider ID for a given robot ID
func (d *dao) GetIdpIDByRobotID(ctx context.Context, robotID int64) (int64, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}

	var idpID int64
	err = ormer.Raw(
		"SELECT identity_provider_id FROM robot_identity_providers WHERE robot_id = ?",
		robotID,
	).QueryRow(&idpID)

	if err == orm.ErrNoRows {
		return 0, nil // No IDP associated with this robot
	}
	if err != nil {
		return 0, err
	}

	return idpID, nil
}

// validateUniqueClaims ensures that claim rules being inserted do not violate
// uniqueness constraints across identity providers and robots.
func (d *dao) validateUniqueClaims(ctx context.Context, idpID int64, claims []model.ClaimRule) error {
	if len(claims) == 0 {
		return nil
	}

	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	// 1️⃣ Check for duplicates in the input batch itself
	type key struct {
		IDP     int64
		RobotID int64
		Path    string
		Value   string
	}
	seen := make(map[key]struct{})
	for _, c := range claims {
		k := key{IDP: c.IdentityProviderID, RobotID: c.RobotID, Path: c.ClaimPath, Value: c.Value}
		if _, ok := seen[k]; ok {
			return errors.New(nil).WithCode(errors.ConflictCode).
				WithMessagef("duplicate claim in input batch for claim_path=%s", c.ClaimPath)
		}
		seen[k] = struct{}{}
	}

	// 2️⃣ Collect all identity_provider IDs and claim paths
	var claimPaths []string
	for _, c := range claims {
		claimPaths = append(claimPaths, c.ClaimPath)
	}

	// 3️⃣ Query existing claims in DB that might conflict
	existingClaims := []model.ClaimRule{}
	_, err = ormer.QueryTable(new(model.ClaimRule)).
		Filter("identity_provider_id", idpID).
		Filter("claim_path__in", claimPaths).
		All(&existingClaims)
	if err != nil {
		return fmt.Errorf("failed to query existing claims: %w", err)
	}

	// 4️⃣ Validate according to rules
	for _, c := range claims {
		for _, e := range existingClaims {
			if c.IdentityProviderID != e.IdentityProviderID || c.ClaimPath != e.ClaimPath {
				continue
			}

			// IDP-level claims (RobotID == 0) can always be added freely.
			// They don't conflict with robot-specific claims.
			if c.RobotID == 0 {
				// Only check for exact IDP-level duplicate (same path, same value, both IDP-level)
				if e.RobotID == 0 && c.Value == e.Value {
					return errors.New(nil).WithCode(errors.ConflictCode).
						WithMessagef("duplicate IDP-level claim for claim_path '%s' and value '%s'", c.ClaimPath, c.Value)
				}
				// IDP claims don't conflict with robot claims, skip other checks
				continue
			}

			// From here, c.RobotID > 0 (robot-specific claim being added)

			// RULE 1: Robot claims cannot override IDP-level claims
			if e.RobotID == 0 {
				return errors.New(nil).WithCode(errors.ConflictCode).
					WithMessagef("claim_path '%s' already owned by identity provider %d; cannot be overridden by robot", c.ClaimPath, e.IdentityProviderID)
			}

			// Overlapping individual claims are allowed across robots; exact
			// duplicate robot claim sets are rejected in validateRobotClaims.
			if c.RobotID == e.RobotID {
				return errors.New(nil).WithCode(errors.ConflictCode).
					WithMessagef("robot %d already has claim_path '%s'", c.RobotID, c.ClaimPath)
			}
		}
	}

	return nil
}

// UpdateJWKSCache updates the cached JWKS keys and timestamps
func (d *dao) UpdateJWKSCache(ctx context.Context, id int64, jwksKeys string, cachedAt, expiresAt time.Time) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	_, err = ormer.Raw(`
		UPDATE identity_providers
		SET jwks_keys = ?, jwks_cached_at = ?, jwks_expires_at = ?, update_time = NOW()
		WHERE id = ?
	`, jwksKeys, cachedAt, expiresAt, id).Exec()

	return err
}

// UpdateJWKSFetchAttempt records when a fetch was attempted (for rate limiting)
func (d *dao) UpdateJWKSFetchAttempt(ctx context.Context, id int64, attemptTime time.Time) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	_, err = ormer.Raw(`
		UPDATE identity_providers
		SET jwks_last_fetch_attempt = ?, update_time = NOW()
		WHERE id = ?
	`, attemptTime, id).Exec()

	return err
}
