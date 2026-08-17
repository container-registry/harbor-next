export const meta = {
  name: 'native-package-ux',
  description: 'Generate, adversarially refute (UX/Cloudsmith/overengineering), and rank UX ideas to make npm+Maven feel native in Harbor; output a product decision board',
  phases: [
    { title: 'Generate', detail: '5 generators fan out package-native UX ideas across surfaces' },
    { title: 'Refute', detail: 'per idea: 3 lens-refuters (UX design / Cloudsmith / overengineering) try to kill it' },
    { title: 'Board', detail: 'adjudicate, dedupe, rank survivors, synthesize the decision board' },
  ],
}

const GROUNDING = `
CONTEXT: Harbor now serves npm + Maven packages ("multiformat"), verified end-to-end, but the END-USER
EXPERIENCE IS NOT NATIVE. Make npm/Maven feel native to developers (not like OCI images). Scope is
npm + Maven NOW, but every idea MUST generalize to future formats (pypi, cargo, nuget, ...).

CURRENT UX GAPS (real, observed on the dev env):
- Packages list as OCI repos with storage-layer names, e.g. "library/maven/mcom_x2eacme_x3awidget2"
  (escaped "com.acme:widget2") and "library/npm/mharbor_x2dmulti_x2dformat_x2ddemo". Unreadable.
- The push/pull widget emits "docker pull core:8080/library/npm/..." for npm AND maven (wrong client).
- The processors already populate rich extra_attrs (npm: name, version, description, dist-tags;
  maven: groupId, artifactId, version, files[]) but the UI DOES NOT DISPLAY any of it.
- No per-ecosystem setup/usage instructions, no version table, no project-level "Packages" view,
  no search-by-package-name, no ecosystem icons.

HARBOR PLUG-IN POINTS you MUST hook ideas into (cite the specific seam; do not invent subsystems):
1. Type label + icon: portal artifact-list-tab; backend defaultIcons (src/controller/artifact/controller.go);
   icons are path-based (src/controller/icon/controller.go + src/lib/icon/const.go); npm/maven icons not wired yet.
2. Pull/usage command: src/portal/.../pull-command/pull-command.component.ts + artifact.ts
   (hasPullCommand, getPullCommandByTag/Digest) branch on artifact.type; today emits "docker pull".
   This is the seam to emit "npm install"/maven <dependency> usage instead.
3. Per-ecosystem tab (Usage/Install, Files, Dependencies, README): the ADDITIONS mechanism -
   artifact-additions.component.ts + models.ts ADDITIONS enum (frontend); processor.ListAdditionTypes /
   AbstractAddition (src/controller/artifact/processor/{npm,maven}); GET /additions/{type}
   (src/server/v2.0/handler/artifact.go). Helm chart values/dependencies tab is the working precedent.
4. Metadata: extra_attrs is populated but unrendered; surfaced via artifact-summary.component.ts.
5. Organization: only a flat repository list today; no project-level Packages view / ecosystem grouping.
6. Naming: src/pkg/multiformat/naming/naming.go EncodeRepo produces the mcom_x2e... escaping. A readable
   tree (maven group dots -> path segments; npm scope/name) is grammar-legal and is what Maven/Nexus do.

COMPETITOR BENCHMARK (what "native" looks like elsewhere):
- Cloudsmith "Set Me Up": personalized, copy-paste setup snippets scoped to the package/repo (npm
  registry config + install; maven <repository>+<dependency> XML), with the user's token injected.
- Hide storage-layer names; show package by scope/group + name. Per-ecosystem version/dist-tag tables.
- Inline dependency + security/scan signals on the package page. Per-ecosystem icons/badges.
- ANTI-PATTERN to avoid: one-size-fits-all blob UI that shows "docker pull" for every ecosystem
  (GitHub Packages). Don't copy table-stakes as if it were differentiation.
`

const IDEAS_SCHEMA = {
  type: 'object', additionalProperties: false, required: ['ideas'],
  properties: { ideas: { type: 'array', items: {
    type: 'object', additionalProperties: false,
    required: ['title','surface','problem','mockup','plugin_point','generalizes','effort'],
    properties: {
      title: { type: 'string' },
      surface: { type: 'string', description: 'which UX surface this belongs to' },
      problem: { type: 'string', description: 'the concrete end-user problem it solves' },
      mockup: { type: 'string', description: 'a concrete ASCII UI mockup of the proposed experience' },
      plugin_point: { type: 'string', description: 'the EXACT Harbor seam/file it hooks (from the grounding inventory)' },
      generalizes: { type: 'string', description: 'how it extends to future formats (pypi/cargo/nuget)' },
      effort: { type: 'string', enum: ['S','M','L'] },
    } } } },
}

const VERDICT_SCHEMA = {
  type: 'object', additionalProperties: false, required: ['lens','verdict','objection','minimal_fix'],
  properties: {
    lens: { type: 'string' },
    verdict: { type: 'string', enum: ['kill','weaken','survive'] },
    objection: { type: 'string', description: 'the single strongest objection on this lens' },
    minimal_fix: { type: 'string', description: 'if weaken: the minimal change that would save the idea; else empty' },
  },
}

// ---------- Phase 1+2: generate, then refute each idea (pipeline) ----------
phase('Generate')
const GENERATORS = [
  { key: 'naming-org', focus: `NAMING & ORGANIZATION. Readable package identity instead of mcom_x2e... escaping (maven group->path tree, npm scope/name); a project-level "Packages" view and/or grouping/filtering repositories by ecosystem; ecosystem icons/badges so npm vs maven is visible at a glance. Cover both npm and maven.` },
  { key: 'setup-usage', focus: `SETUP & USAGE ("Set Me Up"). Per-package and per-repo copy-paste instructions: npm (registry config + npm install, scoped), maven (<repository> + <dependency> XML, settings.xml creds), with robot-token insertion. ALSO fix the pull-command widget that wrongly shows "docker pull" - emit the correct per-ecosystem usage. Cover both npm (npm dev persona) and maven (Java dev persona).` },
  { key: 'landing-detail', focus: `PACKAGE LANDING & DETAIL. Surface the already-populated extra_attrs: npm packument-style detail (name, description, dist-tags, version table, README), maven GAV detail (groupId/artifactId, files[]/classifiers, checksums, snapshot vs release). Use the additions tab + artifact-summary seams. Cover both ecosystems.` },
  { key: 'versions-trust', focus: `VERSION MANAGEMENT & TRUST. Version/dist-tag/channel tables (npm latest/next/beta; maven release vs SNAPSHOT, latest/release), deprecation/yank surfacing, and INLINE trust signals per package (scan results, SBOM, signature) reusing Harbor's existing scan/SBOM/signature surfaces. Cover both ecosystems.` },
  { key: 'discovery-extensible', focus: `DISCOVERY/SEARCH & EXTENSIBILITY. Search-by-package-name and ecosystem filtering; AND the cross-format EXTENSIBLE pattern - e.g. a per-format descriptor that generically drives icon + usage command + additions tabs so adding pypi/cargo later is config not bespoke UI. Cover both ecosystems and the future-format generalization explicitly.` },
]

const refuteIdea = (idea, gkey, idx) => {
  const LENSES = [
    { lens: 'ux-design', ask: `UX/UI DESIGN. Try to KILL this idea: is it confusing, redundant, inconsistent with Harbor's Clarity design language, does it add clicks/cognitive load, is the value unclear, does it duplicate something already in Harbor's UI? Default to a harsh verdict if value is marginal.` },
    { lens: 'cloudsmith-peers', ask: `EXISTING SOLUTIONS (Cloudsmith + JFrog/GitHub/Nexus/Verdaccio). Try to KILL this idea: does a competitor already do this better, is it mere table-stakes (not differentiating), or does it copy a known anti-pattern (e.g. one-size-fits-all blob UI)? Judge whether it actually closes a gap vs the benchmark.` },
    { lens: 'overengineering', ask: `ARCHITECTURAL OVERENGINEERING. Try to KILL this idea: is the effort disproportionate to the value? Does it demand a NEW backend/API/DB/schema when extra_attrs + the additions mechanism + naming codec already suffice? Does it fight Harbor's artifact/processor model or the OCI-as-store design?` },
  ]
  return parallel(LENSES.map(L => () =>
    agent(`${GROUNDING}\n\nREFUTE this single idea on ONE lens only.\nLENS - ${L.ask}\n\nIDEA:\n${JSON.stringify(idea, null, 2)}\n\nReturn your verdict. Set lens to "${L.lens}".`,
      { label: `refute:${L.lens}:${gkey}#${idx}`, phase: 'Refute', schema: VERDICT_SCHEMA, effort: 'high' })
  )).then(vs => ({ idea: { ...idea, generator: gkey }, verdicts: vs.filter(Boolean) }))
}

const perGenerator = await pipeline(
  GENERATORS,
  g => agent(`${GROUNDING}\n\nYOU ARE A UX GENERATOR for this surface:\n${g.focus}\n\nProduce 3-5 DISTINCT, concrete ideas. Each must hook a real Harbor plug-in point from the grounding (name the file/seam), include a concrete ASCII UI mockup, and state how it generalizes to future formats. Favor ideas that close a real gap vs the Cloudsmith benchmark over table-stakes. Be concrete, not generic.`,
    { label: `gen:${g.key}`, phase: 'Generate', schema: IDEAS_SCHEMA, effort: 'high' }),
  (genResult, g) => {
    const ideas = (genResult && genResult.ideas) || []
    return parallel(ideas.map((idea, i) => () => refuteIdea(idea, g.key, i)))
  }
)
const refuted = perGenerator.filter(Boolean).flat().filter(Boolean)
log(`Refuted ${refuted.length} ideas across ${GENERATORS.length} surfaces`)

// ---------- Phase 3: adjudicate + synthesize the decision board ----------
phase('Board')
const BOARD_SCHEMA = {
  type: 'object', additionalProperties: false, required: ['ranked_ideas','board_markdown'],
  properties: {
    ranked_ideas: { type: 'array', items: {
      type: 'object', additionalProperties: false,
      required: ['rank','title','surface','recommendation','score','effort','plugin_point','why','mockup','survived_lenses'],
      properties: {
        rank: { type: 'number' }, title: { type: 'string' }, surface: { type: 'string' },
        recommendation: { type: 'string', enum: ['Adopt now','Adopt later','Reject'] },
        score: { type: 'number', description: '0-100 composite' },
        effort: { type: 'string', enum: ['S','M','L'] },
        plugin_point: { type: 'string' },
        why: { type: 'string', description: 'one-line rationale incl. how it differentiates vs Cloudsmith' },
        mockup: { type: 'string' },
        survived_lenses: { type: 'string', description: 'how it answered/survived the UX, Cloudsmith, and overengineering refutations (or why rejected)' },
      } } },
    board_markdown: { type: 'string', description: 'the full product decision board as markdown' },
  },
}
const board = await agent(`${GROUNDING}

You are the product lead. Below are generated package-native UX ideas, each with three adversarial
verdicts (ux-design / cloudsmith-peers / overengineering). Adjudicate:
1. DEDUPE ideas that overlap across surfaces (merge into the strongest formulation).
2. DROP ideas killed by a fatal lens (verdict "kill") unless a refuter's minimal_fix clearly rescues it -
   in that case adopt the FIXED version and say so.
3. SCORE survivors 0-100: native-feel impact x feasibility (reuses an existing seam = higher) x
   differentiation vs Cloudsmith / overengineering risk. Effort S/M/L.
4. RANK and keep the top ~8-10. Assign each a recommendation: "Adopt now" (high impact, low/med effort,
   reuses a seam), "Adopt later", or "Reject" (include a few explicit rejects with reasons).
Ensure the naming-readability and per-ecosystem setup/usage ideas are present and explicitly judged
(adopted-with-fix or rejected-with-reason), since those are the two clearest gaps.

Then produce board_markdown: a "Product Decision Board" with (a) a one-screen ranking table
(rank | idea | surface | rec | score | effort), then (b) per idea: the user problem, the ASCII mockup,
the Harbor plug-in point (file), how it survived each of the 3 refutation lenses, the Cloudsmith
comparison, effort, and the recommendation.

IDEAS WITH VERDICTS:
${JSON.stringify(refuted, null, 2)}`,
  { label: 'adjudicate+board', phase: 'Board', effort: 'high', schema: BOARD_SCHEMA })

return board
