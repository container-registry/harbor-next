export const meta = {
  name: 'native-package-ux-specs',
  description: 'Draft an implementation spec for each greenlit native-package UX idea (files to touch + dev-env verification); no code changes',
  phases: [ { title: 'Specs', detail: 'one spec-drafter per idea, reading real files' } ],
}

const HARBOR = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support'
const SRC = `${HARBOR}/src`
const BOARD = '/private/tmp/claude-501/-Users-vadim-Development-container-registry-harbor-multi-format-artifact-support/04411ee4-0e60-431f-8215-71b3120ec963/scratchpad/native-package-ux-board.md'

const GROUNDING = `
Harbor now serves npm + Maven ("multiformat"); goal is native package UX. Key facts/seams (verify by reading the real files under ${SRC}):
- Naming: src/pkg/multiformat/naming/naming.go (EncodeRepo => mcom_x2e... escaping; check whether a Decode exists or must be added; codec is _xNN per-byte, injective except >96-char sha256 fallback). repo name = <project>/<format>/<encoded-name>.
- Processors: src/controller/artifact/processor/{npm,maven}/*.go populate extra_attrs (npm: name/version/description/dist-tags; maven: groupId/artifactId/version/files[]). Processor.ListAdditionTypes + AbstractAddition is the additions seam (chart.go is the precedent); GET /additions/{type} in src/server/v2.0/handler/artifact.go.
- Icons: src/lib/icon/const.go (DigestOf*), src/controller/icon/controller.go (builtInIcons -> ./icons/*.png), src/controller/artifact/controller.go defaultIcons keyed on art.Type.
- Portal: src/portal/src/app/base/project/repository/... artifact-list-tab (list + type icon), pull-command/pull-command.component.ts + artifact.ts (hasPullCommand/getPullCommandBy*), artifact-additions/{artifact-additions.component.ts,models.ts ADDITIONS enum, summary.component, files.component}, artifact-summary.component.ts, gridview/repository-gridview.component.
- Projection: src/pkg/multiformat/dao (multi_format_package.native_name+format indexed; multi_format_version.yanked exists; dao.LoadState -> PackageState with Versions/DistTags).
- Dev env for verification: SLOT=1 (Core http://localhost:8180, portal http://localhost:4300, admin/Harbor12345). Existing test data: project library has library/npm/mharbor_x2dmulti_x2dformat_x2ddemo (npm harbor-multi-format-demo@1.0.0) and library/maven/mcom_x2eacme_x3awidget2 (com.acme:widget2:1.0).
Constraints: API-first (swagger.yaml then task build:gen-apis) for any REST change; migrations in make/migrations/postgresql; logging via src/lib/log; frontend lint/test before build. Prefer reusing existing seams over new API/schema.
`

const SPEC_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['summary','depends_on','data_source','backend_changes','frontend_changes','new_api_or_schema','verification','risks','spec_markdown'],
  properties: {
    summary: { type: 'string' },
    depends_on: { type: 'string', description: 'other idea ranks this depends on, or "none"' },
    data_source: { type: 'string', description: 'where the displayed data comes from (extra_attrs field, multi_format_package/multi_format_version column, dao.LoadState, etc.)' },
    backend_changes: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['file','change'], properties: { file:{type:'string'}, change:{type:'string'} } } },
    frontend_changes: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['file','change'], properties: { file:{type:'string'}, change:{type:'string'} } } },
    new_api_or_schema: { type: 'string', description: 'any swagger/migration change required, or "none"' },
    verification: { type: 'string', description: 'concrete end-to-end steps on the SLOT=1 dev env that prove it works (commands + what to see in the browser)' },
    risks: { type: 'array', items: { type: 'string' } },
    spec_markdown: { type: 'string', description: 'the full implementation spec as a self-contained markdown doc' },
  },
}

phase('Specs')
const ideas = [
  {rank:1,effort:'S',title:'Per-ecosystem usage emitter: kill docker pull for npm/maven',surface:'Setup & Usage - pull-command widget',plugin_point:'src/portal/.../artifact/artifact.ts hasPullCommand/getPullCommandByTag/getPullCommandByDigest (existing IMAGE/CNAB/CHART type-switch; CHART emits non-docker helm pull); rendered by pull-command.component.ts. Add NPM/MAVEN enum + cases reading extra_attrs.'},
  {rank:2,effort:'M',title:'Readable native identity everywhere (decode display name + format badge/icons), storage path hidden',surface:'Naming & Organization',plugin_point:'Presentation-layer decode: format from 2nd path segment of r.name, decode 3rd via naming.Decode (lossless _xNN; may need adding), fall back to extra_attrs name/groupId/artifactId. Icons: src/lib/icon/const.go + src/controller/icon/controller.go builtInIcons + src/controller/artifact/controller.go defaultIcons. Surfaces: repository-gridview.component.html, artifact-summary, artifact-list-tab.'},
  {rank:3,effort:'M',title:'Per-ecosystem Install/Usage ADDITIONS tab (full setup snippet, scoped to artifact type)',surface:'Package Landing & Detail (ADDITIONS seam)',plugin_point:'Frontend-only computed tab in artifact-additions.component.ts fed by artifact.extra_attrs + registryUrl input; no processor.AbstractAddition/GET path. For npm/maven set hasPullCommand=false.'},
  {rank:4,effort:'S',title:'Native coordinate as version-list primary identity + channel/dist-tag badge',surface:'Version management & trust',plugin_point:'artifact-list-tab.component.html column-one: when artifact.type in {NPM,MAVEN} render extra_attrs.version as clickable title (digest to tooltip); append dist-tag/RELEASE-SNAPSHOT Clarity clr-label. No parallel Version column, no hiddenArray/sort change.'},
  {rank:5,effort:'M',title:'Search-by-native-name (query the readable coordinate, not the escaped path)',surface:'Discovery / Search',plugin_point:'GET /projects/{name}/repositories accepts q (server/v2.0/handler/repository.go BuildQuery). Cheapest: server-side naming.EscapeComponent over the query, match indexed repository.name. Richer: match multi_format_package.native_name via FilterByNativeName mirroring RepoRecord.FilterByBlobDigest raw-SQL. No new endpoint/schema.'},
  {rank:6,effort:'M',title:'Inline trust chips per version (reuse scan/SBOM/signature)',surface:'Version management & trust',plugin_point:'Frontend-only: artifact-list-tab.component.ts hydrates scan_overview/sbom_overview/signed; ADDITIONS.VULNERABILITIES/SBOMS deep-links exist. npm channel-resolution from extra_attrs dist-tags.'},
  {rank:7,effort:'M',title:'Maven GAV Files tab (classifiers/checksums/snapshot-vs-release) via additions seam',surface:'Package Landing & Detail (ADDITIONS seam)',plugin_point:'Backend: AdditionTypeFiles on maven Processor.ListAdditionTypes + AbstractAddition (chart.go precedent) returning extra_attrs files []FileRef via GET /additions/files (no swagger change). Frontend: ADDITIONS.FILES + files.component exist; add maven columnar variant.'},
  {rank:8,effort:'M',title:'Project-level Packages view: ecosystem-grouped, per-format filter chips with counts',surface:'Discovery / Organization',plugin_point:'Fold grouping into EXISTING Repositories tab (repository-gridview.component) as Group-by-ecosystem toggle + format filter chips, not a new tab. Group from format path-segment / multi_format_package.format facet. Reuses GET /projects/{name}/repositories. Depends on #2.'},
  {rank:9,effort:'M',title:'npm README + Versions/packument additions (dist-tags + cross-version, from PG PackageState)',surface:'Package Landing & Detail (ADDITIONS seam)',plugin_point:'README: reuse ADDITIONS.SUMMARY=readme.md + summary.component; npm processor extracts readme from config blob, fall back to extra_attrs.description. Versions: AbstractAddition on npm processor reading dao.LoadState (PackageState.Versions+DistTags) via injected lookup, served via GET /additions.'},
  {rank:10,effort:'S',title:'Deprecation / yank banner from existing multi_format_version.yanked fact',surface:'Version management & trust',plugin_point:'Reuse PG-authoritative Yanked (multi_format_version.yanked, model.Yanked/AnnYanked/mapper). Clarity inline-alert at top of artifact-summary + strikethrough/[deprecated] badge in version row. Optional long message via one per-version annotation as an addition. No new boolean extra_attr/annotation.'},
]
const specs = await parallel(ideas.map(idea => () =>
  agent(`${GROUNDING}\n\nDraft a precise IMPLEMENTATION SPEC for this greenlit UX idea. Read the actual files named in plugin_point (and adjacent ones) to make the spec concrete and correct — confirm signatures/enums/columns exist before citing them; if a cited thing (e.g. naming.Decode) does NOT exist yet, say so and spec adding it. Full board context is at ${BOARD} if you need the mockup/rationale.\n\nIDEA #${idea.rank} (${idea.effort}): ${idea.title}\nSurface: ${idea.surface}\nProposed plug-in point: ${idea.plugin_point}\n\nThe spec must name exact files to touch, the data source, any (ideally zero) API/schema change, and a concrete end-to-end verification on the SLOT=1 dev env using the existing library/npm and library/maven test packages. Keep it faithful to the idea and minimal. Do NOT write code; this is a spec.`,
    { label: `spec:#${idea.rank}`, phase: 'Specs', schema: SPEC_SCHEMA, effort: 'high' })
))

return ideas.map((idea, i) => ({ rank: idea.rank, title: idea.title, effort: idea.effort, spec: specs[i] }))
