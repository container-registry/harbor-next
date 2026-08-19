# 8gcr-ee — Commercial Layer

This directory carries private 8gcr support material: ADRs, boundary hooks, and
the helper that opens Harbor Next PRs. Commercial code itself lives in `000N-*`
bookmarks; see the root `PATCHES.md`.

It is the **boundary** between this fork and `harbor-next/main`. Anything inside this directory must never appear in a PR to upstream. See `hooks/harbor-next-guard.sh` for the enforced denylist (currently: `8gcr-ee/`, `.claude/`, `.mcp.json`, `.gopls-mcp.json`, `.gsd/`, `.playwright-mcp/`, `CLAUDE.md`).

## Layout

```
8gcr-ee/
├── decision-records/     ADRs documenting why each non-trivial patch exists
├── hooks/                git hooks that enforce the upstream boundary
│   ├── harbor-next-guard.sh   denylist check used by pre-push and pr-harbor-next.sh
│   ├── pre-push               git pre-push hook entry point
│   └── setup.sh               installs the hook into lefthook-local.yml
├── pr-harbor-next.sh     wrapper around `gh pr create` with guard pre-flight
└── .rr-cache/            git rerere cache — shared so the team replays the same conflict resolutions during rebases
```

## Patch workflow

Read `PATCHES.md`. Local bookmarks matching `000N-*` are the authoritative patch
set; no series or patch-file directory exists.

## How patches reach images

Harbor Next owns release ordering in its own `taskfile/commercial-patches`. The
8gcr development and sync workflows discover local `000N-*` bookmarks directly.

## Boundary scripts

- `hooks/setup.sh` — run once after clone, or when switching to an 8gcr branch. Installs `lefthook-local.yml` so `pre-push` runs the guard automatically. Falls back to a direct `.git/hooks/pre-push` install if lefthook isn't on `$PATH`.
- `pr-harbor-next.sh` — use this instead of `gh pr create` when opening a PR against `container-registry/harbor-next`. It runs the guard first and refuses to push if any private path is in the diff.

## When working in here

- Changing commercial code: use the jj megamerge workflow in `PATCHES.md`.
- Touching `decision-records/`: add a new numbered ADR for any non-trivial patch. Follow the existing format (Status, Date, Decision Makers, Context, Decision, Consequences).
- Touching `hooks/` or `pr-harbor-next.sh`: if you change the private-path denylist, update both `harbor-next-guard.sh` and the boundary description in this file.
- Adding a new path that should never leak upstream: add it to the `PRIVATE_PATHS` array in `harbor-next-guard.sh`.

## See also

- Top-level `CLAUDE.md` — project-wide rules and context modules
- `.claude/context/patches.md` — irreducible jj workflow rules
- `.claude/skills/jj-megamerge/SKILL.md` — command-level recipes
