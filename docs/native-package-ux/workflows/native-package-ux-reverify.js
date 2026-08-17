export const meta = {
  name: 'native-package-ux-reverify',
  description: 'Re-verify the 3 non-pass tasks (#1 spec reconciled, #5/#8 backend fixed) with independent spec-only agents',
  phases: [ { title: 'Reverify', detail: 'independent verifiers for #1, #5, #8' } ],
}
const SPECS = '/private/tmp/claude-501/-Users-vadim-Development-container-registry-harbor-multi-format-artifact-support/04411ee4-0e60-431f-8215-71b3120ec963/scratchpad/specs'
const SRC = '/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support/src'
const TASKS = [
  {rank:1, spec:'01-per-ecosystem-usage-emitter-kill-docker-pull-for.md'},
  {rank:5, spec:'05-search-by-native-name-query-the-readable-coordin.md'},
  {rank:8, spec:'08-project-level-packages-view-ecosystem-grouped-pe.md'},
]
const VERDICT = { type:'object', additionalProperties:false, required:['rank','verdict','criteria','gaps','evidence_summary'],
  properties:{ rank:{type:'number'}, verdict:{type:'string',enum:['pass','partial','fail']},
    criteria:{type:'array',items:{type:'object',additionalProperties:false,required:['criterion','status','evidence'],
      properties:{criterion:{type:'string'},status:{type:'string',enum:['pass','partial','fail']},evidence:{type:'string'}}}},
    gaps:{type:'array',items:{type:'string'}}, evidence_summary:{type:'string'} } }
const ENV = `
You are an INDEPENDENT VERIFIER. Your ONLY source of truth for what this task must do is the single spec
file given below — do NOT read any other spec/board/impl notes. Verify the ACTUAL running system + code
conform, with concrete EVIDENCE (API output, file:line, command output — never assertions).
- Code (read-only): ${SRC} (Go) + ${SRC}/portal (Angular).
- Live dev env: Core http://localhost:8180 (admin/Harbor12345), portal http://localhost:4300. Rich readable
  test data exists: project "library" has maven/* (20 repos incl. org/springframework tree + com/acme/widget2),
  npm/* (11 incl. lodash, harbor-multi-format-demo), and docker (nginx, alpine). Repo search:
  GET /api/v2.0/projects/library/repositories?q=name=~<value> . additions API:
  GET /api/v2.0/projects/library/repositories/{repo}/artifacts/{ref}/additions/{type} (repo '/'→%2F).
For UI requirements verify the component code implements the spec AND the backing endpoint returns the right
data/shape (sufficient; no browser needed). Judge each acceptance criterion pass/partial/fail with evidence.
`
phase('Reverify')
const verdicts = await parallel(TASKS.map(t => () =>
  agent(`${ENV}\n\nYOUR SPEC (the ONLY spec you may read): ${SPECS}/${t.spec}\nVerify task #${t.rank}. Set rank=${t.rank}.`,
    { label:`reverify:#${t.rank}`, phase:'Reverify', schema:VERDICT, effort:'high' })
))
return verdicts.filter(Boolean)
