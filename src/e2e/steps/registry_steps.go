//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/imagebuilder"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

func registerRegistry(sc *godog.ScenarioContext) {
	// Given
	sc.Given(`^an image pushed to "([^"]+)"$`, anImagePushedTo)
	sc.Given(`^the multi-arch build fixture$`, theMultiArchBuildFixture)
	sc.Given(`^an immutability rule on "([^"]+)" matching tags "([^"]+)"$`, immutabilityRuleOnProject)

	// When
	sc.When(`^the admin pushes a synthetic image to "([^"]+)"$`, adminPushesSyntheticImage)
	sc.When(`^the admin pushes an image to "([^"]+)"$`, adminPushesSyntheticImage)
	sc.When(`^the admin pulls "([^"]+)"$`, adminPullsImage)
	sc.When(`^buildx pushes the fixture to "([^"]+)" for "([^"]+)"$`, buildxPushPlatforms)
	sc.When(`^buildx pushes the fixture to "([^"]+)" with provenance attestation$`, buildxPushProvenance)
	sc.When(`^each platform manifest is pulled$`, pullEachPlatformManifest)
	sc.When(`^the admin tags the image as "([^"]+)"$`, adminTagsImageAs)
	sc.When(`^the admin deletes tag "([^"]+)"$`, adminDeletesTag)
	sc.When(`^the admin deletes all tags under "([^"]+)"$`, adminDeletesAllTags)
	sc.When(`^the admin triggers an immediate garbage collection$`, triggerGCNow)
	sc.When(`^the admin pushes a (\d+) (KiB|MiB) image to "([^"]+)"$`, adminPushesSizedImage)
	sc.When(`^the admin pushes a second (\d+) (KiB|MiB) image to "([^"]+)"$`, adminPushesSizedImage)
	sc.When(`^the admin pushes a different image to "([^"]+)"$`, adminPushesDifferentImage)

	// Then
	sc.Then(`^the artifact and tag "([^"]+)" appear under "([^"]+)"$`, artifactAndTagAppear)
	sc.Then(`^the pulled digest matches the pushed digest$`, pulledDigestMatches)
	sc.Then(`^"([^"]+)" is a manifest index with exactly (\d+) child manifests$`, manifestIndexChildCount)
	sc.Then(`^the child manifests advertise platforms "([^"]+)" and "([^"]+)"$`, childManifestsAdvertisePlatforms)
	sc.Then(`^every pull succeeds$`, everyPullSucceeds)
	sc.Then(`^tag "([^"]+)" still resolves to the original manifest$`, tagStillResolves)
	sc.Then(`^the GC execution completes successfully$`, gcExecutionSuccessful)
	sc.Then(`^pulling the image by digest returns not found$`, pullByDigestNotFound)
	sc.Then(`^the push is rejected as quota exceeded$`, pushRejectedQuota)
	sc.Then(`^the push is rejected as immutable$`, pushRejectedImmutable)
}

// ============================================================================
// Given
// ============================================================================

func anImagePushedTo(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	digest, err := imagebuilder.PushSynthetic(full, 1024, adminCraneOpts(s))
	if err != nil {
		return ctx, fmt.Errorf("push %s: %w", full, err)
	}
	s.LastImageRef = full
	s.PushedDigest = digest
	return ctx, nil
}

func theMultiArchBuildFixture(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	dir, err := tempDir(s, "e2e-buildx-")
	if err != nil {
		return ctx, err
	}
	// Copy fixture files — keep implementation simple, no embed here so fixtures stay directly editable.
	dockerfile := "FROM scratch\nCOPY README /README\nCMD [\"/README\"]\n"
	readme := "harbor e2e fixture\n"
	if err := writeFile(filepath.Join(dir, "Dockerfile"), dockerfile); err != nil {
		return ctx, err
	}
	if err := writeFile(filepath.Join(dir, "README"), readme); err != nil {
		return ctx, err
	}
	s.BuildFixtureDir = dir
	return ctx, nil
}

func immutabilityRuleOnProject(ctx context.Context, alias, pattern string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, alias)
	body := map[string]any{
		"disabled": false,
		"action":   "immutable",
		"template": "immutable_template",
		"tag_selectors": []map[string]any{
			{"kind": "doublestar", "decoration": "matches", "pattern": pattern},
		},
		"scope_selectors": map[string]any{
			"repository": []map[string]any{
				{"kind": "doublestar", "decoration": "repoMatches", "pattern": "**"},
			},
		},
	}
	resp, err := s.Client.Post(fmt.Sprintf("/api/v2.0/projects/%s/immutabletagrules", project), body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create immutability rule: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	if id, ok := idFromLocation(resp.Header.Get("Location")); ok {
		s.CreatedImmutability[project] = append(s.CreatedImmutability[project], id)
	}
	return ctx, nil
}

// ============================================================================
// When — push / pull / buildx
// ============================================================================

func adminPushesSyntheticImage(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	digest, pushErr := imagebuilder.PushSynthetic(full, 1024, adminCraneOpts(s))
	s.LastImageErr = pushErr
	s.LastImageRef = full
	if pushErr == nil {
		s.PushedDigest = digest
	}
	return ctx, nil
}

func adminPullsImage(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	digest, pullErr := imagebuilder.Pull(full, adminCraneOpts(s))
	s.LastImageErr = pullErr
	if pullErr == nil {
		s.PulledDigest = digest
	}
	return ctx, nil
}

func adminPushesSizedImage(ctx context.Context, amount int, unit, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	digest, pushErr := imagebuilder.PushSynthetic(full, storageUnit(amount, unit), adminCraneOpts(s))
	s.LastImageErr = pushErr
	s.LastImageRef = full
	if pushErr == nil {
		s.PushedDigest = digest
	}
	return ctx, nil
}

func adminPushesDifferentImage(ctx context.Context, ref string) (context.Context, error) {
	// A second, distinct random image to the same ref.
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	// random.Image is entropy-seeded so two calls produce different layers.
	digest, pushErr := imagebuilder.PushSynthetic(full, 2048, adminCraneOpts(s))
	s.LastImageErr = pushErr
	s.LastImageRef = full
	if pushErr == nil {
		s.PushedDigest = digest
	}
	return ctx, nil
}

func buildxPushPlatforms(ctx context.Context, ref, platforms string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	if s.BuildFixtureDir == "" {
		return ctx, fmt.Errorf("multi-arch fixture not staged — missing Given step")
	}
	stdout, stderr, err := imagebuilder.BuildxPush(imagebuilder.BuildxOptions{
		ContextDir: s.BuildFixtureDir,
		Dockerfile: filepath.Join(s.BuildFixtureDir, "Dockerfile"),
		Ref:        full,
		Platforms:  platforms,
		Insecure:   s.Client.Insecure(),
		Creds:      adminCreds(s),
	})
	s.LastCLIStdout = stdout
	s.LastCLIStderr = stderr
	s.LastCLIErr = err
	s.LastImageRef = full
	if errors.Is(err, imagebuilder.ErrBuildxMissing) {
		return ctx, godog.ErrSkip
	}
	if err != nil {
		return ctx, fmt.Errorf("buildx: %v\n%s", err, stdout)
	}
	return ctx, nil
}

func buildxPushProvenance(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	if s.BuildFixtureDir == "" {
		return ctx, fmt.Errorf("multi-arch fixture not staged")
	}
	stdout, stderr, err := imagebuilder.BuildxPush(imagebuilder.BuildxOptions{
		ContextDir: s.BuildFixtureDir,
		Dockerfile: filepath.Join(s.BuildFixtureDir, "Dockerfile"),
		Ref:        full,
		Platforms:  "linux/amd64,linux/arm64",
		Attest:     "type=provenance,mode=max",
		Insecure:   s.Client.Insecure(),
		Creds:      adminCreds(s),
	})
	s.LastCLIStdout = stdout
	s.LastCLIStderr = stderr
	s.LastCLIErr = err
	s.LastImageRef = full
	if errors.Is(err, imagebuilder.ErrBuildxMissing) {
		return ctx, godog.ErrSkip
	}
	if err != nil {
		return ctx, fmt.Errorf("buildx (provenance): %v\n%s", err, stdout)
	}
	return ctx, nil
}

func pullEachPlatformManifest(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if s.LastImageRef == "" {
		return ctx, fmt.Errorf("no image ref recorded")
	}
	idx, err := imagebuilder.Index(s.LastImageRef, adminCraneOpts(s))
	if err != nil {
		return ctx, fmt.Errorf("fetch index: %w", err)
	}
	man, err := idx.IndexManifest()
	if err != nil {
		return ctx, err
	}
	repoPart := repoWithoutTag(s.LastImageRef)
	for _, c := range man.Manifests {
		byDigest := repoPart + "@" + c.Digest.String()
		if _, err := imagebuilder.Pull(byDigest, adminCraneOpts(s)); err != nil {
			s.LastImageErr = err
			return ctx, nil // capture, Then asserts
		}
	}
	return ctx, nil
}

// ============================================================================
// When — tags, GC
// ============================================================================

func adminTagsImageAs(ctx context.Context, newRef string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, newRef)
	if err != nil {
		return ctx, err
	}
	if s.LastImageRef == "" {
		return ctx, fmt.Errorf("no prior image to tag")
	}
	if err := imagebuilder.Copy(s.LastImageRef, full, adminCraneOpts(s)); err != nil {
		return ctx, fmt.Errorf("crane copy: %w", err)
	}
	return ctx, nil
}

func adminDeletesTag(ctx context.Context, tag string) (context.Context, error) {
	s := state.Get(ctx)
	if s.LastImageRef == "" {
		return ctx, fmt.Errorf("no prior image ref to locate repository")
	}
	project, repo, err := projectRepoFromFullRef(s, s.LastImageRef)
	if err != nil {
		return ctx, err
	}
	resp, err := s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts/%s/tags/%s",
		project, encodeRepo(repo), s.PushedDigest, tag))
	captureResp(s, resp, err)
	return ctx, err
}

func adminDeletesAllTags(ctx context.Context, repoRef string) (context.Context, error) {
	s := state.Get(ctx)
	parts := strings.SplitN(repoRef, "/", 2)
	if len(parts) != 2 {
		return ctx, fmt.Errorf("invalid repo ref %q", repoRef)
	}
	project := projectName(s, parts[0])
	repo := parts[1]
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts?with_tag=true", project, encodeRepo(repo)))
	if err != nil {
		return ctx, err
	}
	var artifacts []struct {
		Digest string `json:"digest"`
		Tags   []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	_ = json.Unmarshal(resp.Body, &artifacts)
	for _, a := range artifacts {
		for _, t := range a.Tags {
			_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts/%s/tags/%s",
				project, encodeRepo(repo), a.Digest, t.Name))
		}
	}
	return ctx, nil
}

func triggerGCNow(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	body := map[string]any{
		"schedule":   map[string]any{"type": "Manual"},
		"parameters": map[string]any{"delete_untagged": true, "dry_run": false},
	}
	resp, err := s.Client.Post("/api/v2.0/system/gc/schedule", body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("trigger GC: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	// Poll GC history until the most recent run is terminal.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		hist, err := s.Client.Get("/api/v2.0/system/gc?page=1&page_size=1")
		if err != nil {
			return ctx, err
		}
		var runs []struct {
			ID        int64  `json:"id"`
			JobStatus string `json:"job_status"`
			JobKind   string `json:"job_kind"`
		}
		_ = json.Unmarshal(hist.Body, &runs)
		if len(runs) > 0 && terminalJobStatus(runs[0].JobStatus) {
			s.LastBody = hist.Body
			s.LastResp = &http.Response{StatusCode: hist.StatusCode, Header: hist.Header}
			return ctx, nil
		}
		time.Sleep(2 * time.Second)
	}
	return ctx, fmt.Errorf("GC did not finish within 90s")
}

func terminalJobStatus(s string) bool {
	switch strings.ToLower(s) {
	case "finished", "success", "succeed", "stopped", "error", "fail", "failed":
		return true
	}
	return false
}

// ============================================================================
// Then
// ============================================================================

func artifactAndTagAppear(ctx context.Context, tag, repoRef string) error {
	s := state.Get(ctx)
	parts := strings.SplitN(repoRef, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo ref %q", repoRef)
	}
	project := projectName(s, parts[0])
	repo := parts[1]
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts?with_tag=true", project, encodeRepo(repo)))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("list artifacts: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	s.LastArtifacts = nil
	var artifacts []map[string]any
	_ = json.Unmarshal(resp.Body, &artifacts)
	for _, a := range artifacts {
		tags, _ := a["tags"].([]any)
		for _, t := range tags {
			m, _ := t.(map[string]any)
			if name, _ := m["name"].(string); name == tag {
				s.LastArtifacts = artifacts
				return nil
			}
		}
	}
	return fmt.Errorf("tag %q not found on %s/%s", tag, project, repo)
}

func pulledDigestMatches(ctx context.Context) error {
	s := state.Get(ctx)
	if s.PushedDigest == "" || s.PulledDigest == "" {
		return fmt.Errorf("digests not captured: pushed=%q pulled=%q", s.PushedDigest, s.PulledDigest)
	}
	if s.PushedDigest != s.PulledDigest {
		return fmt.Errorf("digest mismatch: pushed=%s pulled=%s", s.PushedDigest, s.PulledDigest)
	}
	return nil
}

func manifestIndexChildCount(ctx context.Context, ref string, n int) error {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return err
	}
	idx, err := imagebuilder.Index(full, adminCraneOpts(s))
	if err != nil {
		return err
	}
	man, err := idx.IndexManifest()
	if err != nil {
		return err
	}
	// Filter out attestation manifests that buildx may emit so "2 child manifests"
	// refers to the architecture manifests the user asked for.
	count := 0
	for _, m := range man.Manifests {
		if m.Platform == nil || m.Platform.Architecture == "unknown" {
			continue
		}
		count++
	}
	if count != n {
		return fmt.Errorf("child manifest count = %d (want %d)", count, n)
	}
	return nil
}

func childManifestsAdvertisePlatforms(ctx context.Context, p1, p2 string) error {
	s := state.Get(ctx)
	if s.LastImageRef == "" {
		return fmt.Errorf("no image ref recorded")
	}
	idx, err := imagebuilder.Index(s.LastImageRef, adminCraneOpts(s))
	if err != nil {
		return err
	}
	man, err := idx.IndexManifest()
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for _, c := range man.Manifests {
		if c.Platform == nil || c.Platform.Architecture == "unknown" {
			continue
		}
		found[c.Platform.OS+"/"+c.Platform.Architecture] = true
	}
	if !found[p1] || !found[p2] {
		return fmt.Errorf("platforms present: %v; want %s and %s", found, p1, p2)
	}
	return nil
}

func everyPullSucceeds(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr != nil {
		return fmt.Errorf("a pull failed: %v", s.LastImageErr)
	}
	return nil
}

func tagStillResolves(ctx context.Context, tag string) error {
	s := state.Get(ctx)
	if s.LastImageRef == "" {
		return fmt.Errorf("no prior image ref")
	}
	project, repo, err := projectRepoFromFullRef(s, s.LastImageRef)
	if err != nil {
		return err
	}
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts/%s", project, encodeRepo(repo), tag))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("tag %s returns %d", tag, resp.StatusCode)
	}
	var art struct {
		Digest string `json:"digest"`
	}
	_ = json.Unmarshal(resp.Body, &art)
	if art.Digest != s.PushedDigest {
		return fmt.Errorf("tag %s resolves to %s, expected %s", tag, art.Digest, s.PushedDigest)
	}
	return nil
}

func gcExecutionSuccessful(ctx context.Context) error {
	s := state.Get(ctx)
	var runs []struct {
		JobStatus string `json:"job_status"`
	}
	_ = json.Unmarshal(s.LastBody, &runs)
	if len(runs) == 0 {
		return fmt.Errorf("no GC executions reported")
	}
	status := strings.ToLower(runs[0].JobStatus)
	if status != "finished" && status != "success" && status != "succeed" {
		return fmt.Errorf("last GC status = %s", runs[0].JobStatus)
	}
	return nil
}

func pullByDigestNotFound(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageRef == "" || s.PushedDigest == "" {
		return fmt.Errorf("missing ref/digest")
	}
	base := repoWithoutTag(s.LastImageRef)
	_, status, _ := imagebuilder.Head(base+"@"+s.PushedDigest, adminCraneOpts(s))
	if status != http.StatusNotFound {
		return fmt.Errorf("image still resolvable by digest (status=%d)", status)
	}
	return nil
}

func pushRejectedQuota(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr == nil {
		return fmt.Errorf("push succeeded; quota was not enforced")
	}
	if imagebuilder.ClassifyPushError(s.LastImageErr) != "quota_exceeded" {
		return fmt.Errorf("error not classified as quota_exceeded: %v", s.LastImageErr)
	}
	return nil
}

func pushRejectedImmutable(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr == nil {
		return fmt.Errorf("push succeeded; immutability was not enforced")
	}
	if imagebuilder.ClassifyPushError(s.LastImageErr) != "immutable" {
		return fmt.Errorf("error not classified as immutable: %v", s.LastImageErr)
	}
	return nil
}

func lastPushSucceeded(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr != nil {
		return fmt.Errorf("push failed: %v", s.LastImageErr)
	}
	return nil
}

func lastPullSucceeded(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr != nil {
		return fmt.Errorf("pull failed: %v", s.LastImageErr)
	}
	return nil
}

func lastPushForbidden(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr == nil {
		return fmt.Errorf("push unexpectedly succeeded")
	}
	kind := imagebuilder.ClassifyPushError(s.LastImageErr)
	if kind != "forbidden" && kind != "unauthorized" {
		return fmt.Errorf("push not forbidden (%s): %v", kind, s.LastImageErr)
	}
	return nil
}

func lastPushDenied(ctx context.Context) error { return lastPushForbidden(ctx) }

// ============================================================================
// Helpers
// ============================================================================

func encodeRepo(repo string) string {
	return strings.ReplaceAll(repo, "/", "%252F")
}

func projectRepoFromFullRef(s *state.Scenario, fullRef string) (string, string, error) {
	// fullRef = host/project/repo:tag or host/project/path/to/repo:tag
	noHost := strings.TrimPrefix(fullRef, s.RegistryHost+"/")
	noTag := strings.SplitN(noHost, ":", 2)[0]
	parts := strings.SplitN(noTag, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("cannot split %q into project/repo", fullRef)
	}
	return parts[0], parts[1], nil
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
