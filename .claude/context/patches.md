# 8gcr Commercial Branches (jj Megamerge)

This fork carries 8gcr commercial modifications as **standalone jj bookmarks** — real
git branches on the `8gcr` remote, thanks to jj/git colocation — merged into a local
octopus merge (`megamerge`) for development. This replaced a StGit patch queue; see
[ADR-0006](../../8gcr-ee/decision-records/0006-jj-orphan-branches-replace-stgit.md)
for the full rationale and the earlier decision it supersedes (ADR-0001).

Every branch is a **single commit parented directly on a `next/main` commit** —
its diff is purely its own content:

- `000N-*` — the commercial patches, discovered directly from local bookmarks
- `dev` — the development overlay (e2e suite, taskfiles, agent config, ADRs); never in
  `series`, never in a release build

`8gcr/main` is the stable default branch. It is not a megamerge parent; changes
to it go through PRs.

For commands and recipes, use the **`jj-megamerge` skill** or `PATCHES.md`.
This file only carries the rules that aren't in either.

## Irreducible rules

- **Never edit a branch (`000N-*` or `dev`) directly.** Always start with `task jj:setup-dev`
  (or `task jj:setup` for the patches-only production view), edit on top of the `megamerge`
  bookmark, and let `task jj:absorb` route hunks to the branch that owns them.
- **`megamerge` is local only — never pushed.** It's a convenience view of all branches
  combined, not itself an artifact. Only `000N-*` and `dev` get pushed to `8gcr`.
- **Patch diffs must stay pure.** No `8gcr-ee/`, `.claude/`, taskfiles, or `src/e2e` in a
  `000N-*` branch — that content belongs to `dev`. Unit tests for patch code belong in the
  owning patch branch.
- **Plain `git` commands run from inside a jj *workspace* directory (as opposed to a real git worktree) silently target the outer repo.** Set `GIT_CEILING_DIRECTORIES` or use `git -C <path>` explicitly for any raw git command from a jj workspace — this bit us building the initial branches from the old patch queue.
- **Patch discovery is naming-based.** Local bookmarks matching `000N-*` are the
  complete development patch set. Harbor Next owns release ordering separately
  in `taskfile/commercial-patches`.
- **jj has no git-rerere equivalent — but the megamerge is conflict-free by construction.** `dev`'s e2e deps live in their own `require` blocks at the end of `src/go.mod` (never adjacent to patch entries; `go mod tidy` preserves the block structure), and `setup`/`setup-dev` auto-resolve `src/go.sum` by unioning all parents' lines (it's derived, line-independent data). A real conflict appearing means two branches genuinely edited the same region — resolve it once in the working copy and squash to the owning branch.
- **Incremental `jj rebase` restacks can silently drop content** (observed with `--simplify-parents`). `task jj:setup`/`setup-dev` therefore always rebuild the megamerge with a fresh `jj new`, same as CI. Verify with gen-apis + `go build` after any restack, not just a conflict check.

## Naming

`NNNN-short-description` — 4-digit sequence, kebab-case, matches the branch name
exactly (no `.patch` extension, no file — the branch *is* the artifact).

## Layout

```
PATCHES.md                         ← daily jj workflow
8gcr-ee/decision-records/          ← ADRs for 8gcr decisions
```

## Production build note

`container-registry/harbor-next`'s own release pipeline is a near-identical copy of
this repo's `publish-images.yml`/`release-ready.yml` and — as of this writing — still
expects the old StGit patch-file model. ADR-0006 documents the exact diff needed
there; it hasn't been applied yet (out of reach from this repo).
