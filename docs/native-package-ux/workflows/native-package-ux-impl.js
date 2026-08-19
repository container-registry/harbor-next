export const meta = {
  name: 'native-package-ux-impl',
  description: 'Implement all 10 greenlit native-package UX specs (backend + Angular portal), file-ownership sequenced, build/lint gated',
  phases: [
    { title: 'Foundation', detail: 'backend (icons, additions, search, naming.Decode, yank surfacing) + FE enum/decode/icon display' },
    { title: 'Features', detail: 'list-tab, pull-command, additions tabs, gridview+summary (parallel, distinct files)' },
    { title: 'Verify', detail: 'go build + portal lint/build' },
    { title: 'Fix', detail: 'resolve any build/lint failures' },
  ],
}

const HARBOR = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support'
const SRC = `${HARBOR}/src`
const PORTAL = `${SRC}/portal`
const SPECS = '/private/tmp/claude-501/-Users-vadim-Development-container-registry-harbor-multi-format-artifact-support/04411ee4-0e60-431f-8215-71b3120ec963/scratchpad/specs'

const COMMON = `
You implement native-package UX in Harbor (Go backend + Angular 16 / Clarity portal). The detailed,
file-level specs already exist on disk under ${SPECS}/ — READ your assigned spec file(s) IN FULL and
follow them; they name exact files, data sources, and reuse existing seams. Board context (mockups) is
at ${SPECS}/../native-package-ux-board.md.

GROUND TRUTH:
- npm/maven artifact types are the exact strings "NPM" and "MAVEN". extra_attrs already carries native
  identity (npm: name/version/description/dist-tags; maven: groupId/artifactId/version/files[]). The repo
  name is the ESCAPED storage path (npm/mharbor_x2d..., maven/mcom_x2e...) - never build native commands
  from it; use extra_attrs.
- Seams: src/controller/artifact/processor/{npm,maven}; additions = processor.ListAdditionTypes/
  AbstractAddition + GET /additions/{type}; icons = src/lib/icon/const.go + src/controller/icon/controller.go
  + src/controller/artifact/controller.go defaultIcons; portal artifact.ts (ArtifactType enum, pull cmd),
  artifact-list-tab, artifact-additions (models.ts ADDITIONS enum + components), artifact-summary,
  repository gridview; search via GET /projects/{name}/repositories q (server/v2.0/handler/repository.go);
  projection src/pkg/multiformat/dao (multi_format_package.native_name+format indexed; multi_format_version.yanked).
HARD RULES: logging via src/lib/log (no fmt.Printf); API-first - any swagger change => edit
api/v2.0/swagger.yaml then run "task build:gen-apis" (regens server + portal client); migrations only in
make/migrations/postgresql; no AI attribution; match surrounding style; comments sparingly. DO NOT git
commit. Reconciliation already decided: #1 keeps a SHORT one-liner in the pull-command widget (npm install
name@ver / mvn dependency:get); #3 puts the FULL .npmrc/settings.xml setup in the Usage additions tab.
Both ship; do not set hasPullCommand=false.
`
const REPORT = {
  type:'object', additionalProperties:false,
  required:['specs_done','files_changed','build_passed','build_output_tail','notes'],
  properties:{
    specs_done:{type:'array',items:{type:'string'}},
    files_changed:{type:'array',items:{type:'string'}},
    build_passed:{type:'boolean'},
    build_output_tail:{type:'string'},
    notes:{type:'string',description:'deviations, conflicts resolved, anything the verify/fix step or other agents must know (esp. shared enums/types)'},
  },
}

// ---------- Phase 1: Foundation (backend + FE enum/decode) ----------
phase('Foundation')
const foundation = await parallel([
  () => agent(`${COMMON}

YOU OWN THE GO BACKEND for these specs. Read and implement the backend portions of:
- ${SPECS}/02-readable-native-identity-everywhere-decode-displ.md  (icons: npm/maven DigestOfIcon* in src/lib/icon/const.go + builtInIcons in src/controller/icon/controller.go + defaultIcons in src/controller/artifact/controller.go + ship icons/npm.png & icons/maven.png; AND naming.Decode in src/pkg/multiformat/naming/naming.go if the spec calls for it - lossless _xNN reverse + fallback)
- ${SPECS}/05-search-by-native-name-query-the-readable-coordin.md  (search filter)
- ${SPECS}/07-maven-gav-files-tab-classifiers-checksums-snapsh.md  (maven Processor AdditionTypeFiles: ListAdditionTypes + AbstractAddition returning extra_attrs files)
- ${SPECS}/09-npm-readme-versions-packument-additions-dist-tag.md  (npm Processor README + versions additions; this likely needs api/v2.0/swagger.yaml + task build:gen-apis)
- ${SPECS}/10-deprecation-yank-banner-from-existing-multi-format-versi.md  (BACKEND surfacing only: make multi_format_version.yanked reach the UI via extra_attrs/annotation/addition as the spec dictates)
If any spec's backend portion is purely frontend, note it and skip. Run "task build:gen-apis" if you touch swagger.
VERIFY before returning: cd ${SRC} && go build ./... 2>&1 | tail -30 ; then golangci-lint run on the packages you changed. Iterate until green. For icon PNGs: the DigestOfIcon* MUST equal the real sha256 of the PNG bytes you ship, or icon lookup silently misses - compute it.`,
    { label:'backend', phase:'Foundation', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}

YOU OWN THE FRONTEND FOUNDATION (spec #2 frontend) that the feature agents depend on. Read
${SPECS}/02-readable-native-identity-everywhere-decode-displ.md and implement ONLY these shared pieces:
- Add NPM and MAVEN to the ArtifactType enum in src/portal/.../artifact/artifact.ts (exact strings 'NPM','MAVEN').
- Add a reusable, lossless presentation-layer DECODE helper (TS) that turns an escaped repo name into the
  readable native identity (maven group dots/tree -> com.acme:widget2; npm scope/name), with a fallback to
  extra_attrs (name / groupId:artifactId) for the rare sha256 case. Put it where other components can import it.
- Wire per-ecosystem ICON display so NPM/MAVEN rows show their icon (the icon service already fetches by digest;
  ensure the type maps through).
- Apply the readable name in the repository gridview row label, the artifact-summary breadcrumb/header, and the
  artifact-list-tab repo label (the spec lists these surfaces). Hide the escaped storage path (tooltip/details only).
Do NOT edit pull-command, artifact-additions internals, or version-row rendering - other agents own those;
just don't break them. Export the decode helper clearly (report its import path in notes).
VERIFY: cd ${PORTAL} && bun run lint_fix (scope your files) and bunx tsc --noEmit if feasible. Report the decode helper's exported signature + import path in notes (feature agents need it).`,
    { label:'fe-foundation', phase:'Foundation', schema:REPORT, effort:'high' }),
])
const feNote = (foundation[1] && foundation[1].notes) || '(see src/portal artifact.ts + the new decode helper on disk)'
const beNote = (foundation[0] && foundation[0].notes) || '(see backend changes on disk)'

// ---------- Phase 2: Features (parallel, distinct component files) ----------
phase('Features')
const SHARED = `\n\nFE-FOUNDATION already landed: ArtifactType.NPM/MAVEN exist and a decode helper exists - REUSE it, do not redefine. Foundation notes:\n${feNote}\nBackend notes:\n${beNote}\nRead the decode helper + artifact.ts on disk to use the real signature.`
const features = await parallel([
  () => agent(`${COMMON}${SHARED}

YOU OWN src/portal/.../artifact-list-tab/* (the artifact rows). Implement, reusing the shared enum+decode:
- ${SPECS}/04-native-coordinate-as-version-list-primary-identi.md (render extra_attrs.version as the row title for NPM/MAVEN, digest to tooltip; dist-tag / RELEASE-SNAPSHOT Clarity clr-label badge; no parallel column, don't touch sort/hiddenArray contract)
- ${SPECS}/06-inline-trust-chips-per-version-reuse-scan-sbom-s.md (inline scan/SBOM/signature chips per version reusing already-hydrated fields; npm channel-resolution from extra_attrs dist-tags)
VERIFY: cd ${PORTAL} && bun run lint_fix on your files; run the artifact-list-tab spec if present.`,
    { label:'feat:list-tab', phase:'Features', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}${SHARED}

YOU OWN the pull-command widget + the pull-command section of artifact.ts. Implement:
- ${SPECS}/01-per-ecosystem-usage-emitter-kill-docker-pull-for.md (extend hasPullCommand + hasPullCommandForTag for NPM/MAVEN; add getNpmInstallCommand / getMavenDependencySnippet helpers reading extra_attrs; render hbr-copy-input blocks in pull-command.component.html for both dropdown modes; keep one-liner SHORT - mvn one-liner is "mvn dependency:get -Dartifact=g:a:v"). Keep hasPullCommand TRUE for npm/maven (reconciliation). Add i18n keys if needed, mirror to all lang files. Update pull-command.component.spec.ts.
Do NOT touch the ArtifactType enum (foundation owns it) beyond reading it.
VERIFY: cd ${PORTAL} && bun run lint_fix on your files; bun run test --include='**/pull-command.component.spec.ts' --browsers=ChromeHeadless.`,
    { label:'feat:pull-command', phase:'Features', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}${SHARED}

YOU OWN src/portal/.../artifact-additions/* (models.ts ADDITIONS enum + components). Implement the FRONTEND of:
- ${SPECS}/03-per-ecosystem-install-usage-additions-tab-full-s.md (a computed Usage/Install tab fed by extra_attrs + registryUrl: npm .npmrc + npm install; maven settings.xml <server> + pom <repository> + <dependency>; type-scoped; ${TOKEN} placeholder linking to the existing robot/view-token flow - no live secret on a standing page)
- ${SPECS}/07-maven-gav-files-tab-classifiers-checksums-snapsh.md (FRONTEND: ADDITIONS.FILES maven columnar variant; backend addition already shipped by foundation)
- ${SPECS}/09-npm-readme-versions-packument-additions-dist-tag.md (FRONTEND: README via SUMMARY + versions tab; backend additions + swagger already shipped by foundation - use the regenerated client)
All three add ADDITIONS enum entries + conditional tabs in artifact-additions.component - coordinate them in this one agent so they don't conflict.
VERIFY: cd ${PORTAL} && bun run lint_fix on your files.`,
    { label:'feat:additions', phase:'Features', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}${SHARED}

YOU OWN the repository gridview + artifact-summary components. Implement, reusing the shared decode helper:
- ${SPECS}/05-search-by-native-name-query-the-readable-coordin.md (FRONTEND: ensure the repo search box query reaches the backend filter foundation added; results show decoded names)
- ${SPECS}/08-project-level-packages-view-ecosystem-grouped-pe.md (a "Group by ecosystem" toggle + per-format filter chips with counts, folded into the EXISTING Repositories gridview, not a new tab)
- ${SPECS}/10-deprecation-yank-banner-from-existing-multi-format-versi.md (FRONTEND: Clarity inline-alert banner at top of artifact-summary when yanked + a strikethrough/[deprecated] badge; data from the field foundation surfaced)
VERIFY: cd ${PORTAL} && bun run lint_fix on your files.`,
    { label:'feat:gridview+summary', phase:'Features', schema:REPORT, effort:'high' }),
])

// ---------- Phase 3: Verify ----------
phase('Verify')
const verify = await agent(`${COMMON}

All 10 specs have been implemented across backend + portal. Make the WHOLE project compile and lint clean.
1. cd ${SRC} && go build ./... 2>&1 | tail -40  (must pass; if swagger/gen-apis was touched, ensure generated code is present)
2. cd ${PORTAL} && bun run lint_fix 2>&1 | tail -40  (auto-fix; remaining errors must be resolved)
3. cd ${PORTAL} && bun run build 2>&1 | tail -40  (production compile = the frontend gate; this is the real type-check across all changed components)
Resolve any failures you can (import paths, enum mismatches, decode-helper signature drift, leftover console.log
which the no-console lint forbids). Do not change feature behavior. Report exactly what failed and what you fixed;
if something still fails, report it precisely in notes.`,
  { label:'verify+build', phase:'Verify', schema:REPORT, effort:'high' })

// ---------- Phase 4: Fix (only if verify failed) ----------
phase('Fix')
let fix = null
if (verify && !verify.build_passed) {
  fix = await agent(`${COMMON}\n\nThe verify step reports the build is NOT clean:\n${verify.build_output_tail}\nNOTES: ${verify.notes}\n\nFix the remaining go build / portal lint / portal build failures with minimal changes; do not alter feature behavior. VERIFY: cd ${SRC} && go build ./... ; cd ${PORTAL} && bun run build. Report final status.`,
    { label:'fix', phase:'Fix', schema:REPORT, effort:'high' })
}

return {
  foundation: foundation.filter(Boolean),
  features: features.filter(Boolean),
  verify, fix,
  final_build_passed: fix ? fix.build_passed : (verify ? verify.build_passed : false),
}
