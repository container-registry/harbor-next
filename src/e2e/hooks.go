//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/harborclient"
	"github.com/goharbor/harbor/src/e2e/internal/state"
	"github.com/goharbor/harbor/src/e2e/steps"
)

// InitializeTestSuite runs once per go test invocation before any scenario.
// It performs a cheap health probe so the run fails fast when Harbor is
// unreachable — the e2e suite does not own the Harbor stack (see SPEC §5.1).
func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		c := harborclient.FromEnv()
		resp, err := c.Get("/api/v2.0/health")
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[e2e] Harbor not reachable at %s: %v\n", c.BaseURL, err)
			fmt.Fprintf(os.Stderr, "[e2e] Run 'task dev:up' before 'task test:e2e'.\n\n")
			os.Exit(2)
		}
		if resp.StatusCode >= 500 {
			fmt.Fprintf(os.Stderr, "\n[e2e] Harbor health returned %d — stack is not ready\n\n", resp.StatusCode)
			os.Exit(2)
		}
		fmt.Printf("[e2e] Harbor %s — health OK\n", c.BaseURL)
	})

	ctx.AfterSuite(func() {
		fmt.Printf("[e2e] suite complete: %d scenarios (pass=%d fail=%d skip=%d)\n",
			runStats.total.Load(), runStats.pass.Load(), runStats.fail.Load(), runStats.skip.Load())
	})
}

// runStats aggregates counters across parallel scenarios.
var runStats struct {
	total, pass, fail, skip atomic.Int64
}

// InitializeScenario installs per-scenario hooks and registers every step
// definition. Registration happens in package steps — this function only wires
// the ScenarioContext into that package's entry point.
func InitializeScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		runStats.total.Add(1)
		s := state.NewScenario(harborclient.FromEnv())
		return state.Put(ctx, s), nil
	})

	sc.After(func(ctx context.Context, sc *godog.Scenario, scenarioErr error) (context.Context, error) {
		st := lookup(ctx)
		if st == nil {
			return ctx, nil
		}
		if scenarioErr != nil {
			runStats.fail.Add(1)
			captureFailureArtifacts(sc.Name, st)
		} else {
			runStats.pass.Add(1)
		}
		teardown(st)
		return ctx, nil
	})

	steps.Register(sc)
}

// lookup returns the scenario state from a context without panicking when
// the state hasn't been populated yet (e.g. if Before failed to run).
func lookup(ctx context.Context) (out *state.Scenario) {
	defer func() { _ = recover() }()
	return state.Get(ctx)
}

// teardown walks created resources in reverse order and best-effort deletes
// them. Errors are swallowed — teardown never fails a scenario (SPEC §3.4).
func teardown(s *state.Scenario) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.HTTPMock != nil {
		s.HTTPMock.Close()
	}

	// Branding: restore original branding configuration.
	if s.BrandingRestore != nil {
		_ = s.BrandingRestore()
	}

	// Immutability rules first (block project delete if present).
	for project, ids := range s.CreatedImmutability {
		for _, id := range ids {
			_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/immutabletagrules/%d", project, id))
		}
	}

	// Webhook policies before their project.
	for project, ids := range s.CreatedWebhookIDs {
		for _, id := range ids {
			_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies/%d", project, id))
		}
	}

	// Replication policies + executions.
	for _, id := range s.CreatedReplicationIDs {
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/replication/policies/%d", id))
	}
	for _, id := range s.CreatedRegistries {
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/registries/%d", id))
	}

	if s.ProjectFedIDPRestore != nil {
		_ = s.ProjectFedIDPRestore()
	}

	// Robots (must be deleted before IdPs — IdP delete fails if robots still reference it).
	for i := len(s.CreatedRobots) - 1; i >= 0; i-- {
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/robots/%d", s.CreatedRobots[i].ID))
	}

	// Federated IdPs.
	for i := len(s.CreatedIdPs) - 1; i >= 0; i-- {
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/federated-idps/%d", s.CreatedIdPs[i]))
	}

	// Users.
	for i := len(s.CreatedUsers) - 1; i >= 0; i-- {
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/users/%d", s.CreatedUsers[i].ID))
	}

	// Projects last — delete repositories under each project first, then project.
	for i := len(s.CreatedProjects) - 1; i >= 0; i-- {
		p := s.CreatedProjects[i]
		_ = purgeRepositories(s, p)
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s", p))
	}

	// Labels (global).
	sort.Slice(s.CreatedLabels, func(i, j int) bool { return s.CreatedLabels[i] > s.CreatedLabels[j] })
	for _, id := range s.CreatedLabels {
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/labels/%d", id))
	}

	// Temp dirs.
	for _, d := range s.TempDirs {
		_ = os.RemoveAll(d)
	}
}

func purgeRepositories(s *state.Scenario, project string) error {
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/repositories?page=1&page_size=100", project))
	if err != nil || resp.StatusCode != 200 {
		return err
	}
	var repos []struct {
		Name string `json:"name"`
	}
	_ = harborclient.Decode(resp, &repos)
	for _, r := range repos {
		// repo name comes back as "project/name" — strip the project prefix.
		short := strings.TrimPrefix(r.Name, project+"/")
		short = strings.ReplaceAll(short, "/", "%252F") // double-encoded slash for multi-level repos
		_, _ = s.Client.Delete(fmt.Sprintf("/api/v2.0/projects/%s/repositories/%s", project, short))
	}
	return nil
}

func captureFailureArtifacts(scenarioName string, s *state.Scenario) {
	dir := filepath.Join("..", "..", "reports", "failures", sanitize(scenarioName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if len(s.LastBody) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "last-http-body.txt"), s.LastBody, 0o644)
	}
	if len(s.LastCLIStdout) > 0 || len(s.LastCLIStderr) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "cli.stdout"), s.LastCLIStdout, 0o644)
		_ = os.WriteFile(filepath.Join(dir, "cli.stderr"), s.LastCLIStderr, 0o644)
	}
	if s.LastErr != nil || s.LastImageErr != nil || s.LastCLIErr != nil {
		var errs []string
		if s.LastErr != nil {
			errs = append(errs, "http: "+s.LastErr.Error())
		}
		if s.LastImageErr != nil {
			errs = append(errs, "image: "+s.LastImageErr.Error())
		}
		if s.LastCLIErr != nil {
			errs = append(errs, "cli: "+s.LastCLIErr.Error())
		}
		_ = os.WriteFile(filepath.Join(dir, "errors.txt"), []byte(strings.Join(errs, "\n")), 0o644)
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, s)
}
