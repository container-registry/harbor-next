# ADR-0006: jj Orphan Branches Replace StGit as the Commercial-Code Mechanism

**Status**: Accepted
**Date**: 2026-08-13
**Decision Makers**: 8gcr Team
**Technical Area**: Source Code Management, Release Engineering
**Supersedes**: [ADR-0001](0001-managing-8gcr-modifications-on-harbor.md) (mechanism only — problem framing and feature-isolation rationale there remain valid)

> **2026-08-18 amendment:** `8gcr-ee/patches/series` and the surrounding
> directory were removed. Development tooling now discovers local `000N-*`
> bookmarks directly. Harbor Next owns release ordering independently in
> `taskfile/commercial-patches`. References below to the series file describe the
> original migration and are retained as historical context.

## Context

ADR-0001 chose a StGit patch queue (`8gcr-ee/patches/`) as the mechanism for
carrying commercial modifications on top of Harbor. It worked, but two frictions
grew with the patch count and feature complexity:

1. **Cascading conflicts when early patches change** — an accepted cost in
   ADR-0001, but one that got worse as `0003-identity-providers` grew to ~18,000
   lines and started textually depending on `0001-branding` and
   `0002-sftp-replication` (shared `config.module.ts`/`config.component.html`
   nav-item context, `go.mod`/`go.sum` churn).
2. **No native way to work against "everything combined"** — StGit workflows apply
   the whole series into a disposable `wip/*` branch per session; there's no
   persistent, incrementally-updatable view of all features merged together that
   you can just keep developing against.

[Isaac Corbrey's "Jujutsu Megamerges for Fun and Profit"](https://isaaccorbrey.com/notes/jujutsu-megamerges-for-fun-and-profit)
describes exactly this second problem's solution: an octopus merge of every active
branch that you develop on top of, with `jj absorb` routing edits back to the owning
branch automatically. jj (Jujutsu) also turns out to solve the first problem as a
side effect — each commercial feature becomes an independent branch instead of a
member of a fragile linear stack, so a change to one feature's dependencies doesn't
require re-threading every patch after it.

## Decision

Replace the StGit patch queue with **standalone jj bookmarks** (real git branches,
via jj/git colocation) for each commercial feature, merged into a local-only
octopus merge (`megamerge`) for development.

### Model

- **Trunk**: the local `8gcr-main` bookmark (tracks `main@8gcr`; kept current with
  `main@next` via `task jj:sync-main`). Its tree is harbor-next content plus
  8gcr-only extras (`8gcr-ee/`, e2e, docs, CI, Taskfiles) — structurally unchanged
  from what `main`/`8gcr-main` already was under ADR-0001.
- **Commercial branches**: `0001-branding`, `0002-sftp-replication`,
  `0003-identity-providers`, `0004-pgx-monitoring`, `0005-aws-rds-iam-auth` — each a
  single commit (or, for `0005`, a two-commit stack on `0004` — the two aren't
  realistically separable, `0005` inserts directly inside code `0004` added) whose
  parent is `8gcr-main`. Pushed to the `8gcr` remote as real git branches.
- **`megamerge`**: `jj new 0001-branding 0002-sftp-replication 0003-identity-providers 0005-aws-rds-iam-auth`
  (`0004` reachable via `0005`). Local only, never pushed — a development
  convenience, not an artifact.
- **`8gcr-ee/patches/series`**: survives as a plain branch-name manifest (one name
  per line, release order) instead of a StGit patch-file series.

### Daily workflow (`taskfile/jj.yml`)

```
task jj:setup     # sync 8gcr-main, rebuild megamerge, land on fresh working copy
# ... edit on top of megamerge ...
task jj:absorb     # route hunks to the owning branch
task jj:restack     # re-sync when 8gcr-main moves (jj:setup calls this too)
```

Full recipes and verified footguns: `.claude/skills/jj-megamerge/SKILL.md`,
`8gcr-ee/patches/README.md`.

### Dependency graph (informed the branch-construction order)

Textual dependency analysis of the old patch queue (i.e., which patches' hunks used
another patch's additions as context) — the conflicts a naive StGit rebase would
hit, and the conflicts the jj branch-construction had to resolve once to make each
branch stand alone:

```
0001 ──> 0003   (portal config tab + en-us i18n: nav-item, module routes, translation key)
0002 ──> 0003   (go.mod / go.sum: kr/fs, testify version drift)
0002 ──> 0004   (go.mod only)
0002 ──> 0005   (go.mod only)
0004 ──> 0005   (7 files; not realistically separable — kept stacked, not split)
```

`0001` and `0002` are mutually independent. This is why `0005` stays a child of
`0004` rather than becoming a fifth sibling.

## CI Migration Status

### This repo (`container-registry/8gcr`) — done

[PR #253](https://github.com/container-registry/8gcr/pull/253):
- `taskfile/jj.yml` — the daily-workflow tasks above.
- `taskfile/release-ready.yml`'s `apply-commercial-patches`/`check` tasks rewritten
  to fetch the branches named in `8gcr-ee/patches/series` and `jj new`-merge them,
  replacing `stg import`. Fails the build on any unresolved conflict rather than
  silently applying a broken merge.
- `.github/workflows/release-ready.yml` and `.github/workflows/publish-images.yml`
  — "Install StGit" steps replaced with "Install jj" (downloads the pinned
  `jj-v0.44.0-<arch>-unknown-linux-musl.tar.gz` release asset).

Verified before merge: the megamerge of all 5 branches builds
(`task build:gen-apis && go build ./...`), `go vet`s clean, and the three test files
that conflicted against pre-existing `8gcr-main` content (`dao_test.go` with
`-tags db`, `robotjwt_test.go`, `federated_idp_test.go`) compile.

### `container-registry/harbor-next` — not yet applied (no write access from this session)

harbor-next's release pipeline is the one that **always runs in production** — it's
hardcoded to clone this repo and apply commercial patches at every release, unlike
this repo's own gated `vars.PATCHES_REPO` path. It carries a near-identical copy of
`publish-images.yml` and its own release-notes generation script. Both need the
equivalent change:

**`publish-images.yml`** — diffed directly against this repo's (now-migrated)
copy; only the "Install StGit" step differs (harbor-next also pins slightly newer
action SHAs elsewhere — unrelated, don't touch those). Replace:

```yaml
      - name: Install StGit
        run: |
          if command -v stg >/dev/null 2>&1; then
            stg --version
            exit 0
          fi
          if command -v apt-get >/dev/null 2>&1; then
            if command -v sudo >/dev/null 2>&1; then
              sudo apt-get update
              sudo apt-get install -y stgit
            else
              apt-get update
              apt-get install -y stgit
            fi
          elif command -v brew >/dev/null 2>&1; then
            brew install stgit
          else
            echo "::error::StGit is required but no supported package manager was found."
            exit 1
          fi
          stg --version
```

with:

```yaml
      - name: Install jj
        env:
          JJ_VERSION: "0.44.0"
        run: |
          if command -v jj >/dev/null 2>&1; then
            jj --version
            exit 0
          fi
          case "$(uname -m)" in
            x86_64|amd64) arch="x86_64" ;;
            aarch64|arm64) arch="aarch64" ;;
            *)
              echo "::error::Unsupported architecture: $(uname -m)"
              exit 1
              ;;
          esac
          curl -fsSL "https://github.com/jj-vcs/jj/releases/download/v${JJ_VERSION}/jj-v${JJ_VERSION}-${arch}-unknown-linux-musl.tar.gz" -o /tmp/jj.tar.gz
          mkdir -p /tmp/jj-extract
          tar -xzf /tmp/jj.tar.gz -C /tmp/jj-extract
          sudo install -m 755 /tmp/jj-extract/jj /usr/local/bin/jj
          rm -rf /tmp/jj.tar.gz /tmp/jj-extract
          jj --version
```

The `Apply commercial patches` step immediately after (`run: task --taskfile
taskfile/release-ready.yml apply-commercial-patches`) needs no change — it calls
into `taskfile/release-ready.yml`, which is this repo's file, already migrated. If
harbor-next carries its own fork of that Taskfile instead of referencing this
repo's, apply the same task-body rewrite from PR #253.

**Release notes generation** (`taskfile/release-notes.yml` →
`.github/scripts/update-release-notes.sh` → `.github/scripts/format-commercial-patch.mjs`,
invoked from `release-notes-engine.yml`) — this is genuinely harbor-next-only
infrastructure (nothing in this repo calls it). Its `## Commercial Features`
section currently:

```bash
GH_TOKEN="${PATCHES_TOKEN}" gh repo clone container-registry/8gcr "${tmp_dir}/patches-repo" -- --depth=1 --branch main
series="${tmp_dir}/patches-repo/8gcr-ee/patches/series"
# ... loop over series, call format-commercial-patch.mjs on each patch FILE ...
node .github/scripts/format-commercial-patch.mjs "${tmp_dir}/patches-repo/8gcr-ee/patches/${patch}" >> "${patch_notes}"
```

`format-commercial-patch.mjs` parses a StGit patch file's mailbox format (`Subject:`
line + body up to the `---` diffstat marker). None of the 5 branches carry a commit
body — only a one-line subject, same as the original patches — so the replacement
is simpler than the original:

1. In `update-release-notes.sh`, replace the `gh repo clone ... --branch main` +
   series-file-of-patch-*files* loop with a fetch of each *branch* named in the
   manifest and a read of its tip commit's description:
   ```bash
   GH_TOKEN="${PATCHES_TOKEN}" gh repo clone container-registry/8gcr "${tmp_dir}/patches-repo" -- --depth=1 --branch main
   series="${tmp_dir}/patches-repo/8gcr-ee/patches/series"
   if [[ -f "${series}" ]]; then
     while IFS= read -r branch; do
       branch="${branch%%#*}"; branch="${branch#"${branch%%[![:space:]]*}"}"; branch="${branch%"${branch##*[![:space:]]}"}"
       [[ -z "${branch}" ]] && continue
       git -C "${tmp_dir}/patches-repo" fetch --depth=1 origin "${branch}:refs/remotes/patches/${branch}"
       echo "- $(git -C "${tmp_dir}/patches-repo" log -1 --format=%s "refs/remotes/patches/${branch}")" >> "${patch_notes}"
     done < "${series}"
   fi
   ```
2. `format-commercial-patch.mjs` is no longer needed for this (no mailbox format to
   parse) — its call site above replaces it inline. Leave the script file in place
   only if something else still references it; otherwise delete it alongside this
   change.
3. If a future commercial branch's tip commit *does* carry a body worth surfacing
   in release notes, extend the inline `git log` line to `--format=%s%n%n%b` and
   reintroduce trimming/indentation logic equivalent to the old script's — not
   needed for the current 5 branches.

**Everything else in `release-please.yml`/`release-notes-engine.yml`/`publish-images.yml`
(cosign signing, image tagging, `preview-release-notes`/`update-release-notes`
job wiring) is untouched** — only the patch-file-reading surfaces above change.

## Known Gaps (deliberately not addressed in this pass)

- **`.github/agents/scripts/agent-entrypoint.sh`** (the "Agent Upstream Sync
  Resolver") still runs `stg import -3 -S 8gcr-ee/patches/series` against files
  that no longer exist. It needs its own migration to fetch + `jj new`-merge the
  branches instead — a larger, separately-testable piece of automation, tracked as
  a follow-up rather than folded into this pass. Flagged prominently in
  `8gcr-ee/patches/README.md`.
- **`task jj:sync-main`** doesn't yet replicate `taskfile/sync.yml`'s
  GitHub-Actions-workflow-path preservation (`GITHUB_TOKEN` can't push workflow
  file changes) or shared-rerere-cache reuse. Revisit if `8gcr-main` sync conflicts
  become frequent enough to need that hardening back.
- **harbor-next's own pipeline** — documented above, not applied (no write access
  from the session that authored this ADR).

## Consequences

### Positive
- Feature isolation is stronger than the patch queue: a branch's own diff is
  exactly its feature, with no cascading-conflict risk from earlier "patches" in a
  stack — order no longer matters except where features are genuinely coupled
  (`0004`/`0005`).
- A persistent, incrementally-restackable combined view (`megamerge`) replaces
  disposable per-session `wip/*` branches.
- `jj absorb` automates what used to be manual `stg goto` + `stg refresh` +
  `stg push -a` per edit.
- CI failure mode improved: a broken merge now fails the build immediately with a
  conflict list, instead of a StGit import error partway through a linear series.

### Negative
- Learning curve for jj, on top of the one ADR-0001 already accepted for StGit.
- Two known gaps above (upstream-sync agent, sync hardening) mean the migration
  isn't 100% complete yet.

### Mitigations
- `.claude/skills/jj-megamerge/SKILL.md` and `8gcr-ee/patches/README.md` document
  the workflow and every verified footgun encountered building this.
- The old `stgit` skill is kept (marked deprecated) for historical/emergency
  reference rather than deleted.

## References

- [Isaac Corbrey, "Jujutsu Megamerges for Fun and Profit"](https://isaaccorbrey.com/notes/jujutsu-megamerges-for-fun-and-profit)
- [Jujutsu (jj) documentation](https://jj-vcs.github.io/jj/latest/)
- [ADR-0001: Managing 8gcr Modifications on Harbor](0001-managing-8gcr-modifications-on-harbor.md)
- [PR #253](https://github.com/container-registry/8gcr/pull/253) — this repo's CI migration

## Changelog

| Date | Change | Author |
|------|--------|--------|
| 2026-08-18 | Removed the 8gcr series manifest; use `000N-*` bookmark discovery and root `PATCHES.md` | Prasanth Baskar |
| 2026-08-13 | Initial decision | 8gcr Team |
