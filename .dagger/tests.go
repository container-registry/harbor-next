package main

import (
	"context"
	"dagger/harbor/internal/dagger"
	"fmt"
)

// Executes Linter and writes results to a file golangci-lint.report for github-actions
func (m *Harbor) LintReport(ctx context.Context) *dagger.File {
	report := "golangci-lint.report"
	return m.lint(ctx).WithExec([]string{
		"golangci-lint", "-v", "run", "--timeout=10m",
		"--out-format", "github-actions:" + report,
		"--issues-exit-code", "0",
	}).File(report)
}

// Lint Run the linter golangci-lint
func (m *Harbor) Lint(ctx context.Context) (string, error) {
	return m.lint(ctx).WithExec([]string{"golangci-lint", "-v", "run", "--timeout=10m"}).Stderr(ctx)
}

func (m *Harbor) lint(ctx context.Context) *dagger.Container {
	fmt.Println("👀 Running linter.")
  m.lintAPIs(ctx)
  m.mocksCheck(ctx)
	m.Source = m.genAPIs(ctx)
	linter := dag.Container().
		From("golangci/golangci-lint:"+GOLANGCILINT_VERSION+"-alpine").
		WithMountedCache("/lint-cache", dag.CacheVolume("/lint-cache")).
		WithEnvVariable("GOLANGCI_LINT_CACHE", "/lint-cache").
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor/src")
		// WithExec([]string{"golangci-lint", "cache", "clean"})

	return linter
}

func (m *Harbor) goVulnCheck(ctx context.Context) *dagger.Container {
	m.Source = m.genAPIs(ctx)
	return dag.Container().
		From("golang:alpine").
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor/src").
		WithExec([]string{"go", "install", "golang.org/x/vuln/cmd/govulncheck@latest"}).
		WithEntrypoint([]string{"/go/bin/govulncheck"})
}

// Check vulnerabilities in go-code
func (m *Harbor) GoVulnCheck(ctx context.Context) (string, error) {
	fmt.Println("👀 Running Go vulnerabilities check")
	return m.goVulnCheck(ctx).WithExec([]string{"govulncheck", "-show", "verbose", "./..."}).Stdout(ctx)
}

// Generate Vulnerability Report in sarif format for github-actions
func (m *Harbor) GoVulnCheckReport(ctx context.Context) (string, error) {
	fmt.Println("👀 Generating Vulnerability Report")
	return m.goVulnCheck(ctx).WithExec([]string{"govulncheck", "-format", "sarif", "./..."}).Stdout(ctx)
}
