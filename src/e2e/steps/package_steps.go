//go:build e2e

package steps

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/state"
)

// registerPackages installs steps for the native package formats (npm, Maven)
// served by the multiformat adapters at /npm/<project>/ and /maven/<project>/.
//
// Three conventions keep these scenarios trustworthy, each of them a control
// against a way a package test can pass while proving nothing:
//
//   - Package names carry the scenario suffix, so no client cache and no
//     leftover from an earlier run can satisfy an install.
//   - Every client invocation names the registry explicitly, so a missing
//     .npmrc can never send the request to the public registry and have its
//     refusal read as ours.
//   - Where the client refuses an action locally — republishing a version it
//     already knows — the step builds the request itself and reads the status
//     the registry returned. A client-side refusal says nothing about us.
func registerPackages(sc *godog.ScenarioContext) {
	// Given ---------------------------------------------------------------
	sc.Given(`^an npm package "([^"]+)" at version "([^"]+)"$`, anNpmPackage)
	sc.Given(`^an npm package "([^"]+)" at version "([^"]+)" published to "([^"]+)"$`, anNpmPackagePublishedTo)
	sc.Given(`^version "([^"]+)" of "([^"]+)" published to "([^"]+)"$`, aFurtherVersionPublished)
	sc.Given(`^version "([^"]+)" of "([^"]+)" published to "([^"]+)" under tag "([^"]+)"$`, aFurtherVersionPublishedUnderTag)
	sc.Given(`^a Maven artifact "([^"]+)" at version "([^"]+)"$`, aMavenArtifact)

	// When ----------------------------------------------------------------
	sc.When(`^the package is published to "([^"]+)"$`, thePackageIsPublishedTo)
	sc.When(`^the robot publishes the package to "([^"]+)"$`, theRobotPublishesThePackage)
	sc.When(`^the same version is published again with different content$`, theSameVersionIsPublishedAgain)
	sc.When(`^a consumer installs "([^"]+)" from "([^"]+)"$`, aConsumerInstalls)
	sc.When(`^a consumer installs "([^"]+)" from "([^"]+)" without specifying a version$`, aConsumerInstallsAnyVersion)
	sc.When(`^the robot installs "([^"]+)" from "([^"]+)"$`, theRobotInstalls)
	sc.When(`^the robot installs "([^"]+)" from "([^"]+)" with token authentication$`, theRobotInstallsWithToken)
	sc.When(`^the repositories under "([^"]+)" are listed$`, theRepositoriesUnderAreListed)
	sc.When(`^the artifact is deployed to "([^"]+)"$`, theArtifactIsDeployedTo)
	sc.When(`^a consumer project resolves "([^"]+)" from "([^"]+)"$`, aConsumerProjectResolves)

	// Then ----------------------------------------------------------------
	sc.Then(`^the publish succeeds$`, thePublishSucceeds)
	sc.Then(`^the publish is refused$`, thePublishIsRefused)
	sc.Then(`^the publish is rejected as a conflict$`, thePublishIsRejectedAsConflict)
	sc.Then(`^the install succeeds$`, theInstallSucceeds)
	sc.Then(`^the deploy succeeds$`, theDeploySucceeds)
	sc.Then(`^the resolve succeeds$`, theResolveSucceeds)
	sc.Then(`^the installed package contains the published content$`, theInstalledPackageContainsPublishedContent)
	sc.Then(`^the installed version is "([^"]+)"$`, theInstalledVersionIs)
	sc.Then(`^the package is stored in "([^"]+)" with artifact type "([^"]+)"$`, thePackageIsStoredWithArtifactType)
	sc.Then(`^the listed package name is "([^"]+)"$`, theListedPackageNameIs)
	sc.Then(`^nothing is stored in "([^"]+)"$`, nothingIsStoredIn)
}

// ============================================================================
// Naming and URLs
// ============================================================================

// pkgName appends the scenario suffix to the name written in the feature file.
// "@acme/widget" becomes "@acme/widget-<suffix>", which is still a legal npm
// name and cannot collide with anything a previous run left behind.
func pkgName(s *state.Scenario, base string) string { return base + "-" + s.Suffix }

// resolvePkg maps a feature-file package name back to the suffixed name the
// scenario actually published, and fails loudly on a typo rather than quietly
// testing a package that was never created.
func resolvePkg(s *state.Scenario, base string) (string, error) {
	if s.PkgBaseName != base {
		return "", fmt.Errorf("scenario holds package %q, step referenced %q", s.PkgBaseName, base)
	}
	return s.PkgName, nil
}

func npmRegistryURL(s *state.Scenario, project string) string {
	return strings.TrimRight(s.Client.BaseURL, "/") + "/npm/" + project + "/"
}

func mavenRepoURL(s *state.Scenario, project string) string {
	return strings.TrimRight(s.Client.BaseURL, "/") + "/maven/" + project + "/"
}

// publishedMarker is the string the published module exports. It is derived
// from the scenario suffix so an assertion cannot accidentally match content
// published by a different scenario.
func publishedMarker(s *state.Scenario) string { return "published-" + s.Suffix }

// tamperedMarker is the content used when re-publishing an existing version.
// If the registry ever accepted the second publish, an install would return
// this instead of publishedMarker and the scenario would fail.
func tamperedMarker(s *state.Scenario) string { return "tampered-" + s.Suffix }

// ============================================================================
// CLI plumbing
// ============================================================================

// runCLI shells out and records the outcome on scenario state. It deliberately
// does not treat a non-zero exit as a step failure: several scenarios expect
// the client to be refused, and the Then step is what decides.
func runCLI(ctx context.Context, s *state.Scenario, dir, name string, args ...string) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	s.LastCLIErr = cmd.Run()
	s.LastCLIStdout, s.LastCLIStderr = stdout.Bytes(), stderr.Bytes()
}

// cliFailure renders the last shellout for an assertion message. Without the
// captured streams a failing package scenario is almost impossible to triage
// from CI output alone.
func cliFailure(s *state.Scenario) string {
	return fmt.Sprintf("%v\nstdout:\n%s\nstderr:\n%s", s.LastCLIErr, s.LastCLIStdout, s.LastCLIStderr)
}

// writeNpmrc writes a project-scoped .npmrc. Harbor honours HTTP basic
// credentials; docs/artifact-types/npm.md states that bearer tokens are not,
// which is what the @known-issue token scenario probes.
func writeNpmrc(dir, registry, username, password, token string) error {
	scope := strings.TrimPrefix(strings.TrimPrefix(registry, "http://"), "https://")
	scope = "//" + strings.TrimRight(scope, "/") + "/:"

	var b strings.Builder
	fmt.Fprintf(&b, "registry=%s\n", registry)
	if token != "" {
		fmt.Fprintf(&b, "%s_authToken=%s\n", scope, token)
	} else {
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		fmt.Fprintf(&b, "%s_auth=%s\n", scope, auth)
	}
	fmt.Fprintf(&b, "%semail=harbor-e2e@example.com\n", scope)
	return os.WriteFile(filepath.Join(dir, ".npmrc"), []byte(b.String()), 0o600)
}

// ============================================================================
// npm — arrange
// ============================================================================

func anNpmPackage(ctx context.Context, base, version string) (context.Context, error) {
	s := state.Get(ctx)
	dir, err := tempDir(s, "e2e-npm-src-*")
	if err != nil {
		return ctx, err
	}
	s.PkgFormat = "npm"
	s.PkgBaseName = base
	s.PkgName = pkgName(s, base)
	s.PkgVersion = version
	s.PkgDir = dir
	return ctx, writeNpmSources(s, s.PkgVersion, publishedMarker(s))
}

func writeNpmSources(s *state.Scenario, version, marker string) error {
	pkgJSON := fmt.Sprintf(`{"name":%q,"version":%q,"main":"index.js","private":false}`, s.PkgName, version)
	if err := os.WriteFile(filepath.Join(s.PkgDir, "package.json"), []byte(pkgJSON), 0o644); err != nil {
		return err
	}
	body := fmt.Sprintf("module.exports = %q;\n", marker)
	return os.WriteFile(filepath.Join(s.PkgDir, "index.js"), []byte(body), 0o644)
}

func anNpmPackagePublishedTo(ctx context.Context, base, version, projectAlias string) (context.Context, error) {
	ctx, err := anNpmPackage(ctx, base, version)
	if err != nil {
		return ctx, err
	}
	s := state.Get(ctx)
	if err := npmPublish(ctx, s, projectAlias, ""); err != nil {
		return ctx, err
	}
	if s.LastCLIErr != nil {
		return ctx, fmt.Errorf("npm publish %s@%s: %s", s.PkgName, version, cliFailure(s))
	}
	return ctx, nil
}

func aFurtherVersionPublished(ctx context.Context, version, base, projectAlias string) (context.Context, error) {
	return publishFurtherVersion(ctx, version, base, projectAlias, "")
}

func aFurtherVersionPublishedUnderTag(ctx context.Context, version, base, projectAlias, tag string) (context.Context, error) {
	return publishFurtherVersion(ctx, version, base, projectAlias, tag)
}

func publishFurtherVersion(ctx context.Context, version, base, projectAlias, tag string) (context.Context, error) {
	s := state.Get(ctx)
	if _, err := resolvePkg(s, base); err != nil {
		return ctx, err
	}
	if err := writeNpmSources(s, version, publishedMarker(s)); err != nil {
		return ctx, err
	}
	if err := npmPublish(ctx, s, projectAlias, tag); err != nil {
		return ctx, err
	}
	if s.LastCLIErr != nil {
		return ctx, fmt.Errorf("npm publish %s@%s: %s", s.PkgName, version, cliFailure(s))
	}
	return ctx, nil
}

// ============================================================================
// npm — act
// ============================================================================

func npmPublish(ctx context.Context, s *state.Scenario, projectAlias, tag string) error {
	return npmPublishAs(ctx, s, projectAlias, tag, s.Client.Username, s.Client.Password)
}

// npmPublishAs publishes as a named identity — an admin, a project member, or a
// robot. Access-control scenarios need the same publish attempted by different
// people, so the credentials are a parameter rather than always the admin's.
func npmPublishAs(ctx context.Context, s *state.Scenario, projectAlias, tag, username, password string) error {
	project := projectName(s, projectAlias)
	s.PkgProject = project
	registry := npmRegistryURL(s, project)
	if err := writeNpmrc(s.PkgDir, registry, username, password, ""); err != nil {
		return err
	}
	args := []string{"publish", "--registry", registry, "--ignore-scripts", "--no-audit"}
	if tag != "" {
		args = append(args, "--tag", tag)
	}
	runCLI(ctx, s, s.PkgDir, "npm", args...)
	return nil
}

func thePackageIsPublishedTo(ctx context.Context, projectAlias string) (context.Context, error) {
	return ctx, npmPublish(ctx, state.Get(ctx), projectAlias, "")
}

func theRobotPublishesThePackage(ctx context.Context, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	rb := latestRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	project := projectName(s, projectAlias)
	s.PkgProject = project
	registry := npmRegistryURL(s, project)
	if err := writeNpmrc(s.PkgDir, registry, rb.Name, rb.Secret, ""); err != nil {
		return ctx, err
	}
	runCLI(ctx, s, s.PkgDir, "npm", "publish", "--registry", registry, "--ignore-scripts", "--no-audit")
	return ctx, nil
}

// theSameVersionIsPublishedAgain re-publishes an already-published version with
// different content. npm itself refuses this before sending anything — it reads
// the packument, sees the version, and stops — so the step assembles the publish
// request and sends it, which is the only way to learn what the registry does.
func theSameVersionIsPublishedAgain(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	if err := writeNpmSources(s, s.PkgVersion, tamperedMarker(s)); err != nil {
		return ctx, err
	}

	packDir, err := tempDir(s, "e2e-npm-pack-*")
	if err != nil {
		return ctx, err
	}
	runCLI(ctx, s, s.PkgDir, "npm", "pack", "--pack-destination", packDir, "--ignore-scripts")
	if s.LastCLIErr != nil {
		return ctx, fmt.Errorf("npm pack: %s", cliFailure(s))
	}
	tarball, err := singleFileIn(packDir)
	if err != nil {
		return ctx, err
	}
	blob, err := os.ReadFile(filepath.Join(packDir, tarball))
	if err != nil {
		return ctx, err
	}

	sum := sha1.Sum(blob)
	registry := npmRegistryURL(s, s.PkgProject)
	body := map[string]any{
		"_id":       s.PkgName,
		"name":      s.PkgName,
		"dist-tags": map[string]string{"latest": s.PkgVersion},
		"versions": map[string]any{
			s.PkgVersion: map[string]any{
				"_id":     s.PkgName + "@" + s.PkgVersion,
				"name":    s.PkgName,
				"version": s.PkgVersion,
				"main":    "index.js",
				"dist": map[string]any{
					"shasum":  hex.EncodeToString(sum[:]),
					"tarball": registry + s.PkgName + "/-/" + tarball,
				},
			},
		},
		"_attachments": map[string]any{
			tarball: map[string]any{
				"content_type": "application/octet-stream",
				"data":         base64.StdEncoding.EncodeToString(blob),
				"length":       len(blob),
			},
		},
	}

	resp, err := s.Client.Put("/npm/"+s.PkgProject+"/"+s.PkgName, body)
	captureResp(s, resp, err)
	return ctx, err
}

func aConsumerInstalls(ctx context.Context, base, projectAlias string) (context.Context, error) {
	return npmConsume(ctx, base, projectAlias, true, "", "")
}

func aConsumerInstallsAnyVersion(ctx context.Context, base, projectAlias string) (context.Context, error) {
	return npmConsume(ctx, base, projectAlias, false, "", "")
}

func theRobotInstalls(ctx context.Context, base, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	rb := latestRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	return npmConsume(ctx, base, projectAlias, true, rb.Name, rb.Secret)
}

// theRobotInstallsWithToken authenticates with the robot secret as an npm auth
// token, which is the shape the Portal's Usage tab hands to users.
func theRobotInstallsWithToken(ctx context.Context, base, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	rb := latestRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	return npmConsumeWithToken(ctx, base, projectAlias, rb.Secret)
}

func npmConsume(ctx context.Context, base, projectAlias string, pinVersion bool, username, password string) (context.Context, error) {
	s := state.Get(ctx)
	if username == "" {
		username, password = s.Client.Username, s.Client.Password
	}
	return npmInstall(ctx, s, base, projectAlias, pinVersion, username, password, "")
}

func npmConsumeWithToken(ctx context.Context, base, projectAlias, token string) (context.Context, error) {
	return npmInstall(ctx, state.Get(ctx), base, projectAlias, true, "", "", token)
}

// npmInstall runs a real `npm install` in a throwaway consumer project. The
// cache is a fresh directory every time, so a package the registry no longer
// serves can never satisfy an install from a previous step.
func npmInstall(ctx context.Context, s *state.Scenario, base, projectAlias string, pinVersion bool, username, password, token string) (context.Context, error) {
	name, err := resolvePkg(s, base)
	if err != nil {
		return ctx, err
	}
	project := projectName(s, projectAlias)
	registry := npmRegistryURL(s, project)

	dir, err := tempDir(s, "e2e-npm-consumer-*")
	if err != nil {
		return ctx, err
	}
	cacheDir, err := tempDir(s, "e2e-npm-cache-*")
	if err != nil {
		return ctx, err
	}
	consumer := `{"name":"harbor-e2e-consumer","version":"1.0.0","private":true}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(consumer), 0o644); err != nil {
		return ctx, err
	}
	if err := writeNpmrc(dir, registry, username, password, token); err != nil {
		return ctx, err
	}

	spec := name
	if pinVersion {
		spec = name + "@" + s.PkgVersion
	}
	runCLI(ctx, s, dir, "npm", "install", spec,
		"--registry", registry,
		"--cache", cacheDir,
		"--ignore-scripts", "--no-audit", "--no-fund", "--package-lock=false")

	s.InstalledVersion, s.InstalledContent = "", nil
	if s.LastCLIErr == nil {
		if err := readInstalled(s, dir, name); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}

// readInstalled records what actually landed in node_modules, which is the only
// evidence that matters: the registry answered and the client kept the answer.
func readInstalled(s *state.Scenario, consumerDir, name string) error {
	root := filepath.Join(consumerDir, "node_modules", filepath.FromSlash(name))
	manifest, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return fmt.Errorf("read installed manifest: %w", err)
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(manifest, &meta); err != nil {
		return fmt.Errorf("decode installed manifest: %w", err)
	}
	s.InstalledVersion = meta.Version
	s.InstalledContent, err = os.ReadFile(filepath.Join(root, "index.js"))
	return err
}

// ============================================================================
// npm — assert
// ============================================================================

func thePublishSucceeds(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("npm publish failed: %s", cliFailure(s))
	}
	return nil
}

// thePublishIsRefused asserts the client was turned away. The companion step
// "nothing is stored" checks the registry side, because a client that failed
// for its own reasons would satisfy this assertion on its own.
func thePublishIsRefused(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr == nil {
		return fmt.Errorf("npm publish succeeded but should have been refused\nstdout:\n%s", s.LastCLIStdout)
	}
	out := string(s.LastCLIStdout) + string(s.LastCLIStderr)
	if !strings.Contains(out, "401") && !strings.Contains(out, "403") {
		return fmt.Errorf("publish failed without an authorization status from the registry: %s", cliFailure(s))
	}
	return nil
}

func thePublishIsRejectedAsConflict(ctx context.Context) error {
	return mustStatus(state.Get(ctx).LastResp, http.StatusConflict)
}

func theInstallSucceeds(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("npm install failed: %s", cliFailure(s))
	}
	return nil
}

func theInstalledPackageContainsPublishedContent(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("npm install failed: %s", cliFailure(s))
	}
	want := publishedMarker(s)
	if !strings.Contains(string(s.InstalledContent), want) {
		return fmt.Errorf("installed module does not carry the published content %q; got: %s", want, truncate(s.InstalledContent))
	}
	return nil
}

func theInstalledVersionIs(ctx context.Context, want string) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("npm install failed: %s", cliFailure(s))
	}
	if s.InstalledVersion != want {
		return fmt.Errorf("installed version %q (want %q)", s.InstalledVersion, want)
	}
	return nil
}

// ============================================================================
// Registry-side assertions
// ============================================================================

func theRepositoriesUnderAreListed(ctx context.Context, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projectAlias)
	resp, err := s.Client.Get("/api/v2.0/projects/" + project + "/repositories?page=1&page_size=100")
	captureResp(s, resp, err)
	return ctx, err
}

func theListedPackageNameIs(ctx context.Context, base string) error {
	s := state.Get(ctx)
	want, err := resolvePkg(s, base)
	if err != nil {
		return err
	}
	names, err := listedRepositoryNames(s)
	if err != nil {
		return err
	}
	for _, n := range names {
		if n == want {
			return nil
		}
	}
	return fmt.Errorf("no repository listed as %q; listed: %s", want, strings.Join(names, ", "))
}

// listedRepositoryNames strips the project prefix Harbor puts on every
// repository name, leaving what a user would read in the list view.
func listedRepositoryNames(s *state.Scenario) ([]string, error) {
	var repos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(s.LastBody, &repos); err != nil {
		return nil, fmt.Errorf("decode repositories: %w", err)
	}
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, strings.TrimPrefix(r.Name, s.PkgProject+"/"))
	}
	return names, nil
}

func nothingIsStoredIn(ctx context.Context, projectAlias string) error {
	s := state.Get(ctx)
	project := projectName(s, projectAlias)
	resp, err := s.Client.Get("/api/v2.0/projects/" + project + "/repositories?page=1&page_size=100")
	if err != nil {
		return err
	}
	var repos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Body, &repos); err != nil {
		return fmt.Errorf("decode repositories: %w", err)
	}
	if len(repos) != 0 {
		names := make([]string, 0, len(repos))
		for _, r := range repos {
			names = append(names, r.Name)
		}
		return fmt.Errorf("expected an empty project, found: %s", strings.Join(names, ", "))
	}
	return nil
}

// thePackageIsStoredWithArtifactType proves the native package path ran rather
// than the container image path: the stored artifact carries the format's own
// type, not IMAGE.
func thePackageIsStoredWithArtifactType(ctx context.Context, projectAlias, wantType string) error {
	s := state.Get(ctx)
	project := projectName(s, projectAlias)
	resp, err := s.Client.Get("/api/v2.0/projects/" + project + "/repositories?page=1&page_size=100")
	if err != nil {
		return err
	}
	var repos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Body, &repos); err != nil {
		return fmt.Errorf("decode repositories: %w", err)
	}

	var seen []string
	for _, r := range repos {
		short := strings.TrimPrefix(r.Name, project+"/")
		encoded := strings.ReplaceAll(short, "/", "%252F")
		aResp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s/artifacts?page=1&page_size=100", project, encoded))
		if err != nil {
			return err
		}
		var artifacts []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(aResp.Body, &artifacts); err != nil {
			continue
		}
		for _, a := range artifacts {
			if a.Type == wantType {
				return nil
			}
			seen = append(seen, r.Name+"="+a.Type)
		}
	}
	return fmt.Errorf("no artifact of type %q in %q; found: %s", wantType, project, strings.Join(seen, ", "))
}

// ============================================================================
// Maven
// ============================================================================

func aMavenArtifact(ctx context.Context, coordinate, version string) (context.Context, error) {
	s := state.Get(ctx)
	group, artifact, ok := strings.Cut(coordinate, ":")
	if !ok {
		return ctx, fmt.Errorf("invalid Maven coordinate %q; expected <groupId>:<artifactId>", coordinate)
	}
	dir, err := tempDir(s, "e2e-maven-*")
	if err != nil {
		return ctx, err
	}
	s.PkgFormat = "maven"
	s.PkgBaseName = coordinate
	s.MavenGroupID = group
	s.MavenArtifactID = artifact + "-" + s.Suffix
	s.PkgName = group + ":" + s.MavenArtifactID
	s.PkgVersion = version
	s.PkgDir = dir
	return ctx, writeMavenSettings(s, dir)
}

// writeMavenSettings writes credentials for the "harbor" server id used by both
// the producer's distributionManagement and the consumer's repository.
func writeMavenSettings(s *state.Scenario, dir string) error {
	settings := fmt.Sprintf(`<settings>
  <servers>
    <server>
      <id>harbor</id>
      <username>%s</username>
      <password>%s</password>
    </server>
  </servers>
</settings>
`, s.Client.Username, s.Client.Password)
	return os.WriteFile(filepath.Join(dir, "settings.xml"), []byte(settings), 0o600)
}

// theArtifactIsDeployedTo runs a plain `mvn deploy` — no registry-specific
// plugin, which is the business claim under test. The install phase is skipped
// so the artifact never lands in the local repository; the consumer step must
// therefore reach the registry to resolve it.
func theArtifactIsDeployedTo(ctx context.Context, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, projectAlias)
	s.PkgProject = project

	pom := fmt.Sprintf(`<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>%s</groupId>
  <artifactId>%s</artifactId>
  <version>%s</version>
  <packaging>jar</packaging>
  <distributionManagement>
    <repository>
      <id>harbor</id>
      <url>%s</url>
    </repository>
  </distributionManagement>
</project>
`, s.MavenGroupID, s.MavenArtifactID, s.PkgVersion, mavenRepoURL(s, project))
	if err := os.WriteFile(filepath.Join(s.PkgDir, "pom.xml"), []byte(pom), 0o644); err != nil {
		return ctx, err
	}
	runCLI(ctx, s, s.PkgDir, "mvn", "-B", "-ntp",
		"-s", filepath.Join(s.PkgDir, "settings.xml"),
		"-Dmaven.install.skip=true",
		"deploy")
	return ctx, nil
}

func aConsumerProjectResolves(ctx context.Context, coordinate, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	if s.PkgBaseName != coordinate {
		return ctx, fmt.Errorf("scenario holds artifact %q, step referenced %q", s.PkgBaseName, coordinate)
	}
	project := projectName(s, projectAlias)

	dir, err := tempDir(s, "e2e-maven-consumer-*")
	if err != nil {
		return ctx, err
	}
	if err := writeMavenSettings(s, dir); err != nil {
		return ctx, err
	}
	pom := fmt.Sprintf(`<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.acme.consumer</groupId>
  <artifactId>consumer-%s</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
  <repositories>
    <repository>
      <id>harbor</id>
      <url>%s</url>
    </repository>
  </repositories>
  <dependencies>
    <dependency>
      <groupId>%s</groupId>
      <artifactId>%s</artifactId>
      <version>%s</version>
    </dependency>
  </dependencies>
</project>
`, s.Suffix, mavenRepoURL(s, project), s.MavenGroupID, s.MavenArtifactID, s.PkgVersion)
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0o644); err != nil {
		return ctx, err
	}
	runCLI(ctx, s, dir, "mvn", "-B", "-ntp",
		"-s", filepath.Join(dir, "settings.xml"),
		"dependency:resolve")
	return ctx, nil
}

func theDeploySucceeds(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("mvn deploy failed: %s", cliFailure(s))
	}
	return nil
}

func theResolveSucceeds(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("mvn dependency:resolve failed: %s", cliFailure(s))
	}
	return nil
}

// ============================================================================
// Small helpers
// ============================================================================

// latestRobot returns the robot most recently created by a Given step.
func latestRobot(s *state.Scenario) *state.Robot {
	if len(s.CreatedRobots) == 0 {
		return nil
	}
	return &s.CreatedRobots[len(s.CreatedRobots)-1]
}

func singleFileIn(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("no file produced in %s", dir)
}
