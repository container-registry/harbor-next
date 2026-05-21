# Architecture Overview

## System at a glance
Harbor is an OCI-compliant, cloud-native artifact registry. It manages container images, OCI artifacts, and Helm charts (stored as OCI artifacts), and supports multi-architecture images and OCI image indexes.

It is a set of cooperating services behind a single gateway. The **Core** service is the API gateway and orchestrator; **JobService** runs background tasks; **RegistryCtl** performs direct storage operations for garbage collection; the **Portal** serves the UI. Supporting infrastructure is PostgreSQL (metadata), Redis/Valkey (cache + job queue), and a Docker Distribution registry (blob/manifest storage).

## Architecture Diagram

<!-- 
```SVGBob
┌──────────────────────────────────────────────────────────────────────────────┐
│                              "CLIENTS"                                       │
│  ┌─────────────────────┐              ┌────────────────────────────────────┐ │
│  │   "Browser / CLI"   │              │       "Docker, OCI Client"         │ │
│  │   "Angular app"     │              │   "docker push, pull, helm, etc"   │ │
│  └──────────┬──────────┘              └──────────────────┬─────────────────┘ │
└─────────────│─────────────────────────────────────────── │───────────────────┘
         ┌────┴─────────────────────┐                      │
         │                          │                      │
         ▼                          ▼                      ▼
┌──────────────────┐ ┌─────────────────────────────────────────────────────────┐
│   "Portal :80"   │ │                      "CORE :8080"                       │
│                  │ │ ┌─────────────────────────┐ ┌─────────────────────────┐ │
│     "nginx"      │ │ │ "REST API /api/v2.0/*"  │ │ "OCI Dist. v1.1 /v2/*"  │ │
│     "static"     │ │ └─────────────────────────┘ └─────────────────────────┘ │
└──────────────────┘ │              └─────────────┬─────────────┘              │
                     │                            ▼                            │
                     │ ┌─────────────────────────────────────────────────────┐ │
                     │ │   "Middleware: Auth, RBAC, Quota, Readonly, CSRF"   │ │
                     │ └─────────────────────────────────────────────────────┘ │
                     │                            │                            │
                     │                            ▼                            │
                     │ ┌─────────────────────────────────────────────────────┐ │
                     │ │                    "Controllers"                    │ │
                     │ └─────────────────────────────────────────────────────┘ │
                     │                            │                            │
                     │                            ▼                            │
                     │ ┌─────────────────────────────────────────────────────┐ │
                     │ │                 "Data Access Layer"                 │ │
                     │ └─────────────────────────────────────────────────────┘ │
                     └─┬───────────────┬───────────────┬───────────────┬───────┘
                       │               │               │               │
                       ▼               ▼               ▼               ▼
                ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
                │"JobService" │ │"PostgreSQL" │ │  "Valkey"   │ │"Distrib. V3"│
                │   ":8888"   │ │   ":5432"   │ │   ":6379"   │ │   ":5000"   │
                └──────┬──────┘ └─────────────┘ └─────────────┘ └──────┬──────┘
                       │               ▲               ▲               │
                       ├───────────────┴───────────────┘               │
                       │                                               │
                       ▼                                               │
                ┌─────────────┐                                        │
                │"RegistryCtl"│                                        │
                │   ":8080"   │────────────────────────────────────────┤
                └──────┬──────┘                                        │
                       │                                               │
                       ▼                                               ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                          "STORAGE BACKEND"                                   │
│            "Filesystem, S3, GCS, Azure Blob (inmemory for tests)"            │
└──────────────────────────────────────────────────────────────────────────────┘
```
Structure for auto-update: run `task docs:svgbob` from the repo root.
The diagram must be inside a HTML comment, wrapped in ```SVGBob fences.
The Kroki image link must follow immediately after the closing. If missing one will be created.
-->
![Architecture Diagram](https://kroki.io/svgbob/svg/eNrtWU1v2kAQvftXjObUVhaQ0I-UmzEkoqUhwSh3A1tiZbHReiGlp8pnDhysyIcee-RU5dhfwy-pCaGAMcY2JrQNFiCxO367M_ve8y6M7cHY_vZsXkNhbFsQeKFcLhXPawpCuMsFnIKOEyjl0Iv9xKszXExnUijMM-PWJAzS4NYFYda-XIBZ6QpG44YwESpyCWSqEZ3jUpgXXNJbXaoyUDsd9GAtgWPzARg6XfNadD8pFeGa0LYIhDdwFdzelOdoU4DjnYYdt6SjWHc5j-mEH9bau7xgu0k4wnre3yetrEX1-jDZPzIMxN2vAIh1nW6HsIXWh09vFAn5MF4YjKsUcieZBRmvM-ZKtTiJnMYGerHf4k3fg8T9crArB57krLc0_Qt6k7AAq0WlBtJFCdJqR0v3jlOZ9Cucx-DEgguayVPQO0odgRux2D8fwOQq1xroGcBONCdnJ4jR7NEPIL7HjyIMtN4sQtlLCF_ZAPsvmMNwUwoPbvFJazYpuVUZyYHU5e5eoJqXZBEuuwZX3S9EbRo67YsgK9XTxX1BALD919fGic-ajd0HMsYlo8_zydA5MyglzMSAew9kPJBxt2TEgspVkBoNYppQVvuE4TMkox3r-LWveEcILZuQLUJowYRsEVanNtjRTnmn0T55WPjBqCuE9bQGwZk23MOJyVuMKJdlnGsNr1R6Q_oLuwuc7LOZVk_BVRb9Cv-4e8mduNfyMQdzb15nj71tb7Pv3q_EZTKzI5KwjuthqBZtL55Q9Ghbxt_93NwSwHj7e0SV3kc2ogiTsSDaFUnLccCiaW2rgSysktZELn2ZU4xQi0BdZf78fLC_Z9GPLWWZABn2zKztfsT6D_9OQKVWqUpnRXAPqR-L5wUMuSQeUDzVKDH7JidtEZSsCGeyIoL0tcsI5KlRhxea3iZtg_Xhs8GAE5ObL3EV1H5OS-P8Bk8cqw8=)


_The diagram shows the Docker Compose topology; see [Deployment topologies](#deployment-topologies) for how the same components map to Kubernetes._

## Three-layer model
Harbor is conventionally described in three layers:

1. **Consumers** — clients that talk to Harbor: the Web Portal (browser), Docker/`containerd` clients, Helm, and other OCI tooling (e.g. ORAS).
2. **Fundamental services** — the proxy plus the application services (Core, JobService, RegistryCtl, Distribution registry).
3. **Data access** — PostgreSQL (metadata/config), Redis/Valkey (cache + job-state/queue), and the configured object/file storage backend for blobs and manifests.

## Major components

- **Edge / exposure** — terminates external traffic and routes browser and registry-client requests to the Portal, Core REST API, and the OCI distribution endpoints. Its form is deployment-specific (see [Deployment topologies](#deployment-topologies)): an **nginx** reverse-proxy container (built as the `nginx` image in this fork) in Docker Compose, or an Ingress / Gateway API `HTTPRoute` / OpenShift `Route` in Kubernetes.

- **Core** (`src/core/main.go`) — the central service. It hosts:
  - **REST API** under `/api/v2.0`, with Go server stubs generated from `api/v2.0/swagger.yaml` into `src/server/v2.0/{models,restapi}`. Business logic lives in `src/controller`, data access in `src/pkg`, and shared infrastructure in `src/lib`. Request middleware (auth, RBAC, quota, read-only, CSRF) is in `src/server/middleware`.
  - **OCI/Docker registry proxy** (`/v2/*`) — forwards image pull/push to the Distribution registry after authentication and policy checks.
  - **Token service** (`src/core/service/token`) — issues signed bearer tokens authorizing registry pull/push scoped to the user's role.
  - **Authentication & authorization** — pluggable auth backends (local DB, LDAP/AD, OIDC, UAA, AuthProxy; `src/core/auth`) with project- and system-scoped RBAC enforced via Casbin.
  - **Domain managers** invoked through controllers, including project & quota management, OCI artifact lifecycle, tag **retention** (`src/pkg/retention`), **replication** to/from external registries, **scan** orchestration, **webhook notifications** (`src/pkg/notification`), garbage-collection scheduling, and system configuration.

- **JobService** (`src/jobservice/main.go`) — executes asynchronous jobs (scan, replication, GC, retention, audit-log purge, etc.). Jobs run on Redis-backed queues via Harbor's fork of `gocraft/work` (`github.com/goharbor/work`). It communicates with Core over HTTP and reports job state. Job types are Generic (immediate), Scheduled (delayed), and Periodic (cron).

- **RegistryCtl** (`src/registryctl/main.go`) — companion to the Distribution registry that performs direct blob/manifest deletion in the storage backend (wrapping `docker/distribution`'s `storage.Vacuum`). Used by garbage collection to remove unreferenced content, since the registry V2 API has no clean deletion path for GC.

- **Distribution registry** — upstream `docker/distribution` **v3.0.0**, storing blobs and manifests in the configured backend. Distribution v3 supports **filesystem, S3, GCS, and Azure Blob** (plus `inmemory` for testing); the v2-era OpenStack Swift and Alibaba OSS drivers are no longer supported. Enforces access by validating Core-issued tokens.

- **Portal** (`src/portal`) — Angular UI for projects, repositories, artifacts, and system configuration. Its API client is generated from Swagger into `src/portal/ng-swagger-gen`.

- **Exporter** (`src/cmd/exporter`, `src/pkg/exporter`) — exposes Prometheus metrics for the Harbor services.

- **Scanning** — pluggable scanner integration (`src/pkg/scan`). Trivy is the default adapter (`HARBOR_SCANNER_TRIVY_VERSION` in `versions.env`); vulnerability reports and SBOM generation (`src/pkg/scan/sbom`) are supported.

## Service-to-service authentication
Distinct from user-facing auth (LDAP/OIDC/etc.), Harbor's components also authenticate to *one another* across the internal network. Most edges use a shared secret injected at deploy time via config; registry access additionally accepts bearer tokens minted by Core's token service:

| Edge | Credential |
|------|-----------|
| Core → JobService | `CORE_SECRET` |
| Core / JobService → RegistryCtl | `JOBSERVICE_SECRET` |
| Core / JobService → Distribution registry | Basic auth / bearer token |
| Portal → Core | session / token |

## Key Sequences
These handful interactions capture the essence of how Harbor works. Each numbered sequence below maps to a lane in the diagram.

<!--
```SVGBob
   .--------.            .------.           .----------.           .-------.          .----------.          .--------.           .--------.           .---------.
   | Client |            | Core |           | Registry |           | Cache |          | Postgres |          | JobSvc |           | RegCtl |           | Storage |
   '----+---'            '---+--'           '-----+----'           '---+---'          '-----+----'          '----+---'           '----+---'           '----+----'
        |                    |                    |                    |                    |                    |                    |                    |

  "1) Docker login"
        | "GET /v2/"         |                    |                    |                    |                    |                    |                    |
        +---------------------------------------–>|                    |                    |                    |                    |                    |
        | "401 + token URL"  |                    |                    |                    |                    |                    |                    |
        |<- - - - - - - - - - - - - - - - - - - - +                    |                    |                    |                    |                    |
        | "credentials"      |                    |                    |                    |                    |                    |                    |
        +------------------–>|                    |                    |                    |                    |                    |                    |
        |                    | "authenticate (backend)"                |                    |                    |                    |                    |
        |                    +------------------------------------------------------------–>|                    |                    |                    |
        | "signed token (cached)"                 |                    |                    |                    |                    |                    |
        |<- - - - - - - - - -+                    |                    |                    |                    |                    |                    |
                             |                    |                    |                    |                    |                    |                    |
  "2) Image push / pull"
        | "/v2/* (+token)"   |                    |                    |                    |                    |                    |                    |
        +------------------–>|                    |                    |                    |                    |                    |                    |
        |                    | "check manifest cache"                  |                    |                    |                    |                    |
        |                    +---------------------------------------–>|                    |                    |                    |                    |
        |                    | "hit / miss"       |                    |                    |                    |                    |                    |
        |                    |<- - - - - - - - - - - - - - - - - - - - +                    |                    |                    |                    |
        |                    | "blob & manifest"  |                    |                    |                    |                    |                    |
        |                    +------------------–>|                    |                    |                    |                    |                    |
        |                    |                    | "read / write"     |                    |                    |                    |                    |
        |                    |                    +------------------------------------------------------------------------------------------------------–>|
        |                    | "persist metadata" |                    |                    |                    |                    |                    |
        |                    +------------------------------------------------------------–>|                    |                    |                    |
        |                    | "write-back on miss"                    |                    |                    |                    |                    |
        |                    +---------------------------------------–>|                    |                    |                    |                    |

  "3) API request"
        | "/api/v2.0/*"      |                    |                    |                    |                    |                    |                    |
        +------------------–>|                    |                    |                    |                    |                    |                    |
        |                    | "query / update"   |                    |                    |                    |                    |                    |
        |                    +------------------------------------------------------------–>|                    |                    |                    |
        |                    | "read / write"     |                    |                    |                    |                    |                    |
        |                    +---------------------------------------–>|                    |                    |                    |                    |
        | "JSON"             |                    |                    |                    |                    |                    |                    |
        |<- - - - - - - - - -+                    |                    |                    |                    |                    |                    |

  "4) Background job (auto-scan)"
        |                    | "submit job"       |                    |                    |                    |                    |                    |
        |                    +---------------------------------------------------------------------------------–>|                    |                    |
        |                    | "status callback"  |                    |                    |                    |                    |                    |
        |                    |<- - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +                    |                    |

  "5) Garbage collection"
        |                    | "submit GC job"    |                    |                    |                    |                    |                    |
        |                    +---------------------------------------------------------------------------------–>|                    |                    |
        |                    |                    |                    |                    | "MARK unreferenced"|                    |                    |
        |                    |                    |                    |                    |<-------------------+                    |                    |
        |                    |                    |                    |                    |                    | "SWEEP delete"     |                    |
        |                    |                    |                    |                    |                    +------------------–>|                    |
        |                    |                    |                    |                    |                    |                    | "storage.Vacuum"   |
        |                    |                    |                    |                    |                    |                    +------------------–>|
   .----+---.            .---+--.           .-----+----.           .---+---.          .-----+----.          .----+---.           .----+---.           .----+----.
   | Client |            | Core |           | Registry |           | Cache |          | Postgres |          | JobSvc |           | RegCtl |           | Storage |
   '--------'            '------'           '----------'           '-------'          '----------'          '--------'           '--------'           '---------'
```
-->
![Data flow sequence (SVGBob)](https://kroki.io/svgbob/svg/eNrtWdtu2kAQfc9XjPahhqxskjR9iyqlFKGkbYqgl-e1vQEX20t316ki8dB_6B_2SzomQLks2IgEFolFCBjt2qPjMzNnGADw3PHyYGZ5y7bpRrPZK9rprTu94k7eCf4cQj2OeKrxy8xCq5B8zjaENu9GSsvHBXOdBb25rUNoCaW7kqt5663wOw_B8kXrOl4wdrSQrIsXzT10cl8pvp1ZD50no7Ngetq5ZF44bt5pvNF6o-uc_PfasPZsPEHvyHkV3ougzyXEohulZMZj0mx8gdrDRY1Y4_HkG3XLrb-__7zdg3uI3eXZOVDQos9T-Nr-SOzBbnjlQrkX3QtygeQhppyIxYpYz7rdEqwIO-MZwjLdywENmOZQ8RlGexpWiQXOlQ3jF4vtWd6pqJvycByylSCvWwaUrIlYagnrwMbKhoXtogo3SS4TBpnqQQ0_4niuuuWV7RQqdPTARw_6mGe2yjMYMEEfEpZG91xpGEUQscK5vcuFQux6kUaOJpGa1Dx7Hux-5UIhcn4sfHg1pR2xBzlqkR4tfyEiOQuRi79kpMfha4tzW8mFDYVFIe8GXCpseCHhmoVMM2Iz7_amqszYjbjl5koURDqX9Y6VItcur6tw3boByX9meUqbUy1sEKFy8c5qp8fu6Dm4iBDLR8x32QCjmBOAYxyXxm5HpeLAFR-57Xy-I5b1RxZ3lnkGvKzCOywPXSmyNIQfKPEqLNPCVQHDXq2QmCrzE1TUeM42OU1fRq48Y1ArzXSmsH2L47xCk0NsRcq_6IbEfFOFJpN-_sdCIOKYBzoSaWlCNutTTh4JuU2XskE_8-m6_QGyVPJ7Lnka8JBY49yVAT1qDXJmODvfG40WhDzm62v-7p3bSIBaM5nDfDsaZ3rfWJBlCbHIObqmN_YmQ86l2TU1TZSpadBMTVNqappdU9OYeqXxoGbXrml2bZoorzY7RTuddadX3Mn5B14QWYg=)

1. **Docker login** — client hits `/v2/`; the registry returns `401` with the token-service URL; the client submits credentials to Core's token service, which authenticates against the configured backend (DB/LDAP/OIDC) and returns a signed token the client caches.
2.  **Image push/pull** — client → Core (`/v2/*`) for authentication, authorization, and policy checks (e.g. quota on push); Core proxies blob/manifest transfer to the Distribution registry. Core persists metadata in PostgreSQL and caches manifest metadata in Redis (pull path checks the cache before the registry and writes back on a miss).
3.  **API requests** — client → Core (`/api/v2.0/*`) → generated handler → controller → DAO (`src/pkg`) → PostgreSQL / Redis.
4.  **Background jobs** — Core submits a job to JobService over HTTP; JobService executes it, updates status, and calls back Core on completion. Auto-scan is submitted by Core after a manifest PUT (when enabled).
5.  **Garbage collection (two-phase)** — MARK in PostgreSQL finds unreferenced blobs/manifests; SWEEP runs as a JobService job that calls RegistryCtl to delete them from the storage backend.

## Deployment topologies
The component set above is identical across deployments. What changes is how services are exposed, which processes are co-located, and which dependencies are bundled. The diagram above depicts the **Docker Compose** topology.

### Docker Compose (`deploy/compose/`)
- Each service runs as its own container; **nginx** is the single edge proxy that routes to the Portal, the Core API, and `/v2/`.
- **RegistryCtl runs as a separate container** alongside the registry.
- PostgreSQL and Redis/Valkey run as containers in the same Compose project (or can be pointed at external instances).
- Host ports are published directly (dev uses SLOT offsets).

### Kubernetes / Helm (`deploy/chart/` — in development, not yet merged to this branch)
- Core, JobService, Portal, Registry, and Exporter are separate **Deployments** fronted by ClusterIP **Services**; Trivy runs as a **StatefulSet**.
- **No nginx proxy** — external exposure is via **Ingress**, Gateway API **HTTPRoute**, or OpenShift **Route**.
- **RegistryCtl runs as a sidecar container in the registry pod** (it has no Deployment of its own; only its config/secret are templated).
- **Redis/Valkey** ships as an **in-cluster subchart** (`valkey.enabled`, on by default; bundled `valkey` dependency in `Chart.yaml`) or can be pointed at an external instance (`externalRedis`). **PostgreSQL is external** — the chart has no Postgres workload, so you supply it (the 8gcr GitOps bundle provisions one via CloudNativePG through the chart's `extraManifests` hook).
- Persistence is via **PVCs** (registry, jobservice); the chart adds Kubernetes-native resources: ServiceAccounts, PodDisruptionBudgets, and a Prometheus `ServiceMonitor`.
- The chart (`harbor-next`, currently v3.0.0 / appVersion v2.15.0) is published as an OCI artifact and supports GitOps delivery via FluxCD (`deploy/flux/`), with platform guides for k3s, OpenShift, Rancher, Nutanix, and AWS EKS (IRSA).

> The Helm chart details reflect the in-progress chart and may change before it lands.

## Development topology
`task dev:up` runs Core, JobService, RegistryCtl, the Trivy adapter, and supporting services in Docker with hot reload. The Portal can run natively with HMR or in a container. Local ports are SLOT-offset (`SLOT=N` adds `N*100`); service ports and profiling endpoints are described in `devenv/README.md`.
