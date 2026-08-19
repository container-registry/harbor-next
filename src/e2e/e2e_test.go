//go:build e2e

// Package e2e is the godog-based Harbor end-to-end suite. See SPEC.md for the
// scope, architecture, and step catalogue this package implements.
//
// Run with:
//
//	task test:e2e             # full suite (requires `task dev:up` running)
//	task test:e2e:smoke       # @smoke subset
//	task test:e2e:tags TAGS=… # arbitrary tag expression
package e2e

import (
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

var opts = godog.Options{
	Format:        "pretty,junit:../../reports/e2e-junit.xml",
	Paths:         []string{"features"},
	Output:        colors.Colored(os.Stdout),
	Strict:        true,
	StopOnFailure: false,
	Randomize:     -1,
	Concurrency:   4,
}

func init() {
	// Register godog flags on std flag.CommandLine — that's the flag set `go test`
	// parses, so `-args -godog.tags='@smoke'` reaches godog.
	// (godog.BindCommandLineFlags uses pflag instead; std flag is what we need.)
	godog.BindFlags("godog.", flag.CommandLine, &opts)
}

// TestFeatures is the single entrypoint for the godog runner.
func TestFeatures(t *testing.T) {
	// The JUnit formatter opens its output path directly — pre-create the
	// parent directory so that step fails with a clear error rather than a
	// "no such file or directory" from godog at startup.
	_ = os.MkdirAll("../../reports", 0o755)
	_ = os.MkdirAll("../../reports/failures", 0o755)

	o := opts
	o.TestingT = t
	suite := godog.TestSuite{
		Name:                 "harbor-e2e",
		TestSuiteInitializer: InitializeTestSuite,
		ScenarioInitializer:  InitializeScenario,
		Options:              &o,
	}
	if status := suite.Run(); status != 0 {
		t.Fatalf("godog exit status %d", status)
	}
}
