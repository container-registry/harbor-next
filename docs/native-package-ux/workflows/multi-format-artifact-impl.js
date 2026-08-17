export const meta = {
  name: 'multi-format-artifact-impl',
  description: 'Implement npm+maven native package protocols in Harbor (multiformat) per the locked design, with self-build gates and adversarial critics',
  phases: [
    { title: 'Core', detail: 'backend domain: model/dao/naming/store/core/mapper/controller/uivisibility + migration' },
    { title: 'Edges', detail: 'npm & maven HTTP adapters, multiformatauth middleware, artifact processors (parallel)' },
    { title: 'Wire', detail: 'shared-file edits: csrf/tx skippers + RegisterRoutes; full build green' },
    { title: 'Review', detail: 'adversarial critics: correctness, over-complication, Harbor-idiom' },
    { title: 'Fix', detail: 'address confirmed critical findings; rebuild green' },
  ],
}

const HARBOR = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support'
const SRC = `${HARBOR}/src`
const REF = '/Users/vadim/Development/container-registry/multi-format-artifact-via-oci/multi-format-oci'
const DESIGN = '/private/tmp/claude-501/-Users-vadim-Development-container-registry-harbor-multi-format-artifact-support/04411ee4-0e60-431f-8215-71b3120ec963/scratchpad/DESIGN.md'

const COMMON = `
You are implementing a Harbor feature: native npm + Maven package protocols over Harbor's OCI backend ("multiformat").
This is a PORT of a proven standalone Go POC into Harbor as a product feature.

AUTHORITATIVE DESIGN (read it FIRST, in full): ${DESIGN}
REFERENCE POC to port from (real code on disk): ${REF}
HARBOR TREE you write into: ${HARBOR}  (Go module root is ${SRC}, module github.com/goharbor/harbor/src)

HARD RULES:
- Logging ONLY via github.com/goharbor/harbor/src/lib/log (log.Infof/Warningf/Errorf/Debugf). NEVER fmt.Printf or log.Printf.
- Errors via github.com/goharbor/harbor/src/lib/errors where Harbor code does.
- DB access via github.com/goharbor/harbor/src/lib/orm (orm.FromContext / orm.WithTransaction). NOT pgx.
- OCI store access via the global registry client github.com/goharbor/harbor/src/pkg/registry (registry.Cli). Map multi-format-oci store.Store methods to registry.Cli per DESIGN §3.
- Per-version manifest MUST be media type v1.MediaTypeImageManifest (github.com/opencontainers/image-spec/specs-go/v1); the _index MUST be v1.MediaTypeImageIndex. Otherwise Harbor's abstractor rejects it.
- Match surrounding Harbor code style. Comments sparingly (WHY not WHAT). No AI attribution.
- Port the proven multi-format-oci logic FAITHFULLY; do not redesign algorithms. Rewrite only the seams (store, db, routing, logging) to Harbor idioms.
- When you import multi-format-oci packages, rewrite import paths github.com/8gears/multi-format-oci/internal/... -> the Harbor target package paths in DESIGN §2.
`

const FILE_REPORT_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['files_created','files_edited','build_command','build_passed','build_output_tail','notes','api_surface'],
  properties: {
    files_created: { type: 'array', items: { type: 'string' } },
    files_edited: { type: 'array', items: { type: 'string' } },
    build_command: { type: 'string' },
    build_passed: { type: 'boolean' },
    build_output_tail: { type: 'string', description: 'last ~30 lines of the build/test output' },
    notes: { type: 'string', description: 'deviations from design, gaps, anything the next agent or integrator must know' },
    api_surface: { type: 'string', description: 'the EXACT public types/functions/signatures you exported that other components import (package path + signatures), so downstream agents code against reality not guesses' },
  },
}

// ---------- Phase 1: CORE (alone — fixes the type-coupled public API on disk) ----------
phase('Core')
const core = await agent(`${COMMON}

YOUR SCOPE — the entire backend domain (DESIGN waves 0a-0e, 1a-1b, 2a-2e). Build ALL of this so it compiles together:
1. Migration ${HARBOR}/make/migrations/postgresql/0181_2.15.0_schema.up.sql per DESIGN §4 (up-only).
2. ${SRC}/pkg/multiformat/model/model.go  — port internal/core/model/model.go verbatim (PackageState, Version, FileRef).
3. ${SRC}/pkg/multiformat/naming/naming.go + naming_test.go — port internal/core/naming, but EncodeRepo MUST emit grammar-legal lowercase repo components matching [a-z0-9]+([._-][a-z0-9]+)* (DESIGN §1 codec change). Add TestEncodeRepoGrammar.
4. ${SRC}/controller/multiformat/semver/ and mavenver/ — port internal/core/{semver,mavenver} VERBATIM with their tests.
5. ${SRC}/controller/multiformat/const.go — media types. CRITICAL: parameterize the per-version config media type PER FORMAT: npm = "application/vnd.harbor.npm.config.v1+json", maven = "application/vnd.harbor.maven.config.v1+json" (exact literals — processors register on these). Maven file layer media type, _index media type, etc.
6. ${SRC}/controller/multiformat/store.go — store seam over registry.Cli per DESIGN §3.
7. ${SRC}/pkg/multiformat/dao/dao.go — DAO over lib/orm per DESIGN §4 (UpsertVersion, SetMutableState, SetYanked, LoadState, ProjVersion, ListPackageNames, WipeAll) + advisory-lock helper (pg_advisory_lock(hashtext($1)) on a held connection).
8. ${SRC}/controller/multiformat/index_ops.go — port internal/core/index_ops.go (RMW, canonicalIndex) onto the store seam.
9. ${SRC}/controller/multiformat/mapper.go — port internal/core/{core.go,mapper.go} (Publish/SetDistTag/SetYanked/LoadState/PayloadBlob). project_id replaces multi-format-oci namespace. The config media type pushed in each per-version manifest MUST be the FORMAT-SPECIFIC one from const.go.
10. ${SRC}/controller/multiformat/mapper_maven.go — port internal/core/mapper_maven.go (PublishFile multi-file GAV, SNAPSHOT mutability, derived checksums, MavenFileBlob).
11. ${SRC}/controller/multiformat/uivisibility.go — NEW glue ensureVisible(...) per DESIGN §5: blob accounting via blob controller (github.com/goharbor/harbor/src/controller/blob), then repository.Ctl.Ensure, then artifact.Ctl.Ensure. Study ${SRC}/server/middleware/blob/put_manifest.go and ${SRC}/server/registry/manifest.go:putManifest for the EXACT blob.Ctl / repository.Ctl / artifact.Ctl calls and signatures — verify by reading them, do not guess method names.
12. ${SRC}/controller/multiformat/controller.go — Controller interface + constructor wiring store+dao+mapper, and invoking ensureVisible after each per-version manifest push (NEVER for _index). Expose the public methods the HTTP adapters need (Publish, PublishFile, LoadState, SetDistTag, SetYanked, PayloadBlob, MavenFileBlob, ListPackages).
13. The shared adapter-facing Deps/Format interface: port internal/format/format.go to ${SRC}/server/registry/multiformat/format.go (package multiformat) — this is what npm/maven adapters import. Provide a constructor that builds a format.Deps from the controller (so adapters stay close to the multi-format-oci originals).

VERIFY before returning: cd ${SRC} && go build ./controller/multiformat/... ./pkg/multiformat/... ./server/registry/multiformat/... && go test ./controller/multiformat/semver/... ./controller/multiformat/mavenver/... ./pkg/multiformat/naming/...
Iterate until the build passes. Report the EXACT exported API surface (signatures) in api_surface — downstream agents depend on it.`,
  { label: 'core-backend', phase: 'Core', schema: FILE_REPORT_SCHEMA, effort: 'high' })

// ---------- Phase 2: EDGES (parallel; adapters read CORE's real API from disk) ----------
phase('Edges')
const coreAPI = core ? core.api_surface : '(core agent returned null — read the code under src/controller/multiformat and src/server/registry/multiformat directly)'
const edgeTasks = [
  { key: 'npm-adapter', prompt: `YOUR SCOPE — the npm HTTP adapter (DESIGN wave 3a). Port ${REF}/internal/format/npm/npm.go to ${SRC}/server/registry/npm/{route.go,handler.go} (package npm). Adapt: strip TWO leading path segments (prefix + <project>) and thread the project through to the controller; use a CATCH-ALL route for scoped @scope/name names (mergeslash + %2f decoding — DESIGN §1); call the multiformat controller via the format.Deps built in src/server/registry/multiformat. HTTP errors/logging via Harbor idioms. Add a Register(controller) entrypoint that builds Deps and mounts onto the router. Include tests for @scope/name publish+packument path parsing.
VERIFY: cd ${SRC} && go build ./server/registry/npm/... && go vet ./server/registry/npm/...` },
  { key: 'maven-adapter', prompt: `YOUR SCOPE — the maven HTTP adapter (DESIGN wave 3b, §6). Port ${REF}/internal/format/maven/maven.go to ${SRC}/server/registry/maven/{route.go,handler.go} (package maven). Per-file PUT/GET routing under /maven/<project>/<g/with/slashes>/<a>/<v>/<file>; synthesize maven-metadata.xml on GET (GA + SNAPSHOT-GAV) with deterministic <lastUpdated>; derive .sha1/.md5/.sha256/.sha512 over served bytes; accept-and-discard client sidecars + metadata. Thread <project> to the controller's PublishFile. Add a Register(controller) entrypoint. Harbor errors/logging.
VERIFY: cd ${SRC} && go build ./server/registry/maven/... && go vet ./server/registry/maven/...` },
  { key: 'multiformatauth', prompt: `YOUR SCOPE — project RBAC middleware (DESIGN wave 0f, §7) at ${SRC}/server/middleware/multiformatauth/auth.go (package multiformatauth). Model on ${SRC}/server/middleware/v2auth/auth.go (read it). Parse project = first path segment after /npm or /maven; resolve pid via project controller; method→action (GET/HEAD=Pull, PUT/POST=Push, DELETE=Delete); securityCtx.Can on rbac_project.NewNamespace(pid).Resource(rbac.ResourceRepository); 401 WWW-Authenticate Basic on failure; allow anonymous pull on public projects; stash pid on request context for ensureVisible. Provide Middleware() returning the Harbor middleware type.
VERIFY: cd ${SRC} && go build ./server/middleware/multiformatauth/...` },
  { key: 'processors', prompt: `YOUR SCOPE — artifact type processors (DESIGN wave 1c/1d, §5) so packages show as NPM/MAVEN in the UI. Create ${SRC}/controller/artifact/processor/npm/npm.go and ${SRC}/controller/artifact/processor/maven/maven.go. Model on ${SRC}/controller/artifact/processor/wasm/wasm.go and chart/chart.go (read them). Each embeds base.NewManifestProcessor(); GetArtifactType returns "NPM" / "MAVEN"; register via processor.Register on the EXACT config media types "application/vnd.harbor.npm.config.v1+json" and "application/vnd.harbor.maven.config.v1+json"; AbstractMetadata parses the config blob JSON into artifact.ExtraAttrs (npm: name/version/dist-tags; maven: groupId/artifactId/version/files). Also wire icons OPTIONALLY: add DigestOfIconNPM/DigestOfIconMaven to ${SRC}/lib/icon/const.go and builtInIcons entries in ${SRC}/controller/icon/controller.go ONLY if you can do it cleanly (it is not gate-blocking; default icon is fine).
VERIFY: cd ${SRC} && go build ./controller/artifact/processor/... ./lib/icon/... ./controller/icon/...` },
]
const edges = await parallel(edgeTasks.map(t => () =>
  agent(`${COMMON}\n\nCORE's exported API surface (code against THIS; also read the real files under ${SRC}/controller/multiformat and ${SRC}/server/registry/multiformat):\n${coreAPI}\n\n${t.prompt}\nIterate until your build passes.`,
    { label: t.key, phase: 'Edges', schema: FILE_REPORT_SCHEMA, effort: 'high' })
))

// ---------- Phase 3: WIRE (sequential shared-file edits + full build) ----------
phase('Wire')
const wire = await agent(`${COMMON}

YOUR SCOPE — final shared-file wiring (DESIGN wave 4) + make the WHOLE module build. You run LAST; all other components exist on disk now.
1. ${SRC}/core/middlewares/middlewares.go — add "/npm" and "/maven" to csrfSkipper; add "/npm" and "/maven" (PUT/POST/DELETE) to dbTxSkippers. Read the file to match the exact skipper construction patterns.
2. ${SRC}/server/server.go RegisterRoutes() — mount the npm and maven adapters (sibling to registry.RegisterRoutes()), wrapping each route group with the multiformatauth middleware. Construct the multiformat controller and pass it to each adapter's Register(controller). Read server.go and the adapters' Register signatures to wire correctly.
3. Resolve ALL remaining compile errors across the module (import cycles, signature mismatches between core and edges, missing wiring). You may edit any multiformat file to make it cohere, but do NOT change the proven algorithm logic.

VERIFY (must pass before returning): cd ${SRC} && go build ./... 2>&1 | tail -40
Also run: cd ${SRC} && go vet ./controller/multiformat/... ./server/registry/npm/... ./server/registry/maven/... ./server/middleware/multiformatauth/...
Report the final build status honestly. If anything still fails, report exactly what and why in notes.`,
  { label: 'wire+build', phase: 'Wire', schema: FILE_REPORT_SCHEMA, effort: 'high' })

// ---------- Phase 4: REVIEW (parallel adversarial critics, read-only) ----------
phase('Review')
const CRIT_SCHEMA = { type:'object', additionalProperties:false, required:['findings','overall'], properties:{
  findings:{ type:'array', items:{ type:'object', additionalProperties:false, required:['severity','file','issue','fix'], properties:{
    severity:{ type:'string', enum:['blocker','major','minor'] },
    file:{ type:'string' }, issue:{ type:'string' }, fix:{ type:'string' } } } },
  overall:{ type:'string' } } }
const CRIT_BASE = `${COMMON}\n\nThe implementation now exists across ${SRC}/controller/multiformat, ${SRC}/pkg/multiformat, ${SRC}/server/registry/{npm,maven,multiformat}, ${SRC}/server/middleware/multiformatauth, ${SRC}/controller/artifact/processor/{npm,maven}, and the wave-4 edits to middlewares.go + server.go. Review it (READ-ONLY, do not edit). Verify claims against the actual code and the multi-format-oci reference.`
const critics = await parallel([
  { key:'correctness', lens:`CORRECTNESS vs real clients + Harbor APIs. Would a real "npm publish"/"npm install" and "mvn deploy:deploy-file"/"mvn dependency:get" actually work end to end? Check: scoped-name routing, the per-version manifest is v1.MediaTypeImageManifest and _index is v1.MediaTypeImageIndex, config blob pushed before artifact.Ctl.Ensure (else abstractor fails), blob accounting actually replicated, repository.Ctl.Ensure before artifact.Ctl.Ensure, csrfSkipper+dbTxSkippers actually edited, advisory-lock RMW ordering, derived maven checksums, EncodeRepo grammar legality. Flag anything that would 4xx/5xx or make packages NOT appear in the UI.` },
  { key:'overcomplication', lens:`OVER-COMPLICATION / dead code / needless abstraction for the stated gate (push + visible in UI). Flag layers, options, params, or files that add complexity without serving the gate. Flag copy-paste that drifted from the multi-format-oci original in a way that adds risk. Recommend deletions/simplifications. Do NOT recommend adding features.` },
  { key:'harbor-idiom', lens:`HARBOR IDIOM + layering. Does it use lib/log (no fmt.Printf), lib/orm, lib/errors, the router/middleware patterns, the processor registration pattern correctly? Are controller vs pkg vs server layers respected (no server importing pkg/dao directly bypassing controller; no import cycles)? Is the migration file named/structured like its siblings? Flag deviations.` },
].map(c => () => agent(`${CRIT_BASE}\n\nREVIEW LENS — ${c.key}: ${c.lens}`, { label:`critic:${c.key}`, phase:'Review', schema:CRIT_SCHEMA, effort:'high' })))

const allFindings = critics.filter(Boolean).flatMap(c => c.findings || [])
const blockers = allFindings.filter(f => f.severity === 'blocker' || f.severity === 'major')

// ---------- Phase 5: FIX (address confirmed blocking/major findings; rebuild green) ----------
phase('Fix')
let fix = null
if (blockers.length) {
  fix = await agent(`${COMMON}

The adversarial critics found these BLOCKER/MAJOR issues. Fix the ones that are genuinely real (verify each against the code first; ignore false positives, and say which you rejected and why). Make minimal, faithful edits. Do NOT introduce new abstractions.

FINDINGS:
${JSON.stringify(blockers, null, 2)}

After fixing, VERIFY: cd ${SRC} && go build ./... 2>&1 | tail -40  (must pass) and re-run the unit tests under ./controller/multiformat/... ./pkg/multiformat/...
Report what you fixed, what you rejected, and the final build status.`,
    { label:'fix', phase:'Fix', schema: FILE_REPORT_SCHEMA, effort:'high' })
} else {
  log('No blocker/major findings — skipping fix phase.')
}

return {
  core, edges: edges.filter(Boolean), wire,
  critics: critics.filter(Boolean),
  blocker_count: blockers.length,
  fix,
  final_build_passed: fix ? fix.build_passed : (wire ? wire.build_passed : false),
}
