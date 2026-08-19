# Implementation Spec — Idea #3: Per-ecosystem Install/Usage ADDITIONS tab (Set-Me-Up)

## Goal
Add a frontend-only **Usage** tab to the artifact ADDITIONS area, rendered only for `NPM` and `MAVEN` artifacts, that shows a static, copy-pastable ecosystem setup snippet. All content is computed in the browser from `artifact.extra_attrs` (already loaded) plus the registry host. No backend, no API, no schema change.

## Verified ground truth
- **Artifact types**: `ArtifactTypeNPM = "NPM"` (`src/controller/artifact/processor/npm/npm.go:29`), `ArtifactTypeMaven = "MAVEN"` (`src/controller/artifact/processor/maven/maven.go:31`).
- **extra_attrs keys**:
  - npm: `name`, `version`, `description`, `dist-tags` (`npm.go:64,67,70,84`).
  - maven: `groupId`, `artifactId`, `version`, `files` (`maven.go:70,71,77,85`).
- **Frontend model**: `Artifact.extra_attrs?: ExtraAttrs` exists; `ExtraAttrs` is `{ [prop: string]: {} }` (open index) — `artifact.extra_attrs['name']` is valid.
- **Native endpoints**: `<ExtEndpoint>/npm/<project>/` and `<ExtEndpoint>/maven/<project>/` (`src/server/route_multiformat.go`, `registry/npm/route.go` `Prefix="/npm"`, `registry/maven/route.go` `mux.Handle("/maven/", ...)`).
- **registryUrl source**: `AppConfigService.getConfig().registry_url` (pattern in `artifact-list-tab.component.ts:245-246`).
- **The blocker**: `artifact-additions.component.html` wraps the entire `<clr-tabs>` in `*ngIf="additionLinks"`. npm/maven artifacts carry no `addition_links`, so the tab strip is invisible for them unless the guard is widened.
- **`hasPullCommand` (artifact.ts:110-116)** already excludes NPM/MAVEN (they aren't in `ArtifactType`), so no `docker pull` shows today. Coordinate with idea #1 to keep it that way.
- **Reusable copy widget**: `hbr-copy-input` (`src/app/shared/components/push-image/copy-input.component.ts`), already used by `pull-command.component.html`.

## Files to touch (frontend only)

### 1. `artifact-additions/models.ts`
Add to the `ADDITIONS` enum:
```
USAGE = 'usage',
```
Used purely as the clr-tab id; it is never used as a key into `additionLinks`.

### 2. `artifact.ts`
- Add `NPM = 'NPM'` and `MAVEN = 'MAVEN'` to `ArtifactType` (shared with ideas #1/#2/#4 — add only if absent).
- Keep NPM/MAVEN out of `hasPullCommand` (already the case). This is the #1 coordination point.

### 3. NEW `artifact-additions/usage/usage.component.{ts,html,scss}`
- Selector `hbr-artifact-usage`.
- Inputs: `artifact: Artifact`, `registryUrl: string`, `projectName: string`.
- Getters (pure, no side effects):
  - `host()` = `this.registryUrl || location.hostname`
  - `isNpm()` / `isMaven()` on `artifact.type`
  - `npmRegistryUrl()` = `` `${host()}/npm/${projectName}/` ``
  - `mavenRepoUrl()` = `` `${host()}/maven/${projectName}/` ``
  - npm: `pkgName` / `pkgVersion` from extra_attrs; `scope` derived from a leading `@scope/` in the name (`''` when unscoped); `npmrcSnippet` (scoped → `@scope:registry=...` + `//host/path/:_authToken=${TOKEN}`; unscoped → `registry=...` + auth line); `npmInstallSnippet` = `npm install <name>@<version>`.
  - maven: `groupId`/`artifactId`/`version` from extra_attrs; `serverXml` (settings.xml `<server>` with `<id>harbor</id>`, `<username>` placeholder, `<password>${TOKEN}</password>`), `repoXml` (pom.xml `<repository>` → `mavenRepoUrl()`), `dependencyXml` (`<dependency>` with the GAV).
- `${TOKEN}` is a **literal placeholder**, not interpolated; copy hint text points to the existing robot/view-token flow.
- No `console.log`.

Template renders the npm block (`*ngIf="isNpm()"`) with two `hbr-copy-input` widgets, or the maven block (`*ngIf="isMaven()"`) with three. All labels via `translate`.

### 4. `artifact-additions.component.ts`
- Add `@Input() registryUrl: string;`
- Add `isMultiFormat(): boolean { return this.artifact?.type === 'NPM' || this.artifact?.type === 'MAVEN'; }`
- In `ngOnInit`, when `isMultiFormat()` and no `activeTab`, default `currentTabLinkId = 'usage'`.

### 5. `artifact-additions.component.html`
- Change the outer guard from `*ngIf="additionLinks"` to `*ngIf="additionLinks || isMultiFormat()"`.
- Add a new tab:
```
<clr-tab *ngIf="isMultiFormat()">
  <button clrTabLink id="usage" (click)="actionTab('usage')">{{ 'ARTIFACT.USAGE' | translate }}</button>
  <ng-template [clrIfActive]="currentTabLinkId === 'usage'">
    <clr-tab-content id="usage-content">
      <hbr-artifact-usage *ngIf="currentTabLinkId === 'usage'"
        [artifact]="artifact" [registryUrl]="registryUrl" [projectName]="projectName"></hbr-artifact-usage>
    </clr-tab-content>
  </ng-template>
</clr-tab>
```

### 6. `artifact-summary.component.ts`
Inject `AppConfigService`; add `registryUrl` set from `getConfig().registry_url` in `ngOnInit`.

### 7. `artifact-summary.component.html`
Add `[registryUrl]="registryUrl"` to `<artifact-additions>` (currently not passed).

### 8. `artifact.module.ts`
Declare `ArtifactUsageComponent`.

### 9. i18n (`src/i18n/lang/en-us-lang.json` + `zh-cn-lang.json` and any other lang files)
Add `ARTIFACT.USAGE` and the step/label keys; mirror across all lang files to avoid patch drift.

## What is explicitly NOT done
- No `processor.ListAdditionTypes` / `AbstractAddition`.
- No `GET /additions/{type}` route, no swagger.yaml edit, no `task build:gen-apis`.
- No migration.
- No robot minting (deferred; `${TOKEN}` placeholder + link to existing flow only).

## Verification (SLOT=1)
1. `cd src/portal && bun run lint_fix && bun run lint_fix:style && bun run test` (add a `usage.component.spec.ts`).
2. `task dev:frontend:native` (portal 4300, Core 8180). Login admin/Harbor12345.
3. npm: project `library` → repo `library/npm/mharbor_x2dmulti_x2dformat_x2ddemo` → version `harbor-multi-format-demo@1.0.0`. Expect a default **Usage** tab with `.npmrc` snippet (registry/_authToken against `http://<host>/npm/library/`) + `npm install harbor-multi-format-demo@1.0.0`; copy works.
4. maven: repo `library/maven/mcom_x2eacme_x3awidget2` → version `1.0`. Expect Usage tab with `settings.xml <server>`, `pom.xml <repository>` (→ `http://<host>/maven/library/`), and `<dependency>` for `com.acme:widget2:1.0`.
5. Negative: an OCI image artifact shows **no** Usage tab; existing tabs unchanged.
6. Confirm no `docker pull` shown for npm/maven (idea #1 coordination).