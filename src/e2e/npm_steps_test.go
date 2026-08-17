//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

type scenarioState struct {
	endpoint string
	name     string
	version  string
	suffix   string

	publishDir string
	installDir string
	stdout     []byte
	stderr     []byte
	err        error
}

type scenarioKey struct{}

func initializeScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, scenarioKey{}, &scenarioState{
			endpoint: strings.TrimRight(os.Getenv("HARBOR_E2E_ENDPOINT"), "/"),
			suffix:   randomSuffix(),
		}), nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s := stateFrom(ctx)
		_ = os.RemoveAll(s.publishDir)
		_ = os.RemoveAll(s.installDir)
		return ctx, nil
	})

	sc.Step(`^a running Harbor$`, runningHarbor)
	sc.Step(`^an npm package "([^"]+)" version "([^"]+)"$`, npmPackage)
	sc.Step(`^the user runs npm publish to Harbor project "([^"]+)"$`, runNPMPublish)
	sc.Step(`^npm publish succeeds$`, lastCommandSucceeded)
	sc.Step(`^the user runs npm install for that package from Harbor project "([^"]+)"$`, runNPMInstall)
	sc.Step(`^npm install succeeds$`, lastCommandSucceeded)
	sc.Step(`^the installed npm package contains the published module$`, installedPackageContainsPublishedModule)
}

func stateFrom(ctx context.Context) *scenarioState {
	s, ok := ctx.Value(scenarioKey{}).(*scenarioState)
	if !ok || s == nil {
		panic("missing scenario state")
	}
	return s
}

func runningHarbor(ctx context.Context) error {
	s := stateFrom(ctx)
	resp, err := http.Get(s.endpoint + "/api/version")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /api/version returned %d", resp.StatusCode)
	}
	return nil
}

func npmPackage(ctx context.Context, name, version string) error {
	s := stateFrom(ctx)
	s.name = name + "-" + s.suffix
	s.version = version

	dir, err := os.MkdirTemp("", "harbor-npm-publish-*")
	if err != nil {
		return err
	}
	s.publishDir = dir

	packageJSON := fmt.Sprintf(`{"name":%q,"version":%q,"main":"index.js"}`, s.name, s.version)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.js"), []byte("module.exports = 'published-from-harbor';\n"), 0o644)
}

func runNPMPublish(ctx context.Context, project string) error {
	s := stateFrom(ctx)
	registry := registryURL(s, project)
	if err := writeNPMRC(s.publishDir, registry); err != nil {
		return err
	}
	run(ctx, s.publishDir, "npm", "publish", "--registry", registry, "--ignore-scripts", "--no-audit")
	return nil
}

func runNPMInstall(ctx context.Context, project string) error {
	s := stateFrom(ctx)
	dir, err := os.MkdirTemp("", "harbor-npm-install-*")
	if err != nil {
		return err
	}
	s.installDir = dir
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"consumer","version":"1.0.0"}`), 0o644); err != nil {
		return err
	}
	registry := registryURL(s, project)
	if err := writeNPMRC(dir, registry); err != nil {
		return err
	}
	run(ctx, dir, "npm", "install", s.name+"@"+s.version, "--registry", registry, "--ignore-scripts", "--no-audit", "--package-lock=false")
	return nil
}

func lastCommandSucceeded(ctx context.Context) error {
	s := stateFrom(ctx)
	if s.err != nil {
		return fmt.Errorf("command failed: %w\nstdout:\n%s\nstderr:\n%s", s.err, s.stdout, s.stderr)
	}
	return nil
}

func installedPackageContainsPublishedModule(ctx context.Context) error {
	s := stateFrom(ctx)
	data, err := os.ReadFile(filepath.Join(s.installDir, "node_modules", s.name, "index.js"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "published-from-harbor") {
		return fmt.Errorf("installed module did not contain published content")
	}
	return nil
}

func run(ctx context.Context, dir string, name string, args ...string) {
	s := stateFrom(ctx)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NPM_CONFIG_UPDATE_NOTIFIER=false", "NPM_CONFIG_FUND=false")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	s.err = cmd.Run()
	s.stdout = stdout.Bytes()
	s.stderr = stderr.Bytes()
}

func registryURL(s *scenarioState, project string) string {
	return s.endpoint + "/npm/" + project + "/"
}

func writeNPMRC(dir, registry string) error {
	u, err := url.Parse(registry)
	if err != nil {
		return err
	}
	username := envOr("HARBOR_E2E_USERNAME", "admin")
	password := base64.StdEncoding.EncodeToString([]byte(envOr("HARBOR_E2E_PASSWORD", "Harbor12345")))
	prefix := "//" + u.Host + strings.TrimRight(u.Path, "/") + "/:"
	content := fmt.Sprintf("%susername=%s\n%s_password=%s\n%semail=harbor-e2e@example.com\n%salways-auth=true\n", prefix, username, prefix, password, prefix, prefix)
	return os.WriteFile(filepath.Join(dir, ".npmrc"), []byte(content), 0o600)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func randomSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "xxxxxxxx"
	}
	return hex.EncodeToString(b)
}
