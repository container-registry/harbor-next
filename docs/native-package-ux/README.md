# Native Package UX — design record & orchestration prompts

Process artifacts for the npm + Maven "native package experience" work on the
`multi-format-artifact-support` branch. This is documentation of *how* the feature was
designed and built (a multi-agent workflow), not runtime code.

## Contents

- `plan-ux-idea-workflow.md` — the approved plan for the idea-generation workflow
  (generate → adversarially refute → product decision board → greenlight → specs).
- `DESIGN.md` — the implementation design brief for porting the `multi-format-oci` POC
  (npm + Maven over an OCI backend) into Harbor.
- `decision-board.md` — the product decision board: 12 ranked UX ideas (10 adopted,
  2 rejected), each with mockup, the refutations it survived (UI/UX, Cloudsmith,
  over-engineering), and an adopt/later/reject recommendation.
- `specs/` — per-idea implementation specs (files to touch, data source, dev-env
  verification). `specs/INDEX.md` is the index. (#2 and #5 were revised in place
  when the naming approach changed from presentation-decode to storage-tree.)
- `workflows/` — the orchestration scripts ("prompts") that drove each phase:
  - `multi-format-artifact-design.js` — judge-panel design of the Harbor integration.
  - `multi-format-artifact-impl.js` — initial multiformat backend implementation.
  - `native-package-ux.js` — UX idea generation + 3-lens adversarial refutation + board.
  - `native-package-ux-specs.js` — per-idea spec drafting.
  - `native-package-ux-impl.js` — first feature-implementation attempt (presentation-decode approach).
  - `native-package-ux-features.js` — feature implementation on the storage-tree base.
  - `native-package-ux-verify.js` / `native-package-ux-reverify.js` — independent,
    spec-only verification of each task.

## Notes

- Absolute paths inside these files point at the session scratchpad and won't
  resolve from the repo; they are preserved as-authored for the record.
- The Cloudsmith UI/UX comparison (screenshots + `COMPARISON.md`) was produced in
  the same effort but lives outside the repo (it referenced a private comparison repo).
- The implemented result lives in the normal source tree (`src/pkg/multiformat`,
  `src/controller/multiformat`, `src/server/registry/{npm,maven}`,
  `src/controller/artifact/processor/{npm,maven}`, and the portal artifact components).
