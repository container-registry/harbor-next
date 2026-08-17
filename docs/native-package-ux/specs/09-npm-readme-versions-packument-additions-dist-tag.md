# Implementation Spec — IDEA #9: npm README + Versions additions

## Goal
Give npm packages native-registry UX on the artifact detail page by adding two tabs through the **existing ADDITIONS seam**:
- **Summary/README** — rendered markdown, reusing the existing `ADDITIONS.SUMMARY='readme.md'` tab + `summary.component`.
- **Versions** — a cross-version list (all published versions + dist-tags) sourced from the PG projection (`dao.LoadState` -> `model.PackageState`).

npm-only. Maven processor is untouched.

## Key facts verified in code
- The artifact processor interface (`src/controller/artifact/processor/processor.go`) exposes `ListAdditionTypes` + `AbstractAddition`. `controller.populateAdditionLinks` (`controller/artifact/controller.go:812`) auto-builds `addition_links` by lowercasing every type from `ListAdditionTypes`. So returning `"README.MD"` and `"VERSIONS"` auto-exposes `addition_links["readme.md"]` and `addition_links["versions"]`.
- `GET .../additions/{addition}` (`server/v2.0/handler/artifact.go:475`) calls `artCtl.GetAddition` -> `processor.Get(type).AbstractAddition(...)` and writes `Content-Type` + raw `Content`. The handler is generic; the only constraint is the **swagger enum** on the `addition` path param (`api/v2.0/swagger.yaml:1454`), which lists `build_history, values.yaml, readme.md, dependencies, sbom, license, files`.
  - `readme.md` is already enumerated -> README needs **no** swagger change.
  - `versions` is NOT enumerated -> must be added, then `task build:gen-apis`.
- `chart.go` (`processor/chart/chart.go`) is the precedent: overrides `ListAdditionTypes` + `AbstractAddition`, returns `&ps.Addition{Content, ContentType}`.
- The npm processor (`processor/npm/npm.go`) already stamps `extra_attrs.name` (native name), `extra_attrs.version`, `extra_attrs.description`, and `extra_attrs.dist-tags`. It embeds `*base.ManifestProcessor` (gives `RegCli`, default no-op `AbstractAddition`/`ListAdditionTypes`).
- `art.ExtraAttrs` is persisted (JSON column, `pkg/artifact/model.go`) and re-loaded by `artMgr.Get`, which is what `GetAddition` uses. So extra_attrs values are available in `AbstractAddition` with no registry round-trip.
- `dao.DAO.LoadState(ctx, projectID, format, name) (model.PackageState, bool, error)` (`pkg/multiformat/dao/dao.go:131`) returns `Versions []model.Version` (`Version, PayloadDigest, PayloadSize, Yanked, Created, Meta, Files`) and `DistTags map[string]string`. `pkg/multiformat/dao` does **not** import `controller/artifact`, so the processor can import it with no cycle.
- **`naming.Decode` does NOT exist** (the package doc comment claims a convenience Decode but there is no such function). It is **not needed**: the processor already has `art.ProjectID` and `art.ExtraAttrs["name"]`, so `LoadState(art.ProjectID, "npm", name)` is fully determined without decoding the repo name.

## Backend

### 1. `api/v2.0/swagger.yaml` (then `task build:gen-apis`)
Add `versions` to the `getAddition` `addition` path-param enum (~line 1454). No other swagger change. Never hand-edit `src/server/v2.0/restapi/` — regenerate.

### 2. `src/controller/artifact/processor/npm/npm.go`
- Add consts: `AdditionTypeReadme = "README.MD"`, `AdditionTypeVersions = "VERSIONS"`. Reuse the existing local `"npm"` format token (do NOT import `controller/multiformat`).
- In `AbstractMetadata`: additionally stamp `art.ExtraAttrs["readme"]` from `config["readme"]` when present (npm package.json may carry `readme`). Keeps README served from extra_attrs.
- Add `ListAdditionTypes` returning `[]string{AdditionTypeReadme, AdditionTypeVersions}`.
- Add `AbstractAddition(ctx, art, addition)`:
  - `README.MD`: `md := str(art.ExtraAttrs["readme"])`; if empty `md = str(art.ExtraAttrs["description"])`; return `&processor.Addition{Content: []byte(md), ContentType: "text/markdown; charset=utf-8"}` (empty body allowed).
  - `VERSIONS`: `name, _ := art.ExtraAttrs["name"].(string)`; if empty -> return empty DTO. `st, ok, err := dao.New().LoadState(ctx, art.ProjectID, "npm", name)`; on err return err; if `!ok` -> empty DTO. Build a deterministic DTO `{versions:[{version,created(RFC3339),yanked,size}], distTags}` (sort versions for stable output), `json.Marshal`, return `ContentType: "application/json; charset=utf-8"`.
  - default: delegate to base (BadRequest).
- Imports to add: `encoding/json` (already present), `sort`, `github.com/goharbor/harbor/src/pkg/multiformat/dao`. Guard all type assertions.

## Frontend (`src/portal/...artifact-additions/`)
1. `models.ts`: add `VERSIONS = 'versions'` to `ADDITIONS`. (SUMMARY already present.)
2. `artifact-additions.component.ts`: add `getVersions()` returning `additionLinks[ADDITIONS.VERSIONS] || null`. (README: `getSummary()` already keyed on SUMMARY.)
3. `artifact-additions.component.html`: add a `<clr-tab *ngIf="getVersions()">` (id `versions-link`, label `ARTIFACT.VERSIONS`) hosting `<hbr-artifact-versions [versionsLink]="getVersions()">`, mirroring the files tab (lines 98-112). README/summary tab block (lines 65-81) is unchanged and lights up automatically.
4. New `versions/versions.component.{ts,html,scss,spec.ts}` modeled on `files.component.ts`: `@Input() versionsLink`, fetch JSON via `additionsService.getDetailByLink(href, false, false)`, render a `clr-datagrid` of `{version, created, yanked}` + a dist-tags chip row. Register in the additions NgModule alongside `ArtifactFilesComponent`.
5. i18n: add `ARTIFACT.VERSIONS` (+ column labels) to all lang files.
6. Before build: `bun run lint_fix && bun run lint_fix:style && bun run test`.

## Verification (SLOT=1, already live)
1. `task build:gen-apis` -> `task dev:backend:up` (hot reload).
2. `curl -s -u admin:Harbor12345 "http://localhost:8180/api/v2.0/projects/library/repositories/npm%2Fmharbor_x2dmulti_x2dformat_x2ddemo/artifacts/1.0.0?with_addition_links=true"` -> `addition_links` contains `readme.md` and `versions`.
3. `curl .../additions/readme.md` -> text/markdown, body = `multiformat e2e demo` (description fallback; this test pkg has no readme field).
4. `curl .../additions/versions` -> JSON listing 1.0.0 + dist-tags.
5. Portal (port 4300): library > npm/harbor-multi-format-demo > 1.0.0 -> Additions shows **Summary** + **Versions**.
6. Maven control (`library/maven/mcom_x2eacme_x3awidget2`): no Summary/Versions tabs.

## Why minimal
README rides 100% on existing seams (zero API/schema change). Versions adds exactly one swagger enum value + one processor method + one small Angular tab. No migration. No naming.Decode. No import-cycle risk.