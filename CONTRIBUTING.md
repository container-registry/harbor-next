# Contributing to Harbor Next

Thank you for contributing! This guide explains how to open PRs and merge them correctly so the automated release pipeline works as expected.

## Table of Contents

- [Workflow Overview](#workflow-overview)
- [Creating a Pull Request](#creating-a-pull-request)
- [Merging a Pull Request](#merging-a-pull-request)
- [How Releases Work](#how-releases-work)
- [Adding Release Notes to Your PR](#adding-release-notes-to-your-pr)
- [Local Development Setup](#local-development-setup)

---

## Workflow Overview

```
fork/branch -> commit (conventional) -> PR -> CI passes -> squash merge -> release-please -> release
```

All changes go through PRs. Never push directly to `main`.

---

## Creating a Pull Request

### 1. Branch Naming

Use a short, descriptive branch name prefixed by the change type:

```
feat/oidc-federated-login
fix/x509-negative-serial
ci/parallel-image-builds
docs/contributing-guide
```

### 2. Commit Messages (Conventional Commits)

Every commit must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]

Signed-off-by: Your Name <your@email.com>
```

Common types:

| Type | When to use | Release effect |
|------|------------|----------------|
| `feat` | New user-facing feature | Minor version bump |
| `fix` | Bug fix | Patch version bump |
| `feat!` / `fix!` | Breaking change | Major version bump |
| `refactor` | Code change, no behaviour change | No release |
| `docs` | Documentation only | No release |
| `ci` | CI/CD pipeline changes | No release |
| `chore` | Maintenance, dependencies | No release |
| `test` | Tests only | No release |
| `build` | Build system changes | No release |

DCO sign-off is required on every commit. Use `git commit -s` to add it automatically.

### 3. PR Title

The PR title becomes the squash commit message on main, so it must also follow Conventional Commits. The `pr-title` CI check enforces this and will block merging if the format is wrong.

The type prefix must be lowercase, and the subject must start with a capital letter:

Good:
```
feat(portal): Add Repository-Level Pull Command to Artifact List Tab
fix: Allow Negative Serial Numbers in X509 Certificates
ci: Split Image Builds into Parallel Matrix Jobs
```

Bad:
```
Updated the portal
Fix bug
feat: add new feature
Merge pull request #5
```

### 4. Scopes (Optional but Recommended)

Use a scope in parentheses to indicate the component:

```
feat(portal): ...
fix(core): ...
ci(release): ...
```

### 5. PR Description

Use the following template for your PR description:

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
- [ ] Tests (`test:`)

## Release Notes
<!--
Optional. Fill in for user-facing changes (new features, breaking changes, deprecations).
Leave blank for ci:/chore:/refactor:/docs:/test: PRs.
-->

## Testing
- [ ] Unit tests added/updated
- [ ] Manual testing performed

## Checklist
- [ ] PR title follows Conventional Commits format
- [ ] Commits are signed off (`git commit -s`)
- [ ] No new warnings introduced
```

### 6. Breaking Changes

For breaking changes, use `!` after the type and add a `BREAKING CHANGE:` footer in the **squash commit body** (the GitHub merge dialog body field, not the PR description body):

```
feat!: remove legacy v1 API endpoints

BREAKING CHANGE: The /api/v1 endpoints have been removed. Migrate to /api/v2.
```

---

## Merging a Pull Request

### Always Use Squash and Merge

When merging any PR, **always choose "Squash and merge"** on GitHub. Never use "Create a merge commit" or "Rebase and merge".

Why this matters: non-squash merges create `Merge pull request #N` commits on main. These commits do not follow Conventional Commits format and break release-please, which reads commit messages to decide when and how to bump the version.

**How to squash merge:**

1. Click the dropdown arrow next to the merge button
2. Select "Squash and merge"
3. Edit the commit title to match the PR title (GitHub usually pre-fills this)
4. Add any relevant body text or `BREAKING CHANGE:` footer
5. Ensure the `Signed-off-by:` line is present in the body
6. Click "Confirm squash and merge"

### What Lands on Main

After squash merging, exactly one commit lands on main with the message from the PR title. This is the commit release-please reads.

---

## How Releases Work

Releases are fully automated via [release-please](https://github.com/googleapis/release-please).

### The Flow

1. A `feat:` or `fix:` PR is squash-merged to main
2. Release-please scans commits since the last release
3. It opens a `chore: release X.Y.Z` PR that updates `VERSION` and `CHANGELOG.md`
4. Maintainer reviews and merges the release PR (squash merge)
5. GitHub Release is created automatically
6. Docker images are built, signed, and pushed

### Version Bump Rules

| Commit type | Version bump | Example |
|-------------|-------------|---------|
| `fix:` | Patch | `2.16.0` -> `2.16.1` |
| `feat:` | Minor | `2.16.0` -> `2.17.0` |
| `feat!:` / `BREAKING CHANGE:` | Major | `2.16.0` -> `3.0.0` |
| `ci:` / `chore:` / `docs:` / `test:` / `build:` | No release | - |

### What Triggers a Release PR

Release-please only counts commits that touch files outside of these excluded paths:

- `.github/`
- `docs/`
- `tests/`

A `feat:` PR that only changes `.github/` files (e.g. a CI workflow improvement) will NOT trigger a version bump. Use `ci:` for such changes.

### CHANGELOG.md

The changelog is generated automatically from squash commit messages. `ci:`, `chore:`, `test:`, and `build:` commits are hidden from the changelog. Only `feat:`, `fix:`, `perf:`, `revert:`, `refactor:`, and `docs:` appear.

---

## Adding Release Notes to Your PR

For user-facing changes (new features, breaking changes, deprecations), you can add rich release notes that appear on the GitHub Release page as a `## Highlights` section.

Fill in the `## Release Notes` section in the PR description:

```markdown
## Release Notes

Adds federated OIDC support. Configure via the new `federated_oidc` key in `harbor.yml`.
See the [OIDC documentation](https://docs.example.com/oidc) for configuration details.
```

**Rules:**

- Leave it blank for `ci:`, `chore:`, `refactor:`, `docs:` PRs
- Write for your users, not for developers (explain what changed and why it matters)
- Links are fine and encouraged
- HTML comments in the section are stripped automatically

The `## Release Notes` section is extracted by the release pipeline and injected into the GitHub Release body. It does not affect `CHANGELOG.md`.

---

## Local Development Setup

Install [lefthook](https://github.com/evilmartians/lefthook) to enforce these rules locally before pushing:

```bash
lefthook install
```

Hooks enforce:
- Conventional commit message format on every commit
- DCO sign-off presence
- Spell check on staged `.md` and `.yml` files

### Common Task Commands

```bash
task dev:up           # Start dev environment with hot reload
task build            # Build all Go binaries
task test:quick       # API lint + unit tests (fast)
task test:unit        # Go unit tests with race detection
task test:lint        # golangci-lint
task images           # Build and push Docker images
task info             # Print version and build info
```

See [README.md](README.md) for full prerequisites and setup instructions.
