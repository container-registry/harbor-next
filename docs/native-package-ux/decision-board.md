# Product Decision Board: npm/Maven Native UX

Scope now: npm + Maven. Every adopted idea generalizes to pypi/cargo/nuget because it rides a format-agnostic seam (`extra_attrs` contract, path-keyed `defaultIcons`, the additions mechanism, the `multi_format_package` projection). Ground truth verified against the tree: the naming codec is injective and lossless except a rare >96-char sha256 fallback (so client/server decode is viable without a DB column); `multi_format_package(project_id, format, native_name UNIQUE)` + index `idx_multi_format_package_proj_fmt` exist (search/grouping need no schema); `multi_format_version.yanked` already exists; `SetMutableState` overwrites in place (no channel history data).

## Ranking

| # | Idea | Surface | Rec | Score | Effort |
|---|------|---------|-----|-------|--------|
| 1 | Per-ecosystem usage emitter (kill `docker pull`) | Setup & Usage / pull-command | Adopt now | 92 | S |
| 2 | Readable native identity everywhere + icons | Naming & Org | Adopt now | 90 | M |
| 3 | Per-ecosystem Install/Usage ADDITIONS tab (Set-Me-Up) | Package Detail / additions | Adopt now | 84 | M |
| 4 | Native coordinate + channel badge as version identity | Versions & trust | Adopt now | 82 | S |
| 5 | Search-by-native-name | Discovery / search | Adopt now | 78 | M |
| 6 | Inline trust chips + npm channel resolution | Versions & trust | Adopt later | 71 | M |
| 7 | Maven GAV Files tab | Package Detail / additions | Adopt later | 70 | M |
| 8 | Project Packages view (group + chips in existing list) | Discovery / org | Adopt later | 66 | M |
| 9 | npm README + Versions/dist-tags additions | Package Detail / additions | Adopt later | 58 | M |
| 10 | Deprecation/yank banner (reuse `multi_format_version.yanked`) | Versions & trust | Adopt later | 55 | S |
| 11 | Format Descriptor Registry / GET /formats | Extensibility | Reject | 40 | L |
| 12 | Channel pointer history / audit strip | Versions & trust | Reject | 28 | L |

The two flagged gaps are both present and judged: **per-ecosystem setup/usage** is adopted as #1 (one-liner) + #3 (rich Set-Me-Up tab); **naming readability** is adopted as #2 (adopted with the fix that drops the proposed `display_name` DB column/migration in favor of presentation-layer decode).

---

## 1. Per-ecosystem usage emitter (Adopt now, S)

**Problem.** The pull widget emits `docker pull core:8080/library/npm/mharbor_x2dmulti_x2dformat_x2ddemo` for npm AND maven. Wrong client for both, exposes the escaped storage name, copy-paste is a dead end -> Harbor looks broken. Single most damaging gap.

```
npm row dropdown (one-line copy field):
  $ npm install @harbor-multi-format/demo@1.4.2 --registry=<ExtEndpoint>/npm/<project>/   [copy]
maven row dropdown:
  $ mvn dependency:get -Dartifact=com.acme:widget2:2.0.1   [copy]
  (full <dependency> XML -> Usage tab, idea #3)
```

**Plug-in point.** `src/portal/.../artifact/artifact.ts` `hasPullCommand` / `getPullCommandByTag` / `getPullCommandByDigest` (existing IMAGE/CNAB/CHART type-switch; CHART already emits `helm pull`). Add NPM/MAVEN enum + cases reading `extra_attrs`; rendered by `pull-command.component.ts`.

**Lenses.** UX (weaken->fixed): one-line widget can't hold multi-line maven XML and the registry flag was wrong; fixed by emitting a single correct one-liner, deferring XML to the Usage tab, dropping the npm/maven selector. Cloudsmith (weaken->fixed): table-stakes and the URL pointed at the storage path; fixed by building from the live native route `<ExtEndpoint>/npm/<project>/` + unescaped `extra_attrs` name. Overengineering (survive): pure frontend, no backend/API/DB.

**Cloudsmith.** Removes the GitHub-Packages anti-pattern (`docker pull` for everything). Reaches the floor; the rich Set-Me-Up differentiation is #3.

---

## 2. Readable native identity everywhere + icons (Adopt now, M)

**Problem.** Grid/breadcrumb/header render the OCI storage path (`mcom_x2eacme_x3awidget2`) or a sha256 title. npm/maven are visually identical to OCI images (no icon).

```
Repositories / Packages grid:
  [npm]  @harbor/multi-format-demo     3 versions    1d ago
  [mvn]  com.acme:widget2      7 versions    2h ago
  [oci]  library/nginx        12 artifacts   5m ago
  (escaped path hidden; copy/details only)
```

**Plug-in point.** Presentation-layer decode: `format` = 2nd path segment of `r.name`; decode 3rd segment via `naming.Decode` (lossless `_xNN` reverse); rare sha256-fallback -> read `extra_attrs` name/groupId/artifactId. Icons: `src/lib/icon/const.go` `DigestOfIconNPM/Maven` + `src/controller/icon/controller.go` `builtInIcons` + `src/controller/artifact/controller.go` `defaultIcons` (keyed on `art.Type`). Surfaces: `repository-gridview.component.html`, `artifact-summary` breadcrumb + header, `artifact-list-tab` label. **Footgun:** `DigestOfIcon*` must be the real sha256 of the PNG bytes or the lookup silently misses and falls through to NotFound.

**Lenses.** Merged from 4 overlapping ideas (decoded name, icons/badge, identity-from-extra_attrs, identity header). UX (weaken/survive->fixed): the always-on dimmed storage path and digest-title/breadcrumb inconsistency; fixed by hiding the storage path, applying the resolver to title+breadcrumb+list uniformly, and suppressing the same fields from the raw Overview dump. Cloudsmith (weaken): table-stakes accepted as the baseline that unblocks everything else, resolved fully (no garbage anywhere). Overengineering (weaken->fixed): the persisted `display_name`+`format` column/migration is disproportionate; fixed by presentation-layer decode + `extra_attrs` fallback, dropping schema/API.

**Cloudsmith.** "Hide storage-layer names" parity. Not a differentiator alone; it is the precondition for #1/#3/#5.

---

## 3. Per-ecosystem Install/Usage ADDITIONS tab — Set-Me-Up (Adopt now, M)

**Problem.** No setup half: a developer must already know `.npmrc`/`settings.xml` registry config + auth wiring. Even a correct `npm install` fails 401 with no in-product fix.

```
ADDITIONS  [ Usage ] [ Versions ] [ Files ]
 npm:
  1. .npmrc:  @harbor:registry=https://host/<project>/npm/
              //host/<project>/npm/:_authToken=${TOKEN}    [copy]
  2. npm install @harbor/multi-format-demo@0.3.1                    [copy]
 maven (only for maven artifacts):
  settings.xml <server> + pom.xml <repository> + <dependency>  [copy each]
```

**Plug-in point.** Frontend-only computed tab in `artifact-additions.component.ts` fed by `artifact.extra_attrs` (already loaded) + existing `registryUrl` input. Do NOT add a `SET_ME_UP` `processor.AbstractAddition` / `GET /additions` path — content is client-computed, not blob-extracted. Set `hasPullCommand=false` for npm/maven so install lives in exactly one surface.

**Lenses.** Merged from "Set Me Up panel" + "Per-ecosystem Install tab". UX (weaken->fixed): in-tab token dropdown + inline robot creation fights the one-time-secret modal pattern and per-artifact placement is wrong altitude; fixed by package-altitude placement, static text, `${TOKEN}` placeholder linking to the existing robot/view-token flow, type-scoped rendering. Cloudsmith (weaken->fixed): token injection is the real differentiator and Harbor robot secrets are once-only/unrecoverable; fixed by minting a fresh least-privilege project+ecosystem-scoped robot on demand (Harbor RBAC is the edge Cloudsmith lacks) and emitting the maven `<server>` credentials half. Overengineering (weaken->fixed): backend addition is dead weight; fixed to pure frontend, L->M.

**Cloudsmith.** This is the headline parity-plus surface. The least-privilege-robot-per-project+ecosystem provisioning is something Cloudsmith cannot do because it lacks Harbor's project Casbin RBAC.

---

## 4. Native coordinate + channel badge as version identity (Adopt now, S)

**Problem.** Inside a package the row title is a sliced digest; maven 1.4.2 reads as `sha256:9f...`. dist-tags (`latest`/`next`) and RELEASE/SNAPSHOT are invisible. The escaped `1.2.3_x2dbeta.4` tag shows raw.

```
com.acme:widget2
  widget2  2.4.0  [release][latest]   3 files   PASS    2h ago
  widget2  2.5.0-SNAPSHOT [snapshot]   3 files   2 High  1d ago
@harbor/multi-format-demo
  3.1.0 [latest] PASS    3.2.0 [next][beta] 1 Med
```

**Plug-in point.** `artifact-list-tab.component.html` column-one cell: when `artifact.type` in {NPM,MAVEN} render `extra_attrs.version` as the clickable title (digest -> tooltip); dist-tag/RELEASE-SNAPSHOT as a `clr-label` badge beside it. Do NOT add a parallel Version column or touch the `hiddenArray`/sort-comparator contract.

**Lenses.** Merged from "Native coordinate as identity" + "Channel-aware version table". UX (weaken->fixed): a new Version/Channel column duplicates the Tags column, breaks the global datagrid column/sort contract, and pollutes OCI rows; fixed by decoded version in column one + inline badge gated on type. Cloudsmith (weaken->fixed): table-stakes and Signed column empty for npm/maven; fixed by dropping empty trust columns here (trust = #6). Overengineering (survive): pure display binding over transmitted `extra_attrs`.

**Cloudsmith.** The OCI-free spine the differentiated surfaces hang on. Demote digest to copy-on-hover so the version list carries zero storage identity.

---

## 5. Search-by-native-name (Adopt now, M)

**Problem.** Repository search filters the escaped path, so typing `com.acme:widget2` matches nothing. Discovery by the name developers know is impossible.

```
Search: com.acme:widget
  [mvn] com.acme:widget2      7 versions
  [mvn] com.acme:widget-core  2 versions
```

**Plug-in point.** `GET /projects/{name}/repositories` already accepts `q`; frontend passes it unchanged. Cheapest correct path: server-side `naming.EscapeComponent(query)` matched against the indexed `repository.name`. Richer: `FilterByNativeName` against `multi_format_package.native_name` (index `idx_multi_format_package_proj_fmt` verified) mirroring the existing `RepoRecord.FilterByBlobDigest` raw-SQL subquery. No new endpoint/schema.

**Lenses.** Merged from "Search-by-package-name" + "Native-name search". UX (weaken->fixed): match-without-decoded-display reads as a bug and per-row contract divergence confuses; fixed by hard-depending on #2 and OR-ing native_name with the OCI name in one query (single contract). Cloudsmith (weaken->fixed): table-stakes; fixed by reframing as a parity prerequisite + adding format-spanning project-level search. Overengineering (survive): reuses `q` + existing keyword-filter pattern + indexed column.

**Cloudsmith.** Parity floor. The format-spanning unified search (one ranked list across npm+maven+future) is the wedge Nexus/Artifactory under-do.

---

## 6. Inline trust chips + npm channel resolution (Adopt later, M)

**Problem.** Harbor computes scan/SBOM/signature per artifact but never frames it as "is the version `latest` resolves to actually safe?".

```
@harbor/multi-format-demo  channel resolution
  latest -> 3.1.0   [scan PASS][sbom][unsigned]
  next   -> 3.2.0   [scan 1 High][sbom][signed]
```

**Plug-in point.** Frontend-only: `artifact-list-tab.component.ts` already hydrates `scan_overview`/`sbom_overview`/`signed` and the VULNERABILITIES/SBOMS deep-links exist. npm channel resolution lights up from `extra_attrs["dist-tags"]` only.

**Lenses.** UX (weaken->fixed): chips 1:1 duplicate existing co-signed/vul/sbom columns; fixed by dropping the chip vocabulary, keeping only the channel->version resolution and a PASS->High drift alert as the differentiator. Cloudsmith (weaken->fixed): channel resolution only exists for npm (maven has no pointer in extra_attrs); scoped npm-only. Overengineering (survive): pure derived render; channel pivot confined to npm.

**Cloudsmith.** Per-version verdict is table-stakes; the drift alert ("latest flipped to an unsigned/newly-High version") is what peers don't surface. Adopt-later: weakest delta once #4's badge ships; depends on #4.

---

## 7. Maven GAV Files tab (Adopt later, M)

**Problem.** A GAV is a fileset (jar/sources/javadoc/pom + checksums + SNAPSHOT variants). The processor abstracts the full `[]FileRef`; the UI shows none of it.

```
ADDITIONS [ Usage ] [ Files ] [ Dependencies ]
 com.acme:widget2 1.4.0  RELEASE
  widget2-1.4.0.jar          jar  842 kB  9f3c..  [copy]
  widget2-1.4.0-sources.jar  jar  201 kB  1a7d..  sources [copy]
  widget2-1.4.0.pom          pom  2.1 kB  88be..  [copy]
```

**Plug-in point.** Backend: `AdditionTypeFiles` on maven `Processor.ListAdditionTypes` + `AbstractAddition` (chart.go precedent) returning `extra_attrs["files"]`; served via existing `GET /additions/files` (no swagger change). Frontend: `ADDITIONS.FILES` + `files.component` already wired for charts/cnai.

**Lenses.** UX (survive->fixed): existing files.component is a name+size tree (cnai), so "reuse" understates work; fixed by one columnar renderer (migrate cnai) + snapshot as inline badge. Cloudsmith (survive): table-stakes Nexus/Artifactory ship but Harbor shows zero today; pair with a differentiating surface. Overengineering (survive): intended additions seam, data in extra_attrs, no API/DB/migration.

**Cloudsmith.** Parity with Nexus/Artifactory checksum tabs. Don't let it carry competitive weight alone.

---

## 8. Project Packages view: grouping + filter chips in the existing list (Adopt later, M)

**Problem.** Flat repo list, no per-ecosystem grouping or counts, no "show only Maven".

```
Repositories  [Group by ecosystem]  ( All )( npm 18 )( maven 7 )
  npm
    @acme/widgets        40 versions  latest 4.1.0
  maven
    com.acme:widget2     12 versions  latest 2.3.1
```

**Plug-in point.** Fold into the EXISTING Repositories tab (`repository-gridview.component`) as a "Group by ecosystem" toggle + format chips — NOT a new tab. Grouping from the `format` path-segment / `multi_format_package.format` facet (indexed). Reuses `GET /projects/{name}/repositories`. Readable rows depend on #2.

**Lenses.** Merged from two near-identical ideas. UX (weaken->fixed): a sibling Packages tab re-renders the same payload ("which tab?") and inflates an already-overflowing 10-12 tab bar; fixed by a grouping toggle on the existing tab (one authoritative surface). Cloudsmith (survive/weaken->fixed): table-stakes clone; fixed with an inline per-package scan/security column (peers lack it) and ecosystem-aware LATEST (npm dist-tag vs maven release). Overengineering (survive): no new endpoint/table; version-count/latest columns gated on #2/#4, not N+1 fan-out.

**Cloudsmith.** Parity floor that unblocks the native surfaces. Adopt-later: depends on #2.

---

## 9. npm README + Versions/dist-tags additions (Adopt later, M)

**Problem.** npm devs expect the `npm view` packument; dist-tags render as an opaque JSON blob; no README.

```
ADDITIONS [ Usage ] [ Versions ] [ README ]
 Versions:  latest->0.3.1   next->0.4.0-rc1   (all: 0.4.0-rc1, 0.3.1, 0.3.0)
 README:    rendered markdown; tab hidden if absent
```

**Plug-in point.** README: reuse `ADDITIONS.SUMMARY='readme.md'` + `summary.component` (chart/cnai precedent), fall back to `extra_attrs.description`, gate on non-empty. Versions: `AbstractAddition` on npm processor reading `dao.LoadState` (`PackageState.Versions+DistTags`) via an injected lookup interface (not a direct processor->DAO coupling), served through `GET /additions`.

**Lenses.** Merged from "packument tab" + "README tab". UX (weaken->fixed): the all-versions table duplicates the version grid and README often duplicates the install snippet / renders empty; fixed by keeping only dist-tags here, inline README only when non-trivial, tab suppressed when absent. Cloudsmith (weaken->fixed): table-stakes and npm readme lives in the packument not per-version package.json; fixed by sourcing/stamping readme correctly and gating on non-empty. Overengineering (survive/weaken): reuses additions + summary; the per-artifact/per-package mismatch handled by injecting LoadState.

**Cloudsmith.** Pure parity. Lowest differentiation + data-availability risk -> last of the adopts.

---

## 10. Deprecation/yank banner (Adopt later, S)

**Problem.** Deprecation/yank is a first-class trust signal with no warning today.

```
@harbor/multi-format-demo @ 3.0.0
  !! DEPRECATED -- use >=3.2.0  ("security fix") -- by ci-bot
version row:  3.0.0 ~~strike~~ [deprecated]  1 Crit
```

**Plug-in point.** Reuse the EXISTING PG-authoritative `multi_format_version.yanked` (verified present) / `model.Yanked` / `AnnYanked` / mapper. Render as a Clarity `inline-alert` at the top of `artifact-summary` + row strikethrough/badge. Optional long message via one new per-version annotation served as an addition. NO new `deprecated` extra_attr.

**Lenses.** UX (weaken->fixed): a click-gated additions tab defeats a pre-pull warning; fixed by always-visible inline-alert + row strikethrough. Cloudsmith (weaken->fixed): peers own the write path and Harbor has no npm-deprecate handler; scoped initial release to display of existing yank state, write-path as a separate increment. Overengineering (weaken->fixed, decisive): the premise "no field exists" is wrong — `multi_format_version.yanked` already plumbs this and a write-time extra_attr would race the post-publish mutation; fixed by reading the existing PG fact, effort -> S.

**Cloudsmith.** Parity; full value needs the deprecate write-path (out of scope).

---

## 11. Format Descriptor Registry / GET /formats (Reject, L)

**Why rejected.** A parallel format registry duplicates the source-of-truth already in the self-registering `processor.Register` plugins, must be kept in sync or drifts, and introduces an async render-time dependency (flicker / transient `docker-pull`/blob-icon flash) for zero user-visible value. A flat `UsageTpl` string also re-creates the one-size-fits-all anti-pattern this effort exists to escape. The overengineering refuter's minimal_fix is literally "don't build this — extend the existing Processor interface with `IconDigest()`/`UsageCommand()` and iterate `processor.Registry`," i.e. reject the idea as formulated. Every adopted idea reuses its own existing seam without needing this backbone. (UX: weaken, invisible + regression risk; Cloudsmith: survive but non-differentiating.)

## 12. Channel pointer history / audit strip (Reject, L)

**Why rejected.** Verified `dao.SetMutableState` does `UPDATE ... SET mutable_state=?` — it overwrites the dist-tags blob in place. There is no append-only event row, no prior target, no per-move timestamp, no actor anywhere in Harbor. The mockup's "moved 3.1.0->3.2.0 by ci-bot" has zero backing data. Delivering it demands a brand-new append-only PG table + write-path plumbing on every re-point + actor capture the SetDistTag signature doesn't carry — exactly the new-backend/DB/schema the overengineering lens forbids (verdict: kill, confirmed against code). The refuter's minimal_fix (show only the current dist-tag->version mapping + existing trust state) collapses it entirely into idea #6, so nothing remains as a standalone idea. Reject and absorb the salvageable sliver into #6.