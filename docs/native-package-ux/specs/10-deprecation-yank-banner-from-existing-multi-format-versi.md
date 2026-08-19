# Implementation Spec — Idea #10: Deprecation / Yank Banner from `multi_format_version.yanked`

## Goal
Surface the existing per-version "yanked/deprecated" fact in the portal:
- A Clarity inline warning alert at the top of the **artifact-summary** detail page.
- A **strikethrough + `[deprecated]` badge** on the version (digest) row in the **artifact-list-tab**.

Faithful to the idea: reuse the PG-authoritative fact, no new boolean extra_attr semantics invented, no new REST/schema.

## Data-flow truth (verified in code)
The yanked fact exists at the storage layer but is **not yet on the artifact the portal renders**:

| Layer | State | File |
|---|---|---|
| PG column `multi_format_version.yanked` | exists, read in `LoadState` | `src/pkg/multiformat/dao/dao.go:160,176` |
| `model.Version.Yanked` | exists | `src/pkg/multiformat/model/model.go:38` |
| `_index` per-version **descriptor** annotation `AnnYanked` | written by `upsertIndexEntry`, read by `reconcileFromIndex` | `src/controller/multiformat/index_ops.go:39`, `src/controller/multiformat/mapper.go:289` |
| `AnnYanked` const | `"vnd.harbor.multiformat.yanked"` | `src/controller/multiformat/const.go:85` |
| per-version **manifest** annotation | **MISSING** — `Publish` stamps only NativeName/Version/PayloadDig/PayloadSize/Created | `src/controller/multiformat/mapper.go:127-133` |
| npm/maven processor → `extra_attrs` | does **not** read yanked | `processor/npm/npm.go:54`, `processor/maven/maven.go:57` |
| portal | reads `artifact.extra_attrs` (open `ExtraAttrs` map) | `ng-swagger-gen/models/extra-attrs.ts` |

**No yank mutation path exists** (`Publish` hardcodes `yanked=false` at `mapper.go:148`; grep for `yank` finds only the storage/projection plumbing). This idea is the surfacing layer only.

## Backend changes
1. **`src/controller/multiformat/mapper.go`** — in `Publish`, add to `verManifest.Annotations`:
   `AnnYanked: strconv.FormatBool(false)` (import `strconv` already used in index_ops; add to mapper imports). This puts the fact on the per-version manifest the abstractor processes. (Optional: source the bool from the existing `_index` descriptor to preserve a prior yank across re-publish.)
2. **`src/controller/artifact/processor/npm/npm.go`** — add `annYanked = "vnd.harbor.multiformat.yanked"` const (mirror pattern of `annDistTags`, line 37). In `AbstractMetadata`, after the annotations unmarshal, if `manifest.Annotations[annYanked]` is present, set `art.ExtraAttrs["yanked"] = raw == "true"`.
3. **`src/controller/artifact/processor/maven/maven.go`** — same const + same `art.ExtraAttrs["yanked"]` assignment.

Set `extra_attrs["yanked"]` **only when the annotation is present**, so non-multiformat artifacts are unaffected.

## Frontend changes
- **`artifact-summary.component.ts`** — add `isYanked(): boolean { return this.artifact?.extra_attrs?.['yanked'] === true; }`.
- **`artifact-summary.component.html`** — at top of `*ngIf="!loading"` block (before `<artifact-label>`), add:
  `<clr-alert clrAlertType="warning" [clrAlertClosable]="false" *ngIf="isYanked()"> ... {{ 'ARTIFACT.DEPRECATED_BANNER' | translate }} ... </clr-alert>`
- **`artifact-list-tab.component.html`** — on the digest `<a class="digest ...">` (~lines 274-280), bind `[class.deprecated]="artifact?.extra_attrs?.['yanked']"` and append `<span class="label label-warning" *ngIf="artifact?.extra_attrs?.['yanked']">{{ 'ARTIFACT.DEPRECATED_BADGE' | translate }}</span>`.
- **`artifact-list-tab.component.scss`** — `.deprecated { text-decoration: line-through; opacity: .7; }`.
- **`i18n/lang/en-us-lang.json`** + **`zh-cn-lang.json`** (and any other lang files) — add `ARTIFACT.DEPRECATED_BANNER` and `ARTIFACT.DEPRECATED_BADGE`.

## No API / schema change
- `extra_attrs` is the existing open `ExtraAttrs` map on the `Artifact` swagger model → **no `swagger.yaml` change, no `task build:gen-apis`**.
- `multi_format_version.yanked` already exists → **no migration**.

## Verification (SLOT=1)
1. `task dev:backend:up` + `task dev:frontend:native` (portal :4300, core :8180, admin/Harbor12345).
2. Establish a yanked fact (no yank action exists): re-publish a test version with the manifest `AnnYanked` set true (or temporarily flip the stamp and re-push), then `curl http://localhost:8180/api/v2.0/projects/library/repositories/<encoded-repo>/artifacts/1.0.0` → expect `extra_attrs.yanked: true`.
3. Test data: npm `library/npm/pharbor_x2dmulti_x2dformat_x2ddemo` (harbor-multi-format-demo@1.0.0), maven `library/maven/pcom_x2eacme_x3awidget2` (com.acme:widget2:1.0).
4. Browser: version row shows struck-through digest + `deprecated` badge; detail page shows the warning alert above labels. Non-yanked version shows neither.
5. `bun run lint_fix`, `bun run lint_fix:style`, `bun run test` (ChromeHeadless) on the two touched components.

## Risks / caveats
- Banner/badge are **dormant until a yank mutation action exists** (separate idea). Verify by injecting the fact.
- Surfacing needs the fact on the **manifest** (step 1), not just the PG column or `_index` descriptor — the column alone never reaches `extra_attrs`.
- Pre-existing artifacts published before this change lack the manifest annotation → must be re-published to show the badge.
- `Publish` stamps `false`; a future yank action must preserve the value on re-publish to avoid silent un-yanking.
- i18n key parity across all lang files (lint-enforced).