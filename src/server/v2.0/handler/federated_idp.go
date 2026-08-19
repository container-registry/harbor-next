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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-openapi/runtime/middleware"

	"github.com/goharbor/harbor/src/common/rbac"

	"github.com/goharbor/harbor/src/controller/federatedidp"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/commercial"
	pkg "github.com/goharbor/harbor/src/pkg/federatedidp/model"
	"github.com/goharbor/harbor/src/server/v2.0/handler/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/federated_idp"
)

func newFederatedIDPAPI() *fedIDPAPI {
	return &fedIDPAPI{
		fedidpCtl: federatedidp.Ctl,
	}
}

type fedIDPAPI struct {
	BaseAPI
	fedidpCtl federatedidp.Controller
}

// ListClaimRules supports flexible filtering:
// - No params: returns all claims for the IdP
// - idp_only=true: returns only IdP-level claims (robot_id=0)
// - robot_id=0: returns only IdP-level claims
// - robot_id=N (N>0): returns only claims for robot N
// - claim_path=X: filters by claim path
func (fAPI *fedIDPAPI) ListClaimRules(ctx context.Context, params operation.ListClaimRulesParams) middleware.Responder {
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	f, err := fAPI.fedidpCtl.Get(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.requireAccess(ctx, f, rbac.ActionList); err != nil {
		return fAPI.SendError(ctx, err)
	}

	// Validate that idp_only and robot_id are not both specified
	if params.IdpOnly != nil && *params.IdpOnly && params.RobotID != nil {
		return fAPI.SendError(ctx, errors.BadRequestError(nil).
			WithMessage("cannot specify both idp_only and robot_id parameters"))
	}

	var claimPath string
	if params.ClaimPath != nil && *params.ClaimPath != "" {
		claimPath = *params.ClaimPath
	}

	// Determine filtering mode
	idpOnly := params.IdpOnly != nil && *params.IdpOnly

	claims, err := fAPI.fedidpCtl.ListClaimsWithFilters(ctx, params.ID, claimPath, params.RobotID, idpOnly)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	var results []*models.ClaimRule
	for _, c := range claims {
		results = append(results, model.NewClaimRule(&c).ToSwagger())
	}

	return operation.NewListClaimRulesOK().
		WithPayload(results)
}

// DeleteClaimRule
func (fAPI *fedIDPAPI) DeleteClaimRules(ctx context.Context, params operation.DeleteClaimRulesParams) middleware.Responder {
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	f, err := fAPI.fedidpCtl.Get(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.requireAccess(ctx, f, rbac.ActionDelete); err != nil {
		return fAPI.SendError(ctx, err)
	}

	if len(params.Claims.Rules) == 0 {
		return fAPI.SendError(ctx, errors.New(nil).WithMessage("no claim rules provided").WithCode(errors.BadRequestCode))
	}

	var rules []pkg.ClaimRule
	for _, c := range params.Claims.Rules {
		rules = append(rules, model.FromSwagger(c))
	}

	if err = fAPI.fedidpCtl.DeleteClaims(ctx, rules); err != nil {
		return fAPI.SendError(ctx, err)
	}

	return operation.NewDeleteClaimRulesOK()
}

func (fAPI *fedIDPAPI) CreateClaimRules(ctx context.Context, params operation.CreateClaimRulesParams) middleware.Responder {
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	f, err := fAPI.fedidpCtl.Get(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.requireAccess(ctx, f, rbac.ActionCreate); err != nil {
		return fAPI.SendError(ctx, err)
	}

	if len(params.Claims.Rules) == 0 {
		return fAPI.SendError(ctx, errors.New(nil).WithMessage("no claim rules provided").WithCode(errors.BadRequestCode))
	}

	// Parse claims_supported from the IdP (comma-separated string)
	var claimsSupported []string
	if f.ClaimsSupported != "" {
		for _, claim := range strings.Split(f.ClaimsSupported, ",") {
			trimmed := strings.TrimSpace(claim)
			if trimmed != "" {
				claimsSupported = append(claimsSupported, trimmed)
			}
		}
	}

	// Validate each claim rule against claims_supported
	for i, c := range params.Claims.Rules {
		if err := validateClaimRule(c, i, claimsSupported); err != nil {
			return fAPI.SendError(ctx, err)
		}
	}

	var rules []pkg.ClaimRule
	for _, c := range params.Claims.Rules {
		rules = append(rules, model.FromSwagger(c))
	}

	if err = fAPI.fedidpCtl.CreateClaims(ctx, params.ID, rules); err != nil {
		return fAPI.SendError(ctx, err)
	}

	return operation.NewCreateClaimRulesCreated()
}

func (fAPI *fedIDPAPI) CreateFederatedIdp(ctx context.Context, params operation.CreateFederatedIdpParams) middleware.Responder {
	var (
		jwksKeys            string
		supportedAlgorithms string
		claimsSupported     string
	)

	if err := fAPI.validate(params.Idp); err != nil {
		return fAPI.SendError(ctx, err)
	}

	query := q.New(q.KeyWords{"name": params.Idp.Name})
	existing, err := fAPI.fedidpCtl.List(ctx, query)
	if err != nil {
		log.Errorf("failed to validate federatedidp name: %v", err)
		err := errors.New(err).WithMessagef(
			"failed to validate federatedidp name",
		).WithCode(errors.PreconditionCode)
		return fAPI.SendError(ctx, err)
	}

	if len(existing) > 0 {
		err := errors.
			ConflictError(nil).
			WithMessage("federatedidp with this name already exists")
		return fAPI.SendError(ctx, err)
	}

	if params.Idp.OfflineValidation {
		// assign jwksKeys
		jwksKeys = string(toRawMessage(params.Idp.JwksKeys))
	}

	// idp optional params validation
	if len(params.Idp.SupportedAlgorithms) > 0 {
		supportedAlgorithms = strings.Join(params.Idp.SupportedAlgorithms, ",")
	}
	if len(params.Idp.ClaimsSupported) > 0 {
		claimsSupported = strings.Join(params.Idp.ClaimsSupported, ",")
	}

	fIdp := &pkg.FederatedIdp{
		Name:                params.Idp.Name,
		Description:         params.Idp.Description,
		Issuer:              params.Idp.Issuer,
		OpenIDConfigURL:     params.Idp.OpenidConfigURL,
		JWKSURI:             params.Idp.JwksURI,
		JWKSKeys:            jwksKeys,
		OfflineValidation:   params.Idp.OfflineValidation,
		SupportedAlgorithms: supportedAlgorithms,
		ClaimsSupported:     claimsSupported,
		ProjectID:           params.Idp.ProjectID,
		CreationTime:        time.Now(),
		UpdateTime:          time.Now(),
	}

	if err := fAPI.requireAccess(ctx, fIdp, rbac.ActionCreate); err != nil {
		return fAPI.SendError(ctx, err)
	}

	fedIdpID, err := fAPI.fedidpCtl.Create(ctx, fIdp)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	created, err := fAPI.fedidpCtl.Get(ctx, fedIdpID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	return operation.NewCreateFederatedIdpCreated().WithPayload(model.NewFederatedIdp(created).ToSwagger())
}

func (fAPI *fedIDPAPI) DeleteFederatedIdp(ctx context.Context, params operation.DeleteFederatedIdpParams) middleware.Responder {
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	f, err := fAPI.fedidpCtl.Get(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	if err := fAPI.requireAccess(ctx, f, rbac.ActionDelete); err != nil {
		return fAPI.SendError(ctx, err)
	}

	robotIdps, err := fAPI.fedidpCtl.ListRobotIdpByIdpID(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	if len(robotIdps) > 0 {
		return fAPI.SendError(ctx, errors.New(nil).WithMessage("Please delete the associated robots before deleting the federated idp").WithCode(errors.BadRequestCode))
	}

	// for _, robotIdp := range robotIdps {
	// 	if err := fAPI.fedidpCtl.DeleteRobotIdpByRobotID(ctx, robotIdp.RobotID); err != nil {
	// 		return fAPI.SendError(ctx, err)
	// 	}
	// }

	if err := fAPI.fedidpCtl.Delete(ctx, params.ID); err != nil {
		return fAPI.SendError(ctx, err)
	}
	return operation.NewDeleteFederatedIdpOK()
}

func (fAPI *fedIDPAPI) ListFederatedIdps(ctx context.Context, params operation.ListFederatedIdpsParams) middleware.Responder {
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	query, err := fAPI.BuildQuery(ctx, params.Q, params.Sort, params.Page, params.PageSize)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	var projectID int64
	// GET /api/v2.0/federated-idps or GET /api/v2.0/federated-idps?q=Level=system to get all system level federated idps.
	// GET /api/v2.0/federated-idps?q=Level=project,ProjectID=1 to get project level federated idps.
	if levelVal, ok := query.Keywords["Level"]; ok {
		level := levelVal.(string)
		if !isValidLevel(level) {
			return fAPI.SendError(ctx, errors.BadRequestError(nil).WithMessage("invalid level: must be 'system' or 'project'"))
		}
		if level == federatedidp.LEVELPROJECT {
			if _, ok := query.Keywords["ProjectID"]; !ok {
				return fAPI.SendError(ctx, errors.BadRequestError(nil).WithMessage("ProjectID is required when Level=project"))
			}
			pid, err := strconv.ParseInt(query.Keywords["ProjectID"].(string), 10, 64)
			if err != nil || pid <= 0 {
				return fAPI.SendError(ctx, errors.BadRequestError(nil).WithMessage("ProjectID must be a positive integer"))
			}
			projectID = pid
			query.Keywords["ProjectID"] = projectID
		} else {
			// Level=system
			projectID = 0
			query.Keywords["ProjectID"] = 0
		}
	} else {
		// Default to system level when Level not specified
		projectID = 0
		query.Keywords["ProjectID"] = 0
	}

	f := &pkg.FederatedIdp{
		ProjectID: projectID,
	}
	if err := fAPI.requireAccess(ctx, f, rbac.ActionList); err != nil {
		return fAPI.SendError(ctx, err)
	}

	total, err := fAPI.fedidpCtl.Count(ctx, query)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	fIdps, err := fAPI.fedidpCtl.List(ctx, query)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	var results []*models.FederatedIdp
	for _, f := range fIdps {
		results = append(results, model.NewFederatedIdp(f).ToSwagger())
	}

	return operation.NewListFederatedIdpsOK().
		WithXTotalCount(total).
		WithLink(fAPI.Links(ctx, params.HTTPRequest.URL, total, query.PageNumber, query.PageSize).String()).
		WithPayload(results)
}

func (fAPI *fedIDPAPI) PingFederatedIdpJWKS(ctx context.Context, params operation.PingFederatedIdpJWKSParams) middleware.Responder {
	if err := fAPI.requireCommercialFeature(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	// Only require authentication for ping endpoints - they test external URLs
	// and don't access Harbor data. Both system admins and project admins
	// need to use these when creating federated IDPs.

	url, err := lib.ValidateURL(params.Jwks.JwksURI)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	// fetch jwks from url and return the jwks json
	resp, err := http.Get(url)
	if err != nil {
		return fAPI.SendError(ctx, fmt.Errorf("failed to fetch JWKS: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fAPI.SendError(ctx, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode))
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fAPI.SendError(ctx, fmt.Errorf("failed to read JWKS response: %v", err))
	}

	// Return raw JSON directly
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fAPI.SendError(ctx, fmt.Errorf("invalid JSON from JWKS endpoint: %v", err))
	}

	return operation.NewPingFederatedIdpJWKSOK().WithPayload(payload)
}

func (fAPI *fedIDPAPI) PingFederatedIdpOpenIDConfig(ctx context.Context, params operation.PingFederatedIdpOpenIDConfigParams) middleware.Responder {
	if err := fAPI.requireCommercialFeature(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	// Only require authentication for ping endpoints - they test external URLs
	// and don't access Harbor data. Both system admins and project admins
	// need to use these when creating federated IDPs.

	url, err := lib.ValidateURL(params.OpenidConfigURL.OpenidConfigURL)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	// Ensure the URL points to a proper OpenID configuration endpoint
	if !strings.HasSuffix(url, "/.well-known/openid-configuration") {
		return fAPI.SendError(ctx, fmt.Errorf("URL must end with '/.well-known/openid-configuration'"))
	}

	// Fetch OpenID Config JSON
	resp, err := http.Get(url)
	if err != nil {
		return fAPI.SendError(ctx, fmt.Errorf("failed to fetch OpenID configuration: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fAPI.SendError(ctx, fmt.Errorf("OpenID configuration endpoint returned %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fAPI.SendError(ctx, fmt.Errorf("failed to read OpenID configuration response: %v", err))
	}

	// Return raw JSON directly
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fAPI.SendError(ctx, fmt.Errorf("invalid JSON from OpenID configuration endpoint: %v", err))
	}

	return operation.NewPingFederatedIdpOpenIDConfigOK().WithPayload(payload)
}

func (fAPI *fedIDPAPI) GetFederatedIdp(ctx context.Context, params operation.GetFederatedIdpParams) middleware.Responder {
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}

	f, err := fAPI.fedidpCtl.Get(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.requireAccess(ctx, f, rbac.ActionRead); err != nil {
		return fAPI.SendError(ctx, err)
	}

	return operation.NewGetFederatedIdpOK().WithPayload(model.NewFederatedIdp(f).ToSwagger())
}

func (fAPI *fedIDPAPI) UpdateFederatedIdp(ctx context.Context, params operation.UpdateFederatedIdpParams) middleware.Responder {
	var err error
	if err := fAPI.RequireAuthenticated(ctx); err != nil {
		return fAPI.SendError(ctx, err)
	}
	f, err := fAPI.fedidpCtl.Get(ctx, params.ID)
	if err != nil {
		return fAPI.SendError(ctx, err)
	}
	if err := fAPI.requireAccess(ctx, f, rbac.ActionUpdate); err != nil {
		return fAPI.SendError(ctx, err)
	}

	err = fAPI.updateFedIdp(ctx, params, f)

	if err != nil {
		return fAPI.SendError(ctx, err)
	}

	return operation.NewUpdateFederatedIdpOK()
}

func (fAPI *fedIDPAPI) requireAccess(ctx context.Context, f *pkg.FederatedIdp, action rbac.Action) error {
	if err := fAPI.requireCommercialFeature(ctx); err != nil {
		return err
	}
	if f.ProjectID > 0 {
		// Check if project-level federated idp feature is enabled
		if !config.EnableProjectFederatedIDP(ctx) {
			return errors.ForbiddenError(nil).WithMessage("project-level federated identity provider feature is not enabled")
		}
		var ns any
		ns = f.ProjectID
		return fAPI.RequireProjectAccess(ctx, ns, action, rbac.ResourceFederatedIdp)
	} else if f.ProjectID == 0 {
		// For system-level federated IdPs, allow read-only operations (List/Read) for any authenticated user.
		// This enables project admins to use system-level IdPs when creating federated robot accounts.
		// Write operations (Create/Update/Delete) still require system admin access.
		if action == rbac.ActionList || action == rbac.ActionRead {
			return nil // Already authenticated via RequireAuthenticated() in caller
		}
		return fAPI.RequireSystemAccess(ctx, action, rbac.ResourceFederatedIdp)
	}
	return errors.ForbiddenError(nil)
}

func (fAPI *fedIDPAPI) requireCommercialFeature(ctx context.Context) error {
	if !commercial.Enabled(ctx, commercial.IdentityProviders) {
		return errors.ForbiddenError(nil).WithMessage("commercial feature identity_providers is not enabled")
	}
	return nil
}

// Validation constants
const (
	maxNameLength        = 64
	minNameLength        = 1
	maxDescriptionLength = 264
	maxURLLength         = 2048
	minAlgorithmLength   = 3
	maxAlgorithmLength   = 64
	maxClaimPathLength   = 128
	maxClaimValueLength  = 256
)

// mandatoryClaimPaths are the claims a federated IdP always has to match on. They are
// exempt from the claims_supported check: an issuer may leave them out of claims_supported
// (EKS publishes only ["sub","iss"]) while still putting them in every token.
var mandatoryClaimPaths = []string{"iss", "aud"}

// validate performs comprehensive validation for federated IdP creation
func (fAPI *fedIDPAPI) validate(fedIdp *models.FederatedIdp) error {
	// Trim whitespace from name
	fedIdp.Name = strings.TrimSpace(fedIdp.Name)

	// Validate IDP Name
	if err := validateFedIdpName(fedIdp.Name); err != nil {
		return err
	}

	// Validate Description (optional but has max length)
	if err := validateDescription(fedIdp.Description); err != nil {
		return err
	}

	log.Debugf("fedIdp.OfflineValidation: %v", fedIdp.OfflineValidation)

	// Mode-specific validation
	if fedIdp.OfflineValidation {
		return fAPI.validateOfflineMode(fedIdp)
	}
	return fAPI.validateOnlineMode(fedIdp)
}

// validateOfflineMode validates fields for offline validation mode
func (fAPI *fedIDPAPI) validateOfflineMode(fedIdp *models.FederatedIdp) error {
	log.Debugf("validating fedidp for offline mode")

	// Issuer is required for offline mode
	if fedIdp.Issuer == "" {
		return errors.BadRequestError(nil).WithMessage("issuer is required for offline validation")
	}
	if err := validateHTTPSURL(fedIdp.Issuer, "issuer"); err != nil {
		return err
	}

	// JWKS Keys are required for offline mode
	if fedIdp.JwksKeys == nil {
		return errors.BadRequestError(nil).WithMessage("jwks_keys is required for offline validation")
	}
	if err := validateJWKSKeys(fedIdp.JwksKeys); err != nil {
		return err
	}

	// If supported_algorithms not provided, extract from JWKS keys
	if len(fedIdp.SupportedAlgorithms) == 0 {
		algorithms := extractAlgorithmsFromJWKSKeys(fedIdp.JwksKeys)
		if len(algorithms) > 0 {
			fedIdp.SupportedAlgorithms = algorithms
			log.Debugf("extracted supported_algorithms from JWKS keys: %v", algorithms)
		}
	}

	// Validate supported_algorithms if provided or extracted
	if len(fedIdp.SupportedAlgorithms) > 0 {
		if err := validateSupportedAlgorithms(fedIdp.SupportedAlgorithms); err != nil {
			return err
		}
	}

	// In offline mode, openid_config_url and jwks_uri should not be provided
	// We silently ignore them rather than error (they will not be stored)

	return nil
}

// validateOnlineMode validates fields for online validation mode
// For online mode, only name and openid_config_url are required.
// All other fields (issuer, jwks_uri, supported_algorithms, claims_supported) are
// automatically inferred from the OIDC discovery document.
func (fAPI *fedIDPAPI) validateOnlineMode(fedIdp *models.FederatedIdp) error {
	log.Debugf("validating fedidp for online mode")

	// OpenID Config URL is required for online mode
	if fedIdp.OpenidConfigURL == "" {
		return errors.BadRequestError(nil).WithMessage("openid_config_url is required for online validation")
	}
	if err := validateHTTPSURL(fedIdp.OpenidConfigURL, "openid_config_url"); err != nil {
		return err
	}

	// Fetch the discovery document
	discovery, err := fetchOpenIDDiscovery(fedIdp.OpenidConfigURL)
	if err != nil {
		return errors.BadRequestError(err).WithMessage("failed to fetch OpenID discovery document")
	}

	// Extract issuer from discovery (this is the authoritative source)
	if discovery.Issuer == "" {
		return errors.BadRequestError(nil).WithMessage("OpenID discovery document missing issuer")
	}
	// Set issuer from discovery - this is derived, not user-provided
	fedIdp.Issuer = discovery.Issuer

	// Extract JWKS URI from discovery (this is the authoritative source)
	if discovery.JWKSURI == "" {
		return errors.BadRequestError(nil).WithMessage("OpenID discovery document missing jwks_uri")
	}

	// If user provided jwks_uri, validate it matches discovery; otherwise, use discovery value
	if fedIdp.JwksURI != "" {
		if fedIdp.JwksURI != discovery.JWKSURI {
			return errors.BadRequestError(nil).WithMessagef(
				"jwks_uri mismatch: provided=%q, discovery=%q", fedIdp.JwksURI, discovery.JWKSURI,
			)
		}
	} else {
		// Auto-populate jwks_uri from discovery
		fedIdp.JwksURI = discovery.JWKSURI
		log.Debugf("auto-populated jwks_uri from discovery: %s", fedIdp.JwksURI)
	}

	// Populate derived fields from discovery
	if len(discovery.IDTokenSigningAlgValuesSupported) > 0 {
		fedIdp.SupportedAlgorithms = discovery.IDTokenSigningAlgValuesSupported
	}
	if len(discovery.ClaimsSupported) > 0 {
		fedIdp.ClaimsSupported = discovery.ClaimsSupported
	}

	log.Debugf("fedIdp.SupportedAlgorithms: %v", fedIdp.SupportedAlgorithms)
	log.Debugf("fedIdp.ClaimsSupported: %v", fedIdp.ClaimsSupported)

	return nil
}

type OpenIDDiscovery struct {
	Issuer                           string   `json:"issuer"`
	JWKSURI                          string   `json:"jwks_uri"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

func fetchOpenIDDiscovery(url string) (*OpenIDDiscovery, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var discovery OpenIDDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, err
	}
	return &discovery, nil
}

func (fAPI *fedIDPAPI) updateFedIdp(ctx context.Context, params operation.UpdateFederatedIdpParams, f *pkg.FederatedIdp) error {
	if f == nil {
		return errors.BadRequestError(nil).WithMessage("federated idp not found")
	}

	update := params.Idp
	if update == nil {
		return errors.BadRequestError(nil).WithMessage("update payload is required")
	}

	// Validate immutable fields are not being changed
	if err := fAPI.validateUpdateImmutableFields(f, update); err != nil {
		return err
	}

	// Apply allowed updates based on mode
	if err := fAPI.applyAllowedUpdates(f, update); err != nil {
		return err
	}

	// Validate the updated fields
	if update.Description != nil {
		if err := validateDescription(*update.Description); err != nil {
			return err
		}
	}

	// For offline mode, validate jwks_keys if being updated
	if f.OfflineValidation && update.JwksKeys != nil {
		if err := validateJWKSKeys(update.JwksKeys); err != nil {
			return err
		}
	}

	if err := fAPI.fedidpCtl.Update(ctx, f); err != nil {
		return err
	}
	return nil
}

// validateUpdateImmutableFields checks that immutable fields are not being modified
func (fAPI *fedIDPAPI) validateUpdateImmutableFields(existing *pkg.FederatedIdp, update *models.FederatedIdpUpdate) error {
	// Name is immutable
	if update.Name != nil && *update.Name != existing.Name {
		return errors.BadRequestError(nil).WithMessage("cannot modify name after creation")
	}

	// Issuer is immutable
	if update.Issuer != nil && *update.Issuer != existing.Issuer {
		return errors.BadRequestError(nil).WithMessage("cannot modify issuer after creation")
	}

	// Validation mode is immutable
	if update.OfflineValidation != nil && *update.OfflineValidation != existing.OfflineValidation {
		return errors.BadRequestError(nil).WithMessage("cannot switch validation mode after creation")
	}

	// Project ID is immutable (though typically not in update payload, check anyway)
	// ProjectID is not in the update model, so we don't need to check it

	// OpenID Config URL is immutable (only settable at creation for online mode)
	if update.OpenidConfigURL != nil && *update.OpenidConfigURL != existing.OpenIDConfigURL {
		return errors.BadRequestError(nil).WithMessage("cannot modify openid_config_url after creation")
	}

	// JWKS URI is immutable (only settable at creation for online mode)
	if update.JwksURI != nil && *update.JwksURI != existing.JWKSURI {
		return errors.BadRequestError(nil).WithMessage("cannot modify jwks_uri after creation")
	}

	// For online mode, jwks_keys should not be updateable
	if !existing.OfflineValidation && update.JwksKeys != nil {
		return errors.BadRequestError(nil).WithMessage("cannot modify jwks_keys in online validation mode")
	}

	// Claims supported and algorithms are derived in online mode, immutable
	if !existing.OfflineValidation {
		if update.ClaimsSupported != nil {
			return errors.BadRequestError(nil).WithMessage("cannot modify claims_supported in online validation mode (derived from discovery)")
		}
		if update.SupportedAlgorithms != nil {
			return errors.BadRequestError(nil).WithMessage("cannot modify supported_algorithms in online validation mode (derived from discovery)")
		}
	}

	return nil
}

// applyAllowedUpdates applies only the allowed updates based on validation mode
func (fAPI *fedIDPAPI) applyAllowedUpdates(f *pkg.FederatedIdp, update *models.FederatedIdpUpdate) error {
	// Description is always updateable
	if update.Description != nil {
		f.Description = *update.Description
	}

	// For offline mode only: jwks_keys can be updated (key rotation)
	if f.OfflineValidation && update.JwksKeys != nil {
		raw, err := json.Marshal(update.JwksKeys)
		if err != nil {
			return errors.BadRequestError(nil).WithMessage("invalid jwks_keys format")
		}
		f.JWKSKeys = string(raw)
	}

	// Update timestamp
	f.UpdateTime = time.Now()

	return nil
}

// validateHTTPSURL validates that a URL is valid HTTPS and within length limits
func validateHTTPSURL(urlStr string, fieldName string) error {
	if urlStr == "" {
		return errors.BadRequestError(nil).WithMessagef("%s cannot be empty", fieldName)
	}
	if len(urlStr) > maxURLLength {
		return errors.BadRequestError(nil).WithMessagef("%s exceeds maximum length of %d characters", fieldName, maxURLLength)
	}

	validatedURL, err := lib.ValidateURL(urlStr)
	if err != nil {
		return errors.BadRequestError(nil).WithMessagef("invalid %s: %s", fieldName, err.Error())
	}

	if !strings.HasPrefix(validatedURL, "https://") {
		return errors.BadRequestError(nil).WithMessagef("%s must use HTTPS protocol", fieldName)
	}

	return nil
}

// validateDescription validates the description field
func validateDescription(description string) error {
	if len(description) > maxDescriptionLength {
		return errors.BadRequestError(nil).WithMessagef(
			"description exceeds maximum length of %d characters", maxDescriptionLength,
		)
	}
	return nil
}

// validateSupportedAlgorithms validates algorithm strings (uppercase letters and numbers only)
func validateSupportedAlgorithms(algorithms []string) error {
	algorithmPattern := regexp.MustCompile(`^[A-Z0-9]+$`)

	for _, alg := range algorithms {
		if len(alg) < minAlgorithmLength {
			return errors.BadRequestError(nil).WithMessagef(
				"algorithm %q is too short (minimum %d characters)", alg, minAlgorithmLength,
			)
		}
		if len(alg) > maxAlgorithmLength {
			return errors.BadRequestError(nil).WithMessagef(
				"algorithm %q exceeds maximum length of %d characters", alg, maxAlgorithmLength,
			)
		}
		if !algorithmPattern.MatchString(alg) {
			return errors.BadRequestError(nil).WithMessagef(
				"algorithm %q contains invalid characters (only uppercase letters and numbers allowed)", alg,
			)
		}
	}
	return nil
}

// validateJWKSKeys validates JWKS keys structure for offline validation
func validateJWKSKeys(keys any) error {
	if keys == nil {
		return errors.BadRequestError(nil).WithMessage("jwks_keys is required for offline validation")
	}

	// Marshal to JSON bytes first
	jsonBytes, err := json.Marshal(keys)
	if err != nil {
		return errors.BadRequestError(nil).WithMessage("invalid jwks_keys: failed to serialize JSON")
	}

	// Parse as a map to validate structure
	var jwks map[string]any
	if err := json.Unmarshal(jsonBytes, &jwks); err != nil {
		return errors.BadRequestError(nil).WithMessage("invalid jwks_keys: must be a valid JSON object")
	}

	// Check for "keys" array
	keysArray, ok := jwks["keys"]
	if !ok {
		return errors.BadRequestError(nil).WithMessage("jwks_keys must contain a 'keys' array")
	}

	// Validate keys is an array
	keysSlice, ok := keysArray.([]any)
	if !ok {
		return errors.BadRequestError(nil).WithMessage("jwks_keys 'keys' field must be an array")
	}

	if len(keysSlice) == 0 {
		return errors.BadRequestError(nil).WithMessage("jwks_keys 'keys' array cannot be empty")
	}

	// Validate each key has required JWK fields
	for i, key := range keysSlice {
		keyMap, ok := key.(map[string]any)
		if !ok {
			return errors.BadRequestError(nil).WithMessagef("jwks_keys: key at index %d must be a JSON object", i)
		}

		// Check required JWK fields
		if _, hasKty := keyMap["kty"]; !hasKty {
			return errors.BadRequestError(nil).WithMessagef("jwks_keys: key at index %d missing required 'kty' field", i)
		}
		if _, hasKid := keyMap["kid"]; !hasKid {
			return errors.BadRequestError(nil).WithMessagef("jwks_keys: key at index %d missing required 'kid' field", i)
		}
	}

	return nil
}

// validateClaimRule validates a single claim rule
// claimsSupported: if non-empty, claim_path must be in this list
func validateClaimRule(c *models.ClaimRule, index int, claimsSupported []string) error {
	if c == nil {
		return errors.BadRequestError(nil).WithMessagef("claim rule at index %d is nil", index)
	}

	// Validate claim_path
	if c.ClaimPath == "" {
		return errors.BadRequestError(nil).WithMessagef("claim rule at index %d: claim_path is required", index)
	}
	if len(c.ClaimPath) > maxClaimPathLength {
		return errors.BadRequestError(nil).WithMessagef(
			"claim rule at index %d: claim_path exceeds maximum length of %d characters", index, maxClaimPathLength,
		)
	}

	// Validate claim_path against claims_supported (if populated), except for the
	// mandatory claims, which are always allowed
	if len(claimsSupported) > 0 && !isMandatoryClaimPath(c.ClaimPath) &&
		!isClaimPathSupported(c.ClaimPath, claimsSupported) {
		return errors.BadRequestError(nil).WithMessagef(
			"claim rule at index %d: claim_path '%s' is not in the IdP's claims_supported list",
			index, c.ClaimPath,
		)
	}

	// Validate value
	if c.Value == "" {
		return errors.BadRequestError(nil).WithMessagef("claim rule at index %d: value is required", index)
	}
	if len(c.Value) > maxClaimValueLength {
		return errors.BadRequestError(nil).WithMessagef(
			"claim rule at index %d: value exceeds maximum length of %d characters", index, maxClaimValueLength,
		)
	}

	return nil
}

// isMandatoryClaimPath reports whether a claim path is one of the mandatory claims
// that bypass the claims_supported check (case-insensitive)
func isMandatoryClaimPath(claimPath string) bool {
	normalizedPath := strings.ToLower(strings.TrimSpace(claimPath))
	for _, mandatory := range mandatoryClaimPaths {
		if mandatory == normalizedPath {
			return true
		}
	}
	return false
}

// isClaimPathSupported checks if a claim path is in the supported claims list (case-insensitive)
func isClaimPathSupported(claimPath string, claimsSupported []string) bool {
	normalizedPath := strings.ToLower(strings.TrimSpace(claimPath))
	for _, supported := range claimsSupported {
		if strings.ToLower(strings.TrimSpace(supported)) == normalizedPath {
			return true
		}
	}
	return false
}

// validateFedIdpName validates the federated IdP name
func validateFedIdpName(name string) error {
	// Check for empty name
	if name == "" {
		return errors.BadRequestError(nil).WithMessage("name is required")
	}

	// Check length constraints
	if len(name) < minNameLength {
		return errors.BadRequestError(nil).WithMessagef(
			"name must be at least %d character(s)", minNameLength,
		)
	}
	if len(name) > maxNameLength {
		return errors.BadRequestError(nil).WithMessagef(
			"name exceeds maximum length of %d characters", maxNameLength,
		)
	}

	// Check if name starts with a letter
	if !regexp.MustCompile(`^[a-z]`).MatchString(name) {
		// Provide specific error for uppercase
		if regexp.MustCompile(`^[A-Z]`).MatchString(name) {
			return errors.BadRequestError(nil).WithMessage("name must start with a lowercase letter")
		}
		return errors.BadRequestError(nil).WithMessage("name must start with a letter")
	}

	// Check for uppercase letters anywhere (before full pattern check for better error message)
	if regexp.MustCompile(`[A-Z]`).MatchString(name) {
		return errors.BadRequestError(nil).WithMessage("name must be lowercase (uppercase letters not allowed)")
	}

	// Check for spaces
	if strings.Contains(name, " ") {
		return errors.BadRequestError(nil).WithMessage("name cannot contain spaces")
	}

	// Full pattern validation: lowercase letters, numbers, and .-_ separators
	// Must start with letter, separators cannot be consecutive or at the end
	validNamePattern := regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	if !validNamePattern.MatchString(name) {
		return errors.BadRequestError(nil).WithMessage(
			"name contains invalid characters (allowed: lowercase letters, numbers, and separators . - _)",
		)
	}

	return nil
}

func toRawMessage(v any) json.RawMessage {
	switch val := v.(type) {
	case json.RawMessage:
		return val
	case []byte:
		return json.RawMessage(val)
	case string:
		return json.RawMessage([]byte(val))
	default:
		b, _ := json.Marshal(val) // fallback: encode to JSON
		return b
	}
}

// extractAlgorithmsFromJWKSKeys extracts the "alg" field from all keys in a JWKS object
// Returns a slice of unique algorithm strings (normalized to uppercase)
func extractAlgorithmsFromJWKSKeys(keys any) []string {
	if keys == nil {
		return nil
	}

	// Marshal to JSON bytes first
	jsonBytes, err := json.Marshal(keys)
	if err != nil {
		return nil
	}

	// Parse as a map to extract algorithms
	var jwks map[string]any
	if err := json.Unmarshal(jsonBytes, &jwks); err != nil {
		return nil
	}

	// Get the "keys" array
	keysArray, ok := jwks["keys"]
	if !ok {
		return nil
	}

	keysSlice, ok := keysArray.([]any)
	if !ok {
		return nil
	}

	// Extract unique algorithms
	seen := make(map[string]bool)
	var algorithms []string

	for _, key := range keysSlice {
		keyMap, ok := key.(map[string]any)
		if !ok {
			continue
		}

		// Check for "alg" field
		if alg, hasAlg := keyMap["alg"]; hasAlg {
			algStr, ok := alg.(string)
			if !ok || algStr == "" {
				continue
			}
			// Normalize to uppercase
			algStr = strings.ToUpper(algStr)
			if !seen[algStr] {
				seen[algStr] = true
				algorithms = append(algorithms, algStr)
			}
		}
	}

	return algorithms
}

// applyUpdate is deprecated - use applyAllowedUpdates instead
// This function is kept for reference but should not be used
// as it allows modification of immutable fields
