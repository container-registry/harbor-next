# Implementation Spec — Idea #7 (M): Maven GAV Files tab via additions seam

## Goal
Surface a Maven GAV's member files (classifiers, checksums, SNAPSHOT-vs-RELEASE) as a `Files` tab on the artifact detail page, using the existing ADDITIONS seam. Zero swagger/migration change. Reuse existing seams (chart.go / cnai.go precedent).

## What already exists (verified by reading source + live SLOT=1 env)
- **Data**: `src/pkg/multiformat/model/model.go` defines `FileRef{Filename, Classifier omitempty, Extension, Timestamp omitempty, BuildNumber omitempty, Digest, Size}`. The maven config blob is a `[]FileRef` (built in `src/controller/multiformat/mapper_maven.go`).
- **Backend metadata**: `src/controller/artifact/processor/maven/maven.go` `AbstractMetadata` already decodes the config blob into `art.ExtraAttrs["files"]`. Live check on `library/maven/mcom_x2eacme_x3awidget2@1.0` returns `extra_attrs.files=[widget2-1.0.jar, widget2-1.0.pom]`.
- **Seam plumbing**: `controller.populateAdditionLinks` (src/controller/artifact/controller.go:812) calls `processor.Get(art.ResolveArtifactType()).ListAdditionTypes(...)` and `SetAdditionLink(strings.ToLower(t), ...)`. `controller.GetAddition` (line 650) dispatches `processor.Get(...).AbstractAddition(ctx, art, addition)`.
- **Dispatch key**: `ResolveArtifactType()` returns the config media type `application/vnd.harbor.maven.config.v1+json` (confirmed live), which is exactly the maven Processor's registration key — dispatch is correct.
- **REST**: `api/v2.0/swagger.yaml` getAddition enum already includes `files` (~line 1461). Handler `GetAddition` (src/server/v2.0/handler/artifact.go:475) uppercases the path param → `FILES`.
- **Frontend**: `models.ts` has `ADDITIONS.FILES='files'`; `artifact-additions.component.ts` has `getFile()` reading `additionLinks[ADDITIONS.FILES]`; the HTML already renders a `Files` tab when `getFile()` is truthy; `files/files.component.{ts,html}` exists.

## The actual gap
1. **Backend**: maven `*Processor` does **not** override `ListAdditionTypes`/`AbstractAddition`; it inherits `base.ManifestProcessor` (ListAdditionTypes→nil, AbstractAddition→400 BadRequest). So the maven artifact has **no** `addition_links` (confirmed live) and `GET .../additions/files` would 400.
2. **Frontend**: `files.component` only renders the **tree** shape (`FilesItem{name,type,size,children}`) emitted by the **cnai** processor (`src/controller/artifact/processor/cnai/cnai.go` `AdditionTypeFiles`). Maven needs a **columnar** variant.

`naming.Decode` is **not** involved in this idea.

## Backend changes
File: `src/controller/artifact/processor/maven/maven.go`
- Add `const AdditionTypeFiles = "FILES"`.
- `func (p *Processor) ListAdditionTypes(_ context.Context, _ *artifact.Artifact) []string { return []string{AdditionTypeFiles} }`
- `func (p *Processor) AbstractAddition(ctx, art, addition)`:
  - if `addition != AdditionTypeFiles` → `errors.New(nil).WithCode(errors.BadRequestCode).WithMessagef("addition %s isn't supported for %s", addition, ArtifactTypeMaven)` (mirror chart.go).
  - `PullManifest(art.RepositoryName, art.Digest)` → `Payload()` → unmarshal to `v1.Manifest` → pull config blob `mani.Config.Digest` → return `&ps.Addition{ContentType:"application/json; charset=utf-8", Content: blobBytes}`.
  - The config blob is already canonical `[]FileRef` JSON; can return verbatim. (Optional: `p.UnmarshalConfig` into `[]model.FileRef` + re-marshal for stable ordering.)
- Logging via `src/lib/log` only; no `fmt.Printf`.

No swagger regen, no migration.

## Frontend changes
- `files/files.component.ts`: define `MavenFileRef {filename:string; classifier?:string; extension:string; timestamp?:string; buildNumber?:number; digest:string; size:number}`. In `getFiles()`, branch on response shape: if it's an array whose first element has a `filename` property → populate `mavenFiles` + `isMaven=true`; else keep existing `filesList` tree path. Treat missing `classifier` as main artifact, missing `timestamp` as RELEASE.
- `files/files.component.html`: add `*ngIf="isMaven"` `clr-datagrid` with columns Filename / Classifier / Extension / Type(SNAPSHOT|RELEASE) / Size / Checksum(sha256). Keep the existing `*ngIf="!isMaven"` `clr-tree`. Reuse `ARTIFACT.NO_FILES` empty state.
- `files/files.component.spec.ts`: add a Maven-array case; keep/confirm a tree-shape case (cnai regression guard).
- i18n: add `ARTIFACT.MAVEN_FILE_*` header keys to `en-us-lang.json`, `zh-cn-lang.json`, and the other lang files.
- No change to `artifact-additions.component.*` or `models.ts` (already wired).
- Verify `additions.service.ts` `getDetailByLink(href,false,false)` returns parsed JSON for `application/json`; if it returns text, JSON.parse in the component.

## Verification (SLOT=1)
- Backend up: `curl -s -u admin:Harbor12345 'http://localhost:8180/api/v2.0/projects/library/repositories/maven%2Fmcom_x2eacme_x3awidget2/artifacts/1.0'` → has `addition_links.files.href`.
- `curl` that href → 200, `application/json`, JSON array of FileRef rows.
- Negative: same on `library/npm/mharbor_x2dmulti_x2dformat_x2ddemo/.../additions/files` → 400 (npm out of scope).
- Frontend: `bun run lint_fix && bun run lint_fix:style && bun run test --include='**/files.component.spec.ts'` green; portal :4300 → library → `maven/mcom_x2eacme_x3awidget2` → tag 1.0 → `Files` tab renders datagrid rows for `widget2-1.0.jar` (jar/RELEASE) and `widget2-1.0.pom`.
- Regression: a CNAI/model artifact's Files tab still renders the folder tree.

## Out of scope
npm Files tab; naming.Decode; pull-command; icons; gridview.