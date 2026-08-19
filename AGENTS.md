# Harbor Next Agent Notes

Enhanced fork of [goharbor/harbor](https://github.com/goharbor/harbor). Go backend in `src/`, Angular frontend in `src/portal/`, build automation via Taskfile.

## Branch model (jj megamerge)

Every 8gcr branch is one commit parented directly on `next/main`: local `000N-*`
bookmarks are the commercial patches; `dev` is the development overlay (e2e,
taskfiles, agent config) and is never released. `8gcr/main` is the stable default
branch. Work happens on top of the
local `megamerge` octopus; `task jj:absorb` routes edits into the owning branch.

```bash
task jj:setup-dev            # megamerge = main@next + dev + patches (daily driver)
task jj:setup                # patches-only production view (what CI validates)
task jj:absorb               # route working-copy hunks to the owning branch
task jj:sync-patch-branches  # rebase all branches onto next/main tip, force-push to 8gcr
task jj:new-branch NAME=0006-x
```

## Commands

```bash
task build        # Build Go binaries
task test:quick   # API lint + unit tests
task test:ci      # Full CI pipeline
task images       # Build/push Docker images
task dev:up       # Local dev with hot reload
task apply-patches # Apply branches listed in taskfile/commercial-patches with jj
task release-ready # Verify those patches against a clean clone
```

Go tests, builds, and `go mod tidy` need generated server API bindings first:

```bash
task build:gen-apis
```

`task build:gen-apis` is also available as `task b:gen-apis`.

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

**exclude-paths:** changes touching only `.github/`, `docs/`, `tests/`, or `taskfile/` don't bump version. Use `ci:` for CI-only changes.

## Registry

Default: `8gears.container-registry.com/8gcr/`. Override with `REGISTRY_ADDRESS` / `REGISTRY_PROJECT`. Publishing needs `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` secrets.
