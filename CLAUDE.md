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
- Conventional Commits, capitalized subject: `feat: Add Foo`, `fix: Resolve Bar`.
- DCO sign-off required: `git commit -s`.
- **Squash and merge only** — other merge types break release-please.
- No `Co-Authored-By` / AI attribution trailers.

## Release-please

`feat:` → minor, `fix:` → patch, `feat!:` / `BREAKING CHANGE:` → major. `ci:`, `build:`, `chore:`, `test:` are hidden from release notes.

**exclude-paths:** changes touching only `.github/`, `docs/`, or `tests/` don't bump version — use `ci:` for CI-only changes.

## Registry

Default: `8gears.container-registry.com/8gcr/`. Override with `REGISTRY_ADDRESS` / `REGISTRY_PROJECT`. Publishing needs `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` secrets.
