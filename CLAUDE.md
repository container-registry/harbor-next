# Harbor Next

Enhanced fork of [goharbor/harbor](https://github.com/goharbor/harbor). Go backend in `src/`, Angular frontend in `src/portal/`, build automation via Taskfile.

## Commands

```bash
task build        # Build Go binaries
task test:quick   # API lint + unit tests
task test:ci      # Full CI pipeline
task images       # Build/push Docker images
task dev:up       # Local dev with hot reload
```

## PRs

- Branch off `main`, never push direct.
- Conventional Commits, capitalized subject: `feat: Add Foo`, `fix: Resolve Bar`, `upstream: Cherry-Pick Harbor Fix`.
- DCO sign-off required: `git commit -s`.
- **Squash and merge only** — other merge types break release-please.
- No `Co-Authored-By` / AI attribution trailers.
- **New features (`feat:`) must add a `## Release Notes` section to the PR description.** Its prose is extracted and rendered under `## Highlights` on the GitHub Release. See CONTRIBUTING.md → "Adding Release Notes to Your PR".

## Release-please

`main` uses `always-bump-minor`; `VERSION` on `main` tracks the next development release while `.release-please-manifest.json` tracks the published release. `release-X.Y` branches use patch-only versioning. `ci:`, `build:`, `chore:`, `test:` are hidden from release notes.

**exclude-paths:** changes touching only `.github/`, `docs/`, or `tests/` don't bump version — use `ci:` for CI-only changes.

## Registry

Default: `8gears.container-registry.com/8gcr/`. Override with `REGISTRY_ADDRESS` / `REGISTRY_PROJECT`. Publishing needs `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` secrets.
