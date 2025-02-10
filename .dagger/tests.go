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
		"--issues-exit-code", "1",
	}).File(report)
}

// Lint Run the linter golangci-lint
func (m *Harbor) Lint(ctx context.Context) (string, error) {
	return m.lint(ctx).WithExec([]string{"golangci-lint", "-v", "run", "--timeout=10m"}).Stderr(ctx)
}

func (m *Harbor) lint(_ context.Context) *dagger.Container {
	fmt.Println("👀 Running linter.")
	linter := dag.Container().
		From("golangci/golangci-lint:"+GOLANGCILINT_VERSION+"-alpine").
		WithMountedCache("/lint-cache", dag.CacheVolume("/lint-cache")).
		WithEnvVariable("GOLANGCI_LINT_CACHE", "/lint-cache").
		WithMountedDirectory("/src", m.Source).
		WithWorkdir("/src").
		WithExec([]string{"golangci-lint", "cache", "clean"})

	return linter
}

func (m *Harbor) GoVulnCheck(_ context.Context) *dagger.Container {
	fmt.Println("👀 Running linter and printing results to file golangci-lint.txt.")
	linter := dag.Container().
		From("golang:"+GO_VERSION+"-alpine").
		// WithMountedCache("/lint-cache", dag.CacheVolume("/lint-cache")).
		// WithEnvVariable("GOLANGCI_LINT_CACHE", "/lint-cache").
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor/src").
		WithExec([]string{"go", "install", "golang.org/x/vuln/cmd/govulncheck@latest"}).
		Terminal()

	return linter
}
