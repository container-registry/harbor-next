//go:build e2e

package steps

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/golang-jwt/jwt/v5"

	"github.com/goharbor/harbor/src/e2e/internal/fakeidp"
	"github.com/goharbor/harbor/src/e2e/internal/imagebuilder"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

// idpAPIBase is the REST prefix for all FedIDP endpoints.
const idpAPIBase = "/api/v2.0/federated-idps"

var projectFedIDPConfigMu sync.Mutex

// registerIdp wires FedIDP-related Given/When/Then steps.
func registerIdp(sc *godog.ScenarioContext) {
	// ── Background / Given ──────────────────────────────────────────────────

	// Inline offline IdP (generates key pair, stores in scenario state).
	sc.Given(`^project federated identity providers are enabled$`, givenProjectFedIDPEnabled)
	sc.Given(`^project federated identity providers are disabled until scenario cleanup$`, givenProjectFedIDPDisabledUntilCleanup)
	sc.Given(`^an offline federated identity provider "([^"]+)"$`, givenOfflineIdP)
	sc.Given(`^a project-level offline federated identity provider "([^"]+)" on "([^"]+)"$`, givenProjectOfflineIdP)

	// IdP-level claims (robot_id=0).
	sc.Given(`^IdP-level claim "([^"]+)" equals "([^"]+)" on "([^"]+)"$`, givenIdPClaim)
	sc.Given(`^IdP-level claim "([^"]+)" equals the Harbor registry host on "([^"]+)"$`, givenIdPClaimHarborHost)
	sc.Given(`^IdP-level claim "([^"]+)" equals the cluster issuer on "([^"]+)"$`, givenIdPClaimClusterIssuer)

	// Federated robots.
	sc.Given(`^a federated robot "([^"]+)" linked to "([^"]+)" with pull permission on "([^"]+)"$`, givenFedRobot)
	sc.Given(`^robot-level claim "([^"]+)" equals "([^"]+)" on "([^"]+)" for "([^"]+)"$`, givenRobotClaim)

	// K8s cluster prerequisite.
	sc.Given(`^the KIND cluster is running$`, givenKINDClusterRunning)
	sc.Given(`^a federated identity provider created from the cluster OIDC config named "([^"]+)"$`, givenClusterIdP)

	// ── When ────────────────────────────────────────────────────────────────

	// Creation with full offline IdP (stores JWKS from new key pair).
	sc.When(`^the admin creates an offline identity provider "([^"]+)"$`, whenCreateOfflineIdP)
	sc.When(`^the admin creates a project-level offline identity provider "([^"]+)" on "([^"]+)"$`, whenCreateProjectOfflineIdP)
	sc.When(`^the admin creates a project-level offline identity provider "([^"]+)" on "([^"]+)" with the issuer from "([^"]+)"$`, whenCreateProjectOfflineIdPWithIssuerFrom)
	sc.When(`^the admin creates an offline identity provider "([^"]+)" with the issuer from "([^"]+)"$`, whenCreateOfflineIdPWithIssuerFrom)
	sc.When(`^the admin creates an HTTPS online identity provider "([^"]+)"$`, whenCreateHTTPSOnlineIdP)
	sc.When(`^the admin creates an online identity provider "([^"]+)" from an HTTP discovery URL$`, whenCreateHTTPOnlineIdP)
	sc.When(`^the admin creates an online identity provider "([^"]+)" with mismatched JWKS URI$`, whenCreateOnlineIdPWithMismatchedJWKSURI)
	sc.When(`^the admin creates an online identity provider "([^"]+)" from discovery missing issuer$`, whenCreateOnlineIdPDiscoveryMissingIssuer)
	sc.When(`^the admin creates an online identity provider "([^"]+)" from discovery missing JWKS URI$`, whenCreateOnlineIdPDiscoveryMissingJWKSURI)
	sc.When(`^the admin creates an online identity provider "([^"]+)" from invalid JSON discovery$`, whenCreateOnlineIdPInvalidDiscovery)

	// Bare creation for input-validation scenarios.
	sc.When(`^the admin creates an identity provider with name "([^"]*)" and issuer "([^"]*)"$`, whenCreateIdP)
	sc.When(`^the admin creates an identity provider with a 500-character name$`, whenCreateIdPLongName)

	// CRUD actions.
	sc.When(`^the admin updates identity provider "([^"]+)" description to "([^"]+)"$`, whenUpdateIdP)
	sc.When(`^the admin updates identity provider "([^"]+)" name to "([^"]+)"$`, whenUpdateIdPName)
	sc.When(`^the admin updates identity provider "([^"]+)" issuer to "([^"]+)"$`, whenUpdateIdPIssuer)
	sc.When(`^the admin switches identity provider "([^"]+)" to online validation$`, whenSwitchIdPToOnlineValidation)
	sc.When(`^the admin updates identity provider "([^"]+)" OpenID config URL to "([^"]+)"$`, whenUpdateIdPOpenIDConfigURL)
	sc.When(`^the admin updates identity provider "([^"]+)" JWKS URI to "([^"]+)"$`, whenUpdateIdPJWKSURI)
	sc.When(`^the admin updates identity provider "([^"]+)" JWKS keys to a new key$`, whenUpdateIdPJWKSKeysToNewKey)
	sc.When(`^the admin deletes identity provider "([^"]+)"$`, whenDeleteIdP)
	sc.When(`^the admin deletes federated robot "([^"]+)"$`, whenDeleteFedRobot)

	// Claims management (IdP-level).
	sc.When(`^the admin adds claim "([^"]+)" equal to "([^"]+)" to "([^"]+)"$`, whenAddIdPClaim)
	sc.When(`^the admin adds an empty claim path equal to "([^"]+)" to "([^"]+)"$`, whenAddEmptyClaimPath)
	sc.When(`^the admin adds claim "([^"]+)" with an empty value to "([^"]+)"$`, whenAddEmptyClaimValue)
	sc.When(`^the admin adds a too-long claim path to "([^"]+)"$`, whenAddTooLongClaimPath)
	sc.When(`^the admin adds claim "([^"]+)" with a too-long value to "([^"]+)"$`, whenAddTooLongClaimValue)
	sc.When(`^the admin adds duplicate claim rules in one batch to "([^"]+)"$`, whenAddDuplicateClaimBatch)
	sc.When(`^the admin adds a claim using identity provider ID (-?[0-9]+)$`, whenAddClaimToIdpID)
	sc.When(`^the admin lists claims for "([^"]+)" with "([^"]+)"$`, whenListClaimsWithQuery)
	sc.When(`^the admin deletes claim "([^"]+)" from "([^"]+)"$`, whenDeleteIdPClaimByPath)
	sc.When(`^deleting claim ([0-9]+) from "([^"]+)" returns not found$`, whenDeleteNonexistentClaim)
	sc.Then(`^deleting claim ([0-9]+) from "([^"]+)" returns not found$`, whenDeleteNonexistentClaim)

	// Federated robot creation (When form for association scenario).
	sc.When(`^the admin creates a federated robot "([^"]+)" linked to "([^"]+)" with pull permission on "([^"]+)"$`, whenCreateFedRobot)
	sc.When(`^the admin tries to create a federated robot "([^"]+)" linked to "([^"]+)" with pull permission on "([^"]+)"$`, whenTryCreateFedRobot)
	sc.When(`^the admin tries to create a system federated robot "([^"]+)" linked to "([^"]+)"$`, whenTryCreateSystemFedRobot)
	sc.When(`^the admin adds robot claim "([^"]+)" equal to "([^"]+)" to "([^"]+)" on "([^"]+)"$`, whenAddRobotClaim)
	sc.When(`^the admin tries to add robot claim "([^"]+)" equal to "([^"]+)" to "([^"]+)" on "([^"]+)"$`, whenTryAddRobotClaim)
	sc.When(`^the federated robot static secret is used to request the Harbor API$`, whenUseFederatedRobotStaticSecret)

	// JWT auth — valid.
	sc.When(`^the robot presents a JWT with claims aud="([^"]+)", sub="([^"]+)" signed by "([^"]+)"$`, whenPresentJWT)
	sc.When(`^the robot presents a JWT with claims aud="([^"]+)", sub="([^"]+)", team="([^"]+)" signed by "([^"]+)"$`, whenPresentJWTWithTeam)
	sc.When(`^the robot presents a JWT with claims aud="([^"]+)", sub="([^"]+)", groups containing "([^"]+)" signed by "([^"]+)"$`, whenPresentJWTWithGroup)
	sc.When(`^the robot presents a JWT with Kubernetes service account name "([^"]+)" and aud="([^"]+)" signed by "([^"]+)"$`, whenPresentJWTWithKubernetesServiceAccount)
	sc.When(`^the credential provider requests a registry token for "([^"]+)" with claims aud="([^"]+)", sub="([^"]+)" signed by "([^"]+)"$`, whenCredentialProviderRequestsRegistryToken)

	// JWT auth — error paths.
	sc.When(`^the robot presents a JWT signed by a different issuer$`, whenPresentJWTWrongIssuer)
	sc.When(`^the robot presents an expired JWT signed by "([^"]+)" with claims aud="([^"]+)", sub="([^"]+)"$`, whenPresentExpiredJWT)
	sc.When(`^the robot presents a tampered JWT from "([^"]+)" with claims aud="([^"]+)", sub="([^"]+)"$`, whenPresentTamperedJWT)
	sc.When(`^the robot presents an unsupported-algorithm JWT with claims aud="([^"]+)", sub="([^"]+)" signed by "([^"]+)"$`, whenPresentUnsupportedAlgJWT)
	sc.When(`^the robot presents a JWT with unknown kid and claims aud="([^"]+)", sub="([^"]+)" signed by "([^"]+)"$`, whenPresentUnknownKidJWT)
	sc.When(`^the robot presents a JWT missing aud with sub="([^"]+)" signed by "([^"]+)"$`, whenPresentJWTMissingAud)
	sc.When(`^the robot presents a JWT missing sub with aud="([^"]+)" signed by "([^"]+)"$`, whenPresentJWTMissingSub)
	sc.When(`^the same JWT is used to request the Harbor API$`, whenUseLastJWTToRequestHarbor)

	// Online IdP ping endpoints.
	sc.When(`^the admin pings the HTTPS online discovery URL$`, whenPingHTTPSOnlineDiscovery)
	sc.When(`^the admin pings the HTTPS online JWKS URL$`, whenPingHTTPSOnlineJWKS)

	// K8s SA tokens.
	sc.When(`^a K8s SA token is issued for service account "([^"]+)" in namespace "([^"]+)"$`, whenIssueK8sSAToken)
	sc.When(`^the token is used to request the Harbor API$`, whenUseTokenToRequestHarbor)
	sc.When(`^a test pod is deployed pulling the image from "([^"]+)"$`, whenDeployTestPod)

	// ── Then ────────────────────────────────────────────────────────────────

	sc.Then(`^the identity provider "([^"]+)" is returned by the API$`, thenIdPReturned)
	sc.Then(`^the identity provider "([^"]+)" has description "([^"]+)"$`, thenIdPHasDescription)
	sc.Then(`^the online identity provider "([^"]+)" has derived OpenID fields$`, thenOnlineIdPHasDerivedFields)
	sc.Then(`^getting identity provider "([^"]+)" returns not found$`, thenIdPNotFound)
	sc.Then(`^the identity provider list includes "([^"]+)"$`, thenIdPListIncludes)
	sc.Then(`^the identity provider list excludes "([^"]+)"$`, thenIdPListExcludes)
	sc.Then(`^the project-level identity provider list for "([^"]+)" includes "([^"]+)"$`, thenProjectIdPListIncludes)
	sc.Then(`^listing project-level identity providers without a project ID is rejected as bad request$`, thenProjectIdPListWithoutProjectIDBadRequest)
	sc.Then(`^listing identity providers with invalid level is rejected as bad request$`, thenIdPListInvalidLevelBadRequest)
	sc.Then(`^the identity provider list pagination returns one item per page$`, thenIdPListPaginationWorks)
	sc.Then(`^the identity provider "([^"]+)" has ([0-9]+) claims$`, thenIdPHasNClaims)
	sc.Then(`^the identity provider "([^"]+)" has at least ([0-9]+) claims$`, thenIdPHasAtLeastNClaims)
	sc.Then(`^the identity provider "([^"]+)" does not have claim "([^"]+)" equal to "([^"]+)"$`, thenIdPDoesNotHaveIdPLevelClaim)
	sc.Then(`^the identity provider "([^"]+)" has no supported claims$`, thenIdPHasNoSupportedClaims)
	sc.Then(`^the identity provider "([^"]+)" has IdP-level claim "([^"]+)" equal to "([^"]+)"$`, thenIdPHasIdPLevelClaim)
	sc.Then(`^the identity provider "([^"]+)" has robot-level claim "([^"]+)" equal to "([^"]+)" for "([^"]+)"$`, thenIdPHasRobotLevelClaim)
	sc.Then(`^the identity provider "([^"]+)" has ([0-9]+) claims when listed with "([^"]+)"$`, thenIdPHasNClaimsWithQuery)
	sc.Then(`^the robot "([^"]+)" is associated with identity provider "([^"]+)"$`, thenRobotAssociatedWithIdP)
	sc.Then(`^the robot "([^"]+)" is not associated with identity provider "([^"]+)"$`, thenRobotNotAssociatedWithIdP)
	sc.Then(`^the created federated robot response exposes no static secret$`, thenCreatedFederatedRobotHasNoStaticSecret)
	sc.Then(`^the registry token response contains a bearer token$`, thenRegistryTokenResponseContainsBearerToken)
	sc.Then(`^the request is authorized$`, thenRequestAuthorized)
	sc.Then(`^the request is rejected as bad request$`, thenRequestBadRequest)
	sc.Then(`^the request is rejected as bad request with message containing "([^"]+)"$`, thenRequestBadRequestWithMessage)
	sc.Then(`^the pod starts running within ([0-9]+) seconds$`, thenPodRunning)
}

// ============================================================================
// Given helpers
// ============================================================================

func givenOfflineIdP(ctx context.Context, label string) (context.Context, error) {
	return createOfflineIdP(ctx, label, true)
}

func givenProjectFedIDPEnabled(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	projectFedIDPConfigMu.Lock()
	defer projectFedIDPConfigMu.Unlock()

	enabled, err := projectFedIDPEnabled(s)
	if err != nil {
		return ctx, err
	}
	if enabled {
		return ctx, nil
	}

	resp, err := s.Client.Put("/api/v2.0/configurations", map[string]any{
		"enable_project_federated_idp": true,
	})
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusOK {
		return ctx, fmt.Errorf("enable project federated identity providers: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func projectFedIDPEnabled(s *state.Scenario) (bool, error) {
	resp, err := s.Client.Get("/api/v2.0/configurations")
	captureResp(s, resp, err)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("read configurations before enabling project federated identity providers: %d %s", resp.StatusCode, truncate(resp.Body))
	}

	var cfg map[string]struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(resp.Body, &cfg); err != nil {
		return false, fmt.Errorf("decode configurations: %w", err)
	}
	field, ok := cfg["enable_project_federated_idp"]
	if !ok {
		return false, nil
	}
	enabled, ok := field.Value.(bool)
	return ok && enabled, nil
}

func givenProjectFedIDPDisabledUntilCleanup(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	projectFedIDPConfigMu.Lock()

	previous, err := projectFedIDPEnabled(s)
	if err != nil {
		projectFedIDPConfigMu.Unlock()
		return ctx, err
	}
	s.ProjectFedIDPRestore = func() error {
		defer projectFedIDPConfigMu.Unlock()
		return setProjectFedIDPEnabled(s, previous)
	}
	if err := setProjectFedIDPEnabled(s, false); err != nil {
		s.ProjectFedIDPRestore = nil
		projectFedIDPConfigMu.Unlock()
		return ctx, err
	}
	return ctx, nil
}

func setProjectFedIDPEnabled(s *state.Scenario, enabled bool) error {
	resp, err := s.Client.Put("/api/v2.0/configurations", map[string]any{
		"enable_project_federated_idp": enabled,
	})
	captureResp(s, resp, err)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("set enable_project_federated_idp=%t: %d %s", enabled, resp.StatusCode, truncate(resp.Body))
	}
	return nil
}

func createOfflineIdP(ctx context.Context, label string, requireCreated bool) (context.Context, error) {
	s := state.Get(ctx)
	fullLabel := idpLabel(s, label)
	issuer := fmt.Sprintf("https://%s.fake-idp.test", fullLabel)
	return createOfflineIdPWithOptions(ctx, label, issuer, 0, nil, requireCreated)
}

func createOfflineIdPWithOptions(ctx context.Context, label, issuer string, projectID int64, idp *fakeidp.IdP, requireCreated bool) (context.Context, error) {
	s := state.Get(ctx)
	fullLabel := idpLabel(s, label)
	if idp == nil {
		var err error
		idp, err = fakeidp.New(issuer)
		if err != nil {
			return ctx, fmt.Errorf("create fake IdP: %w", err)
		}
	}
	body := map[string]any{
		"name":                 fullLabel,
		"description":          "e2e fake IdP " + s.Suffix,
		"issuer":               issuer,
		"offline_validation":   true,
		"jwks_keys":            idp.JWKS(),
		"supported_algorithms": []string{"RS256"},
	}
	if projectID > 0 {
		body["project_id"] = projectID
	}

	resp, err := s.Client.Post(idpAPIBase, body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, fmt.Errorf("create IdP %s: %w", fullLabel, err)
	}
	if resp.StatusCode != http.StatusCreated {
		if !requireCreated {
			return ctx, nil
		}
		return ctx, fmt.Errorf("create IdP %s: got %d: %s", fullLabel, resp.StatusCode, truncate(resp.Body))
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(resp.Body, &created); err != nil {
		return ctx, fmt.Errorf("decode IdP response: %w", err)
	}
	s.LastIdPID = created.ID
	s.CreatedIdPs = append(s.CreatedIdPs, created.ID)
	s.FakeIdPs[fullLabel] = idp
	return ctx, nil
}

func givenProjectOfflineIdP(ctx context.Context, label, projAlias string) (context.Context, error) {
	return createProjectOfflineIdP(ctx, label, projAlias, true)
}

func createProjectOfflineIdP(ctx context.Context, label, projAlias string, requireCreated bool) (context.Context, error) {
	s := state.Get(ctx)
	project := requireProject(s, projAlias)
	pid := projectID(s, project)
	if pid == 0 {
		return ctx, fmt.Errorf("project %q has no project_id", project)
	}
	issuer := fmt.Sprintf("https://%s.fake-idp.test", idpLabel(s, label))
	return createOfflineIdPWithOptions(ctx, label, issuer, pid, nil, requireCreated)
}

func createOnlineIdP(ctx context.Context, label string, discoveryURL string, extra map[string]any) (context.Context, error) {
	s := state.Get(ctx)
	body := map[string]any{
		"name":               idpLabel(s, label),
		"description":        "e2e online IdP " + s.Suffix,
		"offline_validation": false,
		"openid_config_url":  discoveryURL,
	}
	for k, v := range extra {
		body[k] = v
	}
	resp, err := s.Client.Post(idpAPIBase, body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode == http.StatusCreated {
		var created struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(resp.Body, &created); err != nil {
			return ctx, fmt.Errorf("decode online IdP response: %w", err)
		}
		s.LastIdPID = created.ID
		s.CreatedIdPs = append(s.CreatedIdPs, created.ID)
	}
	return ctx, nil
}

func givenIdPClaim(ctx context.Context, claimPath, claimValue, label string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Post(fmt.Sprintf("%s/%d/claims", idpAPIBase, id), claimRulesPayload(0, claimPath, claimValue))
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("add IdP claim %s=%s: %d %s", claimPath, claimValue, resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func givenIdPClaimHarborHost(ctx context.Context, claimPath, label string) (context.Context, error) {
	s := state.Get(ctx)
	return givenIdPClaim(ctx, claimPath, k8sRegistryHost(s), label)
}

func givenIdPClaimClusterIssuer(ctx context.Context, claimPath, label string) (context.Context, error) {
	out, err := exec.Command("kubectl", "get", "--raw", "/.well-known/openid-configuration").Output()
	if err != nil {
		return ctx, fmt.Errorf("get cluster openid-configuration: %w", err)
	}
	var cfg struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return ctx, fmt.Errorf("decode openid-configuration: %w", err)
	}
	return givenIdPClaim(ctx, claimPath, cfg.Issuer, label)
}

func givenFedRobot(ctx context.Context, robotLabel, idpLabel_, projAlias string) (context.Context, error) {
	return createFedRobot(ctx, robotLabel, idpLabel_, projAlias)
}

func givenRobotClaim(ctx context.Context, claimPath, claimValue, robotLabel, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	robotID, err := namedRobotID(s, robotLabel)
	if err != nil {
		return ctx, err
	}
	idpID, err := namedIdPID(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Post(fmt.Sprintf("%s/%d/claims", idpAPIBase, idpID), claimRulesPayload(robotID, claimPath, claimValue))
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("add robot claim %s=%s: %d %s", claimPath, claimValue, resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func givenKINDClusterRunning(ctx context.Context) (context.Context, error) {
	out, err := exec.Command("kubectl", "cluster-info").CombinedOutput()
	if err != nil {
		return ctx, fmt.Errorf("KIND cluster not reachable (run 'task dev:fedidp:cluster:up'): %w — %s", err, out)
	}
	return ctx, nil
}

func givenClusterIdP(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	fullLabel := idpLabel(s, label)

	// Fetch cluster JWKS.
	jwksRaw, err := exec.Command("kubectl", "get", "--raw", "/openid/v1/jwks").Output()
	if err != nil {
		return ctx, fmt.Errorf("fetch cluster JWKS: %w", err)
	}
	var jwks map[string]any
	if err := json.Unmarshal(jwksRaw, &jwks); err != nil {
		return ctx, fmt.Errorf("decode JWKS: %w", err)
	}

	// Fetch cluster issuer.
	cfgRaw, err := exec.Command("kubectl", "get", "--raw", "/.well-known/openid-configuration").Output()
	if err != nil {
		return ctx, fmt.Errorf("fetch cluster openid-configuration: %w", err)
	}
	var cfg struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		return ctx, fmt.Errorf("decode openid-configuration: %w", err)
	}

	resp, err := s.Client.Post(idpAPIBase, map[string]any{
		"name":                 fullLabel,
		"description":          "K8s cluster IdP " + s.Suffix,
		"issuer":               cfg.Issuer,
		"offline_validation":   true,
		"jwks_keys":            jwks,
		"supported_algorithms": []string{"RS256"},
	})
	if err != nil {
		return ctx, fmt.Errorf("create cluster IdP: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("create cluster IdP: got %d: %s", resp.StatusCode, truncate(resp.Body))
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(resp.Body, &created); err != nil {
		return ctx, fmt.Errorf("decode cluster IdP response: %w", err)
	}
	s.LastIdPID = created.ID
	s.CreatedIdPs = append(s.CreatedIdPs, created.ID)
	return ctx, nil
}

// ============================================================================
// When helpers
// ============================================================================

func whenCreateOfflineIdP(ctx context.Context, label string) (context.Context, error) {
	return createOfflineIdP(ctx, label, false)
}

func whenCreateProjectOfflineIdP(ctx context.Context, label, projAlias string) (context.Context, error) {
	return createProjectOfflineIdP(ctx, label, projAlias, false)
}

func whenCreateProjectOfflineIdPWithIssuerFrom(ctx context.Context, label, projAlias, existingLabel string) (context.Context, error) {
	s := state.Get(ctx)
	existing, err := getIdPByLabel(s, existingLabel)
	if err != nil {
		return ctx, err
	}
	project := requireProject(s, projAlias)
	pid := projectID(s, project)
	if pid == 0 {
		return ctx, fmt.Errorf("project %q has no project_id", project)
	}
	return createOfflineIdPWithOptions(ctx, label, existing.Issuer, pid, nil, false)
}

func whenCreateOfflineIdPWithIssuerFrom(ctx context.Context, label, existingLabel string) (context.Context, error) {
	s := state.Get(ctx)
	existing, err := getIdPByLabel(s, existingLabel)
	if err != nil {
		return ctx, err
	}
	return createOfflineIdPWithOptions(ctx, label, existing.Issuer, 0, nil, false)
}

func whenCreateHTTPSOnlineIdP(ctx context.Context, label string) (context.Context, error) {
	return createOnlineIdP(ctx, label, trustedHTTPSDiscoveryURL(), nil)
}

func whenCreateHTTPOnlineIdP(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	url := startFedIDPOIDCMock(s, fedIDPMockOptions{scheme: "http"})
	return createOnlineIdP(ctx, label, url+"/.well-known/openid-configuration", nil)
}

func whenCreateOnlineIdPWithMismatchedJWKSURI(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	url := startFedIDPOIDCMock(s, fedIDPMockOptions{scheme: "https"})
	return createOnlineIdP(ctx, label, url+"/.well-known/openid-configuration", map[string]any{"jwks_uri": "https://example.invalid/keys"})
}

func whenCreateOnlineIdPDiscoveryMissingIssuer(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	url := startFedIDPOIDCMock(s, fedIDPMockOptions{scheme: "https", missingIssuer: true})
	return createOnlineIdP(ctx, label, url+"/.well-known/openid-configuration", nil)
}

func whenCreateOnlineIdPDiscoveryMissingJWKSURI(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	url := startFedIDPOIDCMock(s, fedIDPMockOptions{scheme: "https", missingJWKSURI: true})
	return createOnlineIdP(ctx, label, url+"/.well-known/openid-configuration", nil)
}

func whenCreateOnlineIdPInvalidDiscovery(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	url := startFedIDPOIDCMock(s, fedIDPMockOptions{scheme: "https", invalidDiscovery: true})
	return createOnlineIdP(ctx, label, url+"/.well-known/openid-configuration", nil)
}

func whenCreateIdP(ctx context.Context, name, issuer string) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Post(idpAPIBase, map[string]any{
		"name":                 name,
		"issuer":               issuer,
		"offline_validation":   true,
		"jwks_keys":            map[string]any{"keys": []any{}},
		"supported_algorithms": []string{"RS256"},
	})
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	// Track for cleanup on success.
	if resp.StatusCode == http.StatusCreated {
		var created struct {
			ID int64 `json:"id"`
		}
		if jsonErr := json.Unmarshal(resp.Body, &created); jsonErr == nil && created.ID != 0 {
			s.CreatedIdPs = append(s.CreatedIdPs, created.ID)
			s.LastIdPID = created.ID
		}
	}
	return ctx, nil
}

func whenCreateIdPLongName(ctx context.Context) (context.Context, error) {
	longName := strings.Repeat("a", 500)
	return whenCreateIdP(ctx, longName, "https://test.example.com")
}

func whenUpdateIdP(ctx context.Context, label, description string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Put(fmt.Sprintf("%s/%d", idpAPIBase, id), map[string]any{
		"description": description,
	})
	captureResp(s, resp, err)
	return ctx, err
}

func whenUpdateIdPName(ctx context.Context, label, name string) (context.Context, error) {
	return updateIdP(ctx, label, map[string]any{"name": idpLabel(state.Get(ctx), name)})
}

func whenUpdateIdPIssuer(ctx context.Context, label, issuer string) (context.Context, error) {
	return updateIdP(ctx, label, map[string]any{"issuer": issuer})
}

func whenSwitchIdPToOnlineValidation(ctx context.Context, label string) (context.Context, error) {
	return updateIdP(ctx, label, map[string]any{"offline_validation": false})
}

func whenUpdateIdPOpenIDConfigURL(ctx context.Context, label, openIDConfigURL string) (context.Context, error) {
	return updateIdP(ctx, label, map[string]any{"openid_config_url": openIDConfigURL})
}

func whenUpdateIdPJWKSURI(ctx context.Context, label, jwksURI string) (context.Context, error) {
	return updateIdP(ctx, label, map[string]any{"jwks_uri": jwksURI})
}

func whenUpdateIdPJWKSKeysToNewKey(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	oldIDP, err := lookupFakeIdP(s, label)
	if err != nil {
		return ctx, err
	}
	newIDP, err := fakeidp.New(oldIDP.Issuer)
	if err != nil {
		return ctx, err
	}
	if _, err := updateIdP(ctx, label, map[string]any{"jwks_keys": newIDP.JWKS()}); err != nil {
		return ctx, err
	}
	if s.LastResp != nil && s.LastResp.StatusCode == http.StatusOK {
		s.FakeIdPs[idpLabel(s, label)] = newIDP
	}
	return ctx, nil
}

func updateIdP(ctx context.Context, label string, body map[string]any) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Put(fmt.Sprintf("%s/%d", idpAPIBase, id), body)
	captureResp(s, resp, err)
	return ctx, err
}

func whenDeleteIdP(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Delete(fmt.Sprintf("%s/%d", idpAPIBase, id))
	captureResp(s, resp, err)
	if err == nil {
		removeInt64(&s.CreatedIdPs, id)
	}
	return ctx, err
}

func whenDeleteFedRobot(ctx context.Context, robotLabel string) (context.Context, error) {
	s := state.Get(ctx)
	robotID, err := namedRobotID(s, robotLabel)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Delete(fmt.Sprintf("/api/v2.0/robots/%d", robotID))
	captureResp(s, resp, err)
	if err == nil && resp.StatusCode == http.StatusOK {
		for i := range s.CreatedRobots {
			if s.CreatedRobots[i].ID == robotID {
				s.CreatedRobots[i].ID = 0
			}
		}
	}
	return ctx, err
}

func whenAddIdPClaim(ctx context.Context, claimPath, claimValue, label string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Post(fmt.Sprintf("%s/%d/claims", idpAPIBase, id), claimRulesPayload(0, claimPath, claimValue))
	captureResp(s, resp, err)
	return ctx, err
}

func whenAddEmptyClaimPath(ctx context.Context, claimValue, label string) (context.Context, error) {
	return whenAddIdPClaim(ctx, "", claimValue, label)
}

func whenAddEmptyClaimValue(ctx context.Context, claimPath, label string) (context.Context, error) {
	return whenAddIdPClaim(ctx, claimPath, "", label)
}

func whenAddTooLongClaimPath(ctx context.Context, label string) (context.Context, error) {
	return whenAddIdPClaim(ctx, strings.Repeat("a", 129), "value", label)
}

func whenAddTooLongClaimValue(ctx context.Context, claimPath, label string) (context.Context, error) {
	return whenAddIdPClaim(ctx, claimPath, strings.Repeat("v", 257), label)
}

func whenAddDuplicateClaimBatch(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	rule := map[string]any{
		"robot_id":   0,
		"claim_path": "dup",
		"value":      "same",
	}
	resp, err := s.Client.Post(fmt.Sprintf("%s/%d/claims", idpAPIBase, id), map[string]any{"rules": []map[string]any{rule, rule}})
	captureResp(s, resp, err)
	return ctx, err
}

func whenAddClaimToIdpID(ctx context.Context, idpID int64) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Post(
		fmt.Sprintf("%s/%d/claims", idpAPIBase, idpID),
		claimRulesPayload(0, "sub", "route-scope-regression"),
	)
	captureResp(s, resp, err)
	return ctx, err
}

func whenListClaimsWithQuery(ctx context.Context, label, rawQuery string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d/claims?%s", idpAPIBase, id, rawQuery))
	captureResp(s, resp, err)
	return ctx, err
}

func whenDeleteIdPClaimByPath(ctx context.Context, claimPath, label string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.DeleteWithBody(fmt.Sprintf("%s/%d/claims", idpAPIBase, id), claimRulesPayload(0, claimPath, ""))
	captureResp(s, resp, err)
	return ctx, err
}

func whenDeleteNonexistentClaim(ctx context.Context, claimID int, label string) (context.Context, error) {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Delete(fmt.Sprintf("%s/%d/claims/%d", idpAPIBase, id, claimID))
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusNotFound {
		return ctx, fmt.Errorf("expected 404, got %d", resp.StatusCode)
	}
	return ctx, nil
}

func whenCreateFedRobot(ctx context.Context, robotLabel_, idpLabel_, projAlias string) (context.Context, error) {
	return createFedRobot(ctx, robotLabel_, idpLabel_, projAlias)
}

func whenTryCreateFedRobot(ctx context.Context, robotLabel_, idpLabel_, projAlias string) (context.Context, error) {
	return createFedRobotAllowFailure(ctx, robotLabel_, idpLabel_, projAlias)
}

func whenTryCreateSystemFedRobot(ctx context.Context, robotLabel_, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	idpID, err := namedIdPID(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	body := map[string]any{
		"name":            fmt.Sprintf("%s-%s", robotLabel_, s.Suffix),
		"duration":        1,
		"level":           "system",
		"federatedidp_id": idpID,
		"permissions": []map[string]any{
			{
				"kind":      "system",
				"namespace": "/",
				"access": []map[string]any{
					{"resource": "repository", "action": "pull"},
				},
			},
		},
	}
	resp, err := s.Client.Post("/api/v2.0/robots", body)
	captureResp(s, resp, err)
	if err == nil && resp.StatusCode == http.StatusCreated {
		trackCreatedRobot(s, resp.Body, "")
	}
	return ctx, err
}

func whenAddRobotClaim(ctx context.Context, claimPath, claimValue, robotLabel_, idpLabel_ string) (context.Context, error) {
	return givenRobotClaim(ctx, claimPath, claimValue, robotLabel_, idpLabel_)
}

func whenTryAddRobotClaim(ctx context.Context, claimPath, claimValue, robotLabel_, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	robotID, err := namedRobotID(s, robotLabel_)
	if err != nil {
		return ctx, err
	}
	idpID, err := namedIdPID(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Post(fmt.Sprintf("%s/%d/claims", idpAPIBase, idpID), claimRulesPayload(robotID, claimPath, claimValue))
	captureResp(s, resp, err)
	return ctx, err
}

func whenUseFederatedRobotStaticSecret(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if len(s.CreatedRobots) == 0 {
		return ctx, fmt.Errorf("no federated robot credentials captured")
	}
	robot := s.CreatedRobots[len(s.CreatedRobots)-1]
	client := s.Client.WithCredentials(robot.Name, robot.Secret)
	resp, err := client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories?page_size=1", robot.Project))
	captureResp(s, resp, err)
	return ctx, err
}

func whenPresentJWT(ctx context.Context, aud, sub, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{"aud": aud, "sub": sub})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentJWTWithGroup(ctx context.Context, aud, sub, group, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{
		"aud":    aud,
		"sub":    sub,
		"groups": []string{"unrelated", group},
	})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentJWTWithKubernetesServiceAccount(ctx context.Context, serviceAccountName, aud, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{
		"aud": aud,
		"kubernetes.io": map[string]any{
			"namespace": "default",
			"serviceaccount": map[string]any{
				"name": serviceAccountName,
			},
		},
	})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenCredentialProviderRequestsRegistryToken(ctx context.Context, repoRef, aud, sub, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{"aud": aud, "sub": sub})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token

	projectAlias, repoName, ok := strings.Cut(repoRef, "/")
	if !ok || projectAlias == "" || repoName == "" {
		return ctx, fmt.Errorf("registry token repo ref %q must be project/repository", repoRef)
	}
	repository := fmt.Sprintf("%s/%s", projectName(s, projectAlias), repoName)
	query := url.Values{}
	query.Set("service", "harbor-registry")
	query.Set("scope", fmt.Sprintf("repository:%s:pull", repository))

	client := s.Client.WithCredentials("jwt", token)
	resp, err := client.Get("/service/token?" + query.Encode())
	captureResp(s, resp, err)
	return ctx, err
}

func whenPresentJWTWithTeam(ctx context.Context, aud, sub, team, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{"aud": aud, "sub": sub, "team": team})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentJWTWrongIssuer(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	impostor, err := fakeidp.New("https://wrong-issuer.evil.com")
	if err != nil {
		return ctx, err
	}
	token, err := impostor.IssueJWT(map[string]any{"sub": "ci-workload"}, 5*time.Minute)
	if err != nil {
		return ctx, fmt.Errorf("issue impostor JWT: %w", err)
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentExpiredJWT(ctx context.Context, idpLabel_, aud, sub string) (context.Context, error) {
	s := state.Get(ctx)
	idp, err := lookupFakeIdP(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	token, err := idp.IssueExpired(map[string]any{"aud": aud, "sub": sub})
	if err != nil {
		return ctx, fmt.Errorf("issue expired JWT: %w", err)
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentTamperedJWT(ctx context.Context, idpLabel_, aud, sub string) (context.Context, error) {
	s := state.Get(ctx)
	idp, err := lookupFakeIdP(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	token, err := idp.IssueTampered(map[string]any{"aud": aud, "sub": sub}, 5*time.Minute)
	if err != nil {
		return ctx, fmt.Errorf("issue tampered JWT: %w", err)
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentUnsupportedAlgJWT(ctx context.Context, aud, sub, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	idp, err := lookupFakeIdP(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": idp.Issuer,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": aud,
		"sub": sub,
	})
	token.Header["kid"] = idp.KID
	signed, err := token.SignedString([]byte("not-the-rsa-key"))
	if err != nil {
		return ctx, fmt.Errorf("issue unsupported-algorithm JWT: %w", err)
	}
	s.LastJWT = signed
	return useBearer(ctx, s, signed)
}

func whenPresentUnknownKidJWT(ctx context.Context, aud, sub, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	idp, err := lookupFakeIdP(s, idpLabel_)
	if err != nil {
		return ctx, err
	}
	impostor, err := fakeidp.New(idp.Issuer)
	if err != nil {
		return ctx, err
	}
	token, err := impostor.IssueJWT(map[string]any{"aud": aud, "sub": sub}, 5*time.Minute)
	if err != nil {
		return ctx, fmt.Errorf("issue unknown-kid JWT: %w", err)
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentJWTMissingAud(ctx context.Context, sub, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{"sub": sub})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenPresentJWTMissingSub(ctx context.Context, aud, idpLabel_ string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpLabel_, map[string]any{"aud": aud})
	if err != nil {
		return ctx, err
	}
	s.LastJWT = token
	return useBearer(ctx, s, token)
}

func whenUseLastJWTToRequestHarbor(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	return useBearer(ctx, s, s.LastJWT)
}

func whenPingHTTPSOnlineDiscovery(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Post(idpAPIBase+"/openid-config", map[string]any{"openid_config_url": trustedHTTPSDiscoveryURL()})
	captureResp(s, resp, err)
	return ctx, err
}

func whenPingHTTPSOnlineJWKS(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	jwksURI := os.Getenv("FEDIDP_HTTPS_OIDC_JWKS_URL")
	if jwksURI == "" {
		discovery, err := fetchDiscoveryForTest(trustedHTTPSDiscoveryURL())
		if err != nil {
			return ctx, err
		}
		jwksURI = discovery.JWKSURI
	}
	resp, err := s.Client.Post(idpAPIBase+"/jwks", map[string]any{"jwks_uri": jwksURI})
	captureResp(s, resp, err)
	return ctx, err
}

func whenIssueK8sSAToken(ctx context.Context, sa, namespace string) (context.Context, error) {
	s := state.Get(ctx)
	// Use the node-reachable Harbor registry host as the audience (matches credential provider config).
	audience := k8sRegistryHost(s)
	if out, err := exec.Command("kubectl", "get", "serviceaccount", sa, "-n", namespace).CombinedOutput(); err != nil {
		if createOut, createErr := exec.Command("kubectl", "create", "serviceaccount", sa, "-n", namespace).CombinedOutput(); createErr != nil {
			return ctx, fmt.Errorf("ensure service account %s/%s: get: %w: %s; create: %v: %s", namespace, sa, err, out, createErr, createOut)
		}
	}
	out, err := exec.Command("kubectl", "create", "token", sa,
		"-n", namespace,
		"--audience", audience,
		"--duration", "600s",
	).Output()
	if err != nil {
		return ctx, fmt.Errorf("kubectl create token for %s/%s: %w", namespace, sa, err)
	}
	s.LastJWT = strings.TrimSpace(string(out))
	return ctx, nil
}

func whenUseTokenToRequestHarbor(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	return useBearer(ctx, s, s.LastJWT)
}

func whenDeployTestPod(ctx context.Context, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projAlias)
	image := fmt.Sprintf("%s/%s/nginx:latest", k8sRegistryHost(s), project)
	seedImage := fmt.Sprintf("%s/%s/nginx:latest", s.RegistryHost, project)
	if _, err := imagebuilder.PushSynthetic(seedImage, 1024, adminCraneOpts(s)); err != nil {
		return ctx, fmt.Errorf("seed K8s pull image %s: %w", seedImage, err)
	}
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: e2e-fedidp-%s
  namespace: default
spec:
  serviceAccountName: default
  containers:
  - name: nginx
    image: %s
    imagePullPolicy: Always
`, s.Suffix, image)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ctx, fmt.Errorf("kubectl apply pod: %w: %s", err, out)
	}
	return ctx, nil
}

// ============================================================================
// Then helpers
// ============================================================================

func thenIdPReturned(ctx context.Context, label string) error {
	s := state.Get(ctx)
	body, err := getIdPByLabel(s, label)
	if err != nil {
		return err
	}
	fullLabel := idpLabel(s, label)
	if body.Name != fullLabel {
		return fmt.Errorf("IdP name %q (want %q)", body.Name, fullLabel)
	}
	if body.OfflineValidation != nil && !*body.OfflineValidation {
		return fmt.Errorf("IdP %s offline_validation=false (want true)", label)
	}
	wantIssuer := fmt.Sprintf("https://%s.fake-idp.test", fullLabel)
	if body.Issuer != wantIssuer {
		return fmt.Errorf("IdP issuer %q (want %q)", body.Issuer, wantIssuer)
	}
	if len(body.SupportedAlgorithms) > 0 && !containsString(body.SupportedAlgorithms, "RS256") {
		return fmt.Errorf("supported_algorithms=%v (want RS256)", body.SupportedAlgorithms)
	}
	return nil
}

func thenIdPHasDescription(ctx context.Context, label, wantDesc string) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d", idpAPIBase, id))
	if err != nil {
		return err
	}
	var body struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return fmt.Errorf("decode IdP: %w", err)
	}
	if body.Description != wantDesc {
		return fmt.Errorf("description %q (want %q)", body.Description, wantDesc)
	}
	return nil
}

func thenIdPNotFound(ctx context.Context, label string) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		// If we can't find the ID it was likely already gone — check by name.
		fullLabel := idpLabel(s, label)
		return idpGoneByName(s, fullLabel)
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d", idpAPIBase, id))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("expected 404 for deleted IdP, got %d", resp.StatusCode)
	}
	return nil
}

func thenIdPListIncludes(ctx context.Context, label string) error {
	s := state.Get(ctx)
	fullLabel := idpLabel(s, label)
	found, count, err := idPListContains(s, idpAPIBase+"?page_size=100", fullLabel)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("IdP %q not found in list of %d items", fullLabel, count)
	}
	return nil
}

func thenIdPListExcludes(ctx context.Context, label string) error {
	s := state.Get(ctx)
	fullLabel := idpLabel(s, label)
	found, _, err := idPListContains(s, idpAPIBase+"?page_size=100", fullLabel)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("IdP %q unexpectedly found in default list", fullLabel)
	}
	return nil
}

func thenProjectIdPListIncludes(ctx context.Context, projAlias, label string) error {
	s := state.Get(ctx)
	project := requireProject(s, projAlias)
	pid := projectID(s, project)
	query := url.Values{}
	query.Set("q", fmt.Sprintf("Level=project,ProjectID=%d", pid))
	query.Set("page_size", "100")
	found, count, err := idPListContains(s, idpAPIBase+"?"+query.Encode(), idpLabel(s, label))
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("project IdP %q not found in list of %d items", idpLabel(s, label), count)
	}
	return nil
}

func thenProjectIdPListWithoutProjectIDBadRequest(ctx context.Context) error {
	s := state.Get(ctx)
	resp, err := s.Client.Get(idpAPIBase + "?q=Level%3Dproject")
	captureResp(s, resp, err)
	if err != nil {
		return err
	}
	return mustStatus(s.LastResp, http.StatusBadRequest)
}

func thenIdPListInvalidLevelBadRequest(ctx context.Context) error {
	s := state.Get(ctx)
	resp, err := s.Client.Get(idpAPIBase + "?q=Level%3Dglobal")
	captureResp(s, resp, err)
	if err != nil {
		return err
	}
	return mustStatus(s.LastResp, http.StatusBadRequest)
}

func thenIdPListPaginationWorks(ctx context.Context) error {
	s := state.Get(ctx)
	resp, err := s.Client.Get(idpAPIBase + "?page=1&page_size=1")
	captureResp(s, resp, err)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list IdPs page_size=1: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	var list []any
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		return fmt.Errorf("decode paginated IdP list: %w", err)
	}
	if len(list) != 1 {
		return fmt.Errorf("page_size=1 returned %d items", len(list))
	}
	if resp.Header.Get("X-Total-Count") == "" {
		return fmt.Errorf("paginated IdP list missing X-Total-Count")
	}
	return nil
}

func thenIdPHasNClaims(ctx context.Context, label string, want int) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	claims, err := listIdPClaims(s, id)
	if err != nil {
		return err
	}
	if len(claims) != want {
		return fmt.Errorf("IdP %s has %d claims (want exactly %d)", label, len(claims), want)
	}
	return nil
}

func thenIdPHasAtLeastNClaims(ctx context.Context, label string, min int) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d/claims", idpAPIBase, id))
	if err != nil {
		return err
	}
	var claims []any
	if err := json.Unmarshal(resp.Body, &claims); err != nil {
		return fmt.Errorf("decode claims: %w", err)
	}
	if len(claims) < min {
		return fmt.Errorf("IdP %s has %d claims (want >= %d)", label, len(claims), min)
	}
	return nil
}

func thenIdPHasNoSupportedClaims(ctx context.Context, label string) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d", idpAPIBase, id))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get IdP %s: status %d", label, resp.StatusCode)
	}
	var body struct {
		ClaimsSupported []string `json:"claims_supported"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return fmt.Errorf("decode IdP: %w", err)
	}
	if len(body.ClaimsSupported) != 0 {
		return fmt.Errorf("claims_supported = %#v (want empty)", body.ClaimsSupported)
	}
	return nil
}

func thenOnlineIdPHasDerivedFields(ctx context.Context, label string) error {
	s := state.Get(ctx)
	body, err := getIdPByLabel(s, label)
	if err != nil {
		return err
	}
	if body.OfflineValidation != nil && *body.OfflineValidation {
		return fmt.Errorf("online IdP %s offline_validation=%v (want false)", label, body.OfflineValidation)
	}
	if body.Issuer == "" || body.JWKSURI == "" {
		return fmt.Errorf("online IdP missing derived issuer/JWKS URI: issuer=%q jwks_uri=%q", body.Issuer, body.JWKSURI)
	}
	if !containsString(body.ClaimsSupported, "sub") || !containsString(body.SupportedAlgorithms, "RS256") {
		return fmt.Errorf("online IdP derived claims/algorithms incomplete: claims=%v algorithms=%v", body.ClaimsSupported, body.SupportedAlgorithms)
	}
	return nil
}

func thenIdPHasIdPLevelClaim(ctx context.Context, label, claimPath, claimValue string) error {
	return thenIdPHasClaim(ctx, label, claimPath, claimValue, 0)
}

func thenIdPDoesNotHaveIdPLevelClaim(ctx context.Context, label, claimPath, claimValue string) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	claims, err := listIdPClaims(s, id)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if claim.RobotID == 0 && claim.ClaimPath == claimPath && claim.Value == claimValue {
			return fmt.Errorf("claim %s=%s still present on IdP %s", claimPath, claimValue, label)
		}
	}
	return nil
}

func thenIdPHasRobotLevelClaim(ctx context.Context, label, claimPath, claimValue, robotLabel_ string) error {
	s := state.Get(ctx)
	robotID, err := namedRobotID(s, robotLabel_)
	if err != nil {
		return err
	}
	return thenIdPHasClaim(ctx, label, claimPath, claimValue, robotID)
}

func thenIdPHasNClaimsWithQuery(ctx context.Context, label string, want int, rawQuery string) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d/claims?%s", idpAPIBase, id, rawQuery))
	captureResp(s, resp, err)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list filtered claims: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	var claims []idpClaim
	if err := json.Unmarshal(resp.Body, &claims); err != nil {
		return fmt.Errorf("decode filtered claims: %w", err)
	}
	if len(claims) != want {
		return fmt.Errorf("filtered claim count=%d (want %d) for query %q", len(claims), want, rawQuery)
	}
	return nil
}

func thenIdPHasClaim(ctx context.Context, label, claimPath, claimValue string, robotID int64) error {
	s := state.Get(ctx)
	id, err := namedIdPID(s, label)
	if err != nil {
		return err
	}
	claims, err := listIdPClaims(s, id)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if claim.RobotID == robotID && claim.ClaimPath == claimPath && claim.Value == claimValue {
			if claim.IdentityProviderID != id {
				return fmt.Errorf(
					"claim %s=%s returned identity_provider_id=%d (want path IdP %d)",
					claimPath,
					claimValue,
					claim.IdentityProviderID,
					id,
				)
			}
			return nil
		}
	}
	return fmt.Errorf("claim %s=%s for robot_id=%d not found on IdP %s", claimPath, claimValue, robotID, label)
}

func thenRobotAssociatedWithIdP(ctx context.Context, robotLabel_, idpLabel_ string) error {
	s := state.Get(ctx)
	robotID, err := namedRobotID(s, robotLabel_)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/robots/%d", robotID))
	if err != nil {
		return err
	}
	var body struct {
		FederatedidpID *int64 `json:"federatedidp_id"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return fmt.Errorf("decode robot: %w", err)
	}
	if body.FederatedidpID == nil || *body.FederatedidpID == 0 {
		return fmt.Errorf("robot %s has no federatedidp_id", robotLabel_)
	}
	idpID, err := namedIdPID(s, idpLabel_)
	if err != nil {
		return err
	}
	if *body.FederatedidpID != idpID {
		return fmt.Errorf("robot linked to IdP %d (want %d)", *body.FederatedidpID, idpID)
	}
	return nil
}

func thenRobotNotAssociatedWithIdP(ctx context.Context, robotLabel_, idpLabel_ string) error {
	s := state.Get(ctx)
	robotID, err := namedRobotID(s, robotLabel_)
	if err != nil {
		return err
	}
	idpID, err := namedIdPID(s, idpLabel_)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/robots/%d", robotID))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	var body struct {
		FederatedidpID *int64 `json:"federatedidp_id"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return fmt.Errorf("decode robot: %w", err)
	}
	if body.FederatedidpID != nil && *body.FederatedidpID == idpID {
		return fmt.Errorf("robot %s is still associated with IdP %s", robotLabel_, idpLabel_)
	}
	return nil
}

func thenCreatedFederatedRobotHasNoStaticSecret(ctx context.Context) error {
	s := state.Get(ctx)
	if len(s.CreatedRobots) == 0 {
		return fmt.Errorf("no federated robot creation response captured")
	}
	robot := s.CreatedRobots[len(s.CreatedRobots)-1]
	if robot.Secret != "" {
		return fmt.Errorf("federated robot creation response exposed static secret for %s", robot.Name)
	}
	return nil
}

func thenRegistryTokenResponseContainsBearerToken(ctx context.Context) error {
	s := state.Get(ctx)
	if err := thenRequestAuthorized(ctx); err != nil {
		return err
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(s.LastBody, &body); err != nil {
		return fmt.Errorf("decode registry token response: %w", err)
	}
	if body.Token == "" && body.AccessToken == "" {
		return fmt.Errorf("registry token response has empty token/access_token: %s", truncate(s.LastBody))
	}
	return nil
}

func thenRequestAuthorized(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastResp == nil {
		return fmt.Errorf("no captured response")
	}
	if s.LastResp.StatusCode >= 400 {
		return fmt.Errorf("expected authorized response, got %d: %s", s.LastResp.StatusCode, truncate(s.LastBody))
	}
	return nil
}

func thenRequestBadRequest(ctx context.Context) error {
	return mustStatus(state.Get(ctx).LastResp, http.StatusBadRequest)
}

func thenRequestBadRequestWithMessage(ctx context.Context, want string) error {
	s := state.Get(ctx)
	if err := mustStatus(s.LastResp, http.StatusBadRequest); err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(string(s.LastBody)), strings.ToLower(want)) {
		return fmt.Errorf("response body %q does not contain %q", truncate(s.LastBody), want)
	}
	return nil
}

func thenPodRunning(ctx context.Context, timeoutSec int) error {
	s := state.Get(ctx)
	podName := fmt.Sprintf("e2e-fedidp-%s", s.Suffix)
	timeout := time.Duration(timeoutSec) * time.Second
	return pollUntil(ctx, 5*time.Second, timeout, func() (bool, error) {
		out, err := exec.Command("kubectl", "get", "pod", podName, "-n", "default",
			"-o", "jsonpath={.status.phase}").Output()
		if err != nil {
			return false, nil // not yet ready, keep polling
		}
		phase := strings.TrimSpace(string(out))
		return phase == "Running", nil
	})
}

// ============================================================================
// Internal helpers
// ============================================================================

// idpLabel converts a scenario-local alias to a suffixed resource name.
func idpLabel(s *state.Scenario, alias string) string {
	return fmt.Sprintf("%s-%s", alias, s.Suffix)
}

func k8sRegistryHost(s *state.Scenario) string {
	if host := hostFromURL(os.Getenv("E2E_K8S_REGISTRY_HOST")); host != "" {
		return host
	}
	if host := hostFromURL(os.Getenv("HARBOR_K8S_URL")); host != "" {
		return host
	}

	port := "8080"
	if i := strings.LastIndex(s.RegistryHost, ":"); i >= 0 && i+1 < len(s.RegistryHost) {
		port = s.RegistryHost[i+1:]
	}
	if out, err := exec.Command("docker", "--version").Output(); err == nil && strings.Contains(strings.ToLower(string(out)), "podman") {
		return "host.containers.internal:" + port
	}
	return "host.docker.internal:" + port
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return strings.TrimRight(raw, "/")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

type fedIDPMockOptions struct {
	scheme           string
	missingIssuer    bool
	missingJWKSURI   bool
	invalidDiscovery bool
}

type oidcDiscoveryForTest struct {
	JWKSURI string `json:"jwks_uri"`
}

func trustedHTTPSDiscoveryURL() string {
	if url := strings.TrimSpace(os.Getenv("FEDIDP_HTTPS_OIDC_DISCOVERY_URL")); url != "" {
		return url
	}
	return "https://accounts.google.com/.well-known/openid-configuration"
}

func fetchDiscoveryForTest(discoveryURL string) (*oidcDiscoveryForTest, error) {
	resp, err := http.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("fetch test OIDC discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch test OIDC discovery: status %d", resp.StatusCode)
	}
	var discovery oidcDiscoveryForTest
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, fmt.Errorf("decode test OIDC discovery: %w", err)
	}
	if discovery.JWKSURI == "" {
		return nil, fmt.Errorf("test OIDC discovery has empty jwks_uri")
	}
	return &discovery, nil
}

func startFedIDPOIDCMock(s *state.Scenario, opts fedIDPMockOptions) string {
	if s.HTTPMock != nil {
		s.HTTPMock.Close()
	}
	idp, err := fakeidp.New("https://placeholder.example.test")
	if err != nil {
		panic(err)
	}
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if opts.invalidDiscovery {
			_, _ = w.Write([]byte(`{invalid-json`))
			return
		}
		doc := map[string]any{
			"issuer":                                base,
			"jwks_uri":                              base + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "email", "name"},
		}
		if opts.missingIssuer {
			delete(doc, "issuer")
		}
		if opts.missingJWKSURI {
			delete(doc, "jwks_uri")
		}
		_ = json.NewEncoder(w).Encode(doc)
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(idp.JWKS())
	})
	server := httptest.NewUnstartedServer(mux)
	if opts.scheme == "https" {
		server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		server.StartTLS()
	} else {
		server.Start()
	}
	base = server.URL
	s.HTTPMock = server
	return base
}

// namedIdPID looks up the numeric ID of an IdP by its scenario-local alias.
// It iterates CreatedIdPs and maps them by listing the API.
func namedIdPID(s *state.Scenario, alias string) (int64, error) {
	fullLabel := idpLabel(s, alias)
	for i := len(s.CreatedIdPs) - 1; i >= 0; i-- {
		id := s.CreatedIdPs[i]
		resp, err := s.Client.Get(fmt.Sprintf("%s/%d", idpAPIBase, id))
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		var item struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(resp.Body, &item); err == nil && item.Name == fullLabel {
			return id, nil
		}
	}

	resp, err := s.Client.Get(idpAPIBase + "?page_size=100")
	if err != nil {
		return 0, err
	}
	var list []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		return 0, fmt.Errorf("decode IdP list: %w", err)
	}
	for _, item := range list {
		if item.Name == fullLabel {
			return item.ID, nil
		}
	}
	return 0, fmt.Errorf("IdP %q not found (looking for %q)", alias, fullLabel)
}

// namedRobotID returns the robot ID for the last robot matching the alias prefix.
func namedRobotID(s *state.Scenario, alias string) (int64, error) {
	prefix := fmt.Sprintf("robot$%s-%s", alias, s.Suffix)
	for i := len(s.CreatedRobots) - 1; i >= 0; i-- {
		if strings.Contains(s.CreatedRobots[i].Name, alias+"-"+s.Suffix) ||
			s.CreatedRobots[i].Name == prefix {
			return s.CreatedRobots[i].ID, nil
		}
	}
	// Fall back to last created robot.
	if len(s.CreatedRobots) > 0 {
		return s.CreatedRobots[len(s.CreatedRobots)-1].ID, nil
	}
	return 0, fmt.Errorf("no robot found for alias %q", alias)
}

// lookupFakeIdP finds the in-process fake IdP by scenario-local alias.
func lookupFakeIdP(s *state.Scenario, alias string) (*fakeidp.IdP, error) {
	fullLabel := idpLabel(s, alias)
	if idp, ok := s.FakeIdPs[fullLabel]; ok && idp != nil {
		return idp, nil
	}
	return nil, fmt.Errorf("no fake IdP registered for %q (did you use 'an offline federated identity provider'?)", alias)
}

func issueJWT(s *state.Scenario, idpLabel_ string, claims map[string]any) (string, error) {
	idp, err := lookupFakeIdP(s, idpLabel_)
	if err != nil {
		return "", err
	}
	token, err := idp.IssueJWT(claims, 5*time.Minute)
	if err != nil {
		return "", fmt.Errorf("issue JWT: %w", err)
	}
	return token, nil
}

type idpClaim struct {
	IdentityProviderID int64  `json:"identity_provider_id"`
	RobotID            int64  `json:"robot_id"`
	ClaimPath          string `json:"claim_path"`
	Value              string `json:"value"`
}

type idpView struct {
	ID                  int64           `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	Issuer              string          `json:"issuer"`
	OfflineValidation   *bool           `json:"offline_validation"`
	OpenIDConfigURL     string          `json:"openid_config_url"`
	JWKSURI             string          `json:"jwks_uri"`
	JWKSKeys            json.RawMessage `json:"jwks_keys"`
	SupportedAlgorithms []string        `json:"supported_algorithms"`
	ClaimsSupported     []string        `json:"claims_supported"`
	ProjectID           int64           `json:"project_id"`
}

func getIdPByLabel(s *state.Scenario, label string) (*idpView, error) {
	id, err := namedIdPID(s, label)
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d", idpAPIBase, id))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get IdP %s: status %d: %s", label, resp.StatusCode, truncate(resp.Body))
	}
	var body idpView
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("decode IdP: %w", err)
	}
	return &body, nil
}

func idPListContains(s *state.Scenario, path, name string) (bool, int, error) {
	resp, err := s.Client.Get(path)
	if err != nil {
		return false, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("list IdPs: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		return false, 0, fmt.Errorf("decode IdP list: %w", err)
	}
	for _, item := range list {
		if item.Name == name {
			return true, len(list), nil
		}
	}
	return false, len(list), nil
}

func listIdPClaims(s *state.Scenario, idpID int64) ([]idpClaim, error) {
	resp, err := s.Client.Get(fmt.Sprintf("%s/%d/claims", idpAPIBase, idpID))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list IdP claims: status %d: %s", resp.StatusCode, truncate(resp.Body))
	}
	var claims []idpClaim
	if err := json.Unmarshal(resp.Body, &claims); err != nil {
		return nil, fmt.Errorf("decode IdP claims: %w", err)
	}
	return claims, nil
}

// createFedRobot creates a robot linked to a FedIDP and tracks it in state.
func createFedRobot(ctx context.Context, robotAlias, idpAlias, projAlias string) (context.Context, error) {
	ctx, err := createFedRobotAllowFailure(ctx, robotAlias, idpAlias, projAlias)
	if err != nil {
		return ctx, err
	}
	s := state.Get(ctx)
	if s.LastResp == nil || s.LastResp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("create federated robot %s: %d %s", robotAlias, s.LastResp.StatusCode, truncate(s.LastBody))
	}
	return ctx, nil
}

func createFedRobotAllowFailure(ctx context.Context, robotAlias, idpAlias, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projAlias)
	idpID, err := namedIdPID(s, idpAlias)
	if err != nil {
		return ctx, err
	}
	name := fmt.Sprintf("%s-%s", robotAlias, s.Suffix)
	body := map[string]any{
		"name":            name,
		"duration":        1,
		"level":           "project",
		"federatedidp_id": idpID,
		"permissions": []map[string]any{
			{
				"kind":      "project",
				"namespace": project,
				"access": []map[string]any{
					{"resource": "repository", "action": "list"},
					{"resource": "repository", "action": "pull"},
				},
			},
		},
	}
	resp, err := s.Client.Post("/api/v2.0/robots", body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, nil
	}
	return ctx, trackCreatedRobot(s, resp.Body, project)
}

func trackCreatedRobot(s *state.Scenario, body []byte, project string) error {
	var created struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("decode robot: %w", err)
	}
	s.LastRobotIdPID = created.ID
	s.CreatedRobots = append(s.CreatedRobots, state.Robot{
		ID:      created.ID,
		Name:    created.Name,
		Secret:  created.Secret,
		Project: project,
	})
	return nil
}

// useBearer hits a private project-scoped endpoint with a Bearer token and captures the response.
func useBearer(ctx context.Context, s *state.Scenario, token string) (context.Context, error) {
	path := "/api/v2.0/users/current"
	if len(s.CreatedRobots) > 0 {
		project := s.CreatedRobots[len(s.CreatedRobots)-1].Project
		path = fmt.Sprintf("/api/v2.0/projects/%s/repositories?page_size=1", project)
	}
	resp, err := s.Client.DoWithBearer(http.MethodGet, path, token, nil)
	captureResp(s, resp, err)
	return ctx, err
}

// idpGoneByName checks that no IdP with the given name appears in the list.
func idpGoneByName(s *state.Scenario, name string) error {
	resp, err := s.Client.Get(idpAPIBase + "?page_size=100")
	if err != nil {
		return err
	}
	var list []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(resp.Body, &list)
	for _, item := range list {
		if item.Name == name {
			return fmt.Errorf("IdP %q still present after delete", name)
		}
	}
	return nil
}

// removeInt64 removes the first occurrence of v from *slice.
func removeInt64(slice *[]int64, v int64) {
	out := (*slice)[:0]
	removed := false
	for _, x := range *slice {
		if x == v && !removed {
			removed = true
			continue
		}
		out = append(out, x)
	}
	*slice = out
}

func claimRulesPayload(robotID int64, claimPath, value string) map[string]any {
	return map[string]any{
		"rules": []map[string]any{
			{
				"robot_id":   robotID,
				"claim_path": claimPath,
				"value":      value,
			},
		},
	}
}
