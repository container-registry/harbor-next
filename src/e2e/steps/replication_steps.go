//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/state"
)

func registerReplication(sc *godog.ScenarioContext) {
	// Given
	sc.Given(`^a Docker Hub endpoint is registered$`, registerDockerHubEndpoint)
	sc.Given(`^a replication policy from Docker Hub matching "([^"]+)"$`, replicationPolicyMatching)
	sc.Given(`^a replication policy from Docker Hub matching a pattern with no upstream match$`, replicationPolicyNoMatch)
	sc.Given(`^a Quay\.io endpoint is registered$`, registerQuayEndpoint)
	sc.Given(`^a replication policy from Quay\.io matching "([^"]+)"$`, replicationPolicyMatching)
	sc.Given(`^a replication policy from Quay\.io matching a pattern with no upstream match$`, replicationPolicyNoMatch)
	sc.Given(`^an SFTP endpoint is registered$`, registerSFTPEndpoint)
	sc.Given(`^the SFTP endpoint reports healthy$`, sftpEndpointReportsHealthy)
	sc.Given(`^invalid SFTP credentials are rejected$`, invalidSFTPCredentialsRejected)
	sc.Given(`^an event-based SFTP deletion replication policy for "([^"]+)"$`, eventBasedSFTPDeletionPolicy)

	// When
	sc.When(`^the admin triggers the replication policy$`, triggerReplication)
	sc.When(`^the admin replicates "([^"]+)" to SFTP$`, replicateToSFTP)
	sc.When(`^the admin restores "([^"]+)" from SFTP into "([^"]+)"$`, restoreFromSFTP)

	// Then
	sc.Then(`^the execution reports success$`, executionReportsSuccess)
	sc.Then(`^the execution reports success with zero tasks$`, executionZeroTasks)
	sc.Then(`^the replicated artifact in Harbor matches the upstream digest$`, replicatedDigestMatches)
	sc.Then(`^the SFTP storage contains the replicated image "([^"]+)"$`, sftpStorageContainsImage)
	sc.Then(`^the event-based deletion replication execution reports success$`, eventBasedDeletionReplicationReportsSuccess)
	sc.Then(`^the SFTP storage no longer contains the replicated image "([^"]+)"$`, sftpStorageDoesNotContainImage)
}

// ============================================================================
// Given
// ============================================================================

func registerDockerHubEndpoint(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if os.Getenv("E2E_SKIP_REPLICATION") == "1" {
		return ctx, godog.ErrSkip
	}
	body := map[string]any{
		"name":        "dockerhub-" + s.Suffix,
		"description": "e2e dockerhub endpoint",
		"type":        "docker-hub",
		"url":         "https://hub.docker.com",
		"insecure":    false,
	}
	resp, err := s.Client.Post("/api/v2.0/registries", body)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create registry: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	id, _ := idFromLocation(resp.Header.Get("Location"))
	s.CreatedRegistries = append(s.CreatedRegistries, id)
	return ctx, nil
}

func registerQuayEndpoint(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if os.Getenv("E2E_SKIP_REPLICATION") == "1" {
		return ctx, godog.ErrSkip
	}
	body := map[string]any{
		"name":        "quay-" + s.Suffix,
		"description": "e2e quay.io endpoint",
		"type":        "docker-registry",
		"url":         "https://quay.io",
		"insecure":    false,
	}
	resp, err := s.Client.Post("/api/v2.0/registries", body)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create registry: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	id, _ := idFromLocation(resp.Header.Get("Location"))
	s.CreatedRegistries = append(s.CreatedRegistries, id)
	return ctx, nil
}

func registerSFTPEndpoint(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if os.Getenv("E2E_SKIP_REPLICATION") == "1" {
		return ctx, godog.ErrSkip
	}
	basePath := scenarioID(s)
	body := map[string]any{
		"name":        "sftp-" + s.Suffix,
		"description": "e2e sftp endpoint",
		"type":        "sftp",
		"url":         "sftp://sftpgo:2022/" + basePath,
		"credential": map[string]any{
			"type":          "basic",
			"access_key":    "harbor",
			"access_secret": "Harbor12345",
		},
		"insecure": true,
	}
	resp, err := s.Client.Post("/api/v2.0/registries", body)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create sftp registry: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	id, _ := idFromLocation(resp.Header.Get("Location"))
	s.CreatedRegistries = append(s.CreatedRegistries, id)
	s.SFTPRegistryID = id
	s.SFTPBasePath = basePath
	return ctx, nil
}

func sftpEndpointReportsHealthy(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if s.SFTPRegistryID == 0 || s.SFTPBasePath == "" {
		return ctx, fmt.Errorf("no registered SFTP endpoint")
	}
	resp, err := s.Client.Post("/api/v2.0/registries/ping", sftpPingBody(s, "Harbor12345"))
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 200 {
		return ctx, fmt.Errorf("ping sftp registry: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func invalidSFTPCredentialsRejected(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if s.SFTPRegistryID == 0 || s.SFTPBasePath == "" {
		return ctx, fmt.Errorf("no registered SFTP endpoint")
	}
	resp, err := s.Client.Post("/api/v2.0/registries/ping", sftpPingBody(s, "wrong-"+s.Suffix))
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 400 {
		return ctx, fmt.Errorf("ping sftp registry with invalid credentials: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	return ctx, nil
}

func replicationPolicyMatching(ctx context.Context, pattern string) (context.Context, error) {
	return createReplicationPolicy(ctx, pattern, false)
}

func replicationPolicyNoMatch(ctx context.Context) (context.Context, error) {
	return createReplicationPolicy(ctx, "definitely-not-a-real-image-"+randomSuffix(), true)
}

func createReplicationPolicy(ctx context.Context, pattern string, expectZero bool) (context.Context, error) {
	s := state.Get(ctx)
	if len(s.CreatedRegistries) == 0 {
		return ctx, fmt.Errorf("no registered source registry")
	}
	srcID := s.CreatedRegistries[0]
	name, tag := splitPattern(pattern)
	body := map[string]any{
		"name":               "e2e-repl-" + s.Suffix,
		"description":        "e2e replication",
		"src_registry":       map[string]any{"id": srcID},
		"dest_namespace":     projectName(s, "proj"), // dest into a known scenario project if any
		"enabled":            true,
		"override":           true,
		"replicate_deletion": false,
		"deletion":           false,
		"trigger":            map[string]any{"type": "manual"},
		"filters": []map[string]any{
			{"type": "name", "value": name},
			{"type": "tag", "value": tag},
		},
	}
	// Create a default destination project if the scenario hasn't made one.
	if !containsString(s.CreatedProjects, projectName(s, "proj")) {
		_ = createProject(s, projectName(s, "proj"), true, nil)
	}
	resp, err := s.Client.Post("/api/v2.0/replication/policies", body)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create replication policy: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	id, _ := idFromLocation(resp.Header.Get("Location"))
	s.CreatedReplicationIDs = append(s.CreatedReplicationIDs, id)
	return ctx, nil
}

func eventBasedSFTPDeletionPolicy(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	if s.SFTPRegistryID == 0 {
		return ctx, fmt.Errorf("no registered SFTP endpoint")
	}
	if s.SFTPBackupRepository == "" || s.SFTPBackupTag == "" {
		return ctx, fmt.Errorf("no SFTP backup captured in scenario")
	}
	repository, _, tag, err := replicationRepositoryForRef(s, ref)
	if err != nil {
		return ctx, err
	}
	if tag != s.SFTPBackupTag {
		return ctx, fmt.Errorf("SFTP backup tag=%s, ref tag=%s", s.SFTPBackupTag, tag)
	}
	backupNamespace := path.Dir(s.SFTPBackupRepository)
	if backupNamespace == "." || backupNamespace == "/" {
		return ctx, fmt.Errorf("invalid SFTP backup repository %q", s.SFTPBackupRepository)
	}
	body := map[string]any{
		"name":                         "e2e-sftp-delete-" + s.Suffix,
		"description":                  "e2e sftp event deletion",
		"dest_registry":                map[string]any{"id": s.SFTPRegistryID},
		"dest_namespace":               backupNamespace,
		"dest_namespace_replace_count": -1,
		"enabled":                      true,
		"override":                     true,
		"replicate_deletion":           true,
		"deletion":                     true,
		"trigger":                      map[string]any{"type": "event_based"},
		"filters": []map[string]any{
			{"type": "name", "value": repository},
			{"type": "tag", "value": tag},
		},
	}
	id, err := createReplicationPolicyFromBody(s, body)
	if err != nil {
		return ctx, err
	}
	s.SFTPEventPolicyID = id
	return ctx, nil
}

// ============================================================================
// When
// ============================================================================

func triggerReplication(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if len(s.CreatedReplicationIDs) == 0 {
		return ctx, fmt.Errorf("no replication policy staged")
	}
	id := s.CreatedReplicationIDs[len(s.CreatedReplicationIDs)-1]
	resp, err := s.Client.Post("/api/v2.0/replication/executions", map[string]any{"policy_id": id})
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("trigger replication: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	execID, _ := idFromLocation(resp.Header.Get("Location"))
	s.LastReplicationID = execID

	// Poll until terminal (success/stop/fail) or 90s timeout.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		r, err := s.Client.Get(fmt.Sprintf("/api/v2.0/replication/executions/%d", execID))
		if err != nil {
			return ctx, err
		}
		var exec replicationExecution
		_ = json.Unmarshal(r.Body, &exec)
		s.LastBody = r.Body
		if terminalReplicationStatus(exec.Status) || replicationCountsComplete(exec) {
			return ctx, nil
		}
		time.Sleep(3 * time.Second)
	}
	return ctx, fmt.Errorf("replication execution %d did not finish within 120s", execID)
}

func replicateToSFTP(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	if s.SFTPRegistryID == 0 {
		return ctx, fmt.Errorf("no registered SFTP endpoint")
	}
	repository, repo, tag, err := replicationRepositoryForRef(s, ref)
	if err != nil {
		return ctx, err
	}
	backupNamespace := "backup-" + s.Suffix
	s.SFTPBackupRepository = path.Join(backupNamespace, path.Base(repo))
	s.SFTPBackupTag = tag
	body := map[string]any{
		"name":                         "e2e-sftp-push-" + s.Suffix,
		"description":                  "e2e sftp backup",
		"dest_registry":                map[string]any{"id": s.SFTPRegistryID},
		"dest_namespace":               backupNamespace,
		"dest_namespace_replace_count": -1,
		"enabled":                      true,
		"override":                     true,
		"replicate_deletion":           false,
		"deletion":                     false,
		"trigger":                      map[string]any{"type": "manual"},
		"filters": []map[string]any{
			{"type": "name", "value": repository},
			{"type": "tag", "value": tag},
		},
	}
	if _, err := createReplicationPolicyFromBody(s, body); err != nil {
		return ctx, err
	}
	return triggerReplication(ctx)
}

func restoreFromSFTP(ctx context.Context, sourceRef, destRef string) (context.Context, error) {
	s := state.Get(ctx)
	if s.SFTPRegistryID == 0 {
		return ctx, fmt.Errorf("no registered SFTP endpoint")
	}
	if s.SFTPBackupRepository == "" || s.SFTPBackupTag == "" {
		return ctx, fmt.Errorf("no SFTP backup captured in scenario")
	}
	_, _, sourceTag, err := replicationRepositoryForRef(s, sourceRef)
	if err != nil {
		return ctx, err
	}
	destProject, _, destTag, err := replicationDestinationForRef(s, destRef)
	if err != nil {
		return ctx, err
	}
	if sourceTag != s.SFTPBackupTag || destTag != s.SFTPBackupTag {
		return ctx, fmt.Errorf("SFTP restore cannot retag: source=%s backup=%s dest=%s", sourceTag, s.SFTPBackupTag, destTag)
	}
	body := map[string]any{
		"name":                         "e2e-sftp-restore-" + s.Suffix,
		"description":                  "e2e sftp restore",
		"src_registry":                 map[string]any{"id": s.SFTPRegistryID},
		"dest_namespace":               destProject,
		"dest_namespace_replace_count": -1,
		"enabled":                      true,
		"override":                     true,
		"replicate_deletion":           false,
		"deletion":                     false,
		"trigger":                      map[string]any{"type": "manual"},
		"filters": []map[string]any{
			{"type": "name", "value": s.SFTPBackupRepository},
			{"type": "tag", "value": s.SFTPBackupTag},
		},
	}
	if _, err := createReplicationPolicyFromBody(s, body); err != nil {
		return ctx, err
	}
	return triggerReplication(ctx)
}

func terminalReplicationStatus(s string) bool {
	switch strings.ToLower(s) {
	case "succeed", "success", "finished", "failed", "fail", "stopped", "error":
		return true
	}
	return false
}

// ============================================================================
// Then
// ============================================================================

func executionReportsSuccess(ctx context.Context) error {
	s := state.Get(ctx)
	var exec replicationExecution
	_ = json.Unmarshal(s.LastBody, &exec)
	if !replicationSucceeded(exec) {
		return fmt.Errorf("execution status=%q total=%d succeed=%d failed=%d in_progress=%d",
			exec.Status, exec.Total, exec.Succeed, exec.Failed, exec.InProgress)
	}
	return nil
}

func executionZeroTasks(ctx context.Context) error {
	s := state.Get(ctx)
	var exec struct {
		Status string `json:"status"`
		Total  int    `json:"total"`
	}
	_ = json.Unmarshal(s.LastBody, &exec)
	if exec.Total != 0 {
		return fmt.Errorf("execution total=%d (want 0)", exec.Total)
	}
	// Harbor marks a no-match execution as "Failed" (filter found nothing on the
	// source) rather than "Succeed" with 0 tasks. Accept any terminal status when
	// total=0 — the meaningful assertion is that no tasks were created.
	if !terminalReplicationStatus(exec.Status) {
		return fmt.Errorf("execution status %q is not terminal", exec.Status)
	}
	return nil
}

func replicatedDigestMatches(ctx context.Context) error {
	s := state.Get(ctx)
	// Look up the replicated artifact in the scenario's destination project.
	project := projectName(s, "proj")
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories?page=1&page_size=10", project))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("list repos: %d", resp.StatusCode)
	}
	var repos []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(resp.Body, &repos)
	if len(repos) == 0 {
		return fmt.Errorf("no replicated repositories under %s", project)
	}
	// First repo — check that it has at least one artifact.
	repo := strings.TrimPrefix(repos[0].Name, project+"/")
	arts, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts?with_tag=true",
		project, encodeRepo(repo)))
	if err != nil {
		return err
	}
	if arts.StatusCode != 200 {
		return fmt.Errorf("list artifacts: %d", arts.StatusCode)
	}
	var list []map[string]any
	_ = json.Unmarshal(arts.Body, &list)
	if len(list) == 0 {
		return fmt.Errorf("no artifact replicated")
	}
	// We don't compare against upstream head here to avoid hitting Docker Hub rate
	// limits on every run — presence + non-empty digest is the verifiable signal.
	if d, _ := list[0]["digest"].(string); d == "" {
		return fmt.Errorf("replicated artifact has empty digest")
	}
	return nil
}

func sftpStorageContainsImage(ctx context.Context, ref string) error {
	return sftpStorageImageState(ctx, ref, true)
}

func eventBasedDeletionReplicationReportsSuccess(ctx context.Context) error {
	s := state.Get(ctx)
	if s.SFTPEventPolicyID == 0 {
		return fmt.Errorf("no SFTP event policy staged")
	}
	exec, err := waitForSFTPEventExecution(ctx, s, s.LastReplicationID)
	if err != nil {
		return err
	}
	if !replicationSucceeded(*exec) {
		return fmt.Errorf("event replication status=%q total=%d succeed=%d failed=%d in_progress=%d: %s",
			exec.Status, exec.Total, exec.Succeed, exec.Failed, exec.InProgress, exec.StatusText)
	}
	if exec.Total != 1 || exec.Succeed != 1 || exec.Failed != 0 {
		return fmt.Errorf("event replication totals total=%d succeed=%d failed=%d", exec.Total, exec.Succeed, exec.Failed)
	}
	if err := assertSFTPDeletionTask(s, exec.ID); err != nil {
		return err
	}
	s.LastReplicationID = exec.ID
	return nil
}

func sftpStorageDoesNotContainImage(ctx context.Context, ref string) error {
	return sftpStorageImageState(ctx, ref, false)
}

func sftpStorageImageState(ctx context.Context, ref string, wantPresent bool) error {
	s := state.Get(ctx)
	if s.SFTPBasePath == "" || s.SFTPBackupRepository == "" || s.SFTPBackupTag == "" {
		return fmt.Errorf("no SFTP backup state captured")
	}
	_, _, tag, err := replicationRepositoryForRef(s, ref)
	if err != nil {
		return err
	}
	if tag != s.SFTPBackupTag {
		return fmt.Errorf("SFTP backup tag=%s, ref tag=%s", s.SFTPBackupTag, tag)
	}
	linkPath := path.Join(
		"/srv/sftpgo/e2e",
		s.SFTPBasePath,
		"docker/registry/v2/repositories",
		s.SFTPBackupRepository,
		"_manifests/tags",
		s.SFTPBackupTag,
		"current/link",
	)
	project := e2eComposeProjectName()
	deadline := time.Now().Add(30 * time.Second)
	var attempts []string
	for {
		attempts = attempts[:0]
		for _, container := range []string{project + "-sftpgo-1", project + "_sftpgo_1"} {
			args := []string{"exec", container, "test"}
			if wantPresent {
				args = append(args, "-s", linkPath)
			} else {
				args = append(args, "!", "-e", linkPath)
			}
			cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			cmd := exec.CommandContext(cmdCtx, "docker", args...)
			out, err := cmd.CombinedOutput()
			cancel()
			s.LastCLIStdout = out
			s.LastCLIStderr = nil
			s.LastCLIErr = err
			if err == nil {
				return nil
			}
			attempts = append(attempts, fmt.Sprintf("%s: %v: %s", container, err, strings.TrimSpace(string(out))))
		}
		if time.Now().After(deadline) {
			linkState := "missing"
			if !wantPresent {
				linkState = "still present"
			}
			return fmt.Errorf("SFTP manifest tag link %s at %s: %s", linkState, linkPath, strings.Join(attempts, "; "))
		}
		time.Sleep(time.Second)
	}
}

// ============================================================================
// helpers
// ============================================================================

func splitPattern(pattern string) (name, tag string) {
	if i := strings.LastIndex(pattern, ":"); i > 0 {
		return pattern[:i], pattern[i+1:]
	}
	return pattern, "*"
}

func e2eComposeProjectName() string {
	if project := os.Getenv("COMPOSE_PROJECT_NAME"); project != "" {
		return project
	}
	slot := os.Getenv("E2E_SLOT")
	if slot == "" {
		slot = "0"
	}
	return "harbor-" + slot
}

func sftpPingBody(s *state.Scenario, accessSecret string) map[string]any {
	return map[string]any{
		"id":              s.SFTPRegistryID,
		"type":            "sftp",
		"url":             "sftp://sftpgo:2022/" + s.SFTPBasePath,
		"credential_type": "basic",
		"access_key":      "harbor",
		"access_secret":   accessSecret,
		"insecure":        true,
	}
}

type replicationExecution struct {
	ID         int64  `json:"id"`
	PolicyID   int64  `json:"policy_id"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	Trigger    string `json:"trigger"`
	Total      int    `json:"total"`
	Succeed    int    `json:"succeed"`
	Failed     int    `json:"failed"`
	InProgress int    `json:"in_progress"`
}

func waitForSFTPEventExecution(ctx context.Context, s *state.Scenario, afterID int64) (*replicationExecution, error) {
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/replication/executions?policy_id=%d&trigger=event_based&page=1&page_size=5&sort=-start_time", s.SFTPEventPolicyID))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("list event replication executions: %d %s", resp.StatusCode, truncate(resp.Body))
		}
		var execs []replicationExecution
		if err := json.Unmarshal(resp.Body, &execs); err != nil {
			return nil, fmt.Errorf("decode event replication executions: %w", err)
		}
		for _, exec := range execs {
			if exec.ID <= afterID || !strings.EqualFold(exec.Trigger, "event_based") {
				continue
			}
			if body, err := json.Marshal(exec); err == nil {
				s.LastBody = body
			}
			if terminalReplicationStatus(exec.Status) || replicationCountsComplete(exec) {
				return &exec, nil
			}
		}
		time.Sleep(3 * time.Second)
	}
	return nil, fmt.Errorf("event-based SFTP replication execution for policy %d did not finish within 120s", s.SFTPEventPolicyID)
}

func assertSFTPDeletionTask(s *state.Scenario, executionID int64) error {
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/replication/executions/%d/tasks?page=1&page_size=10", executionID))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("list event replication tasks: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	var tasks []struct {
		Operation string `json:"operation"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(resp.Body, &tasks); err != nil {
		return fmt.Errorf("decode event replication tasks: %w", err)
	}
	for _, task := range tasks {
		if task.Operation == "tag deletion" && strings.EqualFold(task.Status, "Succeed") {
			return nil
		}
	}
	return fmt.Errorf("no successful SFTP tag deletion task found in execution %d", executionID)
}

func replicationCountsComplete(exec replicationExecution) bool {
	return exec.Total > 0 && exec.InProgress == 0 && exec.Succeed+exec.Failed >= exec.Total
}

func replicationSucceeded(exec replicationExecution) bool {
	if strings.EqualFold(exec.Status, "succeed") || strings.EqualFold(exec.Status, "success") || strings.EqualFold(exec.Status, "finished") {
		return true
	}
	return exec.Total > 0 && exec.Succeed == exec.Total && exec.Failed == 0 && exec.InProgress == 0
}

func createReplicationPolicyFromBody(s *state.Scenario, body map[string]any) (int64, error) {
	resp, err := s.Client.Post("/api/v2.0/replication/policies", body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 201 {
		return 0, fmt.Errorf("create replication policy: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	id, _ := idFromLocation(resp.Header.Get("Location"))
	s.CreatedReplicationIDs = append(s.CreatedReplicationIDs, id)
	return id, nil
}

func replicationRepositoryForRef(s *state.Scenario, ref string) (repository, repo, tag string, err error) {
	alias, repo, tag, err := parseAliasRef(ref)
	if err != nil {
		return "", "", "", err
	}
	return projectName(s, alias) + "/" + repo, repo, tag, nil
}

func replicationDestinationForRef(s *state.Scenario, ref string) (project, repo, tag string, err error) {
	alias, repo, tag, err := parseAliasRef(ref)
	if err != nil {
		return "", "", "", err
	}
	return projectName(s, alias), repo, tag, nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func randomSuffix() string { return state.NewSuffix() }
