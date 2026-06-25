# Harbor Next Agent Notes

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
- Squash and merge only; other merge types break release-please.
- No `Co-Authored-By` or AI attribution trailers.
- New features (`feat:`) must add a `## Release Notes` section to the PR description. Its prose is extracted and rendered under `## Highlights` on the GitHub Release.

## GitHub Actions

- Self-hosted runners are intentionally minimal. When adding a workflow shell command that calls a CLI or interpreter, explicitly install or set up that tool in the same job before using it. Do not assume runner images or JavaScript action runtimes expose tools on `PATH`.
- Pin actions by full commit SHA and keep a version comment, matching the existing workflow style.
- If a workflow calls `node`, add an explicit `actions/setup-node` step first.

## Release-Please

`main` uses `always-bump-minor`; `VERSION` on `main` tracks the next development release while `.release-please-manifest.json` tracks the published release. `release-X.Y` branches use patch-only versioning. `ci:`, `build:`, `chore:`, `test:` are hidden from release notes.

**exclude-paths:** changes touching only `.github/`, `docs/`, or `tests/` don't bump version. Use `ci:` for CI-only changes.

## Registry

Default: `8gears.container-registry.com/8gcr/`. Override with `REGISTRY_ADDRESS` / `REGISTRY_PROJECT`. Publishing needs `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` secrets.
