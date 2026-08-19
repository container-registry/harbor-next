export const meta = {
  name: 'multi-format-artifact-design',
  description: 'Judge-panel design for integrating multi-format-oci npm+maven native-protocol support into Harbor',
  phases: [
    { title: 'Propose', detail: '3 architects independently design the Harbor integration' },
    { title: 'Critique', detail: 'adversarial review of each proposal' },
    { title: 'Synthesize', detail: 'judge merges into one authoritative design + decomposition' },
  ],
}

const REF = '/Users/vadim/Development/container-registry/multi-format-artifact-via-oci/multi-format-oci'
const HARBOR = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support'

const GROUNDING = `
You are designing how to port a PROVEN standalone Go POC ("multi-format-oci") that serves npm and Maven
native package protocols over an OCI backend INTO the Harbor container registry as a product feature.

REFERENCE POC (read these): ${REF}
  - POC-SPEC-REV2.md (architecture: OCI is store not wire; per-package _index control artifact;
    Postgres = rebuildable projection; FS cache; per-version immutable manifests tagged by encoded version)
  - DESIGN-nuget-maven.md (Maven multi-file GAV model + PublishFile core seam; version comparators)
  - internal/format/npm/npm.go, internal/format/maven/maven.go (adapter logic to port)
  - internal/core/{core.go,mapper.go,mapper_maven.go,index_ops.go} (the OCI mapping core)
  - internal/core/{semver,mavenver}/ (version comparators)
  - internal/store/store.go (multi-format-oci's standalone OCI HTTP client — to be REPLACED by Harbor's registry.Cli)
  - internal/db/{db.go,schema.sql} (the PG projection — to be reimplemented on Harbor's orm/migrations)
  - internal/format/format.go (the Deps interface adapters depend on)

HARBOR TARGET (read these; this is where code lands): ${HARBOR}
  - src/server/server.go:RegisterRoutes() — where new /npm and /maven route prefixes register (alongside registry.RegisterRoutes for /v2)
  - src/server/registry/route.go — template for mounting a protocol with middleware + handlers
  - src/pkg/registry/client.go — registry.Cli: PushBlob/PushManifest(repo,ref,mediaType,payload)/PullBlob/PullManifest/ManifestExist/ListTags — the OCI store seam (arbitrary media types supported)
  - src/controller/artifact/controller.go (Ensure), src/controller/artifact/processor/{processor.go,chart/,cnab/,wasm/} — how artifacts get recorded + typed for the UI
  - src/lib/icon/const.go + src/controller/artifact/controller.go defaultIcons — artifact type icons
  - src/server/middleware/security/ + v2auth/auth.go — how project RBAC + security.Context work
  - make/migrations/postgresql/ — golang-migrate files (next NNNN_ sequence) for the projection tables
  - src/lib/orm, src/lib/log, src/lib/config — infra to use (NEVER fmt.Printf or log.Printf)

PROJECT CONSTRAINTS (hard): Taskfile not Make. OpenAPI swagger.yaml is REST API source of truth.
Logging only via src/lib/log. UI visibility is REQUIRED (packages must appear in Harbor's UI with a
sensible type label). Goal gate: maven AND npm packages can be PUSHED to Harbor and are VISIBLE in the UI.
`

const PROPOSAL_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['url_to_project_mapping','code_placement','store_seam','projection','ui_visibility','maven_multifile','auth','migration_plan','decomposition','risks','open_questions'],
  properties: {
    url_to_project_mapping: { type: 'string', description: 'Exact URL scheme for /npm and /maven and how the path maps to a Harbor project + repository + tag. Give concrete example URLs for npm publish/install and mvn deploy/get.' },
    code_placement: { type: 'string', description: 'Where ported multi-format-oci code lives in the Harbor tree (package paths). How much is reused verbatim vs rewritten to Harbor idioms.' },
    store_seam: { type: 'string', description: 'How the OCI store is backed by registry.Cli. Map each multi-format-oci store.Store method to a registry.Cli call. Note any gaps (e.g. tag listing, _index RMW concurrency).' },
    projection: { type: 'string', description: 'Postgres projection: tables, how accessed (orm vs pgx), reuse vs adapt multi-format-oci schema.sql. Where the per-package advisory lock + _index RMW commit point happens in Harbor.' },
    ui_visibility: { type: 'string', description: 'EXACT mechanism to make packages visible in UI: do we call artifact.Ctl.Ensure after pushing per-version manifests? New MAVEN/NPM processors + icons + type labels? Repository naming so they list under the project. This is the load-bearing requirement.' },
    maven_multifile: { type: 'string', description: 'How the Maven multi-file GAV PublishFile model is implemented against registry.Cli (multi-layer manifest, RMW across separate PUTs, SNAPSHOT mutability, derived checksums).' },
    auth: { type: 'string', description: 'How npm/maven client auth maps onto Harbor security.Context + project RBAC (push=write, pull=read). Basic auth / token.' },
    migration_plan: { type: 'string', description: 'Concrete migration file(s) under make/migrations/postgresql with the next sequence number and table DDL summary.' },
    decomposition: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['component','files','depends_on','parallelizable'], properties: { component:{type:'string'}, files:{type:'string'}, depends_on:{type:'string'}, parallelizable:{type:'boolean'} } }, description: 'Ordered implementation components with target files and dependencies, marking which can be built in parallel (distinct files) vs sequential (shared files like server.go).' },
    risks: { type: 'array', items: { type: 'string' } },
    open_questions: { type: 'array', items: { type: 'string' } },
  },
}

phase('Propose')
const ANGLES = [
  { key: 'reuse-max', lens: 'Maximize verbatim reuse of multi-format-oci code: vendor internal/format, internal/core, comparators as a Harbor package with the thinnest possible adapter shims for store (registry.Cli) and db (Harbor orm). Optimize for least new code and fastest path to the working gate.' },
  { key: 'harbor-idiom', lens: 'Maximize Harbor-idiomatic integration: rewrite multi-format-oci logic to follow Harbor controller/pkg/dao layering, lib/orm, lib/log, lib/errors, lib/cache; treat multi-format-oci as a design reference not a vendor. Optimize for long-term maintainability inside Harbor.' },
  { key: 'ui-first', lens: 'Design backward from the UI-visibility gate: start from how artifact.Ctl.Ensure + processors + the existing project/repository/artifact/tag UI render things, and derive the minimal push/projection design that makes npm and maven packages appear correctly with type labels. Be concrete about what the UI shows for a published npm package vs a maven GAV.' },
]
const proposals = await parallel(ANGLES.map(a => () =>
  agent(`${GROUNDING}\n\nDESIGN ANGLE — ${a.key}: ${a.lens}\n\nRead the referenced POC and Harbor files yourself (they are real, on disk). Produce a complete, concrete integration design for npm AND maven. Be specific with real Harbor file paths and registry.Cli/artifact.Ctl method names. Where you are unsure, verify by reading the code rather than guessing.`,
    { label: `propose:${a.key}`, phase: 'Propose', schema: PROPOSAL_SCHEMA, effort: 'high' })
))
const valid = proposals.filter(Boolean)

phase('Critique')
const CRIT_SCHEMA = { type:'object', additionalProperties:false, required:['fatal_flaws','correctness_gaps','overcomplication','missing_for_ui_gate','score'], properties:{
  fatal_flaws:{type:'array',items:{type:'string'}},
  correctness_gaps:{type:'array',items:{type:'string'},description:'Places the design would not actually work against real npm/mvn clients or real Harbor APIs'},
  overcomplication:{type:'array',items:{type:'string'},description:'Parts that are more complex than needed for the push+UI-visible gate'},
  missing_for_ui_gate:{type:'array',items:{type:'string'}},
  score:{type:'number',description:'0-10 likelihood this design reaches the working gate cleanly'},
}}
const critiques = await parallel(valid.map((p,i) => () =>
  agent(`${GROUNDING}\n\nAdversarially review this Harbor integration design proposal. Find where it would FAIL against a real npm/mvn client or real Harbor code, where it is over-engineered for the stated gate (push + visible in UI), and what it omits for UI visibility. Verify claims against the actual Harbor code on disk.\n\nPROPOSAL:\n${JSON.stringify(ANGLES[i] && {angle:ANGLES[i].key, design:p}, null, 2)}`,
    { label: `critique:${ANGLES[i].key}`, phase: 'Critique', schema: CRIT_SCHEMA, effort: 'high' })
))

phase('Synthesize')
const final = await agent(`${GROUNDING}\n\nYou are the lead architect. You have 3 independent design proposals and adversarial critiques of each. Synthesize ONE authoritative, concrete implementation design for porting npm + maven into Harbor, taking the best of each and avoiding the flaws the critics found. Bias toward: maximum reuse of the proven multi-format-oci adapter/comparator logic, the thinnest correct seams onto registry.Cli + Harbor Postgres, and a UI-visibility mechanism that definitely works (artifact.Ctl.Ensure + MAVEN/NPM processors + icons).\n\nPROPOSALS:\n${JSON.stringify(valid,null,2)}\n\nCRITIQUES:\n${JSON.stringify(critiques.filter(Boolean),null,2)}\n\nOutput a complete markdown design document. It MUST contain: (1) Final URL scheme + project/repo/tag mapping with example npm and mvn commands; (2) exact Harbor package paths for all new code; (3) the store seam mapping (multi-format-oci store method -> registry.Cli call); (4) the Postgres projection tables + the next migration file number to use (check make/migrations/postgresql/ for the highest existing NNNN); (5) the UI-visibility mechanism step by step; (6) the Maven multi-file model on registry.Cli; (7) auth/RBAC mapping; (8) an ORDERED, dependency-annotated implementation decomposition into components marking parallel-safe vs sequential (shared-file) work, suitable for handing to an implementation fleet; (9) the verification commands that prove the gate (build, and the real npm publish/install + mvn deploy/get round-trip). Return ONLY the markdown document.`,
  { label: 'synthesize', phase: 'Synthesize', effort: 'high' })

return { design: final, proposals: valid, critiques: critiques.filter(Boolean) }
