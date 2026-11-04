# Model Import, Option B: Extend Registries, Replication and Artifact Views

Prototype answering the objection to extending replication for Hugging Face ("a form full of
greyed-out push/pull and namespace options", "generic extensions are in practice for one
adapter"). There is no Model Sync page, table or API. Every model-related control is driven by
a capability the source declares; no portal code checks for `huggingface`.

Built on [model-import-foundation.md](model-import-foundation.md) (bookmark
`model-import/foundation`).

## Workspace and commits

Workspace `harbor.next-model-ui-b`, bookmark `model-import/option-b`:

| Change | Commit |
|---|---|
| `wppuwzkt` | feat(registry): Add Copy Preview API and Form Capabilities to Registry Info |
| `nrqxqwmr` | feat(portal): Add Strings for Model Import Views |
| `zxkqlvon` | feat(portal): Drive the Replication Rule Form by Source Capabilities |
| `smlqrqol` | feat(portal): Derive Endpoint Credential Inputs from the Credential Pattern |
| `sosvtmou` | feat(portal): Show Model Metadata on CNAI Artifacts |
| `mmvsnwto` | docs: Add Model Import Option B Notes (this file) |

Size against the foundation (added/removed lines):

| Area | Lines |
|---|---|
| Backend code (controller, handler, model, HF adapter) | +130 −14 |
| Backend tests and mock | +211 −1 |
| API (`swagger.yaml`) | +151 |
| Portal code (ts, html, scss) | +1121 −41 |
| Portal tests | +340 −2 |
| i18n (10 lang files, English text) | +470 −40 |

## Extensions, all generic

| Extension | Declared by | Used by the portal for | Other adapters that can use it |
|---|---|---|---|
| `POST /registries/{id}/preview` and `RegistryInfo.supported_preview` | derived by the registry controller from `adapter.ArtifactResolver`, adapters declare nothing | preview panel in the rule form | any source that can resolve a reference to a digest without copying, e.g. a future ModelScope/Kaggle adapter, or OCI registries resolving tag to digest |
| `FilterStyle.label`, `placeholder`, `description` | adapter `Info()` | filter label, placeholder and tooltip (fallback: the existing i18n text) | any adapter whose filters mean something specific (e.g. "Image" vs "Chart") |
| `AdapterPattern.source_only` (`GET /replication/adapterinfos`, foundation) | adapter factory, mirrors `RegistryInfo.source_only` | destination picker, endpoint form helper | any adapter that synthesizes artifacts from another protocol |
| `file` in `supported_resource_filters` (foundation) | adapter `Info()` | Include/Exclude inputs, serialized as `{include, exclude}` | any source with per-file selection |
| `supported_revision_tag` (foundation) | adapter `Info()` | "Also tag each copy with its source revision (sha-…)" | any source with immutable revisions behind moving refs |
| Credential pattern `AccessKeyTypeFix` / `AccessSecretTypeFile` | adapter pattern | fixed, read-only access key; text area only for file secrets | replaces the `google-gcr` type check; GCR and Quay already declare it |

The preview API: body `{repository, reference, file_filter: {include, exclude}}`, answer
`RegistryPreview` (revision, revision_tag, source_url, tags, digest, licenses, task, counts,
sizes, files with class/size/sha256/matched). `400` for an adapter without a resolver
("the registry Local does not support previews"), `422` without `repository`. RBAC: registry
read (system level), like `GET /registries/{id}/info`.

## Rule form behavior

- Source with `supported_preview`: modal widens (46rem instead of 32rem) and a Preview row
  appears under the filters. An exact name filter and an exact `matching` tag filter are used
  directly; a pattern name filter (`org/**`) asks for one example repository. One request per
  click; editing Include/Exclude re-matches the file list locally (counts, sizes, struck-through
  unmatched files) and replaces the digest with "File filter changed, preview again". Copy tags
  are the source tags kept by the tag filter, plus the `sha-` tag when the checkbox is on.
- Source without these declarations (Docker Hub, Harbor, local push): no new control, same
  labels, same width. Verified in the browser: Docker Hub pull form has no `#rule_preview`,
  `#filter_file_include` or `#revision-tag`, labels `Name: Tag: Resource:`, width 640px; push
  form labels `Name: Tag: Label: Resource:`.
- Push destinations exclude source-only types: with registries `huggingface` and `dockerhub`,
  the destination select offers only `dockerhub`.
- Edit restores both value forms of the file filter (string or object) and `revision_tag`; the
  Save button stays disabled until something changes.

## Tests

```
cd src
go test ./pkg/reg/... ./controller/replication/... ./controller/registry/ ./server/v2.0/handler/
golangci-lint run ./controller/registry/... ./server/v2.0/handler/ ./pkg/reg/adapter/huggingface/... ./pkg/reg/model/...
cd portal
npx ng test --watch=false \
  --include=src/app/shared/units/file-filter.spec.ts \
  --include=src/app/base/left-side-nav/replication/replication/create-edit-rule/create-edit-rule.component.spec.ts \
  --include=src/app/base/left-side-nav/registries/create-edit-endpoint/create-edit-endpoint.component.spec.ts \
  --include=src/app/base/project/repository/artifact/artifact-additions/files/files.component.spec.ts \
  --include=src/app/base/project/repository/artifact/artifact-model-metadata/artifact-model-metadata.component.spec.ts \
  --include=src/app/base/project/repository/artifact/artifact-common-properties/artifact-common-properties.component.spec.ts
```

New: `controller/registry/preview_test.go`, preview/info cases in
`server/v2.0/handler/registry_test.go`, pattern/label checks in the HF `adapter_test.go`;
portal `shared/units/file-filter.spec.ts` (matcher cases mirror `FileFilter.Match`),
`artifact-model-metadata.component.spec.ts`, and additions to the create-edit-rule,
create-edit-endpoint and artifact-common-properties specs. The registry controller mock was
extended by hand (mockery v2.53.3 fails here with "package context without types").

## End to end (browser, SLOT=2, 2026-11-04)

1. Endpoint `huggingface` created in the form: provider Hugging Face shows "Source only…",
   Access ID prefilled `token` and read-only, secret is a password input. `201`, healthy.
2. Rule `smollm2-json`, pull, source `huggingface`, Model `HuggingFaceTB/SmolLM2-135M`,
   Revision `main`, Include `*.json`, destination namespace `hf-models`, revision tag on.
3. Preview: revision `93efa2f097d5`, copy tags `main`, `sha-93efa2f097d5`, license
   `apache-2.0`, task `text-generation`, 6 of 9 files 2.78MiB of 259.82MiB, digest
   `sha256:2dc8fb68cff3…`. Adding exclude `tokenizer*` re-matched to 4 files without a request
   (1 preview request total).
4. Saved: `POST /replication/policies` 201 with
   `"revision_tag":true` and `{"type":"file","value":{"include":["*.json"]}}` (the screenshot run
   still sent `"exclude":[]`; empty lists are now omitted).
5. Run, execution 3 `Succeed 1/1`. Task log: `source options: {"file_filter":{"include":["*.json"]},"revision_tag":true}`,
   `copying huggingfacetb/smollm2-135m:[main,sha-93efa2f097d5] to hf-models/smollm2-135m:[main,sha-93efa2f097d5]`.
   Artifact `sha256:2dc8fb68cff37ff1f1ae1bcef28039072cb9a012d4e7c7c817cee9d3d1b3b18e` CNAI, tags
   `sha-93efa2f097d5`, `main`, 2912950 bytes.
6. Rerun, execution 4 `Succeed`: `the artifact hf-models/smollm2-135m:main already exists on the
   destination registry, skip` and the same for `sha-93efa2f097d5`; digest unchanged.
7. Edit rule, clear Include, Preview: digest
   `sha256:2b6f786b1ea21fb04c8466d412b19b00aba263c23d094170886b623f432508e3`, 9 files (the full
   copy digest of the foundation). Not saved, not copied.
8. Artifact view: Model badge in the list; Model section with licenses `apache-2.0`, name,
   source link, source revision `93efa2f…`, created, layers 6; Overview no longer dumps
   `descriptor`/`config`/`modelfs`; Files tab icons by class (`cog` for the six configs).

## Screenshots

Light theme, English, 1600x1000 viewport, uncropped. Files `option-b-shots/NN-<slug>.png` in the
session scratchpad, numbered like the Option A set. No secret is entered in any of them.

| File | Caption |
|---|---|
| `01-nav-entry.png` | Model import lives in Administration > Replications; no new navigation entry. |
| `02-registry-endpoint-hf.png` | New endpoint, provider Hugging Face: "Source only" note, access ID fixed to `token` (read-only), only the token is asked for. |
| `03-policy-list.png` | The HF rule `smollm2-json` is an ordinary pull-based replication rule (source `huggingface`, destination `Local : hf-models`). |
| `04-create-form-empty.png` | Pull rule with the HF source selected: filters renamed by the adapter (Model, Revision, Files with Include/Exclude), Preview row, "Also tag each copy with its source revision" option. No push/pull or namespace field is greyed out. |
| `05-create-form-preview.png` | `HuggingFaceTB/SmolLM2-135M` @ `main`, include `*.json`: revision `93efa2f097d5`, copy tags `main` + `sha-93efa2f097d5`, license, task, 6 of 9 files, digest `sha256:2dc8fb68…`, file list with class icons. |
| `05b-create-form-preview-options.png` | Same form scrolled: destination namespace `hf-models`, revision tag option checked. |
| `06-executions.png` | Executions 3 (first copy, 6 s) and 4 (rerun, skipped, 566 ms), both succeeded. |
| `07-task-log.png` | Task log of execution 3: source options `{"file_filter":{"include":["*.json"]},"revision_tag":true}`, copy of `[main,sha-93efa2f097d5]`. |
| `07a-task-list.png` | Task list of execution 3; tasks show the item count, not the tags (limitation). |
| `08-artifact-detail.png` | Resulting CNAI artifact `sha256:2dc8fb68`: tags `sha-93efa2f097d5` and `main`, Model section (license, name, source link, source revision, created, layers); Overview no longer dumps the model JSON. |
| `08a-artifact-list.png` | Artifact list with the "Model" badge. |
| `08b-artifact-files.png` | Files tab with icons by file class. |
| `09-rule-form-dockerhub-source.png` | Pull rule from Docker Hub: the form as before (Name, Tag, Resource; no file filter, preview or revision tag; original width). |
| `10-push-destinations.png` | Push mode: the destination list holds only `dockerhub`; the `huggingface` endpoint is left out. The select is rendered expanded (`size` attribute set by script) because an open native dropdown cannot be captured. |

## Dev stack

`task dev:infra:up SLOT=2`, `task dev:backend:up SLOT=2 SKIP_TRIVY=true`, portal
`HARBOR_PROXY_TARGET=http://localhost:8280 ng serve --port 4400` from `src/portal`.

| | |
|---|---|
| Portal | http://localhost:4400, `admin` / `Harbor12345` |
| Core API | http://localhost:8280/api/v2.0 |
| Postgres / Redis / Registry | 5632 / 6579 / 50200 |
| State | registries `huggingface` (2), `dockerhub` (3); project `hf-models`; policy 1 `smollm2-json`; executions 3, 4 |

Browser login over plain HTTP needed no change: this base no longer uses gorilla/csrf but
`net/http.CrossOriginProtection`, and the browser sends `Sec-Fetch-Site: same-origin` through
the ng serve proxy.

## Limitations

- Filter labels, placeholders and descriptions come from the adapter in English; they bypass
  portal i18n. An i18n-key variant would couple adapters to portal translations.
- `source_only` is declared twice (adapter pattern for type-level pickers, registry info per
  instance); the HF test asserts they agree.
- Replication tasks still show `[2 item(s) in total]`; the revision tag and digest are only in
  the task log. Showing them needs task-level fields in the backend (not done).
- No preview or file filter on proxy cache projects (proxy cache ignores filters by design).
- The "unchanged" Docker Hub form is verified by DOM assertions and a screenshot of this build,
  not by a pixel diff against an unmodified build.
- The non-English lang files carry the English strings.
- Preview runs the source requests synchronously in the core request (HF: refs, tree listing,
  raw download of matched non-LFS files not yet cached).
