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

For an unsigned local release build and push, authenticate the container
engine first, then run `task release-images-local RELEASE_VERSION=X.Y.Z
IMAGE_TAG=<tag>`. It defaults to `8gears.container-registry.com/8gcr-pr` and
builds both `linux/amd64` and `linux/arm64`.

## PRs

- Branch off `main`, never push direct.
- Conventional Commits, capitalized subject: `feat: Add Foo`, `fix: Resolve Bar`, `upstream: Cherry-Pick Harbor Fix`.
- DCO sign-off required: `git commit -s`.
- **Squash and merge only** — other merge types break release-please.
- No `Co-Authored-By` / AI attribution trailers.
- **New features (`feat:`) must add a `## Release Notes` section to the PR description.** Its prose is extracted and rendered under `## Highlights` on the GitHub Release. See CONTRIBUTING.md → "Adding Release Notes to Your PR".

## Release-please

`feat:` → minor, `fix:` / `upstream:` → patch, `feat!:` / `BREAKING CHANGE:` → major. `ci:`, `build:`, `chore:`, `test:` are hidden from release notes.

**exclude-paths:** changes touching only `.github/`, `docs/`, `tests/`, or `taskfile/` don't bump version — use `ci:` for CI-only changes.

## Registry

Default: `8gears.container-registry.com/8gcr/`. Override with `REGISTRY_ADDRESS` / `REGISTRY_PROJECT`. Publishing needs `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` secrets.
