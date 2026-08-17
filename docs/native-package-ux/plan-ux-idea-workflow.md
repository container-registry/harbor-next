# Plan: Native-Package-UX idea workflow (generate → refute → decision board → greenlight → specs)

## Context
We shipped npm + Maven support ("multiformat") into Harbor and verified it end-to-end, but the
**end-user experience is not native**: packages surface as OCI repos with storage-layer names
(`library/maven/mcom_x2eacme_x3awidget2`), the push/pull widget emits `docker pull` for npm/maven,
the rich `extra_attrs` (npm name/dist-tags, Maven GAV/files) the processors already populate are
**not displayed**, and there is no per-ecosystem setup/usage, version, or discovery experience.

The goal of this task is NOT to write UI code yet. It is to **generate, adversarially vet, and rank
UX/UI ideas** that make npm/Maven feel native to developers, then bring the strongest ones to a
product decision board, greenlight a subset, and draft implementation specs for the approved ideas.

Decisions locked with the user: scope = **npm + Maven now, but each idea must generalize to future
formats**; depth = **thorough**; after the board, **greenlight interactively and draft specs** for
approved ideas.

## Grounding (verified this session — fed to every generator so ideas hook real seams)
Harbor portal plug-in points (cite in ideas):
- Type label + icon: `src/portal/.../artifact-list-tab/*`; backend `defaultIcons` in
  `src/controller/artifact/controller.go`; icons path-based in `src/controller/icon/controller.go`
  + `src/lib/icon/const.go` (npm/maven icons not yet wired).
- Pull/usage command: `src/portal/.../pull-command/pull-command.component.ts` + `artifact.ts`
  (`hasPullCommand`, `getPullCommandBy{Tag,Digest}`) — branches on `artifact.type`, emits
  `docker pull` today; the seam to emit `npm`/`mvn` usage.
- Per-ecosystem tab (Usage/Install, Files, Dependencies): the **additions** mechanism —
  `artifact-additions.component.ts` + `models.ts` `ADDITIONS` enum (frontend),
  `processor.ListAdditionTypes`/`AbstractAddition` (`src/controller/artifact/processor/{npm,maven}`),
  `GET /additions/{type}` (`src/server/v2.0/handler/artifact.go`). Helm chart values/deps is the precedent.
- Metadata: `extra_attrs` populated by the npm/maven processors but **not rendered**; surfaced via
  `artifact-summary.component.ts`.
- Organization: only a flat repository list today — no project-level "Packages" view, no ecosystem grouping.
- Naming: `src/pkg/multiformat/naming/naming.go` `EncodeRepo` (the `mcom_x2e…` escaping).

Competitor benchmark (refuter reference): Cloudsmith "Set Me Up" personalized copy-paste setup;
hide storage-layer names; per-ecosystem version/dist-tag tables; inline dependency/security signals;
avoid one-size-fits-all blob UI. (Artifactory Set-Me-Up, GitHub Packages anti-pattern, Verdaccio.)

## The workflow (run via the Workflow tool after approval)
Name: `native-package-ux`. Phases:

1. **Generate (fan-out).** Parallel generators over a matrix of UX **surfaces** × ecosystem
   **personas** (npm dev, Java/Maven dev). Surfaces: naming & organization; setup & usage
   ("Set Me Up"); package landing/detail (extra_attrs, version table, files, checksums, README);
   version management (dist-tags/channels, SNAPSHOT vs release, deprecate/yank); discovery & search
   (search-by-name, ecosystem filter, badges/icons); pull/usage-command correctness; inline
   trust/security (scan/SBOM/signature). ~10–11 generators. Each returns an **array of discrete
   ideas**; each idea (schema): title, surface, end-user problem, concrete **ASCII UI mockup**,
   the exact Harbor plug-in point it hooks (from Grounding), how it **generalizes to future formats**,
   rough effort. Generators are primed with the Grounding block + competitor benchmark.

2. **Refute (per-idea fan-out — the core of the request).** For **each idea**, spawn **3
   perspective-diverse refuter agents**, each whose only job is to kill it on one lens:
   - **UX/UI design** — confusing, redundant, inconsistent with Harbor/Clarity, adds clicks, unclear value.
   - **Existing solutions (Cloudsmith + peers)** — already done better elsewhere? table-stakes vs
     differentiating? copies an anti-pattern?
   - **Architectural overengineering** — effort disproportionate to value? needs new
     backend/API/schema when `extra_attrs`/additions already suffice? fights Harbor's model?
   Each refuter returns verdict (`kill`/`weaken`/`survive`), strongest objection, and (if weaken)
   the **minimal change that would save it**. Implemented as a pipeline: each generator's ideas are
   refuted as soon as produced (`pipeline(generators, generate, ideas => parallel(ideas × 3 refuters))`).

3. **Adjudicate & rank (barrier).** One adjudicator agent receives all `(idea, 3 verdicts)`,
   **dedupes** overlapping ideas across generators, drops ideas killed by ≥1 fatal lens (or majority),
   folds in refuters' "minimal change to save it", and scores survivors:
   native-feel impact × feasibility (reuses an existing seam?) × differentiation vs Cloudsmith ÷
   overengineering risk. Produces the ranked shortlist (top ~8–10).

4. **Board (synthesis).** A synthesizer writes the **product decision board** (markdown) to
   scratchpad: a one-screen ranking table, then per shortlisted idea — problem, ASCII mockup,
   plug-in point (file), how it answered each of the 3 refutation lenses, Cloudsmith comparison,
   effort, and a recommended decision (**Adopt now / Adopt later / Reject-with-reason**). Returns
   `{ board_markdown, ranked_ideas[] }`.

## Follow-through (main loop, after the workflow returns)
5. **Greenlight.** Present the board, then use **AskUserQuestion** as the interactive decision board —
   top ideas as options with their ASCII mockup as `preview` (batched ≤4 per question / multiselect)
   so you pick which to adopt.
6. **Specs.** For each greenlit idea, draft an **implementation spec** (a second short workflow or
   per-idea agents): files to touch from the Grounding inventory, the backend change
   (processor `ListAdditionTypes`/`AbstractAddition`, `extra_attrs`, icons, naming) and/or frontend
   change (pull-command, additions tab, summary, a packages view), and an end-to-end verification
   recipe on the SLOT=1 dev env. Specs are written to scratchpad; no code is changed under this plan.

## Scale / cost
Thorough: ~10 generators × ~3 ideas ≈ ~30 ideas × 3 refuters ≈ ~90 refuter agents + adjudicator +
synthesizer (~100 agents, concurrency-capped). High token cost by design; matches the "thorough" choice.

## Verification (of this plan's deliverable — ideas, not code yet)
- The decision board exists in scratchpad with a ranked table and, per idea, an ASCII mockup, a named
  Harbor plug-in point (real file), explicit answers to all 3 refutation lenses, a Cloudsmith
  comparison, and an Adopt/Later/Reject recommendation.
- Every "Adopt"/"Adopt later" idea cites a concrete seam from the Grounding inventory (no idea that
  requires inventing a subsystem the codebase can't host).
- At least the naming-readability and per-ecosystem setup/usage ideas (the two clearest gaps) appear
  and survive refutation, or are explicitly rejected with a reason.
- Greenlit ideas each get an implementation spec naming files + an end-to-end dev-env check.
- Implementation of any greenlit spec is a SEPARATE, later step (not part of this plan); it will be
  built and verified on the dev env then.

## Files
- Read-only inputs: the Grounding files above (portal components, processors, naming, icon controller).
- Created (scratchpad only): `native-package-ux-board.md` and per-idea spec files for greenlit items.
- No source files are modified under this plan.
