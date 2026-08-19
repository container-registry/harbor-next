# Implementation Spec — IDEA #6: Inline npm channel resolution + drift alert (trust reuse)

## Goal (faithful to board, scoped)
Surface npm **channel -> version resolution** inline in the artifact list, and a **channel drift alert** when a channel (e.g. `latest`) now resolves to a version that is unsigned or newly vulnerable. Reuse the scan/SBOM/signature signals Harbor already computes; do **not** re-render them as duplicate chips (they already have dedicated columns). Maven is out of channel scope (no pointer in `extra_attrs`); maven and image rows are unaffected.

This is **frontend-only. No API, swagger, migration, or `gen-apis` run.**

## Verified ground truth (read from the tree)
- Row data is loaded in `artifact-list-tab.component.ts`:
  - `listArtifactsResponse` is called with `withScanOverview:true`, `withSbomOverview:true` (lines ~456-466) -> `artifact.scan_overview` / `artifact.sbom_overview` present per row.
  - `signed` is set per row by `checkCosignAndSbomAsync()` ('true'|'false'|'checking').
  - `handleScanOverview(scan_overview)` already unwraps the single-scanner report.
- npm processor `controller/artifact/processor/npm/npm.go`:
  - sets `art.ExtraAttrs["dist-tags"]` = `map[string]string` (channel -> version), line 84.
  - sets `art.ExtraAttrs["version"]`, `["name"]`, `["description"]`.
  - `GetArtifactType` returns `"NPM"` (`ArtifactTypeNPM`, line 29).
- maven processor returns type `"MAVEN"` (`ArtifactTypeMaven`, maven.go:31); no `dist-tags`.
- Frontend `ArtifactType` enum (`artifact.ts:97`) has **only** IMAGE/CHART/CNAB/OPENPOLICYAGENT. **NPM/MAVEN missing — must be added.**
- `extra_attrs` is on the swagger `Artifact` model (`ng-swagger-gen/models/artifact.ts:77`) typed loosely as `ExtraAttrs = {[prop:string]:{}}`.

## Changes

### 1. `artifact.ts`
Add to `ArtifactType`:
```
NPM = 'NPM',
MAVEN = 'MAVEN',
```
Must match processor literals exactly. Do not add them to `multipleFilter` unless another idea requires it.

### 2. `artifact-list-tab.component.ts` (pure helpers, no fetch)
- `getDistTags(a: ArtifactFront): {channel:string; version:string}[]` — returns `[]` unless `a.type === ArtifactType.NPM` and `a.extra_attrs?.['dist-tags']` is a non-null object; otherwise maps its entries. Defensive: values are strings.
- `private resolveChannelTarget(version:string): ArtifactFront | undefined` — finds the row in `this.artifactList` whose `extra_attrs?.version === version` (drift can only be evaluated for versions currently loaded on the page; off-page targets are skipped — no N+1 fetch).
- `channelDrift(channel:{channel,version}): 'unsigned'|'vulnerable'|null` — resolve target row; if `target.signed === 'false'` -> 'unsigned'; else if `handleScanOverview(target.scan_overview)?.summary` has High/Critical>0 -> 'vulnerable'; `'checking'` signed state -> treat as unknown (null). Else null.

### 3. `artifact-list-tab.component.html`
In the column-one identity cell (`div.cell.white-normal`, ~line 263), after the existing identity content, add an npm-gated strip:
```
<ng-container *ngIf="artifact.type === 'NPM' && getDistTags(artifact).length">
  <span *ngFor="let c of getDistTags(artifact)" class="channel-chip">
    <clr-label>{{ c.channel }} {{ 'REPOSITORY.CHANNEL_RESOLVES_TO' | translate }} {{ c.version }}</clr-label>
    <clr-tooltip *ngIf="channelDrift(c) as drift">
      <clr-icon clrTooltipTrigger shape="warning-standard" class="channel-drift"
        [attr.status]="drift === 'vulnerable' ? 'danger' : 'warning'"></clr-icon>
      <clr-tooltip-content *clrIfOpen>
        {{ (drift === 'vulnerable' ? 'ARTIFACT.CHANNEL_DRIFT_VULNERABLE' : 'ARTIFACT.CHANNEL_DRIFT_UNSIGNED') | translate }}
      </clr-tooltip-content>
    </clr-tooltip>
  </span>
</ng-container>
```
**Do NOT** add a `clr-dg-column`, touch `hiddenArray`, sort comparators, or render scan/sbom/signed chips.

### 4. `artifact-list-tab.component.scss`
Add `.channel-chip` spacing and `.channel-drift` color (reuse existing tokens like `.color-red`).

### 5. i18n
Add `REPOSITORY.CHANNEL_RESOLVES_TO` ("->"/"resolves to"), `ARTIFACT.CHANNEL_DRIFT_UNSIGNED`, `ARTIFACT.CHANNEL_DRIFT_VULNERABLE` to `en-us-lang.json` and all other `src/i18n/lang/*-lang.json`.

## Out of scope (per board)
- No scan/SBOM/signed chip vocabulary (existing columns own that).
- No maven channel UI.
- No channel pointer history (board idea #12 rejected: `SetMutableState` overwrites in place; no audit data exists).
- No backend, no `processor.AbstractAddition`, no `GET /additions` path.

## Verification (SLOT=1)
1. `cd src/portal && bun run lint_fix && bun run lint_fix:style && bun run test --browsers=ChromeHeadless`.
2. `curl -su admin:Harbor12345 'http://localhost:8180/api/v2.0/projects/library/repositories/npm%2Fmharbor_x2dmulti_x2dformat_x2ddemo/artifacts?with_scan_overview=true&with_sbom_overview=true' | jq '.[].extra_attrs'` -> expect `type==NPM`, `dist-tags` object, `version`.
3. Portal http://localhost:4300 -> library -> npm/harbor-multi-format-demo: each npm row shows `channel -> version` clr-label; scan/sbom/signed only in their columns.
4. Force drift (point a channel at an unsigned/vulnerable version, scan it via existing Scan Now); reload -> warning icon + tooltip on that channel.
5. maven `com.acme:widget2` and an IMAGE repo: no channel strip, no console errors.
6. Network tab: no new XHR from this feature.