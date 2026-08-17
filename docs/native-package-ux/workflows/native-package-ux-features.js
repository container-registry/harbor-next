export const meta = {
  name: 'native-package-ux-features',
  description: 'Implement the 9 native-package UX features (icons, usage, additions tabs, version badge, trust, search, grouping, yank) on the storage-tree base',
  phases: [
    { title: 'Foundation', detail: 'backend (icons, maven Files + npm README/versions additions, search, yank surfacing) + FE enum/icons' },
    { title: 'Features', detail: 'list-tab, pull-command, additions tabs, gridview+summary (parallel, distinct files)' },
    { title: 'Verify', detail: 'go build + portal lint + portal build' },
    { title: 'Fix', detail: 'resolve build/lint failures' },
  ],
}
const HARBOR = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support'
const SRC = `${HARBOR}/src`
const PORTAL = `${SRC}/portal`
const SPECS = '/private/tmp/claude-501/-Users-vadim-Development-container-registry-harbor-multi-format-artifact-support/04411ee4-0e60-431f-8215-71b3120ec963/scratchpad/specs'

const COMMON = `
You implement native-package UX in Harbor (Go backend + Angular 16/Clarity portal). Detailed file-level
specs are on disk under ${SPECS}/ - READ your assigned spec(s) IN FULL and follow them; board context
(mockups) at ${SPECS}/../native-package-ux-board.md.

IMPORTANT BASE CHANGE (already committed): repository names are now a READABLE STORAGE TREE
(library/maven/org/springframework/boot/spring-boot-starter-test ; library/npm/lodash ; library/npm/<scope>/<name>).
So there is NO decode step and NO presentation-layer decode helper - the repo name and tag are already human
-readable everywhere. Ideas that were about "decoding the escaped name" collapse to just: per-ecosystem ICONS
+ reading extra_attrs. Do NOT add a decode helper.

GROUND TRUTH: artifact types are exactly "NPM" and "MAVEN". extra_attrs carries native identity (npm:
name/version/description/dist-tags; maven: groupId/artifactId/version/files[]) and is on the artifacts list
response. Seams: processors src/controller/artifact/processor/{npm,maven}; additions = ListAdditionTypes/
AbstractAddition + GET /additions/{type} (chart.go precedent); icons = src/lib/icon/const.go + src/controller/
icon/controller.go builtInIcons + src/controller/artifact/controller.go defaultIcons (PNG assets icons/npm.png
and icons/maven.png ALREADY EXIST in the repo - the DigestOfIcon* MUST equal the real sha256 of those PNG
bytes); portal artifact.ts (ArtifactType enum, pull-command), artifact-list-tab, artifact-additions
(models.ts ADDITIONS enum + components), artifact-summary, repository gridview; search via GET /projects/
{name}/repositories q; projection src/pkg/multiformat/dao (multi_format_version.yanked exists; dao.LoadState->PackageState).
HARD RULES: logging via src/lib/log (no fmt.Printf); swagger change => edit api/v2.0/swagger.yaml then run
"task build:gen-apis"; migrations only in make/migrations/postgresql; no AI attribution; match style; comments
sparingly; DO NOT git commit. Reconciliation: #1 keeps a SHORT one-liner in the pull-command widget; #3 puts
the FULL .npmrc/settings.xml setup in the Usage additions tab; both ship; keep hasPullCommand TRUE for npm/maven.
For the install tab, show the auth token as a labeled placeholder (linking to the existing robot/view-token flow),
never a live secret on a standing page.
`
const REPORT = { type:'object', additionalProperties:false,
  required:['specs_done','files_changed','build_passed','build_output_tail','notes'],
  properties:{ specs_done:{type:'array',items:{type:'string'}}, files_changed:{type:'array',items:{type:'string'}},
    build_passed:{type:'boolean'}, build_output_tail:{type:'string'},
    notes:{type:'string',description:'deviations, shared enum/type signatures other agents need, anything verify must know'} } }

phase('Foundation')
const foundation = await parallel([
  () => agent(`${COMMON}

YOU OWN THE GO BACKEND. Implement the backend portions of:
- ${SPECS}/02-readable-native-identity-everywhere-decode-displ.md  -> ONLY the ICONS part (npm/maven DigestOfIcon* in src/lib/icon/const.go = real sha256 of the existing icons/{npm,maven}.png; builtInIcons in src/controller/icon/controller.go; defaultIcons in src/controller/artifact/controller.go). SKIP all decode/naming work (storage-tree already handles readability).
- ${SPECS}/07-maven-gav-files-tab-classifiers-checksums-snapsh.md  (maven Processor AdditionTypeFiles: ListAdditionTypes + AbstractAddition returning extra_attrs files[])
- ${SPECS}/09-npm-readme-versions-packument-additions-dist-tag.md  (npm Processor README + versions additions; needs api/v2.0/swagger.yaml + task build:gen-apis)
- ${SPECS}/05-search-by-native-name-query-the-readable-coordin.md  (with readable names search is largely free - VERIFY q against GET /projects/{name}/repositories matches the readable repo path; add a filter only if needed)
- ${SPECS}/10-deprecation-yank-banner-from-existing-multi-format-versi.md  (BACKEND: make multi_format_version.yanked reach the UI via extra_attrs/annotation/addition)
VERIFY: cd ${SRC} && go build ./... 2>&1 | tail -30 ; golangci-lint run on changed packages. Iterate to green.`,
    { label:'backend', phase:'Foundation', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}

YOU OWN THE FRONTEND FOUNDATION (shared pieces feature agents depend on). Minimal now that names are readable:
- Add NPM and MAVEN to the ArtifactType enum in src/portal/.../artifact/artifact.ts (exact 'NPM','MAVEN').
- Ensure NPM/MAVEN rows show their per-ecosystem ICON (icon service fetches by digest; make the type map through;
  backend ships the digests). Confirm the readable repo name already renders (it does - storage-tree); just don't break it.
Do NOT add a decode helper, do NOT edit pull-command/additions/version-row (other agents own those).
VERIFY: cd ${PORTAL} && bun run lint_fix (scope your files). Report in notes: the exact ArtifactType enum members + any shared icon mapping path feature agents should reuse.`,
    { label:'fe-foundation', phase:'Foundation', schema:REPORT, effort:'high' }),
])
const beNote=(foundation[0]&&foundation[0].notes)||'(read backend changes on disk)'
const feNote=(foundation[1]&&foundation[1].notes)||'(read artifact.ts on disk)'

phase('Features')
const SHARED=`\n\nFoundation landed. ArtifactType.NPM/MAVEN exist; icons wired. Names are readable (no decode). Foundation notes:\nFE: ${feNote}\nBE: ${beNote}\nRead artifact.ts + the changed backend on disk for real signatures.`
const features = await parallel([
  () => agent(`${COMMON}${SHARED}\n\nYOU OWN src/portal/.../artifact-list-tab/* (artifact rows). Implement:\n- ${SPECS}/04-native-coordinate-as-version-list-primary-identi.md (render extra_attrs.version as the NPM/MAVEN row title, digest to tooltip; dist-tag / RELEASE-SNAPSHOT clr-label badge; no parallel column, don't touch sort/hiddenArray)\n- ${SPECS}/06-inline-trust-chips-per-version-reuse-scan-sbom-s.md (inline scan/SBOM/signature chips reusing already-hydrated fields; npm channel-resolution from extra_attrs dist-tags)\nVERIFY: cd ${PORTAL} && bun run lint_fix on your files.`,
    { label:'feat:list-tab', phase:'Features', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}${SHARED}\n\nYOU OWN the pull-command widget + the pull-command section of artifact.ts. Implement ${SPECS}/01-per-ecosystem-usage-emitter-kill-docker-pull-for.md: extend hasPullCommand + hasPullCommandForTag for NPM/MAVEN; add getNpmInstallCommand (npm install <name>@<version> from extra_attrs) + getMavenDependencySnippet (one-liner "mvn dependency:get -Dartifact=g:a:v"); render hbr-copy-input blocks in pull-command.component.html for both dropdown modes; keep hasPullCommand TRUE; i18n keys mirrored to all lang files; update pull-command.component.spec.ts. Read the enum from artifact.ts (foundation added it); do not redefine it.\nVERIFY: cd ${PORTAL} && bun run lint_fix on your files; bun run test --include='**/pull-command.component.spec.ts' --browsers=ChromeHeadless.`,
    { label:'feat:pull-command', phase:'Features', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}${SHARED}\n\nYOU OWN src/portal/.../artifact-additions/* (models.ts ADDITIONS enum + components). Implement the FRONTEND of:\n- ${SPECS}/03-per-ecosystem-install-usage-additions-tab-full-s.md (computed Usage/Install tab from extra_attrs + registryUrl: npm .npmrc + npm install; maven settings.xml server + pom repository + dependency; type-scoped; auth token as a labeled placeholder linking to robot/view-token flow)\n- ${SPECS}/07-maven-gav-files-tab-classifiers-checksums-snapsh.md (FRONTEND: ADDITIONS.FILES maven columnar variant; backend shipped by foundation)\n- ${SPECS}/09-npm-readme-versions-packument-additions-dist-tag.md (FRONTEND: README via SUMMARY + versions tab; backend + swagger shipped by foundation - use regenerated client)\nCoordinate all three ADDITIONS enum + tab additions in this one agent to avoid conflicts.\nVERIFY: cd ${PORTAL} && bun run lint_fix on your files.`,
    { label:'feat:additions', phase:'Features', schema:REPORT, effort:'high' }),
  () => agent(`${COMMON}${SHARED}\n\nYOU OWN repository-gridview + artifact-summary components. Implement:\n- ${SPECS}/05-search-by-native-name-query-the-readable-coordin.md (FRONTEND: ensure the repo search box reaches the backend; results show the readable names - which they now do)\n- ${SPECS}/08-project-level-packages-view-ecosystem-grouped-pe.md (a "Group by ecosystem" toggle + per-format filter chips with counts folded into the EXISTING repositories gridview, not a new tab; ecosystem = 2nd path segment of repo name)\n- ${SPECS}/10-deprecation-yank-banner-from-existing-multi-format-versi.md (FRONTEND: Clarity inline-alert banner atop artifact-summary when yanked + [deprecated] badge; data from the field foundation surfaced)\nVERIFY: cd ${PORTAL} && bun run lint_fix on your files.`,
    { label:'feat:gridview+summary', phase:'Features', schema:REPORT, effort:'high' }),
])

phase('Verify')
const verify = await agent(`${COMMON}\n\nAll features implemented across backend + portal. Make the WHOLE project compile + lint clean:\n1. cd ${SRC} && go build ./... 2>&1 | tail -40 (ensure gen-apis output present if swagger changed)\n2. cd ${PORTAL} && bun run lint_fix 2>&1 | tail -40 (resolve remaining; remove any console.log - no-console lint)\n3. cd ${PORTAL} && bun run build 2>&1 | tail -40 (production compile = the real frontend type-check)\nFix failures (import paths, enum mismatches, leftover console.log) without changing feature behavior. Report precisely what failed and what you fixed.`,
  { label:'verify+build', phase:'Verify', schema:REPORT, effort:'high' })

phase('Fix')
let fix=null
if (verify && !verify.build_passed) {
  fix = await agent(`${COMMON}\n\nVerify reports the build is NOT clean:\n${verify.build_output_tail}\nNOTES: ${verify.notes}\nFix remaining go build / portal lint / portal build failures minimally; don't change behavior. VERIFY: cd ${SRC} && go build ./... ; cd ${PORTAL} && bun run build. Report final status.`,
    { label:'fix', phase:'Fix', schema:REPORT, effort:'high' })
}
return { foundation:foundation.filter(Boolean), features:features.filter(Boolean), verify, fix,
  final_build_passed: fix? fix.build_passed : (verify? verify.build_passed : false) }
