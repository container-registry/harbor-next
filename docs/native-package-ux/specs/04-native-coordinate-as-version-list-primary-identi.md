# Implementation Spec — Idea #4 (S): Native coordinate as version-list primary identity + channel/dist-tag badge

## Goal
Inside a package's version list (the artifact-list-tab datagrid), for NPM and MAVEN artifacts, make the human-readable native **version** the clickable row identity instead of a sliced sha256 digest, and append a small Clarity badge for the channel (npm dist-tag(s); maven RELEASE/SNAPSHOT). Demote the digest to a hover tooltip. Leave OCI/CHART/CNAB rows exactly as they are. **No parallel Version column, no `hiddenArray`/sort-comparator changes, no backend/API/schema/migration changes.**

## Confirmed ground truth (verified against code + SLOT=1 dev env)
- Backend artifact type is the literal string `"NPM"` / `"MAVEN"`: `src/controller/artifact/processor/npm/npm.go:90` and `src/controller/artifact/processor/maven/maven.go:89` (`GetArtifactType`).
- The list endpoint already returns `type` and `extra_attrs` with **no extra query param**. Verified via:
  - `GET /api/v2.0/projects/library/repositories/npm%2Fmharbor_x2dmulti_x2dformat_x2ddemo/artifacts` -> `type:"NPM"`, `extra_attrs:{name, version:"1.0.0", description}`, **no `dist-tags` key** on this package.
  - `GET .../maven%2Fmcom_x2eacme_x3awidget2/artifacts` -> `type:"MAVEN"`, `extra_attrs:{groupId:"com.acme", artifactId:"widget2", version:"1.0", files:[...]}`.
- The list call in `artifact-list-tab.component.ts:456-466` (`listArtifactParams`) does **not** set any `withExtraAttrs`, and none exists in the generated `artifact.service.ts` — extra_attrs is returned regardless. No swagger change required.
- npm dist-tags, when present, are `extra_attrs["dist-tags"]` typed `map[string]string` (npm.go:81-85), e.g. `{"latest":"1.0.0"}`. They are **optional**.
- The frontend `ArtifactType` enum (`artifact.ts:97-102`) has only `IMAGE/CHART/CNAB/OPENPOLICYAGENT` — **NPM and MAVEN are missing and must be added.** This is the only model gap.
- Column-one today (artifact-list-tab.component.html:274-280) renders `{{ artifact.digest | slice : 0 : 15 }}` inside an `<a (click)="goIntoArtifactSummaryPage(artifact)" title="{{ artifact.digest }}">`. The digest tooltip already exists.

## Changes

### 1. `artifact.ts` (frontend model + helpers)
- Extend `ArtifactType`:
  ```
  export enum ArtifactType {
      IMAGE = 'IMAGE',
      CHART = 'CHART',
      CNAB = 'CNAB',
      OPENPOLICYAGENT = 'OPENPOLICYAGENT',
      NPM = 'NPM',
      MAVEN = 'MAVEN',
  }
  ```
  (Values must match the Go constants exactly. Shared with ideas #1/#2 — coordinate the edit.)
- Add pure helpers (kept out of the template for testability):
  - `getNativeVersion(artifact): string` -> `String(artifact?.extra_attrs?.['version'] ?? '')`.
  - `getChannelBadges(artifact): string[]`:
    - NPM: `Object.keys((artifact?.extra_attrs?.['dist-tags'] as Record<string,string>) ?? {})`.
    - MAVEN: a single-element array — `[getNativeVersion(artifact).endsWith('-SNAPSHOT') ? 'SNAPSHOT' : 'RELEASE']` (empty string version -> return `[]`).
    - else `[]`.

### 2. `artifact-list-tab.component.ts`
- Expose to template: `ArtifactType = ArtifactType;` and thin methods `isNativePackage(a) { return a.type === ArtifactType.NPM || a.type === ArtifactType.MAVEN; }`, `nativeVersion(a)`, `channelBadges(a)` delegating to the helpers. No data-loading change.

### 3. `artifact-list-tab.component.html` (column-one cell only)
- Replace the static digest title with a type-gated title; keep the same anchor, click handler, and digest tooltip:
  ```html
  <a href="javascript:void(0)" class="digest margin-left-5"
     (click)="goIntoArtifactSummaryPage(artifact)"
     title="{{ artifact.digest }}">
      <ng-container *ngIf="isNativePackage(artifact) && nativeVersion(artifact); else digestTitle">
          {{ nativeVersion(artifact) }}
      </ng-container>
      <ng-template #digestTitle>{{ artifact.digest | slice : 0 : 15 }}</ng-template>
  </a>
  <span class="label label-light-blue native-channel-badge"
        *ngFor="let badge of channelBadges(artifact)">{{ badge }}</span>
  ```
  (Static Clarity label markup `<span class="label">` — no extra import. Fallback to sliced digest if a native artifact ever lacks a version, so the title is never blank.)
- Do **not** touch any `clr-dg-column`, `hiddenArray[*]`, or `clrDgSortBy`.

### 4. `artifact-list-tab.component.scss`
- `.native-channel-badge { margin-left: 4px; }` (inline, beside the title). Keep existing `.digest`/`.artifact-icon` rules.

### 5. `artifact-list-tab.component.spec.ts`
- Add a test mounting mock artifacts: NPM `{version:'1.0.0','dist-tags':{latest:'1.0.0'}}` -> `channelBadges` = `['latest']`, `nativeVersion` = `'1.0.0'`; MAVEN `{version:'2.5.0-SNAPSHOT'}` -> `['SNAPSHOT']`; MAVEN `{version:'1.0'}` -> `['RELEASE']`; NPM without dist-tags -> `[]`; IMAGE artifact -> column one still shows sliced digest.

## Out of scope / explicitly NOT done
- No swagger.yaml edit, no `task build:gen-apis`, no migration, no Go change.
- No new datagrid column, no change to column hide/sort contract.
- Trust chips / npm channel->version resolution are idea #6, not here.

## Risks
- Shared enum edit with #1/#2 (conflict if parallel).
- `dist-tags` optional -> default to `[]`.
- Maven channel inferred from `-SNAPSHOT` suffix only (timestamped resolved snapshots misclassify as RELEASE; acceptable at S scope).
- `extra_attrs.version` is `any` -> cast to string in helper for strict templates/lint.