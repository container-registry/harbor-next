# Implementation Spec — Idea #1: Per-ecosystem usage emitter (kill `docker pull` for npm/maven)

> RECONCILIATION (approved, supersedes any "multi-line <dependency> XML in the widget" wording below):
> the pull-command WIDGET shows a SHORT one-liner per ecosystem — npm: `npm install <name>@<version>`;
> maven: `mvn dependency:get -Dartifact=<groupId>:<artifactId>:<version>`. The full multi-line
> `<dependency>...</dependency>` XML and `.npmrc`/`settings.xml` setup belong to the per-ecosystem Usage
> additions tab (task #3), NOT the compact widget. For THIS task, verify the maven WIDGET emits the
> one-liner; the multi-line XML requirement is satisfied and verified under #3.

## Goal
In the Setup & Usage / pull-command widget, NPM and MAVEN artifacts must show a native usage snippet instead of a docker pull (or nothing):
- NPM → `npm install <name>@<version>`
- MAVEN → a `<dependency>` XML block (groupId / artifactId / version)

Faithful to the existing CHART precedent (which already emits a non-docker `helm pull ...`), this extends the same type-switch.

## Confirmed facts (read from source + live SLOT=1)
- Artifact type strings are exactly `"NPM"` and `"MAVEN"` (`processor/npm/npm.go` `ArtifactTypeNPM`, `processor/maven/maven.go` `ArtifactTypeMaven`).
- `extra_attrs` is populated by the processors and returned verbatim by the artifacts list API:
  - NPM: `name`, `version`, `description`, optional `dist-tags`.
  - MAVEN: `groupId`, `artifactId`, `version`, `files[]`.
- Live data: `library/npm/mharbor_x2dmulti_x2dformat_x2ddemo` → `{name:"harbor-multi-format-demo",version:"1.0.0"}`; `library/maven/mcom_x2eacme_x3awidget2` → `{groupId:"com.acme",artifactId:"widget2",version:"1.0"}`.
- The widget's `repoName` input is the **encoded** repo path (`npm/mharbor_x2dmulti_x2dformat_x2ddemo`), NOT the native name → must not be used to build native commands.
- `naming.Decode` does **not** exist and is **not** needed: `extra_attrs` already carries native identity. The naming package doc explicitly states Decode is not provided.

## No API / schema change
`ExtraAttrs` already exists in `ng-swagger-gen/models/artifact.ts` (`extra_attrs?: ExtraAttrs`). Do not edit `swagger.yaml`, do not run `task build:gen-apis`, no migration.

## Files to touch (frontend only)
1. `src/portal/src/app/base/project/repository/artifact/artifact.ts`
   - `ArtifactType` enum: add `NPM = 'NPM'`, `MAVEN = 'MAVEN'`.
   - `hasPullCommand()`: also true for `NPM`/`MAVEN`.
   - Add `getNpmInstallCommand(artifact)` → `npm install ${name}@${version}` (drop `@${version}` if version absent; return `''` if name absent). Pass scoped names through verbatim.
   - Add `getMavenDependencySnippet(artifact)` → multi-line `<dependency>...</dependency>` from `groupId`/`artifactId`/`version`; return `''` if groupId or artifactId absent.
2. `src/portal/.../pull-command/pull-command.component.ts`
   - Add `isNpm()`, `isMaven()` predicates and `getNpmCommand()`/`getMavenSnippet()` wrappers (no url/registry args needed).
   - Extend `hasPullCommandForTag()` to include `NPM`/`MAVEN` (keep existing accessory exclusions) so the tag-mode dropdown renders.
3. `src/portal/.../pull-command/pull-command.component.html`
   - In both the non-tag dropdown and the tag-mode dropdown, add `*ngIf="isNpm(artifact)"` (id `pullCommandForNpm`) and `*ngIf="isMaven(artifact)"` (id `pullCommandForMaven`) `hbr-copy-input` blocks bound to the new getters, mirroring the CHART/CNAB blocks; wire `(onCopySuccess)="onCpSuccess(<getter>)"`.
4. `src/portal/src/i18n/lang/en-us-lang.json` (+ `zh-cn-lang.json`, other langs): add only any new static label/tooltip text introduced; reuse existing `ARTIFACT.*` keys where possible.
5. `src/portal/.../pull-command/pull-command.component.spec.ts`: add tests (see verification).

## Out of scope (explicitly)
- Registry endpoint configuration (`npm config set registry`, Maven `settings.xml`). The snippet assumes the client is already pointed at Harbor.
- Adding NPM/MAVEN to the type filter (`multipleFilter`) and gridview icons (separate ideas).

## Verification (SLOT=1)
- API sanity: `curl -s -u admin:Harbor12345 'http://localhost:8180/api/v2.0/projects/library/repositories/npm%252Fmharbor_x2dmulti_x2dformat_x2ddemo/artifacts?with_tag=true'` and the maven equivalent show the `extra_attrs` above (verified).
- Frontend gates: `bun run lint_fix`, `bun run lint_fix:style`, `bun run test --include='**/pull-command.component.spec.ts' --browsers=ChromeHeadless` in `src/portal`.
- Browser at http://localhost:4300 (admin/Harbor12345): library/npm repo shows `npm install harbor-multi-format-demo@1.0.0`; library/maven repo shows the `<dependency>` XML with `com.acme`/`widget2`/`1.0`; copy shows COPY_SUCCESS toast.
- Regression: IMAGE/CHART/CNAB repos unchanged.