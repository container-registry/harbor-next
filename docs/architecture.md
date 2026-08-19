# Harbor Architecture — Deep Dive

This document goes one level deeper than [architecture-overview.md](architecture-overview.md):
[C4-model](https://c4model.com/) flight levels from system context down to
components, the package-level structure of the Go module, and component-level
anatomy of each service (Core, JobService, RegistryCtl, Exporter, Portal). It
stays above function level — the unit of discussion is a package or a
component, not a function.

C4 notation in the diagrams: boxes carry a **name**, a `[type]` line (Person /
Software System / Container / Component / External), and a short
responsibility description; arrows are labeled with the relationship.

| C4 level | View | Section |
|---|---|---|
| 1 | System context — Harbor and its neighbours | [§1](#1-system-context-c4-level-1) |
| 2 | Containers — deployable units inside Harbor | [§2](#2-container-view-c4-level-2) |
| 3 | Components — inside Core | [§4.4](#44-component-view-c4-level-3) |
| 3 | Components — inside JobService | [§7.1](#71-component-view-c4-level-3) |
| — | Dynamic — login / push / pull sequences | [§5](#5-request-flows-login-push-pull-c4-dynamic-views) |
| 4 | Code — packages, layering rules, source | [§3](#3-package-topology-of-the-go-module) |

Diagrams are SVGBob; regenerate the rendered links with `task docs:svgbob`.

---

## 1. System context (C4 level 1)

<!--
```SVGBob
┌──────────────────────┐                ┌────────────────────────────┐
│ "Developer / Admin"  │                │ "OCI client / CI"          │
│ "[Person]"           │                │ "[Software System]"        │
│ "manages projects,"  │                │ "docker, helm, oras,"      │
│ "reviews scans, UI"  │                │ "kubelet, buildkit"        │
└──────────┬───────────┘                └────────────┬───────────────┘
           │ "HTTPS: UI + REST"                      │ "push, pull"
           ▼                                         ▼
┌────────────────────────────────────────────────────────────────────┐
│ "Harbor"                                                           │
│ "[Software System]"                                                │
│ "OCI artifact registry: projects, RBAC, scanning, replication,"    │
│ "retention, quotas, signing, webhooks, GC"                         │
└──────┬─────────────────┬─────────────────┬─────────────────┬───────┘
       │ "authN"         │ "replicate,"    │ "notify"        │ "preheat"
       │                 │ "proxy-cache"   │                 │
       ▼                 ▼                 ▼                 ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ "IdP"        │  │ "Remote"     │  │ "Webhook /"  │  │ "P2P preheat"│
│ "[External]" │  │ "registries" │  │ "Slack"      │  │ "[External]" │
│ "LDAP, OIDC,"│  │ "[External]" │  │ "endpoints"  │  │ "Dragonfly," │
│ "UAA"        │  │ "16 adapters"│  │ "[External]" │  │ "Kraken"     │
└──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrNlb9u2zAQxnc9xYFrWQTt0CGbageN0aIRbAcdgg60dLZY0aRKUnG8FZk7ZBCKPEQeoU_jJylt2Zal-J_atA2h6fTdx9Mdf-Is_z7Lv_3xcwe1NXsa3y07ebP8Fkgbr1GoFDWcgB-NuSTzPW8fl-G0F60OhIKjtE7c6pDK68LuKkBtlPxMarnb7K56amgnTCP0psbiuExa242ZZCM0kGr1BUNr6J7qIhUmqCnEKMYUlGYLdcVO4zXHiQETMmkoXHb22CXZAAVaCoOMiyjhtlZdfqjBD0cM4f7x1vlx43toOO57r_6B5_1-0Dt1TYAX0D3r9QlsXQttmpmYQpoJQSo-P37Csctpvb92mP_Xs4TonOmB0gR-f5UA7YSisdUcV6YtH7LQgsYRN1ZPT0uWoPvWb9EFDJLLEXWaVPCQWa5kgc4GONZRP4_D10xZhxYYPiqyJjiIlUpc6F2LHKgqf4qj_Hyy1lAtmsQyG38kVXBWLcV1Q4FI5WYyJVXANMbILPF2_zNXQnUzfRmyMEayW-ftAbRBrCmwd43vq3-SUZzhThRUel70s4tjZZHUop-KMw0nZDMavA5gNagS2LMbi1oy4UgtpUvcOJrNaE-wMCH1EmoOhe2Hth9QuOi0W5TslC6jKKNUcWlNpdq2ZiMlh2JKN2wvfX9LE169ARax1Fmbg5u91yxBSZrchLUb7zlm_AKjj28W)

Not drawn at this level: the **object storage** backend (fs/S3/GCS/Azure) and
the bundled **Trivy scanner** — both sit *inside* the Harbor system boundary
and appear at Level 2. The pluggable-scanner API can also point at external
scanners, in which case they join this picture as another external system.

---

## 2. Container view (C4 level 2)

Deployable units (Docker Compose containers / Kubernetes workloads) inside
the Harbor system boundary, plus the two kinds of users:

<!--
```SVGBob
┌──────────────────────┐                    ┌────────────────────────────┐
│ "Developer / Admin"  │                    │ "OCI client / CI"          │
│ "[Person]"           │                    │ "[Software System]"        │
└──────────┬───────────┘                    └─────────────┬──────────────┘
           │ "HTTPS"                                      │
           ▼                                              │
┌──────────────────────┐                                  │
│ "Portal"             │                                  │
│ "[Container: nginx"  │                                  │
│ "+ Angular 16 SPA]"  │                                  │
└──────────┬───────────┘                                  │
           │ "/api/v2.0, /c/* XHR"                        │ "/v2/* + token"
           ▼                                              ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ "Core"                                                                       │
│ "[Container: Go, Beego]"                                                     │
│ "API gateway: authn/z, REST API, OCI façade, token service, controllers"     │
└───────────┬──────────────────────────┬───────────────────┬─────────────┬─────┘
            │ "jobs + status hooks"    │ "OCI ops"         │ "SQL"       │ "cache"
            ▼                          ▼                   ▼             ▼
┌──────────────────────┐  ┌──────────────────────┐  ┌────────────┐ ┌─────────┐
│ "JobService"         │  │ "Registry"           │  │"PostgreSQL"│ │"Valkey" │
│ "[Container: Go]"    │  │ "[Container:"        │  │"[Database]"│ │"[Cache]"│
│ "async jobs over"    │  │ "Distribution v3]"   │  │"metadata"  │ │"queues" │
│ "Redis queues"       │  │ "blobs + manifests"  │  └────────────┘ └─────────┘
└────┬──────────────┬──┘  └───────────┬──────────┘
     │ "scan"       │ "GC sweep"      │ "blob I/O"
     ▼              ▼                 │
┌──────────┐ ┌─────────────┐          │
│"Trivy"   │ │"RegistryCtl"│          │
│"adapter" │ │"[Container]"│          │
│"[scan]"  │ │"Vacuum"     │          │
└──────────┘ └──────┬──────┘          │
                    │                 │
                    ▼                 ▼
             ┌─────────────────────────────────────────┐
             │"Object storage  [fs, S3, GCS, Azure]"   │
             │"blobs + manifests"                      │
             └─────────────────────────────────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrNV71u2zAQ3v0UB46NULUJ0CGbqxSJiwJxLSMoIGig5bOiRBZdklLqTEHmDh6MwG_QxY-QrW_iJyll2Y7141iq7aIEIYDHu4_k8bvjaTb-ORs_7NxHUNBm-8EuWK02Gz8COcMIfTZADjrUu30vIPGaj8VbUfqXRgMc38NAKgOjQVLTCaTVRC5YYJOM7SZIy2Q9eUc5gjkUEvsvhgnkeNtRpiWOOyleflzaX9Mqzp3Usoe8aLebJoFSLT72-vDpGSq1xG2H42TRauqITcYl9UnugssjWAYLJPUC5KcQuF7wg1RFOIJ64IY-5fD-A5jNul0N4XBUe_2G463rdODp0fHbdxrojv4Gvl20yCv2yiA6VmpHINktBmQ3xjw91w6Waf7Pvsh_BuNIYD-tkMfnTIOPiC6zyW6g9WYDXCrxjg5PgYbyOtDvNWh9MtugpjSIM3OP_v5Fu6glnACBPPIcNXTUfjjzfZWYSRW6V817DwcGnO6gmcrJSQTdsI5QASQklaGAa8ZuE_esXjo2ECRtYn79QtbHDnWukdSgbPQVT2ale4zH0d5qiCpIo62qi_j7zDpmwtK0oxPvttD1hOTDXCmhPuq9EdLlGN9ILItFV9S_RaW9IRJtkoZfmyU5eOuMStqhAu0VvGXEtz0fJ_BUDAMH5jRiEfIM_Fm8d68TSo8FEJ3MV1_B91HSrlph8UDFou8hhijWdt_CridgKc46p-Mn9O3TwOuhkIKsZkuWNZOtqpN8lpj-VcBOym5rWrK2mrtAODRIheO5AeIOcUDWZLGfoKFfLqI0F4FFIVmyhBpViq1RLquTNvei4ZIXsWDJeEP6JFW4LC0UaQYyptoLKZcktostrNhL9hrPrqgThn2SL9BKvgobaTPdUg5lyp5Xq8SNukW3pdLlv_lrKl1cZM9CLjs36Ej11jBOXQSwekID80SDc8PUoH4fclwmiLxxUaiXqCur_eEcok_-AId_wC0=)

Complete relationship list (including edges left out of the drawing for
legibility):

| From | To | Protocol / credential | Purpose |
|---|---|---|---|
| Person | Portal | HTTPS | UI |
| Portal | Core | session + CSRF token | REST API, login |
| OCI client | Core | `/v2/*` bearer token from `/service/token` | push/pull |
| Core | Registry | basic auth / bearer | proxied OCI operations, registry client |
| Core | JobService | REST, `CORE_SECRET` | submit/stop jobs |
| JobService | Core | webhook, `JOBSERVICE_SECRET` accepted by `secret` security context | job status hooks, config |
| JobService | Trivy adapter | REST | scan requests (Trivy pulls layers back through Core with a robot credential) |
| JobService / Core | RegistryCtl | `Authorization: Harbor <JOBSERVICE_SECRET>` | GC sweep deletions |
| Registry, RegistryCtl | Object storage | storage driver | blob/manifest persistence |
| Core, JobService | PostgreSQL | SQL | metadata, job payload data |
| Core, JobService | Valkey | RESP | sessions, caches, job queues |
| Exporter | Core, PostgreSQL, JobService, Valkey | REST / SQL / RESP | Prometheus metrics (not drawn) |
| Core | IdP | LDAP bind / OIDC flows | authentication |

---

## 3. Package topology of the Go module

Everything except the Portal lives in one Go module (`github.com/goharbor/harbor/src`).
All binaries (`core`, `jobservice`, `registryctl`, `exporter`,
`standalone-db-migrator`) are thin entry points over the same strictly layered
package tree:

<!--
```SVGBob
┌─────────────────────────────────────────────────────────────────────────┐
│                        "src/core  (main binary)"                        │
│  "boot · auth providers · token service · session · /c/* controllers"   │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       "src/server  (HTTP layer)"                        │
│ ┌──────────────────┐ ┌──────────────────┐ ┌───────────────────────────┐ │
│ │ "v2.0/route"     │ │ "registry/"      │ │ "middleware/  (25 pkgs)"  │ │
│ │ "restapi (gen.)" │ │ "OCI /v2 proxy"  │ │ "security quota orm tx …" │ │
│ │ "handler (impl)" │ │ "+ per-op mw"    │ │ "blob session csrf log"   │ │
│ └──────────────────┘ └──────────────────┘ └───────────────────────────┘ │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                "src/controller  (33 domain controllers)"                │
│           "business logic · orchestration · publishes events"           │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                 "src/pkg  (one manager + DAO per domain)"               │
│    "Manager interface + dao/ subpackage · cached/ adds a Redis layer"   │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   "src/lib  (shared infrastructure)"                    │
│ "orm cache config errors log q selector metric trace http redis gtask"  │
└─────────────────────────────────────────────────────────────────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrtls9u00AQxu95itGeAhE1SsUDoHIoB1SE-gLr9cRexd51Z9duc6t65tBDhPIEnLhz4cSj5EmYdZzgklYJICUS2PIh2j9f1qPfft8s5x-X89t_970fLOd38MQjHKlIWUKAYSG1gVgbSbNn4qkNrLXSE7G1Hr5_BVn5DEqytU6QXBjxdooGHFKtFYYBh85pa8LPSEXPQVnjyeY5rxcbzflhy_LlsH-3GMA-TyjFfgs_feOa_b_kNuAGxJAY3fPLy_eQyxnSbnL_omr3R9u8Q3f9ZXcg6vHJy4hs5VGsP3s1QZhq52kWCXg4UegkyfFaEkZcyvErKKepC3VsVww6Es7LUsMwRXPCKzYTF2dvIarHwQZuZqKj7VBVpP0MrirrJVgqwN_A8vaz2FbPpOFzEAx1UeZd9RGUSC9sCcW1eHDyOLfxxlyUownkNhXw68n_2FkWR9u8Q7f3y94v9_TLNuHXgcs3_PQUEtuEfSeHt31zk_Q_peLKacPXLVwzrUKcW1IZmwJJ3-Z7WcW5djwGWKPxTmwp9tz23O6T8w24HEVMrDUIhTQyZX5H8Ob1RQiEFuItcDvcinftJm080kRyNzqCRNoIXBWXUk15MkCrJFOcRCCTxIGED5hot2on-va0x_Z329MG3FzHDK7LuKtKGL8JSXbJSvmK8PEWdY2tCD1SA2Sw54lOAYksNZ4LV9zt5Ki8JSjQE3swWy9TnXlfAjXYpl66qTgStQd-Fz8A73JWEw==)

### Dependency rules

- **`src/server` → `src/controller` → `src/pkg` → `src/lib`.** Lower layers
  never import higher ones; `src/pkg` never imports `src/controller`.
- **`src/server/v2.0/restapi` and `handler` bindings are generated** from
  `api/v2.0/swagger.yaml` (`task build:gen-apis`); only
  `src/server/v2.0/handler/*.go` implementations are hand-written.
- **`src/common`** is the legacy shared layer (security contexts, models,
  the RegistryCtl client, constants). New code goes to `src/lib`/`src/pkg`;
  `common` shrinks over time.
- **`src/jobservice` and `src/registryctl`** are sibling service trees, not
  layers; JobService reaches back into `src/pkg` for the job *payloads*
  (scan, retention, replication, task sweep) so business logic stays in one
  place.
- **`src/testing`** holds mockery-generated mocks (from `.mockery.yaml`);
  **`src/migration`** wraps golang-migrate over
  `make/migrations/postgresql/`.

Key `src/lib` packages, one line each:

| Package | Purpose |
|---|---|
| `lib/orm` | Beego-ORM wrapper: context-scoped transactions, query building |
| `lib/q` | Generic query/filter/pagination model used by all list APIs |
| `lib/cache` | Pluggable cache (`memory`, `redis`) behind one interface |
| `lib/config` | System configuration: defaults, DB-backed manager, REST manager for satellite services |
| `lib/errors` | Semantic error kinds (NotFound, Conflict, …) mapped to HTTP codes |
| `lib/log` | The only sanctioned logger (`log.Infof/...`) |
| `lib/selector` | Artifact selector/filter engine shared by retention, immutable, replication, preheat |
| `lib/metric`, `lib/trace` | Prometheus instrumentation, OpenTelemetry setup |
| `lib/http`, `lib/redis`, `lib/gtask` | Internal HTTP transports, Redis helpers, global goroutine task pool |

---

## 4. Harbor Core

Core (`src/core/main.go`) is a single Beego process hosting four distinct HTTP
personalities: the REST API, the OCI registry façade, the Docker token
service, and the legacy/UI controller endpoints.

### 4.1 Boot sequence

From `src/core/main.go` (in order):

1. Configure the Beego **session provider** (Redis-backed Harbor provider, from `_REDIS_URL_CORE`).
2. Initialize **lib/cache** (`_REDIS_URL_HARBOR`, falls back to the core Redis URL) and enable the config cache.
3. `config.Init()` — configuration manager (env + DB-backed).
4. Optional **Prometheus metrics** server; **OpenTelemetry** tracer; `token.InitCreators()` for the token service.
5. **Database init + migration** (`dao.InitDatabase`, `migration.Migrate`). The `-mode migrate` flag runs migrations and exits (used as a Helm pre-install/upgrade hook); `-mode skip-migrate` skips them.
6. Load config from DB, apply `CONFIG_OVERWRITE_JSON`, set the initial admin password.
7. `api.Init()` (legacy API layer), register **health checkers** and the default **Trivy scanner**.
8. Start the **gtask global pool**, the graceful-shutdown handler, and the registry **regular health check** loop.
9. Initialize **audit log forwarding** and the **webhook notification** subsystem.
10. `server.RegisterRoutes()` — all three route trees (§4.2).
11. Optional **internal TLS** (port 8443).
12. A background goroutine waits for JobService health, then schedules the system periodic jobs (`SYSTEM_ARTIFACT_CLEANUP`, `EXECUTION_SWEEP`) with retry.
13. `web.RunWithMiddleWares("", middlewares.MiddleWares()...)` — start serving.

### 4.2 HTTP surface and global middleware chain

`src/server/server.go` registers three route trees: miscellaneous service
routes (`src/server/route.go`), the OCI registry routes
(`src/server/registry/route.go`), and the v2.0 REST API
(`src/server/v2.0/route`). Every request first passes the global chain
assembled in `src/core/middlewares/middlewares.go`:

<!--
```SVGBob
                            "any HTTP request"
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────┐
│      "global middleware chain (applied to every route, in order)"       │
│                                                                         │
│  "url → mergeslash → trace → metric → requestid → session → csrf"       │
│  "→ orm → notification → transaction → artifactinfo → security"         │
│  "→ log → unauthorized → readonly"                                      │
└────┬────────────┬────────────────┬──────────────┬──────────────┬────────┘
     │            │                │              │              │
     ▼            ▼                ▼              ▼              ▼
┌─────────┐ ┌────────────┐ ┌───────────────┐ ┌──────────┐ ┌──────────────┐
│"/api/"  │ │ "/v2/*"    │ │ "/service/*"  │ │ "/c/*"   │ │"/api/version"│
│"v2.0/*" │ │"OCI proxy" │ │"token, hooks" │ │"login,"  │ │"+ internal"  │
│         │ │            │ │               │ │"OIDC cb" │ │              │
└─────────┘ └────────────┘ └───────────────┘ └──────────┘ └──────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrdVM1OwzAMvvcprJz4mSjaK4wDO8FhL5Cl3hqRJcNJC-OEduDEYUIV2kPsEXiaPglt125jK1qLkJCIWim2PzuO488A3y_G9QyuB4NbILyP0DrmQcOVJvPm2PcPL01e0-T5_36L7IbzsqpjZYZcwUQGgcIHTggi5FLDCZ9OlcQAnAGMkWZAJnLYgcxmKEA6ZTvV3cT7hbWJxyJSkL68wQRpjFZxGxaiIy6wNDiSotiWTSGDQrJorTS62AtLo4NcWW4xNCkQ2jg5koK7yiU7QVsuNjKnDJDLemTK-CIi6WasJuvcrsy4wEWaRy40JJ8wKNPkgdFqx7FBLZKvz7dq9sqr9o2x-gvw0quuunfzmmIcV3gVi_dIXcPz44qjk2ABjWdFC2g7n9aBC_ozn0-lz9ZFzH_mx13_jFV1XassUiwFFvqtVqxxpWIdKRsROeNYyQMWdy8uc1gFuun1YUrmcbZVOXOHugOhMXd2q82oI3Vnex47z0aOQ9JcscNhUyW11xWH3bPJo3_VAzFk9bBavu33KxxD_ATazqd14OUnQRRpGQ==)

| Group | Path | Handler |
|---|---|---|
| REST API | `/api/v2.0/*` | `apiversion` middleware → go-swagger `handler.New()` |
| OCI registry | `/v2/*` | per-operation middleware → proxy to Distribution (§4.5) |
| Token service | `/service/token` | `src/core/service/token` — Docker V2 bearer tokens |
| Job status hooks | `/service/notifications/tasks/:id` (+ legacy variants) | `handler.NewJobStatusHandler()` → `pkg/task` hook handler |
| UI controllers | `/c/login`, `/c/log_out`, `/c/userExists`, OIDC login/callback/onboard, AuthProxy redirect | Beego controllers in `src/core/controllers` |
| Internal | `/api/internal/renameadmin`, `/api/internal/syncquota` | `src/core/api.InternalAPI` |

Middleware worth singling out (all under `src/server/middleware/`):

- **`orm`** injects a DB session into the request context; **`transaction`**
  wraps non-GET requests in a transaction (skipped for blob-upload
  PATCH/PUT and `/service/token` to avoid long-lived transactions).
- **`notification`** collects events during the request and publishes them
  to the event bus *after* the transaction commits (it deliberately sits
  ahead of `transaction` in the chain).
- **`artifactinfo`** parses repository/reference out of `/v2/*` URLs into
  the context for everything downstream.
- **`security`** builds the security context (§4.3); **`log`** runs after it
  so log lines carry the user; **`readonly`** rejects writes when the system
  is read-only (with skippers so you can still log in and flip the config
  back).

### 4.3 Authentication: security context generators and auth providers

The `security` middleware tries a fixed list of context generators
(`src/server/middleware/security/security.go`); the first that recognizes
the request wins:

1. `secret` — internal service-to-service secret (Core↔JobService↔RegistryCtl)
2. `oidcCli` — OIDC CLI secret (docker login with OIDC user)
3. `v2Token` — registry bearer token minted by Core's own token service
4. `idToken` — raw OIDC ID token
5. `authProxy` — auth-proxy tokens
6. `robot` — robot accounts
7. `basicAuth` — username/password
8. `session` — browser session (Portal)
9. `proxyCacheSecret` — proxy-cache pull-through requests

Independent of *how* the request authenticated, *who* it is gets resolved by
the pluggable authenticators in `src/core/auth/`: `db` (local), `ldap`,
`oidc`, `uaa`, `authproxy`. Authorization is project/system-scoped RBAC via
Casbin (`src/pkg/permission`).

The **token service** (`src/core/service/token`) is what makes Docker login
work: the Distribution registry is configured to trust tokens signed by
Core, so `/service/token` authenticates the user, filters the requested
scopes through RBAC, and returns a signed JWT the client replays to `/v2/*`.

### 4.4 Component view (C4 level 3)

The classic [C4 component diagram](https://c4model.com/diagrams/component)
zoom: components inside the Core container, the containers it talks to, and
the relationships between them. Component ↔ package mapping is 1:1 with the
source tree.

<!--
```SVGBob
   "OCI client [System]"     "Portal [Container]"
             │                         │
             │ "/v2/* OCI"             │ "/api/v2.0, session"
             ▼                         ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ "Harbor Core  [Container: Go, Beego]"                                      │
│                                                                            │
│ ┌──────────────────────────────────────────────────────────────────────┐   │
│ │ "Global middleware chain  [Component: src/server/middleware]"        │   │
│ │ "authn (9 generators), session, CSRF, tx, readonly, audit events"    │   │
│ └──────────┬─────────────────────────┬────────────────────────┬────────┘   │
│            │                         │                        │            │
│            ▼                         ▼                        ▼            │
│ ┌──────────────────────┐  ┌──────────────────────┐  ┌────────────────────┐ │
│ │ "OCI façade"         │  │ "REST API handlers"  │  │ "Token service"    │ │
│ │ "[server/registry]"  │  │ "[v2.0/handler]"     │  │ "[core/service]"   │ │
│ │ "per-op policy mw,"  │  │ "go-swagger impls"   │  │ "signs registry"   │ │
│ │ "proxy to Registry"  │  │ "of swagger.yaml"    │  │ "RBAC-scoped JWTs" │ │
│ └──────────┬───────────┘  └──────────┬───────────┘  └────────────────────┘ │
│            │                         │                                     │
│            ▼                         ▼                                     │
│ ┌────────────────────────────────────────────────┐  ┌────────────────────┐ │
│ │"Domain controllers  [src/controller, 33 pkgs]" │  │ "Auth providers"   │ │
│ │ "business logic; publishes events; calls task" │  │ "[core/auth]"      │ │
│ └──────────┬─────────────────────────┬───────────┘  │ "db ldap oidc uaa" │ │
│            │                         │              │ "authproxy"        │ │
│            │                         │              └────────────────────┘ │
│            ▼                         ▼                                     │
│ ┌──────────────────────┐  ┌──────────────────────┐  ┌────────────────────┐ │
│ │ "Managers + DAOs"    │  │ "Event bus+handlers" ├─►│ "Task + scheduler" │ │
│ │ "[src/pkg]"          │  │ "[pkg/notifier +"    │  │ "[pkg/task,"       │ │
│ │ "per-domain CRUD,"   │  │ "controller/event]"  │  │ "pkg/scheduler]"   │ │
│ │ "cached/ hot reads"  │  │ "webhook audit repl" │  │ "executions, crons"│ │
│ └──────────┬───────────┘  └──────────────────────┘  └─────────┬──────────┘ │
│            │                                                  │            │
└────────────┼──────────────────────────────────────────────────┼────────────┘
             │ "SQL + cache"                                    │"submit jobs"
             ▼                                                  ▼
   "PostgreSQL + Valkey"                          "JobService [Container]"
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrlmEFP2zAUx-98iiefthHINE6DUymMgTYBLdsOVQ-OY1KvThzZSUtvaOcdeqhQD7vvwieYuO2b9JPMTkPT0FAKC6zbrEqV-ty_HT___H8xAKDD6j4QzmgQQaPeUxH1mwhMQ0dCRphDoyqCCLOAyiZagek2GnyB25qOzXZGdueV_QL0kKgghkOm4-svLVBUKSaCm8NdXN0-3MXVymjwdTQ4_w8-_ZVkwd5i6QgJVSEpTGVpE_aEBduUeqKJYKFmkjUvmQ9o15L_aFL6uUfUydjjwtGw-Mx1Oe1inRLS0ulIEuOHItB8bYKSxFZUdqi0s45ZksYpyMniOGoF8Ow1eFSnFkdCqucTPiyo1mtvLIjOLJAUuyLgPQtw7LIIaEePqFCR7OCuh7ssdakun0RqWLyN7ziiFgwVCc8_jBYMlUxJH5ZJqZ_fysZoTvHP79ilKL_QSbi2Wz-BytE-tHCgyZBm72bRE9GmARh2GKGTXZ3Tb6RkSeoxFcleMyfQMM5ip9opclNRog9RO5VPojPyIZVrIoRQcEZ64HetnLwn1lQXex6VwPyQK5STV8wLFFxPrFheirMeRAJqWa9MQJxCKr_ewz5HN5Zuu1JdU0SE1IWDTyd68Lx8KcAP4WmV7tIoD3Z4HNT_HjssHXa0I3xjfkTXJFJwQ7M2QuN-2S8WbGxA2PZUE01t5Yr2O9AsdJg7PgJmSXFipcscpYALj5EtCGOHM9WiKvW8LSCYcwURVm00w7gx1GvHfQRKyrXFYTp11wHu4hAEcwnEGOcBfygBkwIjOXtyVcjvSz8K38sH4lI77nscYM-wtwo7lUN1wzV2DS2gaVrNHHc0-GaELn6MTVcTpP-rSIu6se6AikxXQ60pnn7TmEJOR-xAROyUaWNcRQVRQ6mFoAjI1HTd8VlSrX3YsfK2mh0mdoJ-3vGN-mTqxZ5OsInb0BJRUkDna44udVpCtNOCWtKQTx8n9IySONJVuLKASP2F_pjpnpejdFm66cK96usFn_NqWd8JF5zYsOBepH78ToOWbEe04PohFTu-3pefhaPucVEy9wZlfOujIk_S8Yw-Yt6mvTlTQgfCqY_L5txF0S9oUIFR)

| Component | Package(s) | Responsibility |
|---|---|---|
| Global middleware chain | `src/server/middleware` (25 pkgs) | request pipeline: security context, ORM/tx, CSRF, read-only, post-commit event publication |
| OCI façade | `src/server/registry` | `/v2/*`: per-operation policy middleware, reverse proxy to Registry; local handlers for catalog/tags/referrers |
| REST API handlers | `src/server/v2.0/handler` | go-swagger implementations of `api/v2.0/swagger.yaml` |
| Token service | `src/core/service/token` | Docker V2 auth: RBAC-filters requested scopes, signs registry JWTs |
| Auth providers | `src/core/auth/{db,ldap,oidc,uaa,authproxy}` | pluggable identity backends, used by middleware + token service |
| Domain controllers | `src/controller/*` (33) | business logic; publish events; create executions/tasks |
| Managers + DAOs | `src/pkg/*` (+ `pkg/cached`) | per-domain persistence; Redis-wrapped hot reads |
| Event bus + handlers | `src/pkg/notifier`, `src/controller/event` | in-process pub/sub; webhook/audit/replication/p2p/internal reactions |
| Task + scheduler client | `src/pkg/task`, `src/pkg/scheduler` | execution/task model, cron schedules, JobService REST client, status-hook handler |

Not drawn: the **registry client** (`src/pkg/registry`, §10) used by
controllers and the OCI façade's local handlers, and the `/c/*` Beego
login/OIDC controllers (`src/core/controllers`, route table in §4.2).

The 33 controllers in `src/controller/` cluster into eight functional groups:

| Cluster | Controllers | Notes |
|---|---|---|
| Artifact & content | `artifact`, `repository`, `tag`, `blob`, `icon`, `systemartifact` | `artifact` is the hub; depends on `tag`, `repository`, `blob`, processors per media type |
| Scanning & security | `scan`, `scanner`, `securityhub`, `scandataexport` | `scan` drives pluggable scanner adapters (`pkg/scan`), reports + SBOM |
| Replication & registries | `replication`, `registry`, `proxy` | `proxy` implements proxy-cache projects; `registry` manages endpoints + health |
| Policy enforcement | `retention`, `immutable`, `quota` | all built on `lib/selector` rules |
| Identity & access | `user`, `usergroup`, `robot`, `member`, `ldap` | |
| Projects & system | `project`, `config`, `systeminfo`, `health` | |
| Async-task plumbing | `task`, `jobservice`, `jobmonitor`, `gc`, `purge` | thin orchestration over `pkg/task` (§6) |
| Eventing & integrations | `event`, `webhook`, `p2p` | `event` defines topics + metadata; `p2p` = preheat providers (Dragonfly, Kraken) |

**Event subsystem.** Controllers publish topic events through
`src/pkg/notifier` (an in-process pub/sub bus). Subscribers live in
`src/controller/event/handler/`: `webhook` (HTTP/Slack notification
policies), `auditlog`, `replication` (event-based replication triggers),
`p2p` (preheat on push/scan), and `internal` (artifact pull-time/count
updates). Handlers that need to do real work enqueue a job via `pkg/task`
rather than blocking the request — the `notification` middleware guarantees
events only fire after the DB transaction commits.

**Caching.** `src/pkg/cached` wraps the managers for `artifact`,
`manifest`, `project`, `project_metadata`, and `repository` with a
`lib/cache` layer (Redis in production) — these are the hot paths on every
pull. The manifest cache is what lets a pull avoid hitting the registry at
all on a hit.

### 4.5 The OCI proxy (`/v2/*`)

`src/server/registry/route.go` mounts every OCI distribution operation
under a `v2auth` middleware (token/scope check) and stacks per-operation
policy middleware before either a local handler or the reverse proxy to
Distribution:

| Operation | Middleware (in order) | Terminal |
|---|---|---|
| `GET /v2/_catalog` | `metric` | local catalog handler (DB-backed) |
| `GET …/tags/list` | `metric` → `repoproxy` | local tag handler (DB-backed) |
| `GET/HEAD …/manifests/:ref` | `metric` → `repoproxy` → `contenttrust` → `vulnerable` | manifest handler (cache → registry) |
| `PUT …/manifests/:ref` | `metric` → `repoproxy` (disable for proxy projects) → `immutable` → `quota` → `cosign` → `subject` → `blob` | proxy to registry |
| `DELETE …/manifests/:ref` | `metric` → `quota` (refresh) | local delete |
| `HEAD …/blobs/:digest` | `metric` → `blob` | proxy |
| `GET …/blobs/:digest` | `metric` → `repoproxy` | proxy |
| `POST …/blobs/uploads` | `metric` → `repoproxy` → `quota` → `blob` | proxy |
| `PATCH/PUT …/blobs/uploads/:sid` | `metric` → `blob` (+ `quota` on PUT) | proxy |
| `GET …/referrers/:ref` | `metric` | local referrers handler |

Notable: `repoproxy` implements **proxy-cache projects** (pull-through from
an upstream registry); `cosign`/`subject` link signature artifacts to their
subjects (OCI referrers); `quota` enforces project quotas *before* bytes
reach the registry; `contenttrust` and `vulnerable` are pull-side deployment
security policies.

Tag list and catalog are answered **from Harbor's database**, not the
registry — the registry's filesystem view and Harbor's metadata are kept
consistent by Core being the only writer.

---

## 5. Request flows: login, push, pull (C4 dynamic views)

C4 dynamic diagrams for the three flows every Harbor request boils down to.
All client traffic passes the global middleware chain first (see
[§4.2](#42-http-surface-and-global-middleware-chain));
the steps below show what happens after that.

### 5.1 Docker login

<!--
```SVGBob
 ┌────────────┐          ┌────────────┐          ┌────────────┐
 │"OCI client"│          │   "Core"   │          │ "IdP / DB" │
 └─────┬──────┘          └─────┬──────┘          └─────┬──────┘
       │                       │                       │
       │ "1 GET /v2/"          │                       │
       ├──────────────────────►│                       │
       │ "2 401 + token URL"   │                       │
       │◄- - - - - - - - - - - ┤                       │
       │ "3 GET /service/token"│                       │
       │ "(basic auth, scopes)"│                       │
       ├──────────────────────►│                       │
       │                       │ "4 authenticate"      │
       │                       ├──────────────────────►│
       │                       │ "5 identity"          │
       │                       │◄- - - - - - - - - - - ┤
       │                       │ "6 filter scopes via" │
       │                       │ "RBAC, sign JWT"      │
       │ "7 200 {token}"       │                       │
       │◄- - - - - - - - - - - ┤                       │
       │                       │                       │
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNpTeDSl59GUBiLQBAU4oK0eLqDaJiV_Z0-F5JzM1LwSJSAX2RwQR8k5vyhVCc5FklPyTAlQ0FdwcVICcUFmTcGwYg02e2cgm0NLPVwoPsEG8Moga1cyVHB3DVHQLzPSVyJZ-xzi4gMvmraLeLcaKZgYGCpoK5TkZ6fmKYQG-SiR4NVH01t0FbDBR1OWEGm_MSSsilOLyjKTU_XBzlAi3vkaSYnFmckKiaUlGToKxcn5BanFmkqDM6hxpyolE7AHgLkqMzmxJFWJJO1U8wWRbjVVyEwBubSkUol0r-JLMETab6aQlplTkloEjW2FssxEJRKCOsjJ0RmYUjLT8xS8wkOwB7WSuYKRgYFCNTgx1iqRmP8pzxRklD8AU1A7rQ==)

The client caches the JWT and replays it as a bearer token on every `/v2/*`
request; the `v2auth` middleware (and the Registry itself) validate the
signature and scopes. Which backend step 4 hits depends on the configured
auth mode (`db`, `ldap`, `oidc` CLI secret, `uaa`, `authproxy`).

### 5.2 Image push

<!--
```SVGBob
 ┌────────────┐          ┌────────────┐          ┌────────────┐          ┌────────────┐
 │"OCI client"│          │   "Core"   │          │ "Registry" │          │"PostgreSQL"│
 └─────┬──────┘          └─────┬──────┘          └─────┬──────┘          └─────┬──────┘
       │                       │                       │                       │
       │ "1 POST blobs/uploads"│                       │                       │
       ├──────────────────────►│                       │                       │
       │                       │ "2 quota + blob mw"   │                       │
       │                       │ "3 proxy upload"      │                       │
       │                       ├──────────────────────►│                       │
       │ "4 PATCH/PUT chunks"  │                       │                       │
       ├──────────────────────►│ "(proxied)"           │                       │
       │                       ├──────────────────────►│                       │
       │ "5 PUT manifest"      │                       │                       │
       ├──────────────────────►│                       │                       │
       │                       │ "6 immutable, quota," │                       │
       │                       │ "cosign/subject mw"   │                       │
       │                       │ "7 proxy PUT manifest"│                       │
       │                       ├──────────────────────►│                       │
       │                       │ "8 record artifact, tags, blobs; fire events" │
       │                       ├──────────────────────────────────────────────►│
       │ "9 201 Created"       │                       │                       │
       │◄- - - - - - - - - - - ┤                       │                       │
       │                       │                       │                       │
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrVVb1OwzAQ3vsUp5tABJWWfzGhLCAhNdDyAE56DYYkLrYDdEOIkYEhKgyMjDwBj5MnoWmhStRSVcVUrXPL6XyffZ_vy0GaPKXJ_RT2DMO1eDml3t4HrNnH4AWcIo09N4-TOWgLSTh0czE8I58rLTs4EkNHKO1Lqp-eZJjZOcnI8R_j7vSax1m0nFKBmXFrxkgeGCvg1OoNcAPhqnLcDgRrKvwz8Nt0PTHRup8G6vs9HatwHQvNYK1fPIS3aAh4E9pS3HVgwCYauvEcKC00xhY4hw37qOycN8C7iKMrhQY6zlwVuJKxzKm5iiYbY940b0NGcMgi3iKl0ZCwl0F_O8DDMNbMDcgaSNFCI8CeUNyPyip2L8nTBoW9-y3swostW8dNqG8PJHlCNoFJzVvM0xZo5itrMB0OoMUlAd30ZrfCeVZnyvosFdS3D9WNCtiSmKafP7WBsZq-PK7DuC9N3v9PUTNEvgA0mOYg)

Step 2 reserves quota *before* bytes reach the Registry; step 6 is the
per-operation middleware stack on `PUT manifest`
([§4.5](#45-the-oci-proxy-v2)). Step 8's
events run after the DB transaction commits and fan out to webhook /
replication / p2p-preheat / auto-scan handlers, which enqueue JobService
jobs.

### 5.3 Image pull

<!--
```SVGBob
 ┌────────────┐          ┌────────────┐          ┌────────────┐          ┌────────────┐
 │"OCI client"│          │   "Core"   │          │  "Valkey"  │          │ "Registry" │
 └─────┬──────┘          └─────┬──────┘          └─────┬──────┘          └─────┬──────┘
       │                       │                       │                       │
       │ "1 GET manifests/:tag"│                       │                       │
       ├──────────────────────►│                       │                       │
       │                       │ "2 policy middleware" │                       │
       │                       │ "(trust, vuln, proxy)"│                       │
       │                       │ "3 manifest cache?"   │                       │
       │                       ├──────────────────────►│                       │
       │                       │ "4 hit / miss"        │                       │
       │                       │◄- - - - - - - - - - - ┤                       │
       │                       │ "5 on miss: GET manifest"                     │
       │                       ├──────────────────────────────────────────────►│
       │                       │ "6 write back"        │                       │
       │                       ├──────────────────────►│                       │
       │ "7 manifest"          │                       │                       │
       │◄- - - - - - - - - - - ┤                       │                       │
       │ "8 GET blobs/:digest" │                       │                       │
       ├──────────────────────►│ "(proxied)"           │                       │
       │                       ├──────────────────────────────────────────────►│
       │ "9 blob bytes"        │                       │                       │
       │◄- - - - - - - - - - - ┤                       │                       │
       │                       │                       │                       │
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrNVr1OwzAQ3vsUp5taqajiH7owVAgxISHE7iQmterGVexSsiHEyMAQAQMjI0_A4-RJ6niIUmGQ1VhtnFtOl_t8vu_8yVDkL0X-6GCvUK325XT0v094NbqEkDOaKNRuHad0cCRSipVbj-Et4ROaoSWG1zRmUqU6qN1yn_zX9t-2mj7qOG3L6ax0xrbWjNSBcRcuzm9gShJ2R6WSg6EiMTYG_nSbiX_t7cfD-f5Oxz2YCc7CDKYsijhdkHLyfAB3VTqXqg_3c570YZaKh6yHPoD3K54gJOGYnqGPVmyAK8fzHcCYKRhoQqREf1QX7887YPuK_KtxzYcgElPwcOUi4bYZ8WWGWcdeHMEiZYpCQMKJP_42PJ94bCOxuRStN4WONZ-Y4Qu4CLSCRyw2xbdIxbFbyiCjUQ99qnirLwqeGkIgyBR1lLOtD5LXd8YSDfr0mg==)

A cache hit at step 4 skips the Registry entirely for the manifest. Step 2
is where deployment-security policies (`contenttrust`, `vulnerable`) can
reject the pull, and where proxy-cache projects (`repoproxy`) divert to the
upstream registry instead. Pull-time/pull-count updates and the audit event
happen asynchronously after the response.

---

## 6. The async task framework (Core ↔ JobService contract)

Everything asynchronous funnels through two Core-side packages:

- **`src/pkg/scheduler`** — persistent cron schedules (DB table `schedule`),
  each backed by a periodic JobService policy running the `SCHEDULER` job,
  which calls back into a registered Core callback.
- **`src/pkg/task`** — the execution/task model. An **execution** is one
  logical run of something (a GC, a replication, a scan-all); it fans out
  into one or more **tasks**, each mapped 1:1 to a JobService job. Both are
  DB rows whose status is updated by job status webhooks arriving at
  `/service/notifications/tasks/:id`. `ExecutionSweeper` (job `EXECUTION_SWEEP`)
  retires old rows.

The full round trip:

1. A controller (e.g. `controller/gc`) creates an execution + tasks via `pkg/task.Manager`.
2. `pkg/task` POSTs to JobService `POST /api/v1/jobs` with job name, params, kind (generic/scheduled/periodic) and the status-hook URL.
3. JobService runs the job (§7) and fires status webhooks back to Core.
4. The hook handler updates the task row; execution status is refreshed from its tasks.
5. UIs and APIs read progress from the `execution`/`task` tables — never from JobService directly (the `jobmonitor` controller is the exception, for queue/worker introspection).

---

## 7. JobService

JobService (`src/jobservice/main.go`) is a Redis-backed job runner built on
Harbor's fork of gocraft/work (`github.com/goharbor/work`). Bootstrap
(`runtime/bootstrap.go`) wires: stats manager (`mgt`), hook agent, lifecycle
controller (`lcm`), worker pool (`worker/cworker`), periodic scheduler
(`period`), optional sync worker (`sync`), the API server, and an optional
metrics server.

### 7.1 Component view (C4 level 3)

<!--
```SVGBob
                        "Core (pkg/task) [Container]"
                                      │
                                      │ "POST /api/v1/jobs"
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ "JobService  [Container: Go]"                                              │
│                                                                            │
│ ┌──────────────────────────────────────────────────────────────────────┐   │
│ │ "API server  [Component: api/]"                                      │   │
│ │ "REST /api/v1; secret auth"                                          │   │
│ └───────────────────────────────────┬──────────────────────────────────┘   │
│                                     │                                      │
│                                     ▼                                      │
│ ┌──────────────────────────────────────────────────────────────────────┐   │
│ │ "Controller  [Component: core/]"                                     │   │
│ │ "validates request; routes by job kind"                              │   │
│ └──────────┬─────────────────────────┬─────────────────────────────────┘   │
│            │ "generic + scheduled"   │ "periodic (cron)"                   │
│            ▼                         ▼                                     │
│ ┌──────────────────────┐  ┌──────────────────────┐  ┌────────────────────┐ │
│ │ "Worker pool"        │  │ "Period scheduler"   │  │ "Stats manager"    │ │
│ │ "[worker/cworker]"   │  │ "[period/]"          │  │ "[mgt/]"           │ │
│ │ "N workers over"     │  │ "cron policies in"   │  │ "job stats CRUD"   │ │
│ │ "goharbor/work"      │  │ "Redis sorted set"   │  │ "in Redis"         │ │
│ └──────────┬───────────┘  └──────────────────────┘  └────────────────────┘ │
│            │ "run job"                                                     │
│            ▼                                                               │
│ ┌──────────────────────┐  ┌──────────────────────┐  ┌────────────────────┐ │
│ │ "Job implementations"├─►│ "LCM tracker"        ├─►│ "Hook agent"       │ │
│ │ "[job/impl/, pkg/*]" │  │ "[lcm/]"             │  │ "[hook/]"          │ │
│ │ "GC scan replication"│  │ "status transitions" │  │ "async webhook"    │ │
│ │ "retention webhook …"│  │ "+ restore loop"     │  │ "retry + backoff"  │ │
│ └──────────────────────┘  └──────────────────────┘  └─────────┬──────────┘ │
│                                                               │            │
└───────────────────────────────────────────────────────────────┼────────────┘
                                                                │"status hooks"
                                                                ▼
                                    "Core /service/notifications/tasks/:id"
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrtmM2O2jAQx-88xcin3W6lqFf2VNFq26ofK2jVA-LgJAZcgp3aDituqOceOCDEoeeeeIKKp-FJOnagJHysAo1Wu-1aSAh5-E88Mz-PY4D9g9SkYnAW9zqeobp3Ds2aFIZywVSLVKDQWE6-FbcEcv2h8RE8GnNv8Mz7In1d2M90UVlOvi8no__gM664YL2RfoOpAQ8YZFJThSvZInDUsFmykiWOteQ_mpRxbomYjOfXr0FjNphyyejHUjBhqmBruWg60hTkZOsvN0BcooNAMQM0MV1yVCpyspM7jNP8rhzNjizjwuV-lOh08UjHPjrs7qRkFG3TEWCHKYrHHjoGNOIhNUyDYl8Tps0lKJnY3_4QsH1Aj4uQlEzH_AECcoAOF8QOw67BA7gAHXRZmETMhczNxTgjQ5w8C5QU56QYH7dhUBSRkgkZw31SGufL-LNUPSQjljIiubJMz0QuB3-So0hutmGo0dCngnbSKVjJbuSbN07fC9LvVl6gmeY4R2Fmtt8xeUB35N9DqqtBDlaPkBGwdYMri3jAEUsu8s4to9qtoFb_9ILsk-_ILlW-VJ71Qrafr85CrkFLZRiGiJm8PBfgDMiBpy-F9RmU1VPLUJod5Fwlwu6J5K-Oc6c0u0fG08M68H4csT42Pmq4FJosJz-s6fSXs3hbewdG0aC3hshFKGvxSsoeIOfCkEMwNjHDnvXjPQX74vYE2c3QHAX97Xabme2i_s4-kJO_quE-RAW22xiJdssgGwFLcqLtIoTm6RIz8lQPRQA3zLdu9u9UeLjFxeEf12awHP3MOLhAx9rYt9JIynh7q8F_qyG2MR9jKNttcjTto_tG-_wk2k8jcRfMyQM_hS4K5qFSwrvuuvZt0Ra-tLj9OqOIXXpF4-n0GsAT0vD2ikvtLm20V-Uh-Q0tVCRN)

Not drawn: **Redis** is used by the worker pool (queues), period scheduler
(policies), stats manager, and LCM (job stats) — every component except the
API server and hook agent; and the **logger service** (`logger/`, backends
`STD_OUTPUT`/`FILE`/`DB`) which the API server reads for `GET /jobs/{id}/log`.

### 7.2 Runtime flow

How a job moves through those components — enqueue, Redis queues, dequeue,
and the status feedback loop back to Core:

<!--
```SVGBob
          "Harbor Core"                            "Harbor Core"
       "(pkg/task client)"                "(/service/notifications/...)"
                │                                          ▲
                │ "POST /api/v1/jobs"                      │ "status webhook"
                ▼                                          │
┌──────────────────────────────────┐        ┌──────────────┴───────────────┐
│ "API server (api/)"              │        │ "hook agent (hook/)"         │
│ "secret auth (JOBSERVICE_SECRET)"│        │ "async delivery + retry"     │
└───────────────┬──────────────────┘        └──────────────▲───────────────┘
                │                                          │
                ▼                                          │ "status changes"
┌──────────────────────────────────┐        ┌──────────────┴───────────────┐
│ "controller (core/)"             │        │ "LCM (lcm/) + tracker"       │
│ "validate, route by job kind"    │        │ "job stats, restore loop"    │
└───────┬───────────────────┬──────┘        └──────────────▲───────────────┘
        │                   │                              │
        ▼                   ▼                              │
┌───────────────┐   ┌────────────────┐                     │
│ "worker pool" │   │ "period (cron)"│                     │
│ "cworker over"│   │ "scheduler +"  │                     │
│"goharbor/work"│   │ "policy store" │                     │
└───────┬───────┘   └───────┬────────┘                     │
        │                   │                              │
        ▼                   ▼                              │
┌──────────────────────────────────┐                       │
│ "Redis: queues, stats, policies" │                       │
└───────────────┬──────────────────┘                       │
                │ "dequeue"                                │
                ▼                                          │
┌──────────────────────────────────┐                       │
│ "runner → job.Interface.Run()"   ├───────────────────────┘
│ "17 registered job types"        │
└──────────────────────────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrdlr9u2zAQxnc_xYGTjBQWMhXo1hoG6qBFAjvoWtDUxWatkipJOdBWZOjUISgEw0MfwVOQsU-jJympOLaiyH9kF2lawoMhkd8dP_54J4D7Qd5SNZAK2lIhgQ3jwcTG_UMvGg99Q_UYWMhRmOYjDeL5GtWEM_SFNPyCM2q4FNpvtVrNpdByZOkV7Dyy6U2lADk77Z-DTyPuT479T3Kg12wtn6wNNbGGSxyMpBxXpDT9VSOl9KqRpd-z9OuT_K5XcWvFvK0bp5Fb9fqsC-40UYHn3C0fd-H08vnOUKBDCwZ47n9xwZ1Tzn9kCg3Q2IzAOzl90-_0PnTbnY_9TrvXOW-SsijViWAQYMhtHgkcgV2tElIQTWvubr6v_bNVYnViTm_qxjnwnlhXDqV6eU_YiIohavIfY86kMEqGocOc2XpX5rxM5Lv2e_BC9tlvWhqNomyMipQxn9CQB9TgC1AyNgiDBGxlgjEXAakSdS-d5douQG1sGhBKGZHtmM8PcHr-PDCvxnsr9EXQqwHfiv1-Bfy6PpslsiuSsBBcSmVZgkjKkCy2nz-PUHEZWDqVFA8qZLUMW-hIWzFJQUazEQax4_yIrLd3IUOGcpR_A_hOrSgTyZCzBHJIyRaZutDONmI336E2ryfkH4Ps8PK5jo8eBly_gi8xxmjrzaLs5MfKbanf4Mnfari7dDi3swDzTZGnaJHP6khVLIS919m3H67NtLrCoLqgDFu9WHh5P8vSn38omdldyOOXtlMNubaRMMibm0kiXH1778fK_ln9Bl4Rows=)

### 7.3 Components

| Component | Package | Role |
|---|---|---|
| API server | `api/` | `POST/GET /api/v1/jobs`, job actions (stop), logs, periodic executions, `/stats`, `/config`; secret-authenticated |
| Controller | `core/` | Facade: validates the request, routes by kind — generic → enqueue, scheduled → delayed enqueue, periodic → cron policy |
| Worker pool | `worker/cworker/` | N workers over the gocraft/work fork; reaper cleans dead jobs; de-duplicator enforces unique jobs |
| Periodic scheduler | `period/` | Stores cron policies in a Redis sorted set, continuously enqueues due executions |
| LCM | `lcm/` | Per-job trackers; persists status transitions atomically (Lua scripts in `common/rds`); restoration loop retries lost updates |
| Hook agent | `hook/` | Async delivery of `StatusChange` webhooks to the Core status-hook URL, bounded concurrency + exponential backoff |
| Stats manager | `mgt/` | Redis-backed job stats CRUD, cursor pagination |
| Logger | `logger/` | Pluggable backends `STD_OUTPUT`, `FILE`, `DB`, with sweepers and getters (the API's log endpoint reads from here) |
| Sync worker | `sync/` | Reconciles Core's DB-side schedules/executions with the Redis policy store at startup |

### 7.4 Registered job types

Mapped to implementations at bootstrap; payload code mostly lives in
`src/pkg/*` so Core and JobService share domain logic:

| Job name | Implementation | Purpose |
|---|---|---|
| `GARBAGE_COLLECTION` | `job/impl/gc` | Two-phase GC: mark in DB, sweep via RegistryCtl |
| `REPLICATION` | `job/impl/replication` | Transfer artifacts using `pkg/reg` adapters |
| `IMAGE_SCAN` | `pkg/scan` | Vulnerability/SBOM scan via scanner adapter |
| `RETENTION` | `pkg/retention` | Tag retention policy run |
| `SCHEDULER` | `pkg/scheduler` | Periodic wrapper that triggers Core-side callbacks |
| `WEBHOOK`, `SLACK` | `job/impl/notification` | Outbound notification delivery |
| `P2P_PREHEAT` | `pkg/p2p/preheat` | Push images into Dragonfly/Kraken |
| `AUDIT_LOGS_GDPR_COMPLIANT` | `job/impl/gdpr` | Mask user data in audit logs |
| `PURGE_AUDIT_LOG` | `job/impl/purge` | Audit log retention |
| `SCAN_DATA_EXPORT` | `job/impl/scandataexport` | CSV export of scan results |
| `SYSTEM_ARTIFACT_CLEANUP` | `job/impl/systemartifact` | Remove expired system artifacts |
| `EXECUTION_SWEEP` | `pkg/task` | Retire old execution/task rows |
| legacy schedulers, `DEMO` | `job/impl/legacy`, `job/impl/sample` | Deprecated / testing |

All Redis keys are namespaced `{harbor_job_service_namespace}:…` — queues
per job type, job stats hashes, periodic policy sets, and the status-change
retry list.

---

## 8. RegistryCtl

RegistryCtl (`src/registryctl/main.go`) exists because the Distribution V2
API has no deletion primitive suitable for GC. It runs next to the registry
(same container image set, sidecar in Kubernetes), loads **the registry's
own `config.yml`** to open the same storage driver, and exposes a tiny
secret-authenticated deletion API wrapping `storage.Vacuum`:

<!--
```SVGBob
         "Core & JobService (GC sweep)"
                       │
                       │ "DELETE /api/registry/blob|manifest"
                       │ "Authorization: Harbor <JOBSERVICE_SECRET>"
                       ▼
┌──────────────────────────────────────────────┐
│             "registryctl :8080"              │
│                                              │
│ ┌──────────────────────────────────────────┐ │
│ │ "router → secret auth → trace → log"     │ │
│ └────────────────────┬─────────────────────┘ │
│                      ▼                       │
│ ┌──────────────────────────────────────────┐ │
│ │ "handlers: blob, manifest, health"       │ │
│ └────────────────────┬─────────────────────┘ │
│                      ▼                       │
│ ┌──────────────────────────────────────────┐ │
│ │ "docker/distribution storage.Vacuum"     │ │
│ └────────────────────┬─────────────────────┘ │
└──────────────────────┼───────────────────────┘
                       ▼
    "storage backend (fs, S3, GCS, Azure)"
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrtVc1Kw0AQvvcphj1IC8EUvJQiQo1LtQhCU7zKJpk2i2m2zG4Uiwfx4MlDkSB9CB-hT5MnMSmtFn_ioRUq-LGHXZZZZr5vvlmABZijCGEHOspzka6kj1BtO6CvEUc1VoGvkaX3JVfAjvgp73GwxUjahAOpDd3YXqS826GIZR-1YaXxrcSEiuRYGKniJhwL8hTBfufs0OXd8xOHX7jc6fLewffPPM8qWfqYpXdbsyaVorZVsCU3vomg2ag36uwzzx-DfsQyaAvKn6wkk8tKKjFIkD08gUaf0IDIhZ6fDYm89YpdpAbsrRPew9P1MnlZu5ZpuRp5w_05PUIRBxGSbkLhTQuW5rQgRBGZkK148l-J31QiUP4lkh0U00B6STH2QBtFYoC758JPkuFWeyLdBCWzDVE7LfsT5mN3wSx4Imc9DqDa1xa4exa0HdeC1jghrLFX_oKc-A==)

| Endpoint | Purpose |
|---|---|
| `GET /api/health` | health probe (unauthenticated) |
| `DELETE /api/registry/blob/{reference}` | remove a blob from storage |
| `DELETE /api/registry/{name}/manifests/{reference}` | remove a manifest revision |

The only consumer is the shared client in `src/common/registryctl/client.go`
(used by the GC job's SWEEP phase). Auth is the `Authorization: Harbor
<secret>` header validated against `JOBSERVICE_SECRET` with constant-time
comparison.

---

## 9. Exporter

The exporter (`src/cmd/exporter` + `src/pkg/exporter`) is a standalone
Prometheus exporter that deliberately reads from *all four* data planes —
it does not proxy Core's own `/metrics`:

<!--
```SVGBob
             "Prometheus  GET /metrics"
                          │
                          ▼
┌───────────────────────────────────────────────────┐
│       "harbor-exporter (src/cmd/exporter)"        │
│                                                   │
│ "5 Prometheus collectors:"                        │
│ "health · systeminfo · project"                   │
│ "jobservice · statistics"                         │
└────┬──────────────┬───────────────┬─────────────┬─┘
     │              │               │             │
     ▼              ▼               ▼             ▼
┌──────────┐ ┌──────────────┐ ┌────────────┐ ┌─────────┐
│"Core API"│ │"PostgreSQL"  │ │"JobService"│ │ "Redis" │
│"/health" │ │"direct SQL"  │ │"worker API"│ │"queues" │
│"/sysinfo"│ │              │ │            │ │         │
└──────────┘ └──────────────┘ └────────────┘ └─────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrNk79ShDAQxnueYieVFg6VjZ3jOI6OBXq-AIRVuANzboJ_OudqiysYh4egt7HyUXgSk0OQOxQuVmbS5Mt-m83mF4DOYB6JFFWEmQQ4Ob4CV68o5pI58Ouo8sXQ7uu7U-UvVf78X-dS17doGhD5FAjaw8e5IIUEO5K4y9PQbZRd1r33t9NmNE62D52Gc5EkyJUgecBGnRH6iYrg4w3kk1SYxrfXwqzmJKY6CRvyTkUgke5jjiu_8lUslXnkkYLz9b6VNk0uLR-ltIwsnKbQXuUwrLT4alJ76MKwsiXcS7D8ARaG8dAV3-xIEMKhd8rM_Y3gCaluCCcX56xuihHPRDCp0WjigF1iGGs2vvBhbo0eaz1hTJo4WE_0IGimv0_3wLsMM-wm0uQabNuTes-0IW5KP1LZnwVsEfRHw3ho8Qnq0XwS)

| Collector | Source | What it exports |
|---|---|---|
| `health` | Core `/api/v2.0/health` | overall + per-component status |
| `systeminfo` | Core `/api/v2.0/systeminfo` | auth mode, version, self-registration |
| `project` | direct SQL on `project`/`repository`/`artifact`/`quota` | per-project quota, usage, members, repos, pulls, artifact counts |
| `jobservice` | JobService worker API + Redis | queue depth/latency, scheduled jobs, per-type concurrency |
| `statistics` | controllers + DB | total storage, project/repository totals |

---

## 10. Distribution registry and the registry client

Harbor bundles upstream `docker/distribution` v3 as the blob/manifest
store. Core and JobService talk to it exclusively through
`src/pkg/registry.Client`, which implements the full distribution protocol:
ping, catalog, tag list, manifest pull/push/delete, blob pull/push
(monolithic + chunked), **cross-repo blob mount**, and a server-side `Copy`
built from mount + manifest push. The client auto-detects basic vs. bearer
auth; the global instance wraps a `readonly` interceptor so system-wide
read-only mode also blocks direct registry writes.

The registry trusts only tokens signed by Core's token service (§4.3), so
all access control decisions stay in Core.

---

## 11. Portal

The Portal (`src/portal`) is an Angular 16 + Clarity SPA, served by its own
nginx container (port 8080) — Core does not serve UI assets in this
distribution. It talks to Core with session cookies (+ CSRF token) via
`/api/v2.0` and `/c/*`.

<!--
```SVGBob
┌─────────────────────────────────────────────────────────────────┐
│               "portal SPA (Angular 16 + Clarity)"               │
│ ┌───────────────────────────┐ ┌─────────────────────────────┐   │
│ │ "system area (lazy)"      │ │ "project area (lazy)"       │   │
│ │ "users robots registries" │ │ "repos artifacts members"   │   │
│ │ "replication config ..."  │ │ "scanning webhooks ..."     │   │
│ └───────────────────────────┘ └─────────────────────────────┘   │
│ ┌───────────────────────────┐ ┌─────────────────────────────┐   │
│ │ "shared module"           │ │ "ng-swagger-gen client"     │   │
│ │ "guards, interceptors,"   │ │ "40+ services, 100+ models" │   │
│ │ "i18n, dialogs, pipes"    │ │ "from swagger.yaml"         │   │
│ └───────────────────────────┘ └─────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNrdlM9qAjEQxu99iiEni7oolNKr9AUKfYKYHeO02cySZCv2VDz34GEPPt8-SQf_r9jjHjQEksCXX2YmX9LUv039c_t9_dDUK2g3VXJI2sH72wR6E28rpwOMn6EPrzKjtHxUFzuEseU0nVRl3RH3QD-PfwUqLmPCAnRADT2nv4_pHhVl4A806YoEdtVs8aqIIULgKScZ0FJMgTCqEy9gyVFoiWbaiKjAYip71HWeqB0ZnYg9GPYzspBlmTqLLxrtPXkLC5zOmT_jXnCFV3dQ0U1H3AP93vw2Fx_lUHBeOVTtV7VTeDuMC20thqFFuXRH6JP6z2-20iGPAyCfMBgsE4c4UOe8p1EfxJRfZFB045Es5XR0e09e8Gj84geQk3ZsRV5SiVtrnnizwAXsI8yWunCqlcO9-K2-ix9_8wd_MbRO)

- **Shell:** `HarborShellComponent` (header, left nav, theming) under a base
  module; feature areas are lazy-loaded routes.
- **Guards:** `AuthCheckGuard` (valid session), `SystemAdminGuard`,
  `MemberGuard` (project membership) gate route activation.
- **API client:** `ng-swagger-gen` regenerates the typed client from
  `api/v2.0/swagger.yaml` on `npm install` (postinstall hook) — the same
  spec that generates the Go server, so UI and API cannot drift.
- **i18n:** `@ngx-translate` with a custom loader over JSON bundles in
  `src/i18n/lang/` (the most patch-drift-prone files in the 8gcr fork).

---

## 12. Who stores what

| Store | Core | JobService | Registry/RegistryCtl |
|---|---|---|---|
| PostgreSQL | all metadata: projects, artifacts, tags, blobs refs, users, policies, executions/tasks, schedules, audit | reads Core config; job payloads use `pkg/*` DAOs (e.g. scan reports); `DB` log backend | — |
| Redis/Valkey | sessions, lib/cache (manifest/artifact/project caches), config cache | job queues, job stats, periodic policies, status-change retry | registry may use Redis for its own blob-descriptor cache |
| Object storage / FS | — | — | blobs + manifests (the only writer is Distribution; RegistryCtl deletes for GC) |

Two invariants keep this consistent:

1. **Core is the sole writer of metadata.** The registry never updates
   Harbor's database; Core records pushes synchronously in the `/v2/*`
   middleware/handlers.
2. **Deletion is metadata-first.** GC marks unreferenced blobs/manifests in
   PostgreSQL, and only then sweeps physical storage through RegistryCtl —
   so a crash between phases leaves garbage, never dangling metadata.
