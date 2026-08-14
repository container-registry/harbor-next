---
name: jj-megamerge
description: Manage 8gcr-ee commercial features as jj (Jujutsu) branches parented on next/main, merged into a local octopus-merge working copy. Use for daily development on commercial features, restacking branches onto harbor-next/main, adding a new commercial branch, or resolving megamerge conflicts. Replaces the retired stgit skill.
argument-hint: "[task]"
allowed-tools: Bash(jj *), Bash(git *), Bash(task jj:*), Read, Glob, Grep
---

# 8gcr-ee Commercial Features via jj Megamerge

Every 8gcr branch is a **single commit parented directly on a `next/main`
(harbor-next) commit** — its diff is purely its own content. A local-only octopus
merge — the `megamerge` bookmark — combines them; you develop on top of it. See
[ADR-0006](../../../8gcr-ee/decision-records/0006-jj-orphan-branches-replace-stgit.md)
for why this replaced the StGit patch queue, and Isaac Corbrey's
["Jujutsu Megamerges for Fun and Profit"](https://isaaccorbrey.com/notes/jujutsu-megamerges-for-fun-and-profit)
for the underlying pattern.

## Project-Specific Paths

| Item | Value |
|------|-------|
| Base | `main@next` (harbor-next/main) — every branch's parent |
| Commercial branches | `0001-branding`, `0002-sftp-replication`, `0003-identity-providers`, `0004-pgx-monitoring`, `0005-aws-rds-iam-auth` — each a single commit on `main@next` (`0005` stacks on `0004`; they're not realistically separable) |
| Dev overlay | `dev` branch — e2e suite, taskfiles, agent config, ADRs; never in a release build |
| Default branch | `8gcr/main` — stable CI/default branch; change through PRs |
| Megamerge | `megamerge` bookmark — octopus merge of `main@next` + branches, **local only, never pushed** |
| Patch discovery | Local bookmarks matching `000N-*`; no manifest file |
| Decision record | `8gcr-ee/decision-records/0006-jj-orphan-branches-replace-stgit.md` |
| Daily-workflow tasks | `taskfile/jj.yml` (`jj:setup`, `jj:setup-dev`, `jj:restack`, `jj:sync-patch-branches`, `jj:absorb`, `jj:new-branch`) |

## Daily Loop

```bash
task jj:setup-dev   # fresh megamerge = main@next + dev + patches, land on a fresh working copy
                    # (task jj:setup = patches-only production view, what CI validates)
# ... edit files directly in the working copy on top of megamerge ...
task jj:absorb      # route each hunk into the branch that owns it
task jj:sync-patch-branches   # when next/main moves or work is done:
                              # rebase all branches onto its tip, force-push to 8gcr
```

**Never edit a branch (`000N-*` or `dev`) directly** — always start from
`task jj:setup-dev` and let `jj absorb` route your edits. For a hunk absorb can't
place: `JJ_EDITOR=true jj squash --into <branch> --use-destination-message <paths>`.

**Parallel workspaces**: bookmarks are repo-global, so a second workspace should use
`task jj:setup-dev MEGAMERGE_BOOKMARK=mm-<name>` to avoid fighting over `megamerge`.

## Adding a New Commercial Feature

```bash
task jj:new-branch NAME=0006-my-feature   # one commit off main@next
# ... do the work ...
jj describe -m "<subject>"
task jj:setup-dev    # rebuild picks the new branch up automatically (globs 000?-*)
task jj:sync-patch-branches
```

No manifest update is needed. The new local `000N-*` bookmark is discovered by
setup and sync automatically. Harbor Next release ordering remains independent in
its own `taskfile/commercial-patches` file.

## Verified Footguns (don't relearn these)

1. **Plain `git` commands from inside a jj *workspace* directory (not a git worktree) silently target the outer repo.** `git apply` here `Skipped patch` and exits 0. Set `GIT_CEILING_DIRECTORIES` or use `git -C <path>` explicitly for any raw git command run from a jj workspace.
2. **Incremental restacks can silently drop branch content** (observed with `--simplify-parents`: whole files vanished with `jj log -r 'megamerge & conflicts()'` reporting clean). `jj:setup`/`jj:setup-dev` therefore always rebuild the megamerge with a fresh `jj new` — same construction CI uses. After any restack, verify with `task build:gen-apis` + `go build ./...`, not just a conflict check.
3. **`jj squash` into a described commit hangs on an interactive editor** — always `JJ_EDITOR=true jj squash --use-destination-message` in scripts/agents.
4. **`jj new` takes multi-rev revsets directly** (e.g. `jj new 'main@next' 'bookmarks(glob:"000?-*")'`); the `all:` prefix errors on jj 0.44.
5. **`jj describe` has no `--author` flag.** Use `jj metaedit --author '<Name> <email>'` if a commit's authorship needs correcting.
6. **jj doesn't have git rerere.** A megamerge conflict resolved once can re-surface if a later `jj absorb`/rebase touches the same region — expected, not a bug; resolve again, it's normally quick. The routine sources are already engineered away: `dev`'s e2e deps sit in their own `require` blocks at the end of `src/go.mod` (tidy preserves block structure), and `setup`/`setup-dev` auto-union `src/go.sum` from all parents when it's the sole conflict.
7. **Generated API code goes stale across megamerge flavors.** `src/server/v2.0/restapi/` is gitignored and generated; after switching between prod/dev/main trees, `rm -rf src/server/v2.0/restapi && task build:gen-apis` before trusting build results.

## Conflict Resolution Mechanics

- `jj new p1 p2 p3 p4` builds the octopus merge directly as the working commit if you haven't moved on top of it yet — conflicts show as first-class conflict markers in the files, resolved by editing them directly (no separate "merge tool" step needed).
- If you've already moved past the merge commit (`jj new` again on top), resolve in the child, then `jj squash` to fold the resolution back into the merge/branch commit — `jj squash` refuses on a merge commit without `--into`, but if the resolution's *sole* parent is the merge commit, it applies without extra flags.
- After resolving, `jj log -r '<bookmark> & conflicts()'` should be empty — check it, don't assume.

## Export / CI

`container-registry/harbor-next` owns its release patch ordering in
`taskfile/commercial-patches`. The 8gcr development workflow discovers local
`000N-*` bookmarks directly. The `dev` branch is never part of a release build.
