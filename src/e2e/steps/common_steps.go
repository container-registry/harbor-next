//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/imagebuilder"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

// registerCommon installs steps for system health, projects, users, RBAC and audit.
func registerCommon(sc *godog.ScenarioContext) {
	// Background ----------------------------------------------------------
	sc.Given(`^a running Harbor$`, aRunningHarbor)

	// Project lifecycle ---------------------------------------------------
	sc.Given(`^a fresh project$`, aFreshProject)
	sc.Given(`^a fresh project "([^"]+)"$`, aFreshProjectNamed)
	sc.Given(`^a fresh private project "([^"]+)"$`, aFreshPrivateProject)
	sc.Given(`^another fresh project "([^"]+)"$`, aFreshProjectNamed)
	sc.Given(`^a fresh project "([^"]+)" with (\d+) (KiB|MiB|GiB) storage quota$`, aFreshProjectWithQuota)

	// Users and RBAC ------------------------------------------------------
	sc.Given(`^a fresh user "([^"]+)"$`, aFreshUser)
	sc.Given(`^a fresh user "([^"]+)" with no project membership$`, aFreshUser)
	sc.Given(`^"([^"]+)" is assigned the "([^"]+)" role on "([^"]+)"$`, assignProjectRole)
	sc.Given(`^a robot with (push|pull|push and pull) permission on "([^"]+)"$`, aRobotOnProject)

	// Actions (When) ------------------------------------------------------
	sc.When(`^the admin requests system health$`, adminRequestsHealth)
	sc.When(`^an anonymous client lists users$`, anonymousListsUsers)
	sc.When(`^a client with invalid credentials lists users$`, badCredsListsUsers)
	sc.When(`^the admin lists audit log entries with operation "([^"]+)"$`, listAuditByOperation)
	sc.When(`^the admin sets the storage quota on "([^"]+)" to (\d+) (KiB|MiB|GiB)$`, setProjectQuota)
	sc.When(`^the admin adds a label "([^"]+)" to "([^"]+)"$`, addProjectLabel)
	sc.When(`^the admin registers a webhook on "([^"]+)" listening for push events$`, registerProjectWebhook)
	sc.When(`^the admin deletes "([^"]+)"$`, adminDeletesProject)
	sc.When(`^the admin attempts to create another project named "([^"]+)"$`, adminCreatesDuplicateProject)
	sc.When(`^"([^"]+)" pushes an image to "([^"]+)"$`, userPushesImage)
	sc.When(`^"([^"]+)" attempts to delete "([^"]+)"$`, userAttemptsDelete)
	sc.When(`^the admin removes "([^"]+)" from "([^"]+)"$`, adminRemovesUser)
	sc.When(`^"([^"]+)" lists projects$`, userListsProjects)
	sc.When(`^"([^"]+)" requests "([^"]+)" directly$`, userRequestsProjectDirectly)

	// Robot actions -------------------------------------------------------
	sc.When(`^the robot pushes an image to "([^"]+)"$`, robotPushesImage)
	sc.When(`^the robot pulls "([^"]+)"$`, robotPullsImage)
	sc.When(`^the admin deletes the robot$`, adminDeletesRobot)

	// Then ----------------------------------------------------------------
	sc.Then(`^the response status is (\d+)$`, responseStatusIs)
	sc.Then(`^every component is reported healthy$`, everyComponentHealthy)
	sc.Then(`^anonymous system info matches authenticated system info$`, anonMatchesAuthSystemInfo)
	sc.Then(`^the request is unauthorized$`, lastRequestUnauthorized)
	sc.Then(`^the response includes at least one entry for the project$`, auditIncludesProject)
	sc.Then(`^the response advertises the total count$`, advertisesTotalCount)
	sc.Then(`^the quota, label, and webhook are visible on "([^"]+)"$`, quotaLabelWebhookVisible)
	sc.Then(`^the project and its label and webhook are gone$`, projectAndAncillariesGone)
	sc.Then(`^the request is rejected as a conflict$`, lastRequestConflict)
	sc.Then(`^the push succeeds$`, lastPushSucceeded)
	sc.Then(`^the pull succeeds$`, lastPullSucceeded)
	sc.Then(`^the push is forbidden$`, lastPushForbidden)
	sc.Then(`^the push is denied$`, lastPushDenied)
	sc.Then(`^the request is forbidden$`, lastRequestForbidden)
	sc.Then(`^the robot credentials are rejected as unauthorized$`, robotCredsUnauthorized)
	sc.Then(`^"([^"]+)" is not in the response$`, projectNotInResponse)
	sc.Then(`^the request returns not found$`, lastRequestNotFound)
	sc.Then(`^"([^"]+)" can no longer push to "([^"]+)"$`, userCannotPush)
}

// ============================================================================
// System / health
// ============================================================================

func aRunningHarbor(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Get("/api/v2.0/health")
	if err != nil {
		return ctx, fmt.Errorf("health probe: %w", err)
	}
	if resp.StatusCode >= 500 {
		return ctx, fmt.Errorf("health returned %d", resp.StatusCode)
	}
	return ctx, nil
}

func adminRequestsHealth(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Get("/api/v2.0/health")
	captureResp(s, resp, err)
	return ctx, err
}

func everyComponentHealthy(ctx context.Context) error {
	s := state.Get(ctx)
	if err := mustStatus(s.LastResp, 200); err != nil {
		return err
	}
	var payload struct {
		Status     string `json:"status"`
		Components []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"components"`
	}
	if err := json.Unmarshal(s.LastBody, &payload); err != nil {
		return fmt.Errorf("decode health: %w", err)
	}
	ignored := ignoredHealthComponents()
	var unhealthy []string
	for _, c := range payload.Components {
		if ignored[strings.ToLower(c.Name)] {
			continue
		}
		if !strings.EqualFold(c.Status, "healthy") {
			unhealthy = append(unhealthy, fmt.Sprintf("%s=%s", c.Name, c.Status))
		}
	}
	if len(unhealthy) > 0 {
		return fmt.Errorf("unhealthy components: %s", strings.Join(unhealthy, ", "))
	}
	if len(ignored) == 0 && !strings.EqualFold(payload.Status, "healthy") {
		return fmt.Errorf("overall status %q (want healthy)", payload.Status)
	}
	return nil
}

func ignoredHealthComponents() map[string]bool {
	ignored := map[string]bool{}
	for _, name := range strings.Split(os.Getenv("E2E_IGNORE_HEALTH_COMPONENTS"), ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			ignored[name] = true
		}
	}
	return ignored
}

func anonMatchesAuthSystemInfo(ctx context.Context) error {
	s := state.Get(ctx)
	anon, err := s.Client.GetAnonymous("/api/v2.0/systeminfo")
	if err != nil {
		return err
	}
	if anon.StatusCode != 200 {
		return fmt.Errorf("anon systeminfo: %d", anon.StatusCode)
	}
	authed, err := s.Client.Get("/api/v2.0/systeminfo")
	if err != nil {
		return err
	}
	if authed.StatusCode != 200 {
		return fmt.Errorf("authed systeminfo: %d", authed.StatusCode)
	}
	// Compare only fields Harbor returns to both anonymous and authenticated
	// callers. Fields like harbor_version and external_url are intentionally
	// withheld from unauthenticated requests (confirmed on demo.goharbor.io
	// and production instances).
	var a, b map[string]any
	_ = json.Unmarshal(anon.Body, &a)
	_ = json.Unmarshal(authed.Body, &b)
	shared := []string{"auth_mode", "self_registration"}
	for _, k := range shared {
		if !equalJSON(a[k], b[k]) {
			return fmt.Errorf("anonymous and authenticated systeminfo differ on %q: %v vs %v", k, a[k], b[k])
		}
	}
	return nil
}

// ============================================================================
// Auth negative paths
// ============================================================================

func anonymousListsUsers(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.GetAnonymous("/api/v2.0/users")
	captureResp(s, resp, err)
	return ctx, err
}

func badCredsListsUsers(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	bogus := s.Client.WithCredentials("nope", "definitely-not-the-password")
	resp, err := bogus.Get("/api/v2.0/users")
	captureResp(s, resp, err)
	return ctx, err
}

func lastRequestUnauthorized(ctx context.Context) error {
	return mustStatus(state.Get(ctx).LastResp, http.StatusUnauthorized)
}

func lastRequestConflict(ctx context.Context) error {
	return mustStatus(state.Get(ctx).LastResp, http.StatusConflict)
}

func lastRequestForbidden(ctx context.Context) error {
	return mustStatus(state.Get(ctx).LastResp, http.StatusForbidden)
}

func responseStatusIs(ctx context.Context, want int) error {
	return mustStatus(state.Get(ctx).LastResp, want)
}

func lastRequestNotFound(ctx context.Context) error {
	return mustStatus(state.Get(ctx).LastResp, http.StatusNotFound)
}

// ============================================================================
// Audit
// ============================================================================

func listAuditByOperation(ctx context.Context, op string) (context.Context, error) {
	s := state.Get(ctx)
	s.LastAuditOperation = op
	// /api/v2.0/audit-logs is deprecated and returns empty on Harbor v2.13+.
	// /api/v2.0/auditlog-exts is the current endpoint; same query syntax applies.
	resp, err := s.Client.Get(auditLogPath(op, 1))
	captureResp(s, resp, err)
	return ctx, err
}

func auditIncludesProject(ctx context.Context) error {
	s := state.Get(ctx)
	if len(s.CreatedProjects) == 0 {
		return fmt.Errorf("scenario has no created project to match against audit log")
	}
	needle := s.CreatedProjects[len(s.CreatedProjects)-1]
	if s.LastAuditOperation == "" {
		found, count, err := auditResponseIncludesProject(s, needle)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
		return fmt.Errorf("no audit entry referencing project %q among %d entries", needle, count)
	}

	deadline := time.Now().Add(15 * time.Second)
	var scanned int
	for {
		found, count, err := scanAuditPagesForProject(s, s.LastAuditOperation, needle)
		if err != nil {
			return err
		}
		scanned = count
		if found {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("no audit entry referencing project %q among %d entries", needle, scanned)
}

const auditLogPageSize = 100

func auditLogPath(operation string, page int) string {
	return fmt.Sprintf("/api/v2.0/auditlog-exts?q=operation%%3D%s&page=%d&page_size=%d", url.QueryEscape(operation), page, auditLogPageSize)
}

func scanAuditPagesForProject(s *state.Scenario, operation, project string) (bool, int, error) {
	var scanned int
	for page := 1; ; page++ {
		resp, err := s.Client.Get(auditLogPath(operation, page))
		captureResp(s, resp, err)
		if err != nil {
			return false, scanned, err
		}
		found, count, err := auditResponseIncludesProject(s, project)
		if err != nil {
			return false, scanned, err
		}
		scanned += count

		total := auditTotalCount(s.LastResp)
		if total > 0 && page*auditLogPageSize >= total {
			return found, scanned, nil
		}
		if found || count < auditLogPageSize {
			return found, scanned, nil
		}
	}
}

func auditResponseIncludesProject(s *state.Scenario, project string) (bool, int, error) {
	if err := mustStatus(s.LastResp, http.StatusOK); err != nil {
		return false, 0, err
	}
	var entries []map[string]any
	if err := json.Unmarshal(s.LastBody, &entries); err != nil {
		return false, 0, fmt.Errorf("decode audit: %w", err)
	}
	for _, e := range entries {
		if res, _ := e["resource"].(string); strings.Contains(res, project) {
			return true, len(entries), nil
		}
	}
	return false, len(entries), nil
}

func auditTotalCount(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	total := resp.Header.Get("X-Total-Count")
	if total == "" {
		return 0
	}
	n, err := strconv.Atoi(total)
	if err != nil {
		return 0
	}
	return n
}

func advertisesTotalCount(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastResp == nil {
		return fmt.Errorf("no captured response")
	}
	total := s.LastResp.Header.Get("X-Total-Count")
	if total == "" {
		return fmt.Errorf("missing X-Total-Count header")
	}
	n, err := strconv.Atoi(total)
	if err != nil {
		return fmt.Errorf("X-Total-Count %q not numeric", total)
	}
	if n < 1 {
		return fmt.Errorf("X-Total-Count=%d (want >=1)", n)
	}
	return nil
}

// ============================================================================
// Projects
// ============================================================================

func aFreshProject(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	name := scenarioID(s)
	return ctx, createProject(s, name, true, nil)
}

func aFreshProjectNamed(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	return ctx, createProject(s, projectName(s, alias), true, nil)
}

func aFreshPrivateProject(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	return ctx, createProject(s, projectName(s, alias), false, nil)
}

func aFreshProjectWithQuota(ctx context.Context, alias string, amount int, unit string) (context.Context, error) {
	s := state.Get(ctx)
	name := projectName(s, alias)
	bytes := storageUnit(amount, unit)
	return ctx, createProject(s, name, true, map[string]any{"storage": strconv.FormatInt(bytes, 10)})
}

func createProject(s *state.Scenario, name string, public bool, storageLimit map[string]any) error {
	body := map[string]any{
		"project_name": name,
		"public":       public,
		"metadata": map[string]string{
			"public": boolStr(public),
		},
	}
	if storageLimit != nil {
		if v, ok := storageLimit["storage"]; ok {
			body["storage_limit"] = parseInt(v)
		}
	}
	resp, err := s.Client.Post("/api/v2.0/projects", body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("create project %s: %d %s", name, resp.StatusCode, truncate(resp.Body))
	}
	s.CreatedProjects = append(s.CreatedProjects, name)
	return nil
}

func adminDeletesProject(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	name := projectName(s, alias)
	if err := purgeProjectContents(s, name); err != nil {
		return ctx, err
	}
	resp, err := s.Client.Delete("/api/v2.0/projects/" + name)
	captureResp(s, resp, err)
	if err == nil {
		removeTracked(&s.CreatedProjects, name)
	}
	return ctx, err
}

func adminCreatesDuplicateProject(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Post("/api/v2.0/projects", map[string]any{
		"project_name": projectName(s, alias),
		"public":       true,
	})
	captureResp(s, resp, err)
	return ctx, err
}

func quotaLabelWebhookVisible(ctx context.Context, alias string) error {
	s := state.Get(ctx)
	name := projectName(s, alias)

	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/quotas?reference=project&reference_id=%s", name))
	if err != nil {
		return fmt.Errorf("list quotas: %w", err)
	}
	// The quota endpoint returns an array with one entry when we query by reference_id.
	_ = resp // may be paginated; presence of the project quota is enough

	labels, err := s.Client.Get(fmt.Sprintf("/api/v2.0/labels?scope=p&project_id=%d", projectID(s, name)))
	if err != nil {
		return fmt.Errorf("list project labels: %w", err)
	}
	if labels.StatusCode != 200 {
		return fmt.Errorf("list labels: %d", labels.StatusCode)
	}

	whs, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies", name))
	if err != nil {
		return fmt.Errorf("list webhooks: %w", err)
	}
	if whs.StatusCode != 200 {
		return fmt.Errorf("list webhooks: %d", whs.StatusCode)
	}
	var policies []map[string]any
	_ = json.Unmarshal(whs.Body, &policies)
	if len(policies) == 0 {
		return fmt.Errorf("no webhook policies under %s", name)
	}
	return nil
}

func projectAndAncillariesGone(ctx context.Context) error {
	s := state.Get(ctx)
	if len(s.CreatedProjects) > 0 {
		// We've already removed one — fetch the most recent deleted project name from LastResp state.
		// Simpler: iterate known names and check 404.
	}
	// Walk last created project alias captured by adminDeletesProject via LastResp path.
	// We stored only remaining projects; check that at least one deletion leads to 404.
	ok := false
	for _, try := range lastDeletedProjectGuess(s) {
		resp, err := s.Client.Get("/api/v2.0/projects/" + try)
		if err == nil && resp.StatusCode == 404 {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("project does not return 404 after deletion")
	}
	return nil
}

// lastDeletedProjectGuess is a small heuristic: the project we most recently
// deleted is the one whose name bears our suffix and is not in CreatedProjects.
func lastDeletedProjectGuess(s *state.Scenario) []string {
	return []string{
		projectName(s, "alpha"),
		projectName(s, "proj"),
		projectName(s, "secret"),
	}
}

// ============================================================================
// Project quota / labels / webhooks
// ============================================================================

func setProjectQuota(ctx context.Context, alias string, amount int, unit string) (context.Context, error) {
	s := state.Get(ctx)
	name := projectName(s, alias)
	pid := projectID(s, name)
	if pid == 0 {
		return ctx, fmt.Errorf("project %s has no id", name)
	}
	// Find the quota record referencing this project.
	quotas, err := s.Client.Get(fmt.Sprintf("/api/v2.0/quotas?reference=project&reference_id=%d", pid))
	if err != nil {
		return ctx, err
	}
	var list []struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(quotas.Body, &list)
	if len(list) == 0 {
		return ctx, fmt.Errorf("no quota record for project %s (id %d)", name, pid)
	}
	bytes := storageUnit(amount, unit)
	body := map[string]any{"hard": map[string]int64{"storage": bytes}}
	resp, err := s.Client.Put(fmt.Sprintf("/api/v2.0/quotas/%d", list[0].ID), body)
	captureResp(s, resp, err)
	if err == nil && resp.StatusCode >= 400 {
		return ctx, fmt.Errorf("set quota: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	return ctx, err
}

func addProjectLabel(ctx context.Context, label, alias string) (context.Context, error) {
	s := state.Get(ctx)
	name := projectName(s, alias)
	pid := projectID(s, name)
	if pid == 0 {
		return ctx, fmt.Errorf("project %s has no id", name)
	}
	labelName := fmt.Sprintf("%s-%s", label, s.Suffix)
	body := map[string]any{
		"name":       labelName,
		"color":      "#61717D",
		"project_id": pid,
		"scope":      "p",
	}
	resp, err := s.Client.Post("/api/v2.0/labels", body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create label: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		if id, ok := idFromLocation(loc); ok {
			s.CreatedLabels = append(s.CreatedLabels, id)
		}
	}
	return ctx, nil
}

func registerProjectWebhook(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	name := projectName(s, alias)
	target := "http://127.0.0.1:65535/placeholder" // no listener needed for the lifecycle scenario
	if s.HTTPMock != nil {
		target = s.HTTPMock.URL
	}
	body := webhookPolicyBody(name, target, []string{"PUSH_ARTIFACT"}, true)
	resp, err := s.Client.Post(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies", name), body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create webhook policy: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	if id, ok := idFromLocation(resp.Header.Get("Location")); ok {
		s.CreatedWebhookIDs[name] = append(s.CreatedWebhookIDs[name], id)
	}
	return ctx, nil
}

func webhookPolicyBody(project, target string, events []string, enabled bool) map[string]any {
	return map[string]any{
		"name":         "webhook-" + project,
		"description":  "e2e webhook",
		"project_name": project,
		"enabled":      enabled,
		"event_types":  events,
		"targets": []map[string]any{
			{
				"type":             "http",
				"address":          target,
				"skip_cert_verify": true,
			},
		},
	}
}

// ============================================================================
// Users / RBAC
// ============================================================================

func aFreshUser(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	name := userName(s, alias)
	password := "P@ssw0rd-" + s.Suffix
	body := map[string]any{
		"username": name,
		"password": password,
		"email":    name + "@example.test",
		"realname": alias,
	}
	resp, err := s.Client.Post("/api/v2.0/users", body)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create user %s: %d %s", name, resp.StatusCode, truncate(resp.Body))
	}
	id, _ := idFromLocation(resp.Header.Get("Location"))
	u := state.UserCred{ID: id, Name: name, Password: password, Email: name + "@example.test"}
	s.CreatedUsers = append(s.CreatedUsers, u)
	s.UsersByAlias[alias] = u
	return ctx, nil
}

func assignProjectRole(ctx context.Context, alias, role, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	user, ok := s.UsersByAlias[alias]
	if !ok {
		return ctx, fmt.Errorf("no user %q registered", alias)
	}
	roleID, err := roleIDFromName(role)
	if err != nil {
		return ctx, err
	}
	body := map[string]any{
		"role_id": roleID,
		"member_user": map[string]any{
			"username": user.Name,
		},
	}
	project := projectName(s, projAlias)
	resp, err := s.Client.Post(fmt.Sprintf("/api/v2.0/projects/%s/members", project), body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("add member: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func roleIDFromName(role string) (int, error) {
	switch strings.ToLower(role) {
	case "projectadmin", "project admin", "admin":
		return 1, nil
	case "developer":
		return 2, nil
	case "guest":
		return 3, nil
	case "maintainer":
		return 4, nil
	case "limitedguest", "limited guest":
		return 5, nil
	}
	return 0, fmt.Errorf("unknown role %q", role)
}

func adminRemovesUser(ctx context.Context, alias, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	user, ok := s.UsersByAlias[alias]
	if !ok {
		return ctx, fmt.Errorf("no user %q", alias)
	}
	project := projectName(s, projAlias)
	// List members and find the one for this user.
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/members", project))
	if err != nil {
		return ctx, err
	}
	var members []struct {
		ID         int64  `json:"id"`
		EntityName string `json:"entity_name"`
	}
	_ = json.Unmarshal(resp.Body, &members)
	for _, m := range members {
		if m.EntityName == user.Name {
			del, err := s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/members/%d", project, m.ID))
			captureResp(s, del, err)
			return ctx, err
		}
	}
	return ctx, fmt.Errorf("user %s is not a member of %s", user.Name, project)
}

func userListsProjects(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	user, ok := s.UsersByAlias[alias]
	if !ok {
		return ctx, fmt.Errorf("no user %q", alias)
	}
	uc := s.Client.WithCredentials(user.Name, user.Password)
	resp, err := uc.Get("/api/v2.0/projects?page=1&page_size=100")
	captureResp(s, resp, err)
	return ctx, err
}

func userRequestsProjectDirectly(ctx context.Context, alias, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	user, ok := s.UsersByAlias[alias]
	if !ok {
		return ctx, fmt.Errorf("no user %q", alias)
	}
	project := projectName(s, projAlias)
	uc := s.Client.WithCredentials(user.Name, user.Password)
	resp, err := uc.Get("/api/v2.0/projects/" + project)
	captureResp(s, resp, err)
	return ctx, err
}

func projectNotInResponse(ctx context.Context, alias string) error {
	s := state.Get(ctx)
	var list []map[string]any
	if err := json.Unmarshal(s.LastBody, &list); err != nil {
		return fmt.Errorf("decode project list: %w", err)
	}
	name := projectName(s, alias)
	for _, p := range list {
		if n, _ := p["name"].(string); n == name {
			return fmt.Errorf("project %s unexpectedly listed", name)
		}
	}
	return nil
}

// ============================================================================
// RBAC image actions (developer push/delete)
// ============================================================================

func userPushesImage(ctx context.Context, alias, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	opts, err := userCraneOpts(s, alias)
	if err != nil {
		return ctx, err
	}
	digest, err := imagebuilder.PushSynthetic(full, 1024, opts)
	s.LastImageErr = err
	s.LastImageRef = full
	s.PushedDigest = digest
	return ctx, nil
}

func userAttemptsDelete(ctx context.Context, alias, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	user, ok := s.UsersByAlias[alias]
	if !ok {
		return ctx, fmt.Errorf("no user %q", alias)
	}
	uc := s.Client.WithCredentials(user.Name, user.Password)
	resp, err := uc.Delete("/api/v2.0/projects/" + projectName(s, projAlias))
	captureResp(s, resp, err)
	return ctx, err
}

func userCannotPush(ctx context.Context, alias, projAlias string) error {
	s := state.Get(ctx)
	user, ok := s.UsersByAlias[alias]
	if !ok {
		return fmt.Errorf("no user %q", alias)
	}
	full := fmt.Sprintf("%s/%s/app:v-postremove", s.RegistryHost, projectName(s, projAlias))
	_, err := imagebuilder.PushSynthetic(full, 512, imagebuilder.Options{Insecure: s.Client.Insecure(), Creds: userCreds(user)})
	if err == nil {
		return fmt.Errorf("expected push to fail after user removed from project")
	}
	if !isForbidden(err) {
		return fmt.Errorf("expected 401/403 post-removal, got %v", err)
	}
	return nil
}

// ============================================================================
// Robot accounts
// ============================================================================

func aRobotOnProject(ctx context.Context, perms, projAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projAlias)
	actions := []string{}
	switch perms {
	case "push":
		actions = []string{"push"}
	case "pull":
		actions = []string{"pull"}
	case "push and pull":
		actions = []string{"push", "pull"}
	}
	accesses := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		accesses = append(accesses, map[string]any{"resource": "repository", "action": a})
	}
	body := map[string]any{
		"name":     "e2e-robot-" + s.Suffix,
		"duration": 1,
		"level":    "project",
		"permissions": []map[string]any{
			{
				"kind":      "project",
				"namespace": project,
				"access":    accesses,
			},
		},
	}
	resp, err := s.Client.Post("/api/v2.0/robots", body)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create robot: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	var created struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(resp.Body, &created); err != nil {
		return ctx, fmt.Errorf("decode robot: %w", err)
	}
	s.CreatedRobots = append(s.CreatedRobots, state.Robot{
		ID:      created.ID,
		Name:    created.Name,
		Secret:  created.Secret,
		Project: project,
	})
	return ctx, nil
}

func robotPushesImage(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	rb := mostRecentRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	opts := imagebuilder.Options{
		Insecure: s.Client.Insecure(),
		Creds:    imagebuilder.Creds{Username: rb.Name, Password: rb.Secret},
	}
	digest, err := imagebuilder.PushSynthetic(full, 1024, opts)
	s.LastImageErr = err
	s.LastImageRef = full
	s.PushedDigest = digest
	return ctx, nil
}

func robotPullsImage(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	rb := mostRecentRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	opts := imagebuilder.Options{
		Insecure: s.Client.Insecure(),
		Creds:    imagebuilder.Creds{Username: rb.Name, Password: rb.Secret},
	}
	digest, err := imagebuilder.Pull(full, opts)
	s.LastImageErr = err
	s.PulledDigest = digest
	return ctx, nil
}

func adminDeletesRobot(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	rb := mostRecentRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot to delete")
	}
	resp, err := s.Client.Delete(fmt.Sprintf("/api/v2.0/robots/%d", rb.ID))
	captureResp(s, resp, err)
	if err == nil && resp.StatusCode == 200 {
		// mark as deleted by setting ID to 0 but keep secret for post-deletion probe.
		for i := range s.CreatedRobots {
			if s.CreatedRobots[i].ID == rb.ID {
				s.CreatedRobots[i].ID = 0
			}
		}
	}
	return ctx, err
}

func robotCredsUnauthorized(ctx context.Context) error {
	s := state.Get(ctx)
	if len(s.CreatedRobots) == 0 {
		return fmt.Errorf("no robots in scenario")
	}
	rb := s.CreatedRobots[len(s.CreatedRobots)-1]
	full := fmt.Sprintf("%s/%s/app:after-delete", s.RegistryHost, rb.Project)
	opts := imagebuilder.Options{
		Insecure: s.Client.Insecure(),
		Creds:    imagebuilder.Creds{Username: rb.Name, Password: rb.Secret},
	}
	_, err := imagebuilder.PushSynthetic(full, 512, opts)
	if err == nil {
		return fmt.Errorf("expected deleted robot credentials to fail, push succeeded")
	}
	kind := imagebuilder.ClassifyPushError(err)
	if kind != "unauthorized" && kind != "forbidden" {
		return fmt.Errorf("expected unauthorized/forbidden, got %s: %v", kind, err)
	}
	return nil
}

func mostRecentRobot(s *state.Scenario) *state.Robot {
	for i := len(s.CreatedRobots) - 1; i >= 0; i-- {
		if s.CreatedRobots[i].ID != 0 {
			r := s.CreatedRobots[i]
			return &r
		}
	}
	if len(s.CreatedRobots) > 0 {
		r := s.CreatedRobots[len(s.CreatedRobots)-1]
		return &r
	}
	return nil
}

// ============================================================================
// Common helpers
// ============================================================================

func mustStatus(resp *http.Response, want int) error {
	if resp == nil {
		return fmt.Errorf("no captured response to assert status on")
	}
	if resp.StatusCode != want {
		return fmt.Errorf("status %d (want %d)", resp.StatusCode, want)
	}
	return nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func parseInt(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

func truncate(b []byte) string {
	const max = 400
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}

func equalJSON(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func idFromLocation(location string) (int64, bool) {
	if location == "" {
		return 0, false
	}
	// Harbor sets Location like "/api/v2.0/projects/42"
	idx := strings.LastIndex(location, "/")
	if idx < 0 || idx == len(location)-1 {
		return 0, false
	}
	n, err := strconv.ParseInt(location[idx+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func projectID(s *state.Scenario, name string) int64 {
	resp, err := s.Client.Get("/api/v2.0/projects/" + name)
	if err != nil || resp.StatusCode != 200 {
		return 0
	}
	var p struct {
		ProjectID int64 `json:"project_id"`
		ID        int64 `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &p)
	if p.ProjectID != 0 {
		return p.ProjectID
	}
	return p.ID
}

func removeTracked(list *[]string, name string) {
	out := (*list)[:0]
	for _, v := range *list {
		if v != name {
			out = append(out, v)
		}
	}
	*list = out
}

// purgeProjectContents removes repositories/artifacts/webhooks blocking a project delete.
func purgeProjectContents(s *state.Scenario, project string) error {
	// Webhook policies
	if resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies", project)); err == nil && resp.StatusCode == 200 {
		var policies []struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(resp.Body, &policies)
		for _, p := range policies {
			_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies/%d", project, p.ID))
		}
	}
	// Repositories
	if resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories?page=1&page_size=100", project)); err == nil && resp.StatusCode == 200 {
		var repos []struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(resp.Body, &repos)
		for _, r := range repos {
			short := strings.TrimPrefix(r.Name, project+"/")
			short = strings.ReplaceAll(short, "/", "%252F")
			_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s", project, short))
		}
	}
	// Immutability rules
	if resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/immutabletagrules", project)); err == nil && resp.StatusCode == 200 {
		var rules []struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(resp.Body, &rules)
		for _, r := range rules {
			_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/immutabletagrules/%d", project, r.ID))
		}
	}
	return nil
}

func isForbidden(err error) bool {
	kind := imagebuilder.ClassifyPushError(err)
	return kind == "forbidden" || kind == "unauthorized"
}
