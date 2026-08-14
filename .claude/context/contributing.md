# Contributing & Release

## PRs

- Never push to `main`. All changes via PR.
- **Title:** Conventional Commits, lowercase type + capitalized subject. `feat: Add Foo`, `fix: Resolve Bar`, `docs: Update README`.
- **Sign-off:** every commit needs DCO (`git commit -s`). Lefthook enforces locally; install with `lefthook install`.
- **Merge mode:** **Squash and merge only.** Never "Create a merge commit" or "Rebase and merge" — non-squash merges produce `Merge pull request #N` commits that release-please can't parse.
- **No AI attribution trailers.** No `Co-Authored-By: Claude`, no `Generated-By:` lines.

## PR description template

A `## Summary` (what + why), `## Related Issues` (`Fixes #N`), `## Type of Change` checkboxes matching the conventional-commits prefix, optional `## Release Notes` (fill for user-facing changes only), `## Testing` checklist, and a `## Checklist` confirming title format + DCO + no new warnings.

## Release process (release-please)

1. Merge any `feat:` / `fix:` PR to `main`.
2. release-please opens a `chore: release X.Y.Z` PR (updates `VERSION`, `CHANGELOG.md`).
3. Review and merge → GitHub Release + image build/push happen automatically.

Bump rules: `fix:` → patch, `feat:` → minor, `feat!:` or `BREAKING CHANGE:` footer → major. `ci:`/`build:`/`chore:`/`test:` are hidden from release notes.

**`exclude-paths` gotcha:** commits touching only `.github/`, `docs/`, or `tests/` do **not** bump version even with `feat:`/`fix:`. Use `ci:` for CI-only changes so the title matches the actual behavior.

## Image build & release

The image pipeline lives in `.github/workflows/release-please.yml` (this repo) and a near-identical copy in `container-registry/harbor-next`. Both build the same set of 8gcr-flavored images. Triggered by `push` to `main` only when release-please's `release_created == 'true'`.

**4-stage pipeline:**
1. **release-please** — opens or merges the release PR; emits the new tag/version on merge.
2. **build** — matrix of 8 components × 2 platforms = 16 jobs. Per job: checkout the release tag → optionally apply commercial patches (see below) → load `versions.env` into `$GITHUB_ENV` → `task build:gen-apis` + `task build:binary:<comp>:<platform>` for Go components → `docker/build-push-action` builds `dockerfile/<component>.dockerfile` per-arch with `--push-by-digest=true`. Each digest is uploaded as an artifact.
3. **merge** — per-component, downloads both per-arch digest artifacts and runs `docker buildx imagetools create` to assemble + push a multi-arch manifest list tagged `<image>:<version>`.
4. **sign** — installs cosign and runs **keyless signing** (`cosign sign --yes` via GitHub OIDC) on each `harbor-<component>:<version>`. Then augments the GitHub release notes (see below).

**Components built:** `core`, `jobservice`, `registryctl`, `exporter`, `portal`, `registry`, `trivy-adapter`, `nginx`.
**Platforms:** `linux/amd64`, `linux/arm64`.
**Optional remote BuildKit:** if `BUILDX_HOST` is set, uses `driver: remote` and skips QEMU.

### Patch source (the non-obvious bit)

The "Apply commercial patches" step is **conditional**:

| Repo | Behavior |
|------|----------|
| `container-registry/harbor-next` | **Hardcoded** to clone `container-registry/8gcr` via a GitHub App (`SYNC_APP_ID` / `SYNC_APP_PRIVATE_KEY`), then `git apply --binary` each patch in `series` order. Always runs. |
| this repo | Gated by `vars.PATCHES_REPO`. Uses `secrets.PATCHES_TOKEN` (PAT). If the var is unset, the step is skipped → unpatched Harbor build. |

So harbor-next is the public-facing build that always pulls these private patches at build time. See `8gcr-ee/CLAUDE.md` for what this implies about when patch changes ship.

### Registry

- **Default:** `8gears.container-registry.com/8gcr` (override with `vars.REGISTRY_ADDRESS` / `vars.REGISTRY_PROJECT`).
- **Auth to push:** `vars.REGISTRY_USERNAME` + `secrets.REGISTRY_PASSWORD`.
- **Local override:** `task image:all-images REGISTRY_ADDRESS=ttl.sh REGISTRY_PROJECT=harbor-next`.

### Build args from `versions.env`

`versions.env` is grepped into `$GITHUB_ENV`, then these specific keys are passed as `--build-arg`: `ALPINE_VERSION`, `LPROBE_VERSION`, `NGINX_VERSION`, `BUN_VERSION`, `GO_VERSION`, `DISTRIBUTION_VERSION`, `TRIVY_VERSION`, `TRIVY_BASE_IMAGE_VERSION`, `HARBOR_SCANNER_TRIVY_VERSION`. Adding a new build-time pin requires editing both `versions.env` and the build-args list in `release-please.yml`.

### Cosign verification

Keyless signing means the certificate identity is the workflow file path on `main`. Verification snippet is auto-injected into release notes:

```
cosign verify \
  --certificate-identity "https://github.com/<owner>/<repo>/.github/workflows/release-please.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  <registry>/harbor-core:<version>
```

### Release notes generation

The `sign` job appends three programmatic sections to the GitHub release after release-please publishes it:

- **Highlights** — `gh pr list` for PRs merged since the previous tag, filtered to those whose body contains a `## Release Notes` section. Content under that section (up to the next `##`) is extracted, comments stripped, rendered per PR. **This is the only way a PR appears in highlights** — fill in `## Release Notes` on user-facing PRs.
- **Commercial Features** — first line (`Subject: …`) of each patch file, in series order.
- **Container Images** — table of `harbor-<comp>:<version>` + cosign verification snippet.

## CI workflows (one-liner each)

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `build.yml` | PR | Compile check |
| `test.yml` | PR | Unit tests + API lint |
| `release-please.yml` | push to main | Release PR + image build/sign/release (see above) |
| `pr-title.yml` | PR open/edit | Enforce conventional commits |
| `labeler.yml` | PR open | Component labels |
| `dependency-review.yml` | PR | Block high-severity CVEs |
| `spellcheck.yml` | PR + main | Typos in docs/configs |
| `scorecard.yml` | weekly + main | OpenSSF score |
| `sync-and-verify.yml` | dispatch + daily cron | Sync `harbor-next/main`, verify patches |
| `welcome.yml` | first issue/PR | Welcome contributors |

## Lefthook hook coverage

Spell check (staged `.md`/`.yml`), conventional commit format, DCO sign-off.
