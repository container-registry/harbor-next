# 8gcr Commercial Branches

8gcr commercial changes are standalone Jujutsu bookmarks backed by Git branches
on the private `8gcr` remote. Development combines them in a local octopus merge
named `megamerge`; the branches themselves remain the release artifacts.

See [ADR-0006](8gcr-ee/decision-records/0006-jj-orphan-branches-replace-stgit.md)
for why this replaced the old StGit patch queue.

## Branch model

- `main@next` is the Harbor Next base.
- `000N-*` bookmarks are commercial patches. Their numeric names define stable
  discovery order; no manifest file is required.
- `dev` contains development-only tooling, tests, agent configuration, and ADRs.
- `megamerge` is a local-only merge of `main@next`, all `000N-*` bookmarks, and,
  for `jj:setup-dev`, `dev`.
- `main@8gcr` is the stable CI/default branch. Change it through a PR.

Harbor Next release branches keep their own explicit release ordering in
`taskfile/commercial-patches`. That file belongs to Harbor Next and is independent
of local 8gcr bookmark discovery.

## Daily workflow

```bash
task jj:setup-dev
# edit on top of the generated megamerge
task jj:absorb
task jj:sync-patch-branches
```

`jj:sync-patch-branches` fetches both private remotes, rebases every local
`000N-*` bookmark plus `dev` onto current `main@next`, refuses to push conflicts,
and pushes only to `8gcr`.

For a patches-only production view, use `task jj:setup`.

## Adding a patch

```bash
task jj:new-branch NAME=0007-short-description
# implement and describe the change
task jj:setup-dev
task jj:sync-patch-branches
```

No manifest update is needed. Once the local bookmark exists, setup and sync
discover it from the `000N-*` naming convention.

## Conflict handling

After a restack, inspect conflicts with:

```bash
jj log -r '(bookmarks(glob:"000?-*") | dev | megamerge) & conflicts()'
```

Resolve a conflicted branch by creating a child, editing the conflict, and
squashing the resolution back:

```bash
jj new <conflicted-revision>
# edit files
JJ_EDITOR=true jj squash --use-destination-message
```

Then rebuild from scratch with `task jj:setup-dev`. Fresh rebuilds are preferred
because incremental megamerge restacks can silently simplify away content.

## Useful commands

```bash
task patches:list
jj diff -r 000N-short-description
jj evolog -r 000N-short-description
jj resolve --list
```

When working in a `jj workspace add` workspace, use `jj` commands or explicit
`git -C <path>` commands. Plain Git discovery can otherwise select the outer
colocated repository instead of the workspace you intended.
