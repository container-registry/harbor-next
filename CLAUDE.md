# Harbor (8gcr fork)

Harbor is a CNCF graduated container registry. This repo is the **8gcr commercial fork** of [goharbor/harbor](https://github.com/goharbor/harbor), tracking upstream `harbor-next/main` with commercial modifications carried as standalone jj/git branches merged into a local megamerge (see `.claude/skills/jj-megamerge/SKILL.md` and ADR-0006).

## Critical rules (don't violate)

- **Build system is Taskfile, never Make.** Only `make/migrations/postgresql/` survives. `task --list-all` enumerates every entry point.
- **OpenAPI is the source of truth for the REST API.** Edit `api/v2.0/swagger.yaml`, then `task build:gen-apis`. Never hand-edit `src/server/v2.0/restapi/` or `src/server/v2.0/handler/`.
- **Logging:** `github.com/goharbor/harbor/src/lib/log` only. Never `log.Printf` / `fmt.Printf`.
- **Commercial branches:** every 8gcr branch is a single commit parented directly on a `next/main` (harbor-next) commit — these branches, not files, are the artifact. Local `000N-*` bookmarks are discovered directly as commercial patches; `dev` is the development overlay (e2e suite, taskfiles, agent config, ADRs) and is never released. Never edit a branch directly — start from `task jj:setup-dev` (or `task jj:setup` for the patches-only production view), edit on top of the `megamerge` bookmark, and let `task jj:absorb` route hunks to the owning branch. Patch content must stay pure: no `8gcr-ee/`, `.claude/`, taskfiles, or `src/e2e` in a `000N-*` branch — that content belongs to `dev`; unit tests for patch code belong in the owning patch. See `.claude/skills/jj-megamerge/SKILL.md`.
- **PRs:** Conventional Commits title (`feat: Capitalized Subject`), DCO sign-off (`git commit -s`), **squash-merge only** (release-please depends on it).
- **No AI attribution trailers** in commits (no `Co-Authored-By: Claude`, no `Generated-By:`).

## Context modules

@.claude/context/stack.md
@.claude/context/workflow.md
@.claude/context/architecture.md
@.claude/context/patches.md
@.claude/context/contributing.md
@.claude/context/gopls-mcp.md

## Deeper references

- `QUICKSTART.md` — full dev setup
- `PATCHES.md` — jj megamerge recipes (branch creation, restack, conflict resolution)
- `versions.env` — single source of truth for Go, tools, base images, external deps
- `https://gopls-mcp.org` — gopls-mcp tool docs
