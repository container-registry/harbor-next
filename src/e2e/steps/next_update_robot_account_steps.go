//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/state"
)

type nextRobotAccess struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type nextRobotPermission struct {
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	Access    []nextRobotAccess `json:"access"`
}

type nextRobotPayload struct {
	Name           string                `json:"name"`
	Description    string                `json:"description,omitempty"`
	Level          string                `json:"level"`
	Duration       int64                 `json:"duration"`
	Disable        bool                  `json:"disable"`
	FederatedidpID *int64                `json:"federatedidp_id,omitempty"`
	Permissions    []nextRobotPermission `json:"permissions"`
}

type nextRobotDetails struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Level       string                `json:"level"`
	Duration    *int64                `json:"duration"`
	Disable     bool                  `json:"disable"`
	Permissions []nextRobotPermission `json:"permissions"`
}

type nextRobotCreated struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

func registerNextUpdateRobotAccount(sc *godog.ScenarioContext) {
	sc.Given(`^a project robot "([^"]+)" has pull permission in "([^"]+)"$`, givenProjectRobotHasPull)
	sc.Given(`^a federated robot "([^"]+)" linked to "([^"]+)" requesting robot management and pull permission on "([^"]+)"$`, givenFederatedRobotWithRobotManagementAndPull)

	sc.When(`^the admin creates federated robot "([^"]+)" linked to "([^"]+)" requesting robot management and pull permission on "([^"]+)"$`, whenAdminCreatesFederatedRobotWithRobotManagementAndPull)
	sc.When(`^the admin updates federated robot "([^"]+)" requesting robot management and pull permission on "([^"]+)"$`, whenAdminUpdatesFederatedRobotWithRobotManagementAndPull)
	sc.When(`^federated robot "([^"]+)" attempts to create child robot "([^"]+)" with pull permission in "([^"]+)" using JWT claims aud="([^"]+)", sub="([^"]+)" signed by "([^"]+)"$`, whenFederatedRobotAttemptsCreateChildWithJWT)
	sc.When(`^federated robot "([^"]+)" attempts to update robot "([^"]+)" description to "([^"]+)" using JWT claims aud="([^"]+)", sub="([^"]+)" signed by "([^"]+)"$`, whenFederatedRobotAttemptsUpdateRobotWithJWT)

	sc.Then(`^robot "([^"]+)" has robot permission actions "([^"]*)" in "([^"]+)"$`, thenRobotHasRobotPermissionActions)
	sc.Then(`^robot "([^"]+)" does not have robot create or update permission in "([^"]+)"$`, thenRobotDoesNotHaveCreateOrUpdatePermission)
}

func givenProjectRobotHasPull(ctx context.Context, alias, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projectAlias)
	return createNamedRobot(ctx, alias, project, projectRobotPermissions(project, nil, []string{"pull"}))
}

func givenFederatedRobotWithRobotManagementAndPull(ctx context.Context, alias, idpAlias, projectAlias string) (context.Context, error) {
	return createFederatedRobotWithRobotManagementAndPull(ctx, alias, idpAlias, projectAlias)
}

func whenAdminCreatesFederatedRobotWithRobotManagementAndPull(ctx context.Context, alias, idpAlias, projectAlias string) (context.Context, error) {
	return createFederatedRobotWithRobotManagementAndPull(ctx, alias, idpAlias, projectAlias)
}

func whenAdminUpdatesFederatedRobotWithRobotManagementAndPull(ctx context.Context, alias, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	details, err := namedRobotDetails(s, alias)
	if err != nil {
		return ctx, err
	}
	project := projectName(s, projectAlias)
	body := updateRobotPayload(details, "federated robot management permissions requested", projectRobotPermissions(project, robotManagementActions(), []string{"pull"}))
	resp, err := s.Client.Put(fmt.Sprintf("/api/v2.0/robots/%d", details.ID), body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusOK {
		return ctx, fmt.Errorf("update federated robot %q: %d %s", alias, resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func whenFederatedRobotAttemptsCreateChildWithJWT(ctx context.Context, _ string, childAlias, projectAlias, aud, sub, idpAlias string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpAlias, map[string]any{"aud": aud, "sub": sub})
	if err != nil {
		return ctx, err
	}
	project := projectName(s, projectAlias)
	resp, err := s.Client.DoWithBearer(http.MethodPost, "/api/v2.0/robots", token, robotPayload(robotAccountName(s, childAlias), project, projectRobotPermissions(project, nil, []string{"pull"}), nil))
	captureResp(s, resp, err)
	if err == nil && resp.StatusCode == http.StatusCreated {
		if trackErr := trackCreatedRobotAlias(s, childAlias, resp.Body, project); trackErr != nil {
			return ctx, trackErr
		}
	}
	return ctx, err
}

func whenFederatedRobotAttemptsUpdateRobotWithJWT(ctx context.Context, _ string, targetAlias, description, aud, sub, idpAlias string) (context.Context, error) {
	s := state.Get(ctx)
	token, err := issueJWT(s, idpAlias, map[string]any{"aud": aud, "sub": sub})
	if err != nil {
		return ctx, err
	}
	details, err := namedRobotDetails(s, targetAlias)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.DoWithBearer(http.MethodPut, fmt.Sprintf("/api/v2.0/robots/%d", details.ID), token, updateRobotPayload(details, description, details.Permissions))
	captureResp(s, resp, err)
	return ctx, err
}

func thenRobotHasRobotPermissionActions(ctx context.Context, alias, wantActions, projectAlias string) error {
	s := state.Get(ctx)
	details, err := namedRobotDetails(s, alias)
	if err != nil {
		return err
	}
	project := projectName(s, projectAlias)
	got := robotActions(details.Permissions, project)
	want := actionSet(wantActions)
	if !sameStringSet(got, want) {
		return fmt.Errorf("robot %q robot actions %v, want %v", alias, sortedKeys(got), sortedKeys(want))
	}
	return nil
}

func thenRobotDoesNotHaveCreateOrUpdatePermission(ctx context.Context, alias, projectAlias string) error {
	s := state.Get(ctx)
	details, err := namedRobotDetails(s, alias)
	if err != nil {
		return err
	}
	actions := robotActions(details.Permissions, projectName(s, projectAlias))
	for _, blocked := range []string{"create", "update"} {
		if actions[blocked] {
			return fmt.Errorf("robot %q still has robot:%s permission", alias, blocked)
		}
	}
	return nil
}

func createNamedRobot(ctx context.Context, alias, project string, permissions []nextRobotPermission) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Post("/api/v2.0/robots", robotPayload(robotAccountName(s, alias), project, permissions, nil))
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("create robot %q: %d %s", alias, resp.StatusCode, truncate(resp.Body))
	}
	return ctx, trackCreatedRobotAlias(s, alias, resp.Body, project)
}

func createFederatedRobotWithRobotManagementAndPull(ctx context.Context, alias, idpAlias, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projectAlias)
	idpID, err := namedIdPID(s, idpAlias)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Post("/api/v2.0/robots", robotPayload(robotAccountName(s, alias), project, projectRobotPermissions(project, robotManagementActions(), []string{"pull"}), &idpID))
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("create federated robot %q: %d %s", alias, resp.StatusCode, truncate(resp.Body))
	}
	return ctx, trackCreatedRobotAlias(s, alias, resp.Body, project)
}

func robotPayload(name, project string, permissions []nextRobotPermission, idpID *int64) nextRobotPayload {
	return nextRobotPayload{
		Name:           name,
		Description:    "e2e robot " + name,
		Level:          "project",
		Duration:       -1,
		FederatedidpID: idpID,
		Permissions:    permissions,
	}
}

func updateRobotPayload(details *nextRobotDetails, description string, permissions []nextRobotPermission) nextRobotPayload {
	duration := int64(-1)
	if details.Duration != nil {
		duration = *details.Duration
	}
	return nextRobotPayload{
		Name:        details.Name,
		Description: description,
		Level:       details.Level,
		Duration:    duration,
		Disable:     details.Disable,
		Permissions: permissions,
	}
}

func namedRobotDetails(s *state.Scenario, alias string) (*nextRobotDetails, error) {
	id, err := namedRobotIDForAlias(s, alias)
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/robots/%d", id))
	captureResp(s, resp, err)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get robot %q: %d %s", alias, resp.StatusCode, truncate(resp.Body))
	}
	var details nextRobotDetails
	if err := json.Unmarshal(resp.Body, &details); err != nil {
		return nil, fmt.Errorf("decode robot %q: %w", alias, err)
	}
	return &details, nil
}

func namedRobotIDForAlias(s *state.Scenario, alias string) (int64, error) {
	if rb, ok := s.RobotsByAlias[alias]; ok && rb.ID != 0 {
		return rb.ID, nil
	}
	return namedRobotID(s, alias)
}

func trackCreatedRobotAlias(s *state.Scenario, alias string, body []byte, project string) error {
	var created nextRobotCreated
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("decode created robot: %w", err)
	}
	if created.ID == 0 || created.Name == "" {
		return fmt.Errorf("create robot response missing id/name: %s", truncate(body))
	}
	rb := state.Robot{ID: created.ID, Name: created.Name, Secret: created.Secret, Project: project}
	s.CreatedRobots = append(s.CreatedRobots, rb)
	s.RobotsByAlias[alias] = rb
	return nil
}

func projectRobotPermissions(namespace string, robotActions, repoActions []string) []nextRobotPermission {
	accesses := make([]nextRobotAccess, 0, len(robotActions)+len(repoActions))
	for _, action := range robotActions {
		accesses = append(accesses, nextRobotAccess{Resource: "robot", Action: action})
	}
	for _, action := range repoActions {
		accesses = append(accesses, nextRobotAccess{Resource: "repository", Action: action})
	}
	return []nextRobotPermission{{Kind: "project", Namespace: namespace, Access: accesses}}
}

func robotManagementActions() []string {
	return []string{"create", "read", "update", "list", "delete"}
}

func robotActions(permissions []nextRobotPermission, namespace string) map[string]bool {
	actions := map[string]bool{}
	for _, permission := range permissions {
		if permission.Kind != "project" || permission.Namespace != namespace {
			continue
		}
		for _, access := range permission.Access {
			if access.Resource == "robot" {
				actions[access.Action] = true
			}
		}
	}
	return actions
}

func actionSet(actions string) map[string]bool {
	set := map[string]bool{}
	for _, action := range strings.Split(actions, ",") {
		action = strings.TrimSpace(action)
		if action != "" {
			set[action] = true
		}
	}
	return set
}

func sameStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if !b[key] {
			return false
		}
	}
	return true
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func robotAccountName(s *state.Scenario, alias string) string {
	return fmt.Sprintf("%s-%s", alias, s.Suffix)
}
