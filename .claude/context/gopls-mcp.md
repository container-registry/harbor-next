# gopls-mcp — when and how

Compiler-grade Go navigation via MCP. Full docs: https://gopls-mcp.org. This file is just the project-specific bits and the decision rules.

The binary is upstream `xieyuschen/gopls-mcp` (post-v1.0 consolidation). It exposes **7 tools** — the ones that genuinely need Go's type system, plus a meta-tool. Upstream deliberately dropped the older discovery/search/read tools (`go_search`, `go_build_check`, `go_list_*`, `go_get_package_symbol_detail`, `go_read_file`, `go_get_started`, `go_analyze_workspace`) as redundant with native Grep/Glob/Read/Bash. Don't call them — they no longer exist.

The 7 tools: `go_definition`, `go_implementation`, `go_symbol_references`, `go_get_call_hierarchy`, `go_get_dependency_graph`, `go_dryrun_rename_symbol`, `go_list_tools`.

## Project setup

- Workdir is `src/` (where `go.mod` lives), set via the `-workdir src/` flag in `.mcp.json` (inline args, **not** a `-config` file). `.mcp.json` also passes `-directory-filters -portal` to skip the Angular tree (~95% fewer file descriptors; the file watcher honors it too).
- Most tools accept `Cwd`; if omitted, they use the primary view (the `-workdir`). A `Cwd` mismatch fails the call.
- First call is slow (~5–10s, cache warm); subsequent ~50–150ms. Any nav call warms the cache — there's no dedicated warm-up/build tool.
- To type-check the workspace, use Bash: `cd src && go build ./...` (the old `go_build_check` tool is gone).

## When to reach for gopls-mcp vs native tools

| Use gopls-mcp (type-aware) | Use Grep/Read/Bash |
|----------------------------|--------------------|
| Symbol definitions, references | String literals, comments, TODO/FIXME |
| Interface implementations (finds unexported types + mocks) | Log messages, error strings |
| Call hierarchies | Filename search, non-Go files |
| Dependency graph between packages | Listing a package's symbols → Read the file |
| Rename impact assessment / dry-run | Finding a symbol by name → Grep on the identifier |
| — | Type-check / build → `go build ./...` |

## SymbolLocator (the input most nav tools want)

Nav tools wrap the locator in a top-level `locator` object: `{"locator": {…}, "include_body": true}`.

| `locator` field | Required | Notes |
|-------|----------|-------|
| `symbol_name` | yes | exact identifier, no package prefix |
| `context_file` | yes | absolute path to anchor file |
| `package_identifier` | no | import alias for cross-package lookup |
| `parent_scope` | no | receiver for methods (`Server`), enclosing fn for locals |
| `kind` | no | `function`/`method`/`struct`/`interface`/`variable`/`const` |
| `line_hint`, `signature_snippet` | no | disambiguation |

## Useful flags

- `include_body: true` (top-level, alongside `locator`) on `go_definition` and `go_implementation` — returns the full body, no follow-up read needed.
- `direction` on `go_get_call_hierarchy`: `incoming`, `outgoing`, or `both` (default `both`).
- `go_get_dependency_graph`: `package_path` (default main module root), `include_transitive` (default false), `max_depth` (0 = unlimited), `Cwd`.
- `go_list_tools`: `includeInputSchema` / `includeOutputSchema` / `category_filter` — call this to re-derive exact parameter schemas if anything below drifts.

## Common chains

| Goal | Chain |
|------|-------|
| Find a symbol by name | Grep the identifier → `go_definition(include_body=true)` on a hit |
| What does X do? | `go_definition(locator={…}, include_body=true)` |
| Who calls X? | `go_get_call_hierarchy(direction="incoming")` |
| What does X call? | `go_get_call_hierarchy(direction="outgoing")` |
| What implements I? | `go_implementation(kind="interface")` |
| Package architecture | `go_get_dependency_graph(package_path="github.com/goharbor/harbor/src/...")` |
| Explore unfamiliar pkg | Read the package's files directly (no list tool); use `go_definition` for cross-package jumps |
| Safe rename | `go_symbol_references` → `go_dryrun_rename_symbol(new_name=…)` |
| Build errors | Bash: `cd src && go build ./...` |
