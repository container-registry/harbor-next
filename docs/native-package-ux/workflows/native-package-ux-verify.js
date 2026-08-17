export const meta = {
  name: 'native-package-ux-verify',
  description: 'Independent spec-only verification: one verifier per task, each sees ONLY its spec, checks the running dev env + code for conformance',
  phases: [ { title: 'Verify', detail: '10 independent verifiers, one per task spec' } ],
}
const SPECS = '/private/tmp/claude-501/-Users-vadim-Development-container-registry-harbor-multi-format-artifact-support/04411ee4-0e60-431f-8215-71b3120ec963/scratchpad/specs'
const SRC = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support/src'

const TASKS = [
  {rank:1, spec:'01-per-ecosystem-usage-emitter-kill-docker-pull-for.md'},
  {rank:2, spec:'02-readable-native-identity-everywhere-decode-displ.md'},
  {rank:3, spec:'03-per-ecosystem-install-usage-additions-tab-full-s.md'},
  {rank:4, spec:'04-native-coordinate-as-version-list-primary-identi.md'},
  {rank:5, spec:'05-search-by-native-name-query-the-readable-coordin.md'},
  {rank:6, spec:'06-inline-trust-chips-per-version-reuse-scan-sbom-s.md'},
  {rank:7, spec:'07-maven-gav-files-tab-classifiers-checksums-snapsh.md'},
  {rank:8, spec:'08-project-level-packages-view-ecosystem-grouped-pe.md'},
  {rank:9, spec:'09-npm-readme-versions-packument-additions-dist-tag.md'},
  {rank:10, spec:'10-deprecation-yank-banner-from-existing-multi-format-versi.md'},
]
const VERDICT = {
  type:'object', additionalProperties:false,
  required:['rank','verdict','criteria','gaps','evidence_summary'],
  properties:{
    rank:{type:'number'},
    verdict:{type:'string', enum:['pass','partial','fail']},
    criteria:{type:'array', items:{type:'object', additionalProperties:false, required:['criterion','status','evidence'],
      properties:{ criterion:{type:'string'}, status:{type:'string',enum:['pass','partial','fail']}, evidence:{type:'string',description:'concrete proof: API response excerpt, file:line, or command output - NOT an assertion'} }}},
    gaps:{type:'array', items:{type:'string'}, description:'specific spec requirements not met (empty if pass)'},
    evidence_summary:{type:'string'},
  },
}

phase('Verify')
const ENV = `
You are an INDEPENDENT VERIFIER. Your ONLY source of truth for what this task must do is the single spec
file given below — do NOT read any other spec, the board, or implementation notes. Verify whether the
ACTUAL running system + code conform to that spec. Gather EVIDENCE (do not assume, do not trust comments):
- Codebase (read-only): ${SRC} (Go backend) and ${SRC}/portal (Angular). Cite file:line.
- Running dev env: Harbor Core http://localhost:8180 (Basic auth admin / Harbor12345), portal http://localhost:4300.
  Stable readable test artifacts you can probe: npm "library/npm/harbor-multi-format-demo" and maven
  "library/maven/com/acme/widget2" (maven versions 1.0 and 2.0). Use curl against the REST API
  (/api/v2.0/...) and the native routes (/npm/library/..., /maven/library/...). The artifact additions
  API is GET /api/v2.0/projects/library/repositories/{repo}/artifacts/{digest-or-tag}/additions/{type}
  (repo path is URL-encoded, '/'→%2F). Inspect artifact.extra_attrs and addition_links on the artifact GET.
For UI/portal requirements, verify the relevant Angular component code implements the spec AND that the
backing data/endpoint it consumes returns the right shape (that is sufficient evidence; you need not drive a browser).
Judge each acceptance criterion in the spec's verification/goal section: pass / partial / fail, each with
concrete evidence. Overall verdict = fail if any core criterion fails, partial if minor gaps, pass if all met.
Be skeptical and specific. If a feature is dormant by design (e.g. a trigger action doesn't exist yet),
verify the surfacing logic is correct and say so explicitly rather than failing it.
`
const verdicts = await parallel(TASKS.map(t => () =>
  agent(`${ENV}\n\nYOUR SPEC (the ONLY spec you may read): ${SPECS}/${t.spec}\nVerify task #${t.rank}. Set rank=${t.rank}.`,
    { label:`verify:#${t.rank}`, phase:'Verify', schema:VERDICT, effort:'high' })
))
return verdicts.filter(Boolean)
