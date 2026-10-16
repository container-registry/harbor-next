# Harbor Next

Enhanced fork of [goharbor/harbor](https://github.com/goharbor/harbor). Go backend in `src/`, Angular frontend in `src/portal/`, build automation via Taskfile.

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

Go test/build need generated API first: `task build:gen-apis`.

For an unsigned local release build and push, authenticate the container
engine first, then run `task release-images-local RELEASE_VERSION=X.Y.Z
IMAGE_TAG=<tag>`. It defaults to `8gears.container-registry.com/8gcr-pr` and
builds both `linux/amd64` and `linux/arm64`.

## Commercial Patch Branches

Commercial features are git branches on `container-registry/8gcr`, listed in release order in `taskfile/commercial-patches` (`release-X.Y` lists `release-X.Y/000N-*`). Each is based on this repo's `main` (or `release-X.Y`). They are siblings, not a stack, and releases build their jj octopus merge:

```bash
task apply-patches                                 # octopus of every listed branch onto @
task apply-patches OVERLAY_BRANCHES=fix/my-branch  # plus extra 8gcr branches, e.g. a PR under test
task release-ready:check                           # the same against a throwaway local clone
```

- Build and test a patch change only in the combined tree. A single patch branch is not guaranteed to compile on its own: the `0001-commercial-feature-gate` portal imports components that live in `0002-branding` and `0004-federated-robot-accounts`.
- A fix to a commercial feature is a PR into its `000N-*` branch, plus a backport PR into `release-X.Y/000N-*` for each supported release line.
- StGit and `.patch` files are retired.

## Workspaces

Work in jj workspaces next to the main checkout, one per task, named `harbor.<target>-<slug>` where `<target>` is the repository the work lands in: `next-` (this repo), `8gcr-` (commercial branches), `upstream-` (goharbor/harbor). Example: `jj workspace add ../harbor.8gcr-0001-env-value -r 0001-commercial-feature-gate@<8gcr-remote>`. A workspace has no `.git`, so `git`, `gh` repo detection and the git-based tasks above (`apply-patches` fetches with git) fail there. Inside a workspace use jj (`jj git push --remote <r> --bookmark <b>`) and `gh -R <owner>/<repo>`, and build the combined tree with jj directly:

```bash
jj git fetch --all-remotes
jj new "heads(main@<remote> | $(sed -e 's/#.*//' -e '/^[[:space:]]*$/d' -e 's/$/@<8gcr-remote>/' taskfile/commercial-patches | paste -sd'|' -))"
jj log -r '@ & conflicts()'                        # must print nothing
```

jj has no `commit -s`; for DCO set `templates.commit_trailers = "format_signed_off_by_trailer(self)"` in the jj user config.

## PRs

- Branch off `main`, never push direct.
- Conventional Commits, capitalized subject: `feat: Add Foo`, `fix: Resolve Bar`, `upstream: Cherry-Pick Harbor Fix`.
- DCO sign-off required: `git commit -s`.
- **Squash and merge only** — other merge types break release-please.
- No `Co-Authored-By` / AI attribution trailers.
- **New features (`feat:`) must add a `## Release Notes` section to the PR description.** Its content is extracted and rendered under `## Summary` on the GitHub Release. See CONTRIBUTING.md → "Adding Release Notes to Your PR".

## Release-please

`main` uses `always-bump-minor`; `VERSION` on `main` tracks the next development release while `.release-please-manifest.json` tracks the published release. `release-X.Y` branches use patch-only versioning. `ci:`, `build:`, `chore:`, `test:` are hidden from release notes.

**exclude-paths:** changes touching only `.github/`, `docs/`, `tests/`, or `taskfile/` don't bump version — use `ci:` for CI-only changes.

## Registry

Default: `8gears.container-registry.com/8gcr/`. Override with `REGISTRY_ADDRESS` / `REGISTRY_PROJECT`. Publishing needs `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` secrets.
