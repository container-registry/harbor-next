# Contributing to the Harbor Next Helm Chart

This guide covers everything specific to working on the chart in
`deploy/chart/`. Repo-wide rules — Conventional Commits, DCO sign-off,
squash-merge, release-please — live in the root
[CONTRIBUTING.md](../../CONTRIBUTING.md) and apply here too. Use the
`(chart)` scope: `feat(chart):`, `fix(chart):`, `ci(chart):`, `docs(chart):`.

## Table of Contents

- [Before You Start](#before-you-start)
- [Where Things Live](#where-things-live)
- [Chart Design Principles](#chart-design-principles)
- [The Change Workflow](#the-change-workflow)
- [Verifying Your Change](#verifying-your-change)
- [Backward Compatibility Rules](#backward-compatibility-rules)
- [Versioning and Releases](#versioning-and-releases)
- [Command Cheat Sheet](#command-cheat-sheet)

---

## Before You Start

All chart tasks run from the **repo root** through the `helm:` namespace
(alias `h:`) — there is no Taskfile inside the chart directory. Tool
versions are pinned in the root `versions.env`; never hardcode a version
anywhere else.

Local toolbox:

| Tool | Needed for | Install |
|------|-----------|---------|
| [task](https://taskfile.dev) | everything | `brew install go-task` |
| helm v4 + [helm-unittest](https://github.com/helm-unittest/helm-unittest) plugin | render, lint, tests | `brew install helm`, then `helm plugin install https://github.com/helm-unittest/helm-unittest.git --verify=false` |
| [helm-docs](https://github.com/norwoodj/helm-docs) | README generation | `brew install norwoodj/tap/helm-docs` |
| [ys](https://yamlscript.org) (YAMLScript) | schema-check, values doctor, rendered-config, migration tests | `brew reinstall ys` |
| Docker | ct, kube-linter, ah, trivy (run containerized) | — |
| [kubescape](https://kubescape.io) (optional) | `task helm:security` | `brew install kubescape` |

Smoke-check your setup on an untouched tree first: `task helm:lint` must pass before you change anything.

## Where Things Live

- `values.yaml` — the chart's public API; helm-docs `# --` annotations feed the README. Mirrored by the hand-maintained `values.schema.json`.
- `README.md` — **generated** by helm-docs from `README.md.gotmpl` + values annotations; never edit directly.
- `templates/` — `{component}.{kind}.yaml` naming; shared logic in `_helpers.tpl` and per-domain `_helpers-{database,redis,tls,trace,urls,jobservice}.tpl`.
- `tests/` — helm unittest suites (`{component}_test.yaml` + snapshots); golden fixtures for the migrator (`migrate/`) and values doctor (`lint/`).
- `ci/` — values for ct lint and the GitOps determinism gate; `example/` — one directory per example, every `values*.yaml` render-checked in CI; `docs/` — migration docs and platform guides.

The four tools in `tools/` (all need `ys`, all run from the chart dir):

| Tool | Purpose | Wired into |
|------|---------|-----------|
| `schema-drift.ys` | Two-way drift between `values.yaml` and `values.schema.json` (undocumented, unknown, and stale keys). Baseline: `.schema-drift-allow` | `task helm:schema-check` |
| `values-lint.ys` | "Values doctor" — valid-but-wrong configs the schema can't express (runs over every example and CI values file) | `task helm:values-lint` |
| `rendered-config-check.ys` | Parses the registry/jobservice `config.yml` out of rendered ConfigMaps and checks invariants that otherwise surface as CrashLoopBackOff | `task helm:rendered-config` |
| `harbor-migrate.ys` | Translates goharbor/harbor-helm 2.x values to this chart; executable twin of `docs/MIGRATION-REFERENCE.md` | `task helm:migrate`, golden-tested by `task helm:migrate:test` |

## Chart Design Principles

The chart's structure is the result of deliberate decisions, and the
reasoning is articulated in the commit history. Before "fixing" something
that looks odd, run `git log deploy/chart` and read the commit that
introduced it — and when you change a deliberate behavior, articulate the
new reasoning in your own commit message so the next person can do the
same.

### Passthrough, not typed values

The chart must never need a change because Harbor gained a config option
or env var. Every component has a generic config surface; typed
per-setting values were deliberately removed. Before adding a typed
value, check whether the passthrough already reaches the setting — it
almost always does.

`<component>.config` always means "this component's primary config
surface", but its shape follows what the process consumes:

| Components | Consume | `config` shape |
|---|---|---|
| core, exporter, trivy | env vars | nested map flattened by the `harbor.toEnvVars` helper into `UPPER_SNAKE_CASE` env vars, delivered via `envFrom`; `<component>.secret` is the same idea for sensitive keys |
| registry, jobservice | a config file | the **verbatim config-file body** (YAML passthrough); `jobservice.env` carries supplementary env vars |
| portal | nginx.conf | none — not key-value-driven, so `portal.existingConfigMap` is its only customization |

### Three customization tiers

Uniformly across components: chart defaults → inline `<component>.config`
in values → `<component>.existingConfigMap` (externally owned ConfigMap;
the chart skips generation and mounts the named one). The invariant that
makes tier 3 safe: **chart-managed runtime wiring lives on the Deployment
as env vars, never inside the generated ConfigMap** — credentials and
connection wiring stay correct no matter who owns the config file.
Registry honors `REGISTRY_<SECTION>_<KEY>` env overrides, jobservice
honors `JOB_SERVICE_POOL_REDIS_URL`. Where chart-managed keys must merge
into user config (`harbor.registry.chartManagedConfig`), **user keys win
on collision**.

### Defaults must not collide with user choices

The default `registry.config.storage` carries no driver — only meta keys
(`cache`, `delete`, `redirect`, …). At render time: zero drivers →
filesystem is injected; two or more → render fails naming them. Any
non-meta key counts as a driver, so new distribution storage drivers work
without chart edits. Apply the same thinking to new defaults: a default
the user must *delete* to make their own choice work is a bug.

### Fail at template time, not as CrashLoopBackOff

If a values combination cannot work, `fail` during `helm template` with a
message naming the offending value. Existing guards: multi-driver
storage, HPA replica bounds, PDB min/max exclusivity, required values,
and the `autoGenSecrets: false` pin surface (`harbor.autoGenValue`).
Follow the pattern for new constraints — `tools/rendered-config-check.ys`
is the backstop, not the first line of defense.

### Credentials never enter ConfigMaps

Sensitive material lives in Secrets, by reference where possible. The
`existing*` family is the chart-wide BYO convention — `existingSecret`,
`existingTlsSecret`, `existingCaSecret`, `existingClaim`,
`existingConfigMap` — follow it for new features. Inline secret values
are a dev convenience only.

### Escape hatches, narrowest first

When a need isn't covered by a typed value, the order of preference is:
component `config`/`secret` passthrough → an `existing*` reference →
`extraManifests` (static extra objects) / `extraTemplateManifests`
(strings rendered through `tpl`, so they can use `.Values`/`.Release`).
If an escape hatch covers a use case, prefer documenting it (an example
directory, a guide section) over growing the values surface.

### Probes are data, not template code

`<component>.probes.{startup,liveness,readiness}` hold full Kubernetes
probe specs rendered through the shared `harbor.probes` helper; per-key
overrides coalesce with defaults, `null` omits a probe. Tune probes via
values, never by encoding component-specific probe logic in templates.

### Schema philosophy

The schema root is closed (`additionalProperties: false`) so top-level
typos fail at render time, but passthrough blocks are deliberately open
(`additionalProperties: true`) — the schema must never block a config key
the application would accept. The worst anti-pattern, found and fixed
several times: **schema-accepted-but-template-ignored keys**. Never
advertise a value the templates don't consume; `schema-drift.ys` exists
to catch exactly this class of drift.

## The Change Workflow

The steps for "add or change chart behavior", in order. Skipping one is
what breaks the pipeline.

### 1. Values first

Add or change the key in `values.yaml` with a helm-docs annotation
(`# -- description` above the key), then mirror it in
`values.schema.json`. The schema root is closed (`additionalProperties:
false`): a new top-level or typed key that isn't mirrored is **rejected
at install time**. Keys inside passthrough blocks stay unmirrored by
design (see [Schema philosophy](#schema-philosophy)). `schema-drift.ys`
fails CI on values↔schema mismatch in either direction. Verify:

```bash
task helm:schema-check
```

Only add a `.schema-drift-allow` baseline entry as a last resort, with a
`#` comment explaining why.

### 2. Templates

- Naming: `{component}.{kind}.yaml`; shared logic goes into the matching
  `_helpers-{domain}.tpl`, not inline.
- Must pass kube-linter and the Trivy config scan (securityContext,
  probes, resource hygiene).
- **Determinism**: two consecutive renders with `ci/gitops-values.yaml`
  must be byte-identical (`task helm:gitops-determinism`). No unguarded
  `randAlphaNum`, `now`, or `lookup` in output — generated secret
  material only behind the `autoGenSecrets` pinning surface. Argo CD
  diffs every render; nondeterminism means permanent sync churn.
- If the change touches the registry or jobservice `config.yml`
  contents, `task helm:rendered-config` checks the semantic invariants
  (storage driver count, worker pool, metric port, …).

### 3. Tests

Extend the component's `tests/{component}_test.yaml`: assert the new
behavior **and** that nothing renders when the feature is off (the
default). If the default rendered output changes, snapshots fail —
update with:

```bash
task helm:unittest:update
```

Review the snapshot diff line by line: it is the blast radius of your
change. Anything in the diff you did not intend is a regression, not
noise.

### 4. Docs

Never edit `README.md` — it is generated from `values.yaml` annotations
plus `README.md.gotmpl`. After any values or gotmpl change:

```bash
task helm:docs
```

and commit the regenerated `README.md`. CI diffs the generated output
against the committed file and fails on drift.

### 5. Examples and guides

- A change that affects a scenario under `example/*/` must update that
  values file — every `example/*/values*.yaml` is render-checked by
  `task helm:examples` automatically.
- New scenario: new directory with `values.yaml` + `README.md`, plus a
  row in the `example/README.md` table. Nothing but the index README
  lives at the `example/` root.
- Platform guides in `docs/guide/` explain context and highlights and
  link to example files — never paste whole values files into a guide.

### 6. The migration contract

`tools/harbor-migrate.ys` and `docs/MIGRATION-REFERENCE.md` are the same
specification in two forms. If your change renames, moves, or retypes
anything a goharbor/harbor-helm user would migrate, update **both in the
same PR**, then:

```bash
task helm:migrate:test       # golden tests + helm template of the output
task helm:migrate:update     # regenerate goldens — review the diff deliberately
```

A golden-file diff you can't explain is a migrator bug.

## Verifying Your Change

The PR gate (`.github/workflows/chart-ci.yml`) triggers on
`deploy/chart/**`, `taskfile/helm.yml`, `versions.env`, and the workflow
itself. Every step maps to a task you can run locally:

| CI step | Task | Catches |
|---------|------|---------|
| Lint | `helm:lint` | helm lint, ct lint, kube-linter, Artifact Hub metadata, schema compile + drift |
| Render templates | `helm:helm-template` | template errors with required values set |
| Unit tests | `helm:unittest` | assertion + snapshot regressions |
| GitOps determinism | `helm:gitops-determinism` | nondeterministic output (Argo CD churn) |
| Render examples | `helm:examples` | broken example values files |
| README in sync | `helm:helm-docs` | hand-edited or stale README.md |
| Migration golden tests | `helm:migrate:test` | migrator drift from goldens |
| Values doctor | `helm:values-lint:test` + `helm:values-lint` | valid-but-wrong example/CI values |
| Rendered config semantics | `helm:rendered-config` | config.yml that renders but crash-loops |
| Trivy config scan | `helm:trivy-chart` | HIGH/CRITICAL misconfigurations |

Run the bundle locally before pushing:

```bash
task helm:ci             # lint + render + tests + determinism + examples + migrate + scanners
task helm:helm-docs helm:values-lint:test helm:values-lint helm:rendered-config
```

For changes that warrant a real cluster check, install into a throwaway
namespace and push/pull an image — `example/k3d-local/README.md` is the
shortest path:

```bash
task helm:install VALUES_FILE=deploy/chart/example/k3d-local/values.yaml
```

## Backward Compatibility Rules

`values.yaml` is the chart's public API, and `helm upgrade` against a
running release is the contract. A values file written for the previous
release must keep working.

- **Never rename, remove, or retype a value outside a breaking release.**
  Deprecate instead: keep the old path honored (fallback in the helper),
  mark it `# -- DEPRECATED: use <new.path>` in `values.yaml`, and remove
  it only in a `feat(chart)!:` commit.
- **Don't flip defaults silently.** A changed default changes running
  deployments on upgrade even when the user touched nothing. Either keep
  the old default, or document the flip in the PR's `## Release Notes`
  section — and treat security-relevant flips as breaking.
- **The schema may loosen, never tighten, for existing keys.** A stricter
  type/enum/pattern rejects previously-valid user values at upgrade time.
  New keys can be strict from day one.
- **Upgrades must survive in place.** Never change for an existing
  component: resource names, `spec.selector` / pod labels (immutable on
  Deployments and StatefulSets), PVC names or specs, StatefulSet
  `volumeClaimTemplates`, or Service ports that `externalURL` traffic
  depends on. If you must, that's a breaking release with documented
  manual steps.
- **Snapshot diffs are the compatibility review.** Before committing an
  updated snapshot, read it as "what changes for a user upgrading from
  the last release" — every hunk needs an answer.
- **The migration contract follows the interface.** Any values-interface
  change lands with the matching `harbor-migrate.ys` +
  `MIGRATION-REFERENCE.md` update in the same PR.
- **Breaking on purpose?** `feat(chart)!:` with a `BREAKING CHANGE:`
  footer in the squash commit, plus user-facing upgrade instructions in
  the PR's `## Release Notes` section.

## Versioning and Releases

The chart releases on its own line, independent of the app (repo)
version. A dedicated release-please instance, configured by
`release-please-config-chart.json` and `.release-please-manifest-chart.json`,
watches commits scoped to `deploy/chart/` and opens a
`chore: release harbor-next chart X.Y.Z` PR (labelled `chart-release:
pending`). Merging that PR tags `chart-vX.Y.Z`, writes the new `version`
into `Chart.yaml`, updates `deploy/chart/CHANGELOG.md`, and triggers the
`chart` job in `release-please.yml`, which packages, pushes, cosign-signs,
and publishes the Artifact Hub metadata to
`oci://8gears.container-registry.com/8gcr/charts`.

- **`version` in `Chart.yaml` is release-please-managed**, not hand-set.
  The `release-type: helm` strategy bumps it on each chart release. Bumps
  follow standard semver from the chart commit types: `fix(chart):` is a
  patch, `feat(chart):` a minor, `feat(chart)!:` / `BREAKING CHANGE:` a
  major. The major lands on the chart's own line, so a breaking chart
  change never forces the app/repo version.
- **`appVersion` is yours to set** (in a `feat(chart):` / `fix(chart):`
  commit). It is the Harbor app version the chart targets and the default
  image tag, and it is NOT overridden at release time, so point it at a
  Harbor version whose images are already published. The chart release
  does not build images.
- **Chart commits do not release the app.** `deploy/chart` is in the app
  release-please `exclude-paths`, so a commit touching only the chart never
  bumps the repo `VERSION`. Root-level helpers the chart relies on
  (`taskfile/helm.yml`, `versions.env`) still count toward the app.
- **Chart releases run on `main` only** for now. Maintenance branches
  (`release-X.Y`) release the app, not the chart.
- **Bootstrap (one-time):** the initial release is pinned with
  `"release-as": "2.0.0"` in `release-please-config-chart.json`. After the
  first `chart-v2.0.0` release PR merges, delete that `release-as` line so
  subsequent releases compute their version from commits.

## Command Cheat Sheet

```bash
task helm:ci                  # full local quality gate
task helm:lint                # all linters incl. schema compile + drift
task helm:unittest            # helm unittest suites
task helm:unittest:update     # ...and update snapshots (review the diff!)
task helm:docs                # regenerate README.md
task helm:examples            # render-check all example values
task helm:schema-check        # AJV strict compile + values<->schema drift
task helm:values-lint         # values doctor over example/ and ci/ values
task helm:rendered-config     # semantic check of rendered config.yml files
task helm:gitops-determinism  # byte-identical double render
task helm:migrate -- old.yaml new.yaml   # run the 2.x values translator
task helm:migrate:test        # migrator golden tests
task helm:migrate:update      # regenerate migrator goldens
task helm:install VALUES_FILE=...        # install into current kube context
```
