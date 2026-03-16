# Harbor Next - Claude Code Instructions

## Project Overview

Harbor Next is an enhanced fork of [goharbor/harbor](https://github.com/goharbor/harbor) - a cloud-native container registry. It adds patches, improvements, and CI/CD automation on top of the upstream Harbor project.

- **Go backend** in `src/` (multi-module, Go 1.25+)
- **Angular frontend** in `src/portal/` (Angular 16, built with Bun)
- **Build automation** via Taskfile (see `Taskfile.yml` and `taskfile/`)

## Key Commands

```bash
task build            # Build all Go binaries (linux/amd64 locally, multi-arch in CI)
task test:quick       # API spec lint + unit tests (fast)
task test:ci          # Full CI pipeline with reports
task test:unit        # Go unit tests with race detection
task test:lint        # golangci-lint via Docker
task test:lint:api    # Swagger spec lint via Spectral Docker
task images           # Build and push all Docker images
task dev:up           # Start full dev environment with hot reload
task info             # Print version and build info
```

## Contribution Workflow

All changes go through PRs - never push directly to `main`.

```
git checkout -b feat/my-feature
# ... make changes with conventional commits (git commit -s) ...
git push origin feat/my-feature
gh pr create
```

PR title must follow Conventional Commits with lowercase type prefix and capitalized subject: `feat: Add New Feature`, `fix: Resolve Issue`, `docs: Update README`, etc.
All commits require DCO sign-off: `git commit -s`.

**PR description** must follow this template:

```markdown
## Summary
<!-- Brief description of what this PR does -->

## Related Issues
<!-- Fixes #123 -->

## Type of Change
- [ ] Bug fix (`fix:`)
- [ ] New feature (`feat:`)
- [ ] Breaking change (`feat!:` / `fix!:`)
- [ ] Documentation (`docs:`)
- [ ] Refactoring (`refactor:`)
- [ ] CI/CD or build changes (`ci:` / `build:`)
- [ ] Dependencies update (`chore:`)

## Release Notes
<!--
Optional. Fill in for user-facing changes (new features, breaking changes, deprecations).
Leave blank for ci:/chore:/refactor: PRs.
-->

## Testing
- [ ] Unit tests added/updated
- [ ] Manual testing performed

## Checklist
- [ ] PR title follows Conventional Commits format
- [ ] Commits are signed off (`git commit -s`)
- [ ] No new warnings introduced
```

**Merging PRs:** Always use **Squash and merge**. Never "Create a merge commit" or "Rebase and merge". Non-squash merges create `Merge pull request #N` commits that break release-please's commit parser.

## Release Process

Releases are automated via release-please:
1. Merge any `feat:` or `fix:` PR to `main`
2. Release-please opens a "chore: release X.Y.Z" PR automatically
3. Review the PR (it updates `VERSION` and `CHANGELOG.md`)
4. Merge the release PR -> GitHub Release is created + images are built and pushed

Version bump rules:
- `fix:` -> patch (2.15.0 -> 2.15.1)
- `feat:` -> minor (2.15.0 -> 2.16.0)
- `feat!:` or `BREAKING CHANGE:` footer -> major

`ci:`, `build:`, `chore:`, `test:` commits are hidden from release notes.

**exclude-paths:** Commits that only touch `.github/`, `docs/`, or `tests/` do NOT trigger a version bump even with `feat:` or `fix:`. Use `ci:` for CI-only changes to avoid misleading PR titles.

## File Structure

```
Taskfile.yml          # Root task runner (includes taskfile/*.yml)
taskfile/
  build.yml           # Go binary compilation, swagger codegen
  image.yml           # Docker multi-arch image builds
  test.yml            # Linting, unit tests, vulnerability scanning
  dev.yml             # Local dev environment (docker-compose)
versions.env          # Pinned versions for all tools and base images
VERSION               # Current release version (managed by release-please)
src/                  # Go backend source
  go.mod
  core/               # Core registry service
  jobservice/         # Background job service
  registryctl/        # Registry controller
  cmd/exporter/       # Prometheus exporter
  portal/             # Angular frontend
    package.json
    bun.lock
api/v2.0/swagger.yaml # Harbor REST API spec
dockerfile/           # Dockerfiles for each service
devenv/               # Docker Compose for local development
.github/workflows/    # CI/CD pipelines
```

## GitHub Actions Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `build.yml` | PRs to main | Compile check |
| `test.yml` | PRs to main | Unit tests + API lint |
| `release-please.yml` | Push to main | Release PR automation + image publishing |
| `pr-title.yml` | PR opened/edited | Enforce conventional commit format |
| `labeler.yml` | PR opened | Auto-label by component |
| `dependency-review.yml` | PRs to main | Block high-severity CVEs |
| `spellcheck.yml` | PRs + main | Typos in docs/configs |
| `scorecard.yml` | Weekly + main | OpenSSF security score |
| `welcome.yml` | First issue/PR | Welcome new contributors |

## Local Git Hooks (lefthook)

Install: `lefthook install` (requires [lefthook](https://github.com/evilmartians/lefthook))

Hooks enforce:
- Spell check on staged `.md`/`.yml` files
- Conventional commit message format
- DCO sign-off on every commit

## Image Registry

Images are pushed to `8gears.container-registry.com/8gcr/` by default.
Override with `REGISTRY_ADDRESS` and `REGISTRY_PROJECT` vars (e.g., `task image:all-images REGISTRY_ADDRESS=ttl.sh REGISTRY_PROJECT=harbor-next`).

Required secrets for image publishing: `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`.
