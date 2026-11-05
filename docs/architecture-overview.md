# Architecture Overview

## System at a glance
Harbor is an OCI-compliant, cloud-native artifact registry. It manages container images, OCI artifacts, and Helm charts (stored as OCI artifacts), and supports multi-architecture images and OCI image indexes.

It is a set of cooperating services behind a single gateway. The **Core** service is the API gateway and orchestrator; **JobService** runs background tasks, including garbage collection; the **Portal** serves the UI. Supporting infrastructure is PostgreSQL (metadata), Redis/Valkey (cache + job queue), and a Docker Distribution registry (blob/manifest storage).

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
                       │                                               ▲
                       └───────────────────"GC delete"─────────────────┤
                                                                       │
                                                                       ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                          "STORAGE BACKEND"                                   │
│            "Filesystem, S3, GCS, Azure Blob (inmemory for tests)"            │
└──────────────────────────────────────────────────────────────────────────────┘
```
Structure for auto-update: run `task docs:svgbob` from the repo root.
The diagram must be inside a HTML comment, wrapped in ```SVGBob fences.
The Kroki image link must follow immediately after the closing. If missing one will be created.
-->
![Architecture Diagram](https://kroki.io/svgbob/svg/eNrtWE1v2kAQvftXjObUVhaQ0I-UmzEE0dKQYJS7gS2xsthovZCSU8WZAwcr8qHHHjlVOfbX8EtqQhBgbGMbk7QNKxuJ3fHbndk3z7OeWqOp9f3FXGNhag0hsKFcKRfP6gpCuOYAzkGnCYRy7MZ-4t0Zr7ozCxTmmXFjEgZpcOKCsOhfD8AidAWjeU2YCFW5DDLViM5xzcwNLuntHlUZqN0uurDWwLH1AAzdnnklOr-UinBFaEcEwpu4CW5t83OyzcB2L8OKG9JJrKfsR3fCTzt89vSC3RZhC_68v086s1az14PJ3pZhIO5-B0D4DToDwg65Pn56oUhIh_HcYFylkDvJrKSxnzBXa8WZ5dw2UIu9Nm9-jxLXy9G-FHjms97W9G_odmIIWCsqdZDOy5BWu1q6f5zKpN_g0gZnElzQTJ6C_lHqCByL1fHlBCZXudZE1wRWoj7Ze0GMJo9eAPE1fhJhIn-xCCUvIXRlC-y_IA7jbS48qMUXrdWi5EZlJAdSjzu1QC0vySJc9AyuOn-I2jJ0OhBBVmqnq3VBALD118fGjs-arcMHMsYlo8f7ydA5MyglzMSAZw9kPJBxv2TEgspVkJpNYppQUQeE4QskoxXr-PVc9rYQOm1C9gihEyZkj7C5tNGeKuW9Wnv4McRPRkMhrK81CS5ywzmcmLzNiHJRwWWu4aVKr8lgpbrAWZ3NtEYKLrPoFfjH6iV34rT1Yw7m3r3NHrv73mc_fNywy2QWRyTBj-thqBatFk_IerIr4-9-be8JYLz1I2KW3kcWogiLGUK05vjqDxZfpLEkQ4tQwgnGAvgpQDItIFiRoXb7kPIfftJGpV6tSaUiOAelz8WzAobcEBconmqUmAOTk44ISlaEkqyIIN32GIE8NRrwStM7pGOwAXw1GHBicvM1boJaL2lr7D-SpzPW)


_The diagram shows the Docker Compose topology; see [Deployment topologies](#deployment-topologies) for how the same components map to Kubernetes._

## Three-layer model
Harbor is conventionally described in three layers:

1. **Consumers** — clients that talk to Harbor: the Web Portal (browser), Docker/`containerd` clients, Helm, and other OCI tooling (e.g. ORAS).
2. **Fundamental services** — the proxy plus the application services (Core, JobService, Distribution registry).
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

- **Distribution registry** — `distribution/distribution` v3 built with the patches in `patches/distribution`, storing blobs and manifests in the configured backend. Patch 0004 adds the GC delete API (`DELETE /api/registry/blob/{digest}`, `DELETE /api/registry/{repo}/manifests/{digest}`, wrapping `storage.Vacuum`) that garbage collection calls, since the V2 API has no way to remove blob data. The chart's `image.source: upstream` mode runs vanilla `goharbor/registry-photon` instead, so it keeps the goharbor `registryctl` sidecar for GC. Distribution v3 supports **filesystem, S3, GCS, and Azure Blob** (plus `inmemory` for testing); the v2-era OpenStack Swift and Alibaba OSS drivers are no longer supported. Enforces access by validating Core-issued tokens.

- **Portal** (`src/portal`) — Angular UI for projects, repositories, artifacts, and system configuration. Its API client is generated from Swagger into `src/portal/ng-swagger-gen`.

- **Exporter** (`src/cmd/exporter`, `src/pkg/exporter`) — exposes Prometheus metrics for the Harbor services.

- **Scanning** — pluggable scanner integration (`src/pkg/scan`). Trivy is the default adapter (`HARBOR_SCANNER_TRIVY_VERSION` in `versions.env`); vulnerability reports and SBOM generation (`src/pkg/scan/sbom`) are supported.

## Service-to-service authentication
Distinct from user-facing auth (LDAP/OIDC/etc.), Harbor's components also authenticate to *one another* across the internal network. Most edges use a shared secret injected at deploy time via config; registry access additionally accepts bearer tokens minted by Core's token service:

| Edge | Credential |
|------|-----------|
| Core → JobService | `CORE_SECRET` |
| JobService → Distribution registry GC API (`/api/registry/*`) | `JOBSERVICE_SECRET` |
| Core / JobService → Distribution registry | Basic auth / bearer token |
| Portal → Core | session / token |

## Key Sequences
These handful interactions capture the essence of how Harbor works. Each numbered sequence below maps to a lane in the diagram.

<!--
```SVGBob
   .--------.            .------.           .----------.           .-------.          .----------.          .--------.           .---------.
   | Client |            | Core |           | Registry |           | Cache |          | Postgres |          | JobSvc |           | Storage |
   '----+---'            '---+--'           '-----+----'           '---+---'          '-----+----'          '----+---'           '----+----'
        |                    |                    |                    |                    |                    |                    |

  "1) Docker login"
        | "GET /v2/"         |                    |                    |                    |                    |                    |
        +---------------------------------------–>|                    |                    |                    |                    |
        | "401 + token URL"  |                    |                    |                    |                    |                    |
        |<- - - - - - - - - - - - - - - - - - - - +                    |                    |                    |                    |
        | "credentials"      |                    |                    |                    |                    |                    |
        +------------------–>|                    |                    |                    |                    |                    |
        |                    | "authenticate (backend)"                |                    |                    |                    |
        |                    +------------------------------------------------------------–>|                    |                    |
        | "signed token (cached)"                 |                    |                    |                    |                    |
        |<- - - - - - - - - -+                    |                    |                    |                    |                    |
                             |                    |                    |                    |                    |                    |
  "2) Image push / pull"
        | "/v2/* (+token)"   |                    |                    |                    |                    |                    |
        +------------------–>|                    |                    |                    |                    |                    |
        |                    | "check manifest cache"                  |                    |                    |                    |
        |                    +---------------------------------------–>|                    |                    |                    |
        |                    | "hit / miss"       |                    |                    |                    |                    |
        |                    |<- - - - - - - - - - - - - - - - - - - - +                    |                    |                    |
        |                    | "blob & manifest"  |                    |                    |                    |                    |
        |                    +------------------–>|                    |                    |                    |                    |
        |                    |                    | "read / write"     |                    |                    |                    |
        |                    |                    +---------------------------------------------------------------------------------–>|
        |                    | "persist metadata" |                    |                    |                    |                    |
        |                    +------------------------------------------------------------–>|                    |                    |
        |                    | "write-back on miss"                    |                    |                    |                    |
        |                    +---------------------------------------–>|                    |                    |                    |

  "3) API request"
        | "/api/v2.0/*"      |                    |                    |                    |                    |                    |
        +------------------–>|                    |                    |                    |                    |                    |
        |                    | "query / update"   |                    |                    |                    |                    |
        |                    +------------------------------------------------------------–>|                    |                    |
        |                    | "read / write"     |                    |                    |                    |                    |
        |                    +---------------------------------------–>|                    |                    |                    |
        | "JSON"             |                    |                    |                    |                    |                    |
        |<- - - - - - - - - -+                    |                    |                    |                    |                    |

  "4) Background job (auto-scan)"
        |                    | "submit job"       |                    |                    |                    |                    |
        |                    +---------------------------------------------------------------------------------–>|                    |
        |                    | "status callback"  |                    |                    |                    |                    |
        |                    |<- - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +                    |

  "5) Garbage collection"
        |                    | "submit GC job"    |                    |                    |                    |                    |
        |                    +---------------------------------------------------------------------------------–>|                    |
        |                    |                    |                    |                    | "MARK unreferenced"|                    |
        |                    |                    |                    |                    |<-------------------+                    |
        |                    |                    | "SWEEP: DELETE /api/registry/*"         |                    |                    |
        |                    |                    |<-------------------------------------------------------------+                    |
        |                    |                    | "storage.Vacuum + cache clear"          |                    |                    |
        |                    |                    +---------------------------------------------------------------------------------->|
   .----+---.            .---+--.           .-----+----.           .---+---.          .-----+----.          .----+---.           .----+----.
   | Client |            | Core |           | Registry |           | Cache |          | Postgres |          | JobSvc |           | Storage |
   '--------'            '------'           '----------'           '-------'          '----------'          '--------'           '---------'
```
-->
![Data flow sequence (SVGBob)](https://kroki.io/svgbob/svg/eNrtWc1u2kAQvvMUoz3UkJVNkqaXClVKCUJJ0xZB2p6X9QZcjJfurlNF4tB36Bv2STrmL_wsMSpgkyaLEGY0A8P4m5lvGADw3MnxYO54q7KZol3spWl6j1mjtIBvh1ANAxEZvJg7KJVKLMiG0BSdQBt1vySuMt5dUB1CQ2rTUUIvSq9ku3XHl6xbRirWQfvEGSdxi-LTmXfGGQudJdFYc0W8ZG7XtH6R86BZeHDQcrITFtARclKCC8l7QkEoO0FE5pwj9doNlO9OyyQP56ZX1N3s_Pn1-91-PcGInB2fAAUjeyKCL81rkktEhhUXNnvQfceDK-Fjfgcs1OSQELI3MKRFxGpDWGy6SZg4MwKKbYb5Fvklkq0fmybSLrJrHiM66ETCnyRNkScF3fLb88gZmj1CIOeSjxX_tASX_aQrDmLdhTK-hOFC2U9K_hEU6eiOje7US1JbkhpxzHvQZ1FwK7SBEbBJ1n5k2R1TI9INDOKpH-hpM8jlzmTWHVPj0Q5lG17NIEJyiQfNhypt_kFECeYjbn6qwEwSKAc_tuqO61MuFSMDoTQOPtAXhvnMMHIgGMmCGtgjMsKBm5AkkNFCNflf62rSlV-X4LxxCUr8iJNSsdCP2SDAnuwdl49eSPZ63GDg1D3WkXiAeSQIwLPPpN0X1qfDUMhV6_Mnkh_NPoyxI6ksZyV4j8W0o2Qc-fAdKUkR51Hpas6Q3aeCSMftPvI6tMuR1O2pO_9bWmnDTKyR8Idh0qXIgdPczR90PYjelKDOVDsZG7kMQ8FNIKONwVOvzvDzzMGzpZB8PG9-gDhS4lYoEXHhkzz8qFhiQnc4EbS-1WqNt3BRu67d1GDEf9RkPzDjQPufTCpbYWSX8dDjPYb3lfE47mOejv5sAB4KpsjTnZDc8XzkTVcjKysratsuUdvSidqWU9S2sqK2lRU94JWVa1tZ2bZL68VOmqbzmDVK_wLRN75D)

1. **Docker login** — client hits `/v2/`; the registry returns `401` with the token-service URL; the client submits credentials to Core's token service, which authenticates against the configured backend (DB/LDAP/OIDC) and returns a signed token the client caches.
2.  **Image push/pull** — client → Core (`/v2/*`) for authentication, authorization, and policy checks (e.g. quota on push); Core proxies blob/manifest transfer to the Distribution registry. Core persists metadata in PostgreSQL and caches manifest metadata in Redis (pull path checks the cache before the registry and writes back on a miss).
3.  **API requests** — client → Core (`/api/v2.0/*`) → generated handler → controller → DAO (`src/pkg`) → PostgreSQL / Redis.
4.  **Background jobs** — Core submits a job to JobService over HTTP; JobService executes it, updates status, and calls back Core on completion. Auto-scan is submitted by Core after a manifest PUT (when enabled).
5.  **Garbage collection (two-phase)** — MARK in PostgreSQL finds unreferenced blobs/manifests; SWEEP runs as a JobService job that calls the registry's GC delete API to remove them from the storage backend (the `registryctl` sidecar in the chart's upstream mode).

## Deployment topologies
The component set above is identical across deployments. What changes is how services are exposed, which processes are co-located, and which dependencies are bundled. The diagram above depicts the **Docker Compose** topology.

### Docker Compose (`deploy/compose/`)
- Each service runs as its own container; **nginx** is the single edge proxy that routes to the Portal, the Core API, and `/v2/`.
- PostgreSQL and Redis/Valkey run as containers in the same Compose project (or can be pointed at external instances).
- Host ports are published directly (dev uses SLOT offsets).

### Kubernetes / Helm (`deploy/chart/` — in development, not yet merged to this branch)
- Core, JobService, Portal, Registry, and Exporter are separate **Deployments** fronted by ClusterIP **Services**; Trivy runs as a **StatefulSet**.
- **GC**: with the default `image.source: 8gcr`, the registry container serves the GC delete API itself. With `image.source: upstream` (vanilla goharbor images), a `registryctl` sidecar runs in the registry pod and `REGISTRY_CONTROLLER_URL` points at it.
- **No nginx proxy** — external exposure is via **Ingress**, Gateway API **HTTPRoute**, or OpenShift **Route**.
- **Redis/Valkey** ships as an **in-cluster subchart** (`valkey.enabled`, on by default; bundled `valkey` dependency in `Chart.yaml`) or can be pointed at an external instance (`externalRedis`). **PostgreSQL is external** — the chart has no Postgres workload, so you supply it (the 8gcr GitOps bundle provisions one via CloudNativePG through the chart's `extraManifests` hook).
- Persistence is via **PVCs** (registry, jobservice); the chart adds Kubernetes-native resources: ServiceAccounts, PodDisruptionBudgets, and a Prometheus `ServiceMonitor`.
- The chart (`harbor-next`, currently v3.0.0 / appVersion v2.15.0) is published as an OCI artifact and supports GitOps delivery via FluxCD (`deploy/flux/`), with platform guides for k3s, OpenShift, Rancher, Nutanix, and AWS EKS (IRSA).

> The Helm chart details reflect the in-progress chart and may change before it lands.

## Development topology
`task dev:up` runs Core, JobService, the patched registry, the Trivy adapter, and supporting services in Docker with hot reload. The Portal can run natively with HMR or in a container. Local ports are SLOT-offset (`SLOT=N` adds `N*100`); service ports and profiling endpoints are described in `devenv/README.md`.
