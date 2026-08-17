# Implementation Spec — IDEA #8: Project-level Packages view (ecosystem grouping + per-format filter chips)

## Goal
Fold ecosystem grouping and per-format filter chips (with live counts) into the **existing** project Repositories tab. No new tab, no new endpoint, no schema change. Frontend-only.

## Scope confirmation (verified against the tree + live SLOT=1)
- `naming.Decode` does **NOT** exist in `src/pkg/multiformat/naming/naming.go` (only `EscapeComponent`, `EncodeRepo`, `EncodeTag`). #8 does **not** add it — readable row labels are #2's deliverable (presentation-layer reverse of `_xNN`). #8 **depends on #2**.
- The repository list payload (`ng-swagger-gen/models/repository.ts`) carries **no `format` field** — only `name`, `artifact_count`, `pull_count`, `update_time`. Format must be derived from `name`.
- `name` shape verified live: `library/maven/mcom_x2eacme_x3awidget2`, `library/maven/mcom_x2eacme_x3awidget`, `library/npm/mharbor_x2dmulti_x2dformat_x2ddemo`. Format = `name.split('/')[1]`.
- `GET /projects/{name}/repositories?q=name=~library/maven/` returns only maven repos with `X-Total-Count: 2` (verified). Two `name=~` clauses AND together in one `q` (verified). This is the chip-filter + count seam.

## Data source
Derived entirely from `repository.name` returned by the already-used `GET /projects/{name}/repositories`. Per-format counts from the `X-Total-Count` header on a `q=name=~<project>/<format>/` request. No `multi_format_package.format` access, no facet endpoint.

## Files to touch (all under `src/portal/`)
1. `src/app/base/project/repository/repository-gridview.component.ts`
   - State: `groupByEcosystem` (localStorage-persisted), `activeFormat: string|null`, `formatCounts: {[fmt]:number}`, `groupedRepositories` view-model.
   - `FORMATS = ['npm','maven']`; `formatOf(repo)` → segment[1] if in FORMATS else `'other'`.
   - `toggleGroupBy()`, `selectFormat(fmt)` → set state, re-run `clrLoad(this.currentState)`.
   - In **all three** q-building sites (`ngOnInit` searchSub, `loadNextPage`, `clrLoad`): when `activeFormat` is a real format, append `name=~<projectName>/<activeFormat>/` to the comma-joined `q` (alongside the existing `name=~<lastFilteredRepoName>` clause when present).
   - `refreshFormatCounts()`: one `page_size=1` list call per format, read `X-Total-Count` → `formatCounts`. Called from `refresh()` and after `clrLoad`.
   - When grouping on, regroup the loaded page into `groupedRepositories` (display-only; no extra fetch).
2. `repository-gridview.component.html` — toolbar gets the group toggle + chip row (`All`, `npm <n>`, `maven <n>`); list datagrid gets a grouped render path (group header row + nested rows) gated on `groupByEcosystem`; existing 4 columns + `getLink` unchanged. Row label/icon stays #2's output.
3. `repository-gridview.component.scss` — chip + group-header styles.
4. `repository-gridview.component.spec.ts` — headers stub with `x-total-count`, branch mock on `q` containing `maven/`/`npm/`, specs for `formatOf`, chip filter, grouping.
5. `src/i18n/lang/en-us-lang.json` (+ every other `*-lang.json`) — add `REPOSITORY.GROUP_BY_ECOSYSTEM`, `FORMAT_ALL/NPM/MAVEN/OTHER`.

## No API / schema change
`q` + `X-Total-Count` on `GET /projects/{name}/repositories` already exist. No swagger edit, no `task build:gen-apis`, no migration, no Go change.

## Known limitations (see risks)
- `other`/OCI chip cannot filter server-side (no single prefix) → page-local fallback or omit it.
- Group headers and counts within the grouped view are per loaded page; server pagination unchanged.

## Verification
See the verification field — backend seam already reproduced live (maven `X-Total-Count: 2`, npm `1`); then portal lint/test (`bun run lint_fix`, `lint_fix:style`, targeted `test`), then browser e2e on http://localhost:4300 confirming chips/counts/grouping against the existing library npm+maven test packages.