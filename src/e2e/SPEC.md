# Harbor End-to-End Test Specification (godog / Gherkin)

Specification for the Harbor e2e test suite built on [godog](https://github.com/cucumber/godog). Defines the full scope, architecture, feature files, step contract, fixtures, lifecycle, and task orchestration. This document is the source of truth for the suite — step-definition Go files must satisfy the contract listed here.

**Status:** implemented. The godog suite is live under `src/e2e/` and the prior Ginkgo baseline has been deleted. `Part 13 — Delete plan` is kept as a historical record of the migration sequence.

---

## 1. Purpose and scope

### Goal

Provide ~25 realistic, independently runnable end-to-end scenarios that build trust that Harbor works for the journeys users actually perform, exercised through the real APIs and the real CLIs (`docker buildx`, `cosign`) users invoke.

### Non-goals

- Exhaustive per-endpoint coverage (that's what unit and integration tests do).
- Button-level UI coverage (Robot suite does this; we are not replacing Robot here).
- Performance / load testing.
- LDAP / OIDC auth matrix — covered elsewhere.
- Multi-site / HA topology.

### What we're replacing

The current Ginkgo suite at `src/e2e/` (added in commit `a0edd965e`) — 12 `*_test.go` files, 10 `Describe` blocks, 36 `It` specs, three of them `Ordered` with mutable cross-spec state. See `Part 2` for the inventory.

### Test selection principles

1. **Trust-per-test over coverage-per-test.** A scenario earns its place by validating a journey that would genuinely rattle confidence if it broke. Single-verb API checks (list, count, filter) get folded into richer flows.
2. **Each scenario is independent.** No `Ordered`. Unique resource suffix per scenario. Parallel + randomised order must pass.
3. **Declarative, user-visible phrasing.** Gherkin describes what a user expects, never HTTP verbs, paths, or JSON.
4. **Realistic tools for hard flows.** Multi-arch / attestation / signing scenarios shell out to `docker buildx` and `cosign` — catching interop regressions that programmatic libs miss.
5. **Tests run through Task-owned orchestration.** `task e2e` prepares a patched tmp workspace from upstream, and `task e2e:test` builds Harbor, starts it, waits for health, runs the suite, and always tears the stack down afterward. CI and local execution use the same Task entrypoint.

---

## 2. Ginkgo suite inventory (the retrospective spec)

All files under `src/e2e/`, package `e2e_test`. Entry `e2e_suite_test.go` calls `RunSpecs(t, "Harbor E2E Suite")`. HTTP wrapper lives inline in `harbor_client_test.go`. Env vars `HARBOR_URL` / `HARBOR_USERNAME` / `HARBOR_PASSWORD` with defaults `http://harbor.128.140.12.238.nip.io` / `admin` / `Harbor12345`. Deps `onsi/ginkgo/v2 v2.28.1`, `onsi/gomega v1.39.1`. Run via `cd src && go test -v ./e2e`.

| File | Suite | Scenarios (It) | Ordered? | Notes |
|---|---|---|---|---|
| `project_test.go` | Project Lifecycle | 6: create public / create private / duplicate-name 409 / delete / list+paginate / update visibility | No | Best-effort delete in AfterEach |
| `user_test.go` | User Management | 3: create+authenticate / current-admin / search | No | Tracks `userID`, deletes in AfterEach |
| `robot_account_test.go` | Robot Accounts | 5: create project-scoped robot / robot push / robot pull / get / delete+auth-fails | **Yes** | Shells out to `podman`; hardcoded registry host |
| `registry_test.go` | Registry Push/Pull | 5: push / artifact in API / pull / list tags / delete tag | **Yes** | Uses `alpine:3.20` and podman CLI |
| `replication_test.go` | Replication | 3: create docker-hub endpoint / create policy / list adapters | No | Policy + endpoint cleanup per test |
| `scan_test.go` | Vulnerability Scanning | 3: push image / trigger scan / wait+verify report | **Yes** | 120 s `Eventually` with 5 s polling |
| `quota_test.go` | Quotas & Retention, GC | 4: create project with 1 GiB quota / list system quotas / GC schedule / GC history | No | |
| `label_test.go` | Labels | 2: create global label / list+delete | No | |
| `audit_log_test.go` | Audit Logs | 3: list / filter-by-op / `X-Total-Count` | No | Read-only |
| `systeminfo_test.go` | System Info | 4: anon info / auth info / health / 401 on bad creds | No | Read-only |

**Issues being corrected by this migration:**
- Three `Ordered` blocks carry mutable state across `It`s.
- Hardcoded registry hostname in two files.
- Scan timeout of 120 s is flaky — tighter and explicit now.
- Cleanup errors silently ignored; orphans accumulate.
- Host-level `podman` dependency; no daemon-free path for push/pull.
- No coverage of cosign, attestations, multi-arch, webhooks, retention, RBAC negative paths, replication round-trip.

Every Ginkgo `It` is either covered by a new scenario in `Part 7` or intentionally dropped (bare list/filter reads are folded into richer flows or covered incidentally by the admin-auth path).

---

## 3. Architecture

### 3.1 Directory layout

```
src/e2e/
├── SPEC.md                      # this document
├── e2e_test.go                  # TestFeatures entrypoint
├── hooks.go                     # InitializeTestSuite + InitializeScenario
├── features/
│   ├── system.feature
│   ├── projects_and_rbac.feature
│   ├── registry.feature
│   ├── signing_and_attestation.feature
│   ├── replication_and_scan.feature
│   └── webhooks.feature
├── steps/
│   ├── common_steps.go          # auth, project / user / label lifecycle, HTTP capture
│   ├── registry_steps.go        # push/pull via go-containerregistry + buildx shellout
│   ├── signing_steps.go         # cosign sign / verify / attest
│   ├── replication_steps.go     # registry endpoints + replication policies
│   ├── scan_steps.go            # trigger + poll scan + stop
│   └── webhook_steps.go         # httptest.Server listener
├── internal/
│   ├── harborclient/            # REST client (ported from harbor_client_test.go)
│   ├── imagebuilder/
│   │   ├── crane.go             # synthetic images via go-containerregistry
│   │   └── buildx.go            # docker buildx shellout
│   ├── cosign/                  # cosign CLI shellout
│   └── fixtures/
│       ├── dockerfile_multiarch # minimal Dockerfile for buildx scenario
│       └── sbom_spdx.json       # SBOM predicate for attestation scenario
└── tools/
    └── stepgen/                 # regenerates docs/e2e-step-catalogue.md
```

### 3.2 Module layout

Stays in `src/go.mod` (user decision — simpler for this repo's workflow). godog is added; `ginkgo/v2` and `gomega` are removed after the old suite is deleted. `google/go-containerregistry` is already present at v0.20.7.

### 3.3 Component relationships

```
   ┌────────────────────┐            ┌──────────────────────────┐
   │  task e2e          │            │  task e2e:test           │
   │  clone upstream    │            │  image build → compose up│
   │  apply patches     │            │  → health → go test      │
   │  copy harness      │            │  → always compose down   │
   └─────────┬──────────┘            └───────────┬──────────────┘
             │                                   │
             ▼                                   ▼
   ┌──────────────────────┐    ┌────────────────────────────────┐
   │ Harbor (core, job,   │◄───┤ godog.TestSuite                │
   │ registry, registryctl,│   │  Options{Strict, Randomize:-1, │
   │ postgres, redis,     │    │   Concurrency:4,               │
   │ trivy)               │    │   Format: pretty+junit}        │
   └──────────────────────┘    └──────────┬─────────────────────┘
                                          │
                 ┌────────────────────────┴────────────────────────┐
                 │                                                 │
                 ▼                                                 ▼
       ┌──────────────────┐                              ┌──────────────────┐
       │ Feature files    │  Gherkin scenarios bind to   │ Step definitions │
       │ (.feature)       │  regex patterns              │ (steps/*.go)     │
       └──────────────────┘                              └────────┬─────────┘
                                                                  │
                        ┌─────────────────────────────────────────┼──────────────┐
                        ▼                                         ▼              ▼
              ┌───────────────────┐                  ┌──────────────────┐  ┌─────────┐
              │ internal/         │                  │ internal/        │  │ cosign  │
              │ harborclient      │                  │ imagebuilder     │  │ buildx  │
              │ (REST over HTTP)  │                  │ (crane + buildx) │  │ (exec)  │
              └───────────────────┘                  └──────────────────┘  └─────────┘
```

### 3.4 Lifecycle

| Scope | Hook | Responsibility |
|---|---|---|
| Suite | `BeforeSuite` | Probe `/api/v2.0/health`; fail fast if Harbor unreachable. Emit banner with target URL + version. |
| Suite | `AfterSuite` | Write run summary (scenario counts) to stdout. |
| Scenario | `sc.Before` | Mint unique `suffix = "e2e-" + shortUUID()`. Build `harborclient.Client` from env. Allocate empty `scenarioState`, store on `ctx` via typed key. |
| Scenario | `sc.After` | If scenario failed: capture last HTTP body, compose logs of `core`/`jobservice`, scenario name into `reports/failures/<scenario>/`. Always: walk created resources in reverse order and best-effort delete (never fail the scenario on teardown errors). Close any `httptest.Server`. Remove per-scenario temp dirs. |
| Step | (implicit via `ctx`) | Each step receives `ctx` and returns `(ctx, error)` so godog can chain state. `Then` steps return only `error`. |

### 3.5 State isolation

- Typed context key (struct, never string) carries a `*scenarioState` from `sc.Before` through every step.
- Fields: `suffix`, `client`, `createdProjects []string`, `createdUsers []int64`, `createdRobots []int64`, `tempDirs []string`, `httpMock *httptest.Server`, `imageRef string`, `lastResp *http.Response`, `lastBody []byte`.
- No package-level variables hold anything scenario-specific. All state lives in `scenarioState` or in `ctx`.
- Every created resource name includes the `suffix`, so parallel scenarios never collide even with identical scenario text.

### 3.6 Step signature contract (Arrange–Act–Assert)

| Gherkin keyword | Role | Signature shape |
|---|---|---|
| `Given` | Arrange / precondition | `func(ctx, params...) (context.Context, error)` |
| `When` | Act | `func(ctx, params...) (context.Context, error)` |
| `Then` | Assert | `func(ctx, params...) error` |

Godog is **regex-only**. Capture groups map directly to typed Go parameters. Step defs register with the keyword-specific builder (`sc.Given`, `sc.When`, `sc.Then`) — not generic `sc.Step` — so intent is explicit and enforced.

---

## 4. Runtime configuration

| Variable | Default | Notes |
|---|---|---|
| `HARBOR_URL` | `http://localhost:${PORT_CORE}` (from taskfile) | Preserved for drop-in compat with existing `HarborClient`. |
| `HARBOR_USERNAME` | `admin` | |
| `HARBOR_PASSWORD` | `Harbor12345` | Matches dev stack default. |
| `SLOT` | `0` | Inherited from `taskfile/dev.yml`; offsets all ports by `+SLOT*100`. |
| `KEEP_UP` | `false` | Set `true` to leave stack up after run for debugging. |
| `E2E_TIMEOUT` | `30m` | Outer go test timeout. |
| `TAGS` | *(empty)* | Godog tag expression passed via `-args -godog.tags=...`. |

Build tag: every file in `src/e2e/` and `src/e2e/steps/`, `src/e2e/internal/...` carries `//go:build e2e` to exclude them from `task test:unit` and plain `go test ./...`.

Godog `Options`:

| Option | Value | Reason |
|---|---|---|
| `Format` | `pretty,junit:../../reports/e2e-junit.xml` | Humans read `pretty`, CI ingests JUnit XML. |
| `Paths` | `[]string{"features"}` | Feature files colocated. |
| `TestingT` | `t` | Use go test integration instead of `TestMain`. |
| `Strict` | `true` | Undefined / pending steps fail the run. |
| `StopOnFailure` | `false` | Surface every break per CI run, not just the first. |
| `Randomize` | `-1` | Scenario order randomized per run — exposes coupling. |
| `Concurrency` | `4` | Parallel scenarios; harness must be stateless (enforced by `suffix`). |

---

## 5. Entry point contract (`task e2e` / `task e2e:test`)

### 5.1 Primary flow

The canonical local and CI command is `task e2e`.

That task:

1. verifies required tools are present
2. recreates a deterministic project-local `tmp/harbor-next-e2e` workspace
3. clones `github.com/container-registry/harbor-next` on `main`
4. discovers local `000N-*` bookmarks
5. octopus-merges their Git refs into the clone with `task patches:apply`
6. copies the local Task/E2E harness into the clone
7. executes `task e2e:test` inside that patched tmp workspace

Separation of concerns:

| Command | Owns |
|---|---|
| `task e2e` | Upstream clone, patch application, tmp workspace preparation, delegation into the patched workspace. |
| `task e2e:test` | Runtime prechecks, image build, token key generation, stack startup, Harbor health wait, godog execution, teardown. |
| `task test:e2e:deployed` | Optional remote-target runner for an already-deployed Harbor instance. |

Rationale: keeps CI and local execution on one path, tests the patched upstream tree instead of this checkout directly, and guarantees teardown after each run.

### 5.2 Task catalogue

| Task | Purpose | Inputs | Side effects |
|---|---|---|---|
| `e2e` | Clone upstream, apply patches, copy the harness, and run the full local workflow | `TAGS`, `E2E_TIMEOUT` | Recreates `tmp/harbor-next-e2e`, writes `reports/e2e-junit.xml` |
| `e2e:test` | Build Harbor, start it, wait for health, run the suite, always tear it down | `TAGS`, `E2E_TIMEOUT` | Starts and removes the local compose stack |
| `test:e2e` | Compatibility alias for `task e2e:test` | `TAGS`, `E2E_TIMEOUT` | Same as `e2e:test` |
| `test:e2e:smoke` | Shortcut: `test:e2e` with `TAGS=@smoke` | — | Same as `e2e:test` |
| `test:e2e:tags` | Arbitrary tag expression | `TAGS='@x && ~@y'` | Same as `e2e:test` |
| `test:e2e:steps` | Regenerate `docs/e2e-step-catalogue.md` via `stepgen` | — | Writes catalogue file |

Internal helpers live in `taskfile/e2e.yml` and cover tool checks, patch application, harness copy, image build, compose lifecycle, Harbor health waiting, and the `go test` invocation.

### 5.3 Runtime guarantees

`task e2e:test` hard-fails when required tools are missing. There is no skip-on-missing behavior for the core signing/build flows.

### 5.4 Guarantees

- `task e2e` is the single orchestration entrypoint for both local runs and CI.
- `task e2e:test` always tears the compose stack down after execution, whether the suite passes or fails.
- The suite runs against Harbor started from the patched tmp clone, not from the current checkout.
- Required signing/build tools are enforced up front instead of producing skip-based partial coverage.
- Suite exit status is the godog result, with teardown still guaranteed via Task `defer`.

### 5.5 Typical developer loop

```bash
task e2e                                            # full local flow
task e2e TAGS='@smoke'                              # targeted subset through the main entrypoint
task e2e:test TAGS='@registry && ~@wip'             # rerun inside an already-prepared workspace
task test:e2e:deployed URL=https://harbor.example.com PASSWORD='…'
```

---

## 6. Test selection — the 25 scenarios

Distribution:

| Feature file | Scenarios | Tags |
|---|---|---|
| `system.feature` | 3 | `@smoke @system @audit @security` |
| `projects_and_rbac.feature` | 5 | `@projects @rbac @robot` |
| `registry.feature` | 6 | `@registry @buildx @multiarch @gc @quota @immutability @smoke` |
| `signing_and_attestation.feature` | 4 | `@signing @sigstore @attestation @buildx` |
| `replication_and_scan.feature` | 4 | `@replication @scan` |
| `webhooks.feature` | 3 | `@webhooks` |
| **Total** | **25** | |

**Smoke subset** (run by `task test:e2e:smoke`): 7 scenarios tagged `@smoke` — at least one per feature file, each a representative user journey: Harbor health, project-admin lifecycle, robot push/pull scope, single-arch round-trip, cosign sign + verify, Trivy scan report, push triggers webhook.

### 6.1 Priority tiers and failure severity

Priority drives CI lane selection and triage severity when a scenario breaks. Tiers are intentionally coarse — this is e2e, not a unit matrix.

| Tier | Tag | Coverage | On-failure severity | Meaning |
|---|---|---|---|---|
| **P0** | `@smoke` (7) | Core user journeys: health, project config, RBAC scope, push/pull, sign, scan, webhook | **Critical** — blocks merge to `main` and release | If these are red, Harbor is not shippable. |
| **P1** | core journeys without `@smoke` (~11) | Duplicate-name 409, developer permission boundary, private-project invisibility, multi-arch buildx, tag lifecycle, GC, replication round-trip, disabled/history webhook, scan-stop | **High** — blocks release, PR mergeable with follow-up | A feature is partly broken but not the fast-path. |
| **P2** | edge-case `@*` (~7) | Tampered signature verify, in-toto / SBOM attestation, quota exceeded, tag immutability, non-matching replication filter, anon admin 401, audit-log filter | **Medium** — tracked; does not block release | Boundary conditions and variants — regressions here worth watching but not emergencies. |

Coverage by pattern (rough): ~13 happy-path scenarios, ~7 error / rejection scenarios (401 / 403 / 404 / 409 / quota / immutable / wrong-key / disabled-webhook), ~5 edge / boundary scenarios (scan-stop mid-run, non-matching replication, cross-project robot denial, non-member project invisibility, tampered signature). Meets test-master's "test happy paths AND error/edge cases" requirement.

---

## 7. Feature specifications

The Gherkin below **is** the spec. Step defs must satisfy every line. All six files share `Given a running Harbor` in their `Background` — the `BeforeSuite` hook has already confirmed `/api/v2.0/health` so the Background step is a cheap sanity check that reuses the session client.

### 7.1 `features/system.feature`

```gherkin
Feature: Harbor system health and access boundaries

  Background:
    Given a running Harbor

  @smoke @system
  Scenario: Harbor reports healthy across all components
    When the admin requests system health
    Then every component is reported healthy
    And anonymous system info matches authenticated system info

  @system @security
  Scenario: Admin APIs reject anonymous and invalid credentials
    When an anonymous client lists users
    Then the request is unauthorized
    When a client with invalid credentials lists users
    Then the request is unauthorized

  @system @audit
  Scenario: Admin can filter the audit log by operation
    Given a fresh project
    When the admin lists audit log entries with operation "create"
    Then the response includes at least one entry for the project
    And the response advertises the total count
```

### 7.2 `features/projects_and_rbac.feature`

```gherkin
Feature: Project lifecycle and role-based access

  Background:
    Given a running Harbor

  @smoke @projects
  Scenario: Project admin configures a project end-to-end and delete cascades
    Given a fresh project "alpha"
    When the admin sets the storage quota on "alpha" to 1 GiB
    And the admin adds a label "team-x" to "alpha"
    And the admin registers a webhook on "alpha" listening for push events
    Then the quota, label, and webhook are visible on "alpha"
    When the admin deletes "alpha"
    Then the project and its label and webhook are gone

  @projects
  Scenario: Duplicate project names are rejected
    Given a fresh project "alpha"
    When the admin attempts to create another project named "alpha"
    Then the request is rejected as a conflict

  @rbac
  Scenario: A developer can push but not delete, and loses access when removed
    Given a fresh private project "proj"
    And a fresh user "dev"
    And "dev" is assigned the "Developer" role on "proj"
    When "dev" pushes an image to "proj/app:v1"
    Then the push succeeds
    When "dev" attempts to delete "proj"
    Then the request is forbidden
    When the admin removes "dev" from "proj"
    Then "dev" can no longer push to "proj"

  @smoke @rbac @robot
  Scenario: A project-scoped robot can push and pull only within its project
    Given a fresh project "proj-a"
    And a fresh project "proj-b"
    And a robot with push and pull permission on "proj-a"
    When the robot pushes an image to "proj-a/app:v1"
    Then the push succeeds
    When the robot pulls "proj-a/app:v1"
    Then the pull succeeds
    When the robot pushes an image to "proj-b/app:v1"
    Then the push is forbidden
    When the admin deletes the robot
    Then the robot credentials are rejected as unauthorized

  @rbac @projects
  Scenario: A private project is invisible to non-members
    Given a fresh private project "secret"
    And a fresh user "outsider" with no project membership
    When "outsider" lists projects
    Then "secret" is not in the response
    When "outsider" requests "secret" directly
    Then the request returns not found
```

### 7.3 `features/registry.feature`

```gherkin
Feature: Container image push, pull, and storage lifecycle

  Background:
    Given a running Harbor

  @smoke @registry
  Scenario: Single-arch image round-trips through Harbor
    Given a fresh project "proj"
    When the admin pushes a synthetic image to "proj/app:v1"
    Then the artifact and tag "v1" appear under "proj/app"
    When the admin pulls "proj/app:v1"
    Then the pulled digest matches the pushed digest

  @registry @multiarch @buildx
  Scenario: Multi-arch buildx image is a manifest index with pullable platforms
    Given a fresh project "proj"
    And the multi-arch build fixture
    When buildx pushes the fixture to "proj/app:v1" for "linux/amd64,linux/arm64"
    Then "proj/app:v1" is a manifest index with exactly 2 child manifests
    And the child manifests advertise platforms "linux/amd64" and "linux/arm64"
    When each platform manifest is pulled
    Then every pull succeeds

  @registry
  Scenario: A second tag on the same image can be deleted without affecting the first
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin tags the image as "proj/app:v1-copy"
    And the admin deletes tag "v1-copy"
    Then tag "v1" still resolves to the original manifest

  @registry @gc
  Scenario: Deleting the last tag and running GC removes the blob
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin deletes all tags under "proj/app"
    And the admin triggers an immediate garbage collection
    Then the GC execution completes successfully
    And pulling the image by digest returns not found

  @registry @quota
  Scenario: Storage quota is enforced at push time
    Given a fresh project "proj" with 1 MiB storage quota
    When the admin pushes a 600 KiB image to "proj/app:v1"
    Then the push succeeds
    When the admin pushes a second 600 KiB image to "proj/app:v2"
    Then the push is rejected as quota exceeded

  @registry @immutability
  Scenario: Immutability rule blocks overwriting a matched tag
    Given a fresh project "proj"
    And an immutability rule on "proj" matching tags "v*"
    And an image pushed to "proj/app:v1"
    When the admin pushes a different image to "proj/app:v1"
    Then the push is rejected as immutable
```

### 7.4 `features/signing_and_attestation.feature`

```gherkin
Feature: Image signing and attestation accessories

  Background:
    Given a running Harbor

  @smoke @signing @sigstore
  Scenario: Cosign signature appears as an accessory and verifies
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    And a freshly generated cosign key pair
    When the admin signs "proj/app:v1" with the cosign key
    Then "proj/app:v1" has a cosign signature accessory
    When the accessory is verified with the matching public key
    Then verification passes

  @signing @sigstore
  Scenario: Verification with the wrong key fails
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    And a freshly generated cosign key pair "A"
    And a freshly generated cosign key pair "B"
    When the admin signs "proj/app:v1" with key pair "A"
    And the accessory is verified with the public key of pair "B"
    Then verification fails

  @signing @attestation @buildx
  Scenario: buildx provenance attestation appears as an in-toto accessory
    Given a fresh project "proj"
    And the multi-arch build fixture
    When buildx pushes the fixture to "proj/app:v1" with provenance attestation
    Then "proj/app:v1" has an attestation accessory
    And the attestation payload type is in-toto

  @signing @attestation @sigstore
  Scenario: SBOM attestation round-trips through Harbor
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    And an SPDX JSON SBOM predicate
    And a freshly generated cosign key pair
    When the admin attaches the SBOM as an attestation on "proj/app:v1"
    Then "proj/app:v1" has an SBOM attestation accessory
    And the accessory predicate matches the pushed predicate
```

### 7.5 `features/replication_and_scan.feature`

```gherkin
Feature: Replication and vulnerability scanning

  Background:
    Given a running Harbor

  @replication
  Scenario: A Docker Hub tag is replicated to Harbor on demand
    Given a Docker Hub endpoint is registered
    And a replication policy from Docker Hub matching "library/alpine:3.20"
    When the admin triggers the replication policy
    Then the execution reports success
    And the replicated artifact in Harbor matches the upstream digest

  @replication
  Scenario: A non-matching replication filter produces zero tasks
    Given a Docker Hub endpoint is registered
    And a replication policy from Docker Hub matching a pattern with no upstream match
    When the admin triggers the replication policy
    Then the execution reports success with zero tasks

  @smoke @scan
  Scenario: Trivy returns a scan report with severities within the timeout
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin triggers a scan on "proj/app:v1"
    Then the scan report completes within 60 seconds
    And the report includes a severity summary and at least one CVE

  @scan
  Scenario: A running scan can be stopped and ends in a stopped state
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin triggers a scan on "proj/app:v1"
    And the admin immediately stops the scan on "proj/app:v1"
    Then the scan report final status is stopped
```

### 7.6 `features/webhooks.feature`

```gherkin
Feature: Project webhook notifications

  Background:
    Given a running Harbor

  @smoke @webhooks
  Scenario: Push triggers a configured webhook
    Given a fresh project "proj"
    And an in-process webhook listener
    And a webhook policy on "proj" for push events targeting the listener
    When the admin pushes an image to "proj/app:v1"
    Then the listener receives a push event whose digest matches the pushed artifact

  @webhooks
  Scenario: Disabled webhook policy does not fire
    Given a fresh project "proj"
    And an in-process webhook listener
    And a webhook policy on "proj" for push events targeting the listener
    And the policy is disabled
    When the admin pushes an image to "proj/app:v1"
    Then the listener receives no event within 10 seconds

  @webhooks
  Scenario: Webhook delivery history records a successful job
    Given a fresh project "proj"
    And an in-process webhook listener
    And a webhook policy on "proj" for push events targeting the listener
    When the admin pushes an image to "proj/app:v1"
    And the listener receives the event
    Then the webhook delivery history lists at least one successful job
```

---

## 8. Step catalogue

The contract that step-definition files must satisfy. Each row is the regex the step is registered under, the role (Given / When / Then), the parameters it extracts, and the responsibility. This catalogue is the source for `docs/e2e-step-catalogue.md`, which `task test:e2e:steps` regenerates by walking `sc.Given/When/Then` calls.

### 8.1 Given steps (arrange / preconditions)

| Regex | Parameters | Responsibility |
|---|---|---|
| `^a running Harbor$` | — | Assert `/api/v2.0/health` 200 — cheap probe against cached client. |
| `^a fresh project$` | — | Create project `e2e-<suffix>`; register for cleanup. |
| `^a fresh project "([^"]+)"$` | name | Create project `<name>-<suffix>`; register for cleanup. |
| `^a fresh private project "([^"]+)"$` | name | As above, with `public=false`. |
| `^another fresh project "([^"]+)"$` | name | Convenience synonym for `a fresh project "X"` — semantic, not a different step. |
| `^a fresh project "([^"]+)" with (\d+) (KiB\|MiB\|GiB) storage quota$` | name, amount, unit | Create project with storage quota set. |
| `^an image pushed to "([^"]+)"$` | ref | Push a 1 KiB synthetic `random.Image` to `<proj>/<repo>:<tag>` resolving the project prefix from scenario state. |
| `^a fresh user "([^"]+)"$` | name | Create user `<name>-<suffix>` with random password; register for cleanup. |
| `^a fresh user "([^"]+)" with no project membership$` | name | Same as above; documentation step. |
| `^"([^"]+)" is assigned the "([^"]+)" role on "([^"]+)"$` | user, role, project | POST project member with role. |
| `^a robot with (push\|pull\|push and pull) permission on "([^"]+)"$` | perms, project | Create project-scoped robot with given permissions; store secret on state. |
| `^a freshly generated cosign key pair$` | — | `cosign generate-key-pair` in per-scenario temp dir. |
| `^a freshly generated cosign key pair "([^"]+)"$` | label | Same, keyed by label for multi-key scenarios. |
| `^the multi-arch build fixture$` | — | Copy `internal/fixtures/dockerfile_multiarch` into per-scenario temp dir. |
| `^an SPDX JSON SBOM predicate$` | — | Load `internal/fixtures/sbom_spdx.json` and stage for attest. |
| `^a Docker Hub endpoint is registered$` | — | POST `/registries` for `docker-hub`; register for cleanup. |
| `^a replication policy from Docker Hub matching "([^"]+)"$` | pattern | POST replication policy with name filter; register for cleanup. |
| `^a replication policy from Docker Hub matching a pattern with no upstream match$` | — | Same with a deliberately non-matching filter. |
| `^an immutability rule on "([^"]+)" matching tags "([^"]+)"$` | project, tagPattern | POST immutability rule; register for cleanup. |
| `^an in-process webhook listener$` | — | `httptest.NewServer` on loopback; store on state; register teardown. |
| `^a webhook policy on "([^"]+)" for push events targeting the listener$` | project | POST webhook policy pointing at the listener URL. |
| `^the policy is disabled$` | — | Disable the most recently created webhook policy. |

### 8.2 When steps (act)

| Regex | Parameters | Responsibility |
|---|---|---|
| `^the admin requests system health$` | — | GET `/health`; capture response. |
| `^an anonymous client lists users$` | — | GET `/users` without auth header; capture response. |
| `^a client with invalid credentials lists users$` | — | GET `/users` with bogus Basic auth; capture response. |
| `^the admin lists audit log entries with operation "([^"]+)"$` | op | GET `/audit-logs?q=operation=<op>`; capture. |
| `^the admin sets the storage quota on "([^"]+)" to (\d+) (KiB\|MiB\|GiB)$` | project, amount, unit | PUT quota. |
| `^the admin adds a label "([^"]+)" to "([^"]+)"$` | label, project | Create label + attach to project. |
| `^the admin registers a webhook on "([^"]+)" listening for push events$` | project | POST webhook policy; store policy ID on state. |
| `^the admin deletes "([^"]+)"$` | project | DELETE project and assert cascade. |
| `^the admin attempts to create another project named "([^"]+)"$` | name | POST `/projects` with duplicate name; capture response. |
| `^"([^"]+)" pushes an image to "([^"]+)"$` | user, ref | crane.Push using `<user>`'s credentials. |
| `^"([^"]+)" attempts to delete "([^"]+)"$` | user, project | DELETE project as `<user>`; capture response. |
| `^the admin removes "([^"]+)" from "([^"]+)"$` | user, project | DELETE project member. |
| `^the robot pushes an image to "([^"]+)"$` | ref | crane.Push using robot credentials. |
| `^the robot pulls "([^"]+)"$` | ref | crane.Pull using robot credentials. |
| `^the admin deletes the robot$` | — | DELETE most recent robot. |
| `^"([^"]+)" lists projects$` | user | GET `/projects` as `<user>`; capture response. |
| `^"([^"]+)" requests "([^"]+)" directly$` | user, project | GET `/projects/<name>` as `<user>`. |
| `^the admin pushes a synthetic image to "([^"]+)"$` | ref | crane.Push a deterministic `random.Image(1024, 1)`. |
| `^the admin pulls "([^"]+)"$` | ref | crane.Pull; store pulled digest on state. |
| `^buildx pushes the fixture to "([^"]+)" for "([^"]+)"$` | ref, platforms | `docker buildx build --platform <p> --push` of fixture. |
| `^buildx pushes the fixture to "([^"]+)" with provenance attestation$` | ref | `docker buildx build --attest=type=provenance,mode=max --push`. |
| `^each platform manifest is pulled$` | — | For each child manifest, crane.Pull by digest. |
| `^the admin tags the image as "([^"]+)"$` | ref | POST `/repositories/.../tags` with new tag. |
| `^the admin deletes tag "([^"]+)"$` | tag | DELETE tag under last-used repo. |
| `^the admin deletes all tags under "([^"]+)"$` | repoRef | Delete every tag listed. |
| `^the admin triggers an immediate garbage collection$` | — | POST `/system/gc/schedule` with `schedule.type=Manual`; poll to completion. |
| `^the admin pushes a (\d+) (KiB\|MiB) image to "([^"]+)"$` | amount, unit, ref | crane.Push with `random.Image` sized accordingly. |
| `^the admin pushes a second (\d+) (KiB\|MiB) image to "([^"]+)"$` | amount, unit, ref | Same; deterministic second image with different layer content. |
| `^the admin pushes a different image to "([^"]+)"$` | ref | crane.Push a second distinct `random.Image` to same ref. |
| `^the admin signs "([^"]+)" with the cosign key$` | ref | `cosign sign --key cosign.key`. |
| `^the admin signs "([^"]+)" with key pair "([^"]+)"$` | ref, label | `cosign sign` with labelled key. |
| `^the accessory is verified with the matching public key$` | — | `cosign verify --key cosign.pub`. |
| `^the accessory is verified with the public key of pair "([^"]+)"$` | label | `cosign verify --key <label>.pub`. |
| `^the admin attaches the SBOM as an attestation on "([^"]+)"$` | ref | `cosign attest --type spdxjson --predicate sbom.json --key cosign.key`. |
| `^the admin triggers the replication policy$` | — | POST `/replication/executions`; poll until terminal. |
| `^the admin triggers a scan on "([^"]+)"$` | ref | POST `.../artifacts/<digest>/scan`; record start time. |
| `^the admin immediately stops the scan on "([^"]+)"$` | ref | POST `.../artifacts/<digest>/scan/stop`. |
| `^the admin pushes an image to "([^"]+)"$` | ref | crane.Push synthetic image (alias for `admin pushes a synthetic image`). |
| `^the listener receives the event$` | — | Block up to 10 s for listener to record any event. |

### 8.3 Then steps (assert)

| Regex | Parameters | Responsibility |
|---|---|---|
| `^every component is reported healthy$` | — | Parse `/health` response; assert all components `status=healthy`. |
| `^anonymous system info matches authenticated system info$` | — | Diff both payloads; only fields marked sensitive may differ. |
| `^the request is unauthorized$` | — | Assert last response `401`. |
| `^the response includes at least one entry for the project$` | — | Parse log list; assert ≥1 entry where `resource` contains the scenario's project name. |
| `^the response advertises the total count$` | — | Assert `X-Total-Count` header present and `>= 1`. |
| `^the quota, label, and webhook are visible on "([^"]+)"$` | project | Three GETs; assert each artifact present. |
| `^the project and its label and webhook are gone$` | — | GET project → 404; GET label by ID → 404; GET webhook policy → 404. |
| `^the request is rejected as a conflict$` | — | Assert last response `409`. |
| `^the push succeeds$` | — | Assert last crane.Push err nil. |
| `^the pull succeeds$` | — | Assert last crane.Pull err nil. |
| `^the push is forbidden$` | — | Assert last crane.Push err maps to `403`. |
| `^the push is denied$` | — | Synonym of `forbidden`; asserts last push error is a denial (`401` or `403`). |
| `^the push is rejected as quota exceeded$` | — | Assert last push error payload contains `PROJECTQUOTAERROR` or equivalent. |
| `^the push is rejected as immutable$` | — | Assert last push error maps to `412` / immutability denial. |
| `^the request is forbidden$` | — | Assert last response `403`. |
| `^the robot credentials are rejected as unauthorized$` | — | Using the stored robot secret, crane.Push → `401`. |
| `^"([^"]+)" is not in the response$` | project | Assert project name absent from last project list. |
| `^the request returns not found$` | — | Assert last response `404`. |
| `^"([^"]+)" can no longer push to "([^"]+)"$` | user, project | Retry crane.Push as `<user>`; assert `403`. |
| `^the artifact and tag "([^"]+)" appear under "([^"]+)"$` | tag, repoRef | GET `/.../artifacts`; assert exactly one matching artifact with the given tag. |
| `^the pulled digest matches the pushed digest$` | — | Compare state-held digests. |
| `^"([^"]+)" is a manifest index with exactly (\d+) child manifests$` | ref, n | Inspect manifest media type + child count. |
| `^the child manifests advertise platforms "([^"]+)" and "([^"]+)"$` | p1, p2 | Assert child platforms set equals `{p1, p2}`. |
| `^every pull succeeds$` | — | Assert no error from any pull in the batch. |
| `^tag "([^"]+)" still resolves to the original manifest$` | tag | GET tag; assert digest unchanged. |
| `^the GC execution completes successfully$` | — | Assert latest GC execution status `Success`. |
| `^pulling the image by digest returns not found$` | — | crane.Head by digest; assert `404`. |
| `^"([^"]+)" has a cosign signature accessory$` | ref | GET accessories; assert one with subject type `signature.cosign`. |
| `^verification passes$` | — | Last `cosign verify` exit 0. |
| `^verification fails$` | — | Last `cosign verify` exit non-zero. |
| `^"([^"]+)" has an attestation accessory$` | ref | GET accessories; assert one with subject type `attestation`. |
| `^the attestation payload type is in-toto$` | — | Inspect accessory manifest; assert payload mediaType includes `in-toto`. |
| `^"([^"]+)" has an SBOM attestation accessory$` | ref | GET accessories; assert one with predicate type SPDX. |
| `^the accessory predicate matches the pushed predicate$` | — | Diff pulled predicate against fixture. |
| `^the execution reports success$` | — | Replication execution status `Succeed`. |
| `^the replicated artifact in Harbor matches the upstream digest$` | — | crane.Head Harbor ref and upstream ref; compare digests. |
| `^the execution reports success with zero tasks$` | — | Status `Succeed` and task count `0`. |
| `^the scan report completes within (\d+) seconds$` | secs | Poll `/.../scan-reports` until `Status=Success` within `<secs>`. |
| `^the report includes a severity summary and at least one CVE$` | — | Assert report payload has `severity`, non-empty `summary`, `vulnerabilities[*].id` non-empty. |
| `^the scan report final status is stopped$` | — | Poll until terminal; assert `Stopped`. |
| `^the listener receives a push event whose digest matches the pushed artifact$` | — | Assert listener got `PUSH_ARTIFACT` event within 10 s with matching digest. |
| `^the listener receives no event within (\d+) seconds$` | secs | Assert listener channel stays empty for `<secs>`. |
| `^the webhook delivery history lists at least one successful job$` | — | GET delivery history; assert ≥1 entry with `status=Success`. |

### 8.4 Duplicate-step discipline

Any step registration whose regex overlaps an existing one must be rejected in code review. `task test:e2e:steps` regenerates `docs/e2e-step-catalogue.md`; a CI check diffs the committed catalogue against the regenerated output. Agents adding new scenarios search the catalogue first and reuse phrasing.

---

## 9. Fixtures

| Path | Purpose | Notes |
|---|---|---|
| `internal/fixtures/dockerfile_multiarch` | Minimal `FROM scratch` Dockerfile for buildx scenarios | ~5 lines; no external build context dependencies. |
| `internal/fixtures/sbom_spdx.json` | SPDX JSON SBOM predicate for SBOM attestation scenario | Trivial valid SPDX doc. |
| Synthetic images | Generated at runtime via `crane.Push(random.Image(size, layers))` | No test data needed on disk; deterministic by layer seed when required. |
| Cosign key pairs | Generated per scenario in per-scenario temp dir | Never committed; removed by `sc.After`. |

No live Docker Hub pulls other than the replication scenarios (which are explicit about that dependency).

---

## 10. AI-agent-friendly conventions

Deliberate choices that make the suite easy to extend across future LLM sessions. LLMs tend to duplicate step defs when they can't see the existing set, over-specify scenarios with implementation detail, and batch multiple actions into one step. Countermeasures:

1. **One action per step.** Every `When` performs exactly one user-visible action; every `Given` asserts exactly one precondition.
2. **Declarative Gherkin only.** No HTTP verbs, paths, JSON, headers, status codes, or CSS selectors in `.feature` files.
3. **Step catalogue (`docs/e2e-step-catalogue.md`) is committed and CI-checked.** Any agent adding a scenario greps it first; duplicate phrasings are rejected in review.
4. **Object–subject–verb naming.** `a project named "X"`, `the robot pushes "X"`, `the push is denied`.
5. **Scenarios ≤ 6 steps including Background.** Longer scenarios indicate granularity issues or two scenarios fused.
6. **Background holds shared preconditions only.** Max 2 steps. Per-scenario uniqueness happens in `sc.Before`.
7. **Fixtures live under one roof.** `internal/fixtures/` is the only place new synthetic images, Dockerfiles, keys, predicates are added.
8. **Failure artifacts captured automatically.** On failure `sc.After` dumps last HTTP body, core + jobservice compose logs, and scenario name to `reports/failures/<scenario>/`.
9. **Data tables for repeated fixtures.** `Given the following users: | name | role |` is preferred over N separate `Given` steps.
10. **External systems mocked in-process.** Webhook scenarios use `httptest.NewServer`; no external mock service to run.

---

## 11. Tool preflight

- `task e2e` and `task e2e:test` hard-fail when required host tools are missing.
- Required tools are checked before orchestration starts: `git`, `stg`, `task`, `docker`, `docker buildx`, `podman`, `cosign`, plus a working compose frontend (`docker compose` or `podman compose`).
- The suite no longer relies on skip-on-missing behavior for the core signing/build paths.
- Replication scenarios may still be explicitly disabled with `E2E_SKIP_REPLICATION=1` when the environment is intentionally offline.

---

## 12. Reporting and CI

- `reports/e2e-junit.xml` is written by the JUnit formatter and uploaded as a workflow artifact by the GitHub Actions job calling `task e2e`.
- `reports/failures/<scenario>/` contains per-scenario failure artifacts — body, logs, scenario name.
- CI runs a single orchestration command: `task e2e`.

---

## 13. Delete plan for the Ginkgo suite (historical — completed)

Originally sequenced across PRs so the Ginkgo suite stayed runnable until the godog suite reached parity. The migration ultimately landed as a single cutover rather than the per-feature staging below, and the Ginkgo files are now gone. Retained for context:

1. Scaffold + taskfile entry point + empty feature dir + `internal/harborclient` port.
2. `system.feature` (3 scenarios) — proves the harness.
3. `projects_and_rbac.feature` (5 scenarios).
4. `registry.feature` (6 scenarios) — adds `imagebuilder/crane.go` and `buildx.go`.
5. `replication_and_scan.feature` (4 scenarios).
6. `signing_and_attestation.feature` (4 scenarios) — adds `internal/cosign/cli.go`.
7. `webhooks.feature` (3 scenarios).
8. ~~Delete Ginkgo: all 12 `src/e2e/*_test.go` files, drop `ginkgo/v2` + `gomega` from `src/go.mod`, `go mod tidy`.~~ **Done.**
9. ~~Update `CLAUDE.md` §Integration Testing to reference the new suite.~~

### Files added/modified/deleted

| Action | Path |
|---|---|
| Create | `src/e2e/SPEC.md` *(this file)* |
| Create | `src/e2e/e2e_test.go`, `src/e2e/hooks.go` |
| Create | `src/e2e/features/*.feature` (6 files) |
| Create | `src/e2e/steps/*.go` (6 files) |
| Create | `src/e2e/internal/harborclient/client.go` |
| Create | `src/e2e/internal/imagebuilder/{crane,buildx}.go` |
| Create | `src/e2e/internal/cosign/cli.go` |
| Create | `src/e2e/internal/fixtures/{dockerfile_multiarch,sbom_spdx.json}` |
| Create | `src/e2e/tools/stepgen/main.go` |
| Create | `docs/e2e-step-catalogue.md` *(generated)* |
| Modify | `src/go.mod`, `src/go.sum` (add godog; later drop ginkgo/gomega) |
| Modify | `taskfile/test.yml` (add `e2e*` tasks) |
| Modify | `CLAUDE.md` §Integration Testing |
| Delete | All 12 existing `src/e2e/*.go` files (after step 7) |

### Existing utilities to reuse

- `HarborClient` in `src/e2e/harbor_client_test.go` → port verbatim into `internal/harborclient` with exported methods; keep Basic-Auth + env var contract.
- `github.com/google/go-containerregistry` (already at v0.20.7 in `src/go.mod`) — `crane.Push`, `crane.Pull`, `random.Image` for synthetic fixtures.
- Patterns from `tests/apitests/python/library/cosign.py` and `tests/resources/Cosign_Util.robot` → reference for cosign CLI invocations.
- Patterns from `tests/resources/Docker-Util.robot` → reference for buildx shellout shape.

---

## 14. Verification checklist

- [ ] `task e2e` recreates `tmp/harbor-next-e2e`, clones upstream, copies patches, applies the stack, copies the harness, and enters `task e2e:test`.
- [ ] `task e2e:test` builds Harbor images from the patched tmp clone, starts Harbor, waits for health, runs all 25 scenarios, and returns the suite status.
- [ ] Rapid iteration: rerunning `task e2e:test` in a prepared workspace only repeats the runtime path.
- [ ] Independence: three consecutive `task e2e:test` runs against fresh local startups pass with `Randomize: -1` and `Concurrency: 4`.
- [ ] `task e2e TAGS='@smoke'` runs only `@smoke` scenarios.
- [ ] `task e2e:test TAGS='@registry && ~@wip'` runs the expected subset.
- [ ] Failure behavior: force a scenario failure — `reports/failures/<scenario>/` populated; stack still tears down automatically.
- [ ] Tool preflight: with `cosign` / `docker buildx` absent, the task fails before any partial run starts.
- [ ] Resource hygiene: project / user / robot counts identical before and after a run.
- [ ] `reports/e2e-junit.xml` generated and parseable.
- [ ] `task test:e2e:steps` produces `docs/e2e-step-catalogue.md` identical to committed copy (CI check).
- [ ] Local/CI parity: the workflow does not contain Harbor-specific orchestration beyond `task e2e`.
- [x] After deletion step: `grep -r ginkgo src/ .github/ taskfile/` is empty (aside from transitive `bsm/ginkgo` in `go.sum`); `go mod tidy` drops `ginkgo/v2` + `gomega`.

---

## 15. Test-quality guardrails

Auditing this spec against the five classic e2e anti-patterns (testing mock behaviour, test-only methods in production, over-mocking without understanding, incomplete mocks, integration tests as afterthought) surfaces conventions the spec must bake in so agents extending the suite don't reintroduce them.

### 15.1 Anti-pattern coverage

| Anti-pattern (test-master) | Risk for a Harbor e2e suite | How this spec prevents it |
|---|---|---|
| **Testing mock behaviour** | Asserting `mockClient.Push` was called instead of `crane.Pull` returning the same digest | Scenarios run against **real Harbor**; assertions are always on observable outcomes (digest equality, 4xx status, accessory presence, listener delivery). Step defs never assert on internal call counts. |
| **Test-only methods in production** | Adding endpoint shortcuts like `/test/reset-project` in `src/core/...` to simplify cleanup | **Forbidden.** Cleanup goes through real Harbor APIs the user has. `sc.After` uses the same `DELETE /projects/{name}` a real admin would. No test-only endpoints, env flags, or config branches in `src/core/` or `src/pkg/`. |
| **Over-mocking without understanding** | Mocking Harbor's `/v2/` responses in a godog step, then missing a real registry behavior | Only **external** services are mocked: (a) webhook receiver via `httptest.NewServer`. Everything Harbor-adjacent (registry, trivy, jobservice, registry controller) is the real component started by `task e2e:test`. |
| **Incomplete mocks** | A mocked listener that only decodes `event` and drops `digest`, masking a real payload-shape regression | The `httptest.Server` listener stores and exposes the **raw** request body; assertions validate the full real-shape payload emitted by Harbor. |
| **Integration tests as afterthought** | Adding a feature, shipping it, writing the e2e test "next sprint" | The migration sequence (Part 13) lands **feature file → step defs → delete old spec** one capability at a time. No feature is done until its scenarios pass. The delete plan explicitly keeps Ginkgo alive until godog reaches parity so there's never a coverage gap. |

### 15.2 External dependency posture

Explicit about what's real vs mocked — test-master flags over-mocking as a hidden failure mode, so this matrix must not drift silently:

| System | Real or mocked | Notes |
|---|---|---|
| Harbor core, jobservice, registry, registryctl, trivy | **Real** | Started by `task e2e:test`; not mocked. |
| PostgreSQL, Redis | **Real** | Via compose; not mocked. |
| Docker Hub (for replication scenarios only) | **Real, but narrow** | Pulls `library/alpine:3.20` once per replication run. If offline CI matters, set `E2E_SKIP_REPLICATION=1`. |
| Internal registry for push/pull | **Real** (`crane` → Harbor → distribution) | Synthetic images via `random.Image`; no live pulls for basic scenarios. |
| Webhook receiver | **Mocked** (`httptest.NewServer`) | In-process; real protocol (HTTP + JSON), real payload shape. |
| Cosign / buildx | **Real CLIs** | Shelled out to on the host. Absent → task preflight fails. |

### 15.3 Flakiness policy

Test-master: *"Ignore flaky tests — quarantine and fix them; don't just re-run until green."*

- **No automatic retries** in the suite. godog does not have a retry-on-failure knob and we will not add one — failures are real and must be triaged.
- **Quarantine tag** `@quarantine` — a flaky scenario is tagged, CI excludes it via `-godog.tags='~@quarantine'` on `main`, and a fix is owed within 5 working days. If the fix slips, the scenario is **deleted**, not left dormant.
- **No shared state between scenarios.** Randomize (`-1`) and Concurrency (`4`) actively expose hidden coupling — flakes caused by coupling are the scenario's own bug, not a framework quirk.
- **Deterministic fixtures.** `random.Image(size, layers)` with an explicit seed where byte-level determinism matters; cosign keys generated fresh per scenario; temp dirs per scenario.
- **Generous async budgets, with explicit upper bounds.** Scan polling 60 s, Harbor health probe 300 s for fresh local startup, GC polling 60 s, webhook receipt 10 s. Any scenario that needs a higher budget must justify it in-code.

### 15.4 TDD discipline for the migration

Anti-pattern 5 ("tests as afterthought") applies to the rollout itself, not just to future features. Rollout convention:

1. Within a capability PR (e.g. PR #3 = `projects_and_rbac.feature`), the `.feature` file lands **first** in the diff. It must register as pending/undefined until the step defs in the same PR implement them.
2. `Strict: true` in `godog.Options` makes undefined steps **fail** the run — so "feature file merged, step defs forgotten" is impossible.
3. Delete-the-Ginkgo-suite PR (step 8 of Part 13) only lands **after** all six feature files are green on `main` — never in the same PR as new scenarios.

### 15.5 Severity + triage mapping

When `task e2e` reports failures in CI, use this quick-triage table:

| Failing scenario tags | Severity | First action |
|---|---|---|
| Any `@smoke` | **Critical** | Revert or hotfix — do not release. Page oncall if on `main`. |
| Any `@rbac` `@projects` `@registry` without `@smoke` | **High** | Block release; file a P1 issue; triage next business day. |
| Any `@sigstore` `@buildx` `@attestation` | **High** | Block release; investigate — these fail in realistic user flows. |
| `@replication` (upstream dependent) | **Medium** | Check Docker Hub rate-limit / network before filing a bug. |
| `@webhooks` `@quota` `@gc` `@immutability` | **Medium** | File a P2; release may proceed with a note. |
| `@quarantine` | **Info** | Already known; counts against the 5-day fix SLA. |

---

## 16. Open questions / future extensions

- **CI lane wiring.** GitHub Actions job definition for `test:e2e:smoke` (PR) and `test:e2e` (main) is tracked separately.
- **Helm-chart push/pull.** Could be a 26th scenario but adds `helm` as a required CLI; deferred until the suite has proven stable.
- **OIDC / LDAP auth flows.** Explicitly out of scope for first cut; covered by Robot suite and Python API tests.
- **Performance baseline.** Not in scope; would live in a separate `task test:perf` driven by k6 or similar.

---

## 17. Sources

- [cucumber/godog](https://github.com/cucumber/godog) — README, concurrency + context semantics
- [godog on pkg.go.dev](https://pkg.go.dev/github.com/cucumber/godog) — Options, formatters
- [Intro to Godog, Part 1 — BDD for Go](https://thedumpsterfireproject.com/posts/godog-part-1/) — Arrange–Act–Assert step signature conventions, typed context keys
- [Intro to Godog, Part 2 — context & hooks](https://thedumpsterfireproject.com/posts/godog-part-2/) — context chaining + hook lifecycle
- [Go + Gherkin BDD for Kubernetes](https://sazardev.netlify.app/blog/en/go-gherkin-bdd-kubernetes-testing/) — declarative infra feature files, polling, per-scenario cleanup, artifact capture on failure
- [Peter Warnock — godog BDD Testing Framework for Go](https://peterwarnock.com/tools/godog-bdd-testing-framework-for-go/) — DI patterns, step reuse, scale advice
- [cucumber.io — Writing Better Gherkin](https://cucumber.io/docs/bdd/better-gherkin/) — declarative-over-imperative
- [Automation Panda — BDD 101](https://automationpanda.com/2017/01/30/bdd-101-writing-good-gherkin/) — one action per step
- [andredesousa/gherkin-best-practices](https://github.com/andredesousa/gherkin-best-practices) — naming + reuse discipline
- [LLM-based Acceptance Test Generation — industrial case study (2024)](https://arxiv.org/html/2504.07244v1) — why one-action-per-step matters for LLM-driven BDD
- [Why Use Gherkin for Automated Test Scripts in 2026](https://testquality.com/why-use-gherkin-for-automated-test-scripts-in-2026/) — resilience to refactor via declarative scenarios
