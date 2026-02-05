# Architecture Overview

## System at a glance
Harbor is a multi-service container registry platform with a Go backend, Angular UI, and supporting infrastructure (PostgreSQL, Redis/Valkey, and a Docker Distribution registry). The Core service is the API gateway and orchestrator; JobService runs background tasks; RegistryCtl performs direct storage operations for garbage collection; the Portal provides the UI.
## Architecture diagram

<!-- 
```SVGBob
┌──────────────────────────────────────────────────────────────────────────────┐
│                              "CLIENTS"                                       │
│  ┌─────────────────────┐              ┌────────────────────────────────────┐ │
│  │ "Browser / CLI"     │              │ "Docker, OCI Client"               │ │
│  │ "Angular app"       │              │ "docker push, pull, helm, etc"     │ │
│  └──────────┬──────────┘              └──────────────────┬─────────────────┘ │
└─────────────│───────────────────────────────────────────│────────────────────┘
      ┌───────┴───────┐                                   │
      │               │                                   │
      ▼               ▼                                   ▼
┌─────────────┐ ┌──────────────────────────────────────────────────────────────┐
│  "Portal"   │ │                         "CORE :8080"                         │
│  ":80"      │ │ ┌───────────────────────────┐ ┌───────────────────────────┐  │
│  "nginx"    │ │ │ "REST API /api/v2.0/*"    │ │ "OCI Dist. v1.1 /v2/*"    │  │
│  "static"   │ │ └─────────────┬─────────────┘ └─────────────┬─────────────┘  │
└─────────────┘ │               └───────────┬─────────────────┘                │
                │                           ▼                                  │
                │ ┌──────────────────────────────────────────────────────────┐ │
                │ │     "Middleware: Auth, RBAC, Quota, Readonly, CSRF"      │ │
                │ └──────────────────────────────────────────────────────────┘ │
                │                           │                                  │
                │                           ▼                                  │
                │ ┌──────────────────────────────────────────────────────────┐ │
                │ │                    "Controllers"                         │ │
                │ └──────────────────────────────────────────────────────────┘ │
                │                           │                                  │
                │                           ▼                                  │
                │ ┌──────────────────────────────────────────────────────────┐ │
                │ │                  "Data Access Layer"                     │ │
                │ └──────────────────────────────────────────────────────────┘ │
                └────┬───────────────┬───────────────┬───────────────┬─────────┘
                     │               │               │               │
                     ▼               ▼               ▼               ▼
              ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
              │"JobService" │ │"PostgreSQL" │ │  "Valkey"   │ │"Distrib. V3"│
              │  ":8888"    │ │  ":5432"    │ │  ":6379"    │ │  ":5000"    │
              └──────┬──────┘ └─────────────┘ └─────────────┘ └──────┬──────┘
                     │               ▲               ▲               │
                     ├───────────────┴───────────────┘               │
                     │                                               │
                     ▼                                               │
              ┌─────────────┐                                        │
              │"RegistryCtl"│                                        │
              │  ":8080"    │────────────────────────────────────────┤
              └──────┬──────┘                                        │
                     │                                               │
                     ▼                                               ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                          "STORAGE BACKEND"                                   │
│    "Filesystem, S3, GCS, Azure Blob, OpenStack Swift, Alibaba OSS"           │
└──────────────────────────────────────────────────────────────────────────────┘
```
Structure for auto-update: run `task` in docs/ directory.
The diagram must be inside a HTML comment, wrapped in ```SVGBob fences.
The Kroki image link must follow immediately after the closing. If missing one will be created.
-->
![Architecture Diagram](https://kroki.io/svgbob/svg/eNrtWctu2kAU3fsrru6ysoCEPlJ2xpAoLQ0JRtkPZkosJjYaD1C6qlh34YWFvOiyS1ZVlvkavqSmCQ1gDDYOkAeWhcT4-ozv9TlnHh65P0fuj1dzOtLI7cPSA9XSafGsqiFEO3zAO9DRI5TSmcfe8ttxptPpA-a51bUphzT4VcFJvoECABYsvUm5DGX1FFRmUFNgsE5z4IrZaDPCgbRaOBUUBK__A4dW276S_V_GZLii7FoGKnRcBO6uynO4KsCbfwx33ZIO17rLu08nerf9JyCvhM_gSatofxNNN6FCDWEZrHSFeYTB7fy1QMtChMGtFEfUzvYtYCOGi-cWF4Thg1SX-G-5UoTcUeYog6tNF3P_wybAm6mYszHcqWTMhmF-w9lkfP-rFLUqKOenkCYtI905TGXSb2aicGy7BcMWKegcpA7Aj5mKmOrAFkQYOs504G7AyryN4cb2RW-h3N0NOneoa0T1m0heEgb7DA3DWZLMXZ3wi1GvM9olnOZAaQt_HlDJK6oMF21LEP8PJXXLZD0ZVK1yPGsIIcDuc6uStw6Tooxre4ImJ2hgELNMwS3GKLeXjmJ7gu4Jun2CYoEIAoquU9uGEulRjq-ZoG6ycX-38Z4UUVgRW6SIgorYIq2_o-E8nehAFn38ZNU0yjuGTnGiE3-VY4sGp9pFCR_Eh5eENWlvatKN46k6N2opuMxisOSThY1_zMzy_bZ3b7OH823vsx8-BuIymQwufqEhuh0mn8Y_UvQwGcsHf1a3hLLc_RVTmzcJVwdLHiXKjsSa2o0PFU9bCbrpY4U2xvLoqYJhjCqE62iyk7DTPbLfiWSYmAI7ZVPMTa-X_50BtWq5opwUwV_Dfi6eFTDiC7kHxWODUbtnC3otg5aV4UTVZFC-tzmFPLNqMpRb1NQE0ZugdY2vwr_IjBqpEShrMx824m6mvIDTk_4CQg_fbA==)


## Major components
- **Core** (`src/core/main.go`)
  - Serves REST API (`/api/v2.0`) generated from `api/v2.0/swagger.yaml`.
  - Proxies Docker/OCI registry requests (`/v2/*`) to the Distribution registry after auth and policy checks.
  - Hosts the portal UI in production and manages auth, RBAC, projects, artifacts, and quotas.
  - Entry points for business logic in `src/controller`, with data access in `src/pkg` and shared infra in `src/lib`.

- **API layer**
  - Go server stubs are generated into `src/server/v2.0/models` and `src/server/v2.0/restapi`.
  - Controllers implement business logic; middleware lives in `src/server/middleware`.

- **JobService** (`src/jobservice/main.go`)
  - Executes background jobs (scan, replication, GC, retention, audit purge, etc.).
  - Uses Redis-backed queues via the Harbor fork of `gocraft/work`.
  - Communicates with Core over HTTP and reports job state.

- **RegistryCtl** (`src/registryctl/main.go`)
  - Performs direct blob/manifest deletion in the storage backend.
  - Used by garbage collection to remove unreferenced content.

- **Portal** (`src/portal`)
  - Angular UI for projects, repositories, artifacts, and system configuration.
  - Frontend API client is generated from Swagger into `src/portal/ng-swagger-gen`.

- **Infrastructure**
  - **PostgreSQL**: metadata and configuration storage.
  - **Redis/Valkey**: cache and job queue.
  - **Distribution registry**: stores blobs and manifests in configured storage (filesystem/S3/GCS/etc).

## Data flow highlights
- **Image push/pull**
  - Client → Core (`/v2/*`) for auth/authorization and policy checks.
  - Core → Distribution registry for blob/manifest storage and retrieval.
  - Core persists metadata in PostgreSQL and caches manifest metadata in Redis.

- **API requests**
  - Client → Core (`/api/v2.0/*`) → generated handler → controller → DAO (`src/pkg`) → PostgreSQL/Redis.

- **Background jobs**
  - Core submits jobs to JobService (HTTP).
  - JobService executes tasks, updates job status, and calls back Core when complete.
  - GC jobs use RegistryCtl to delete blobs/manifests in the storage backend.

## Development topology (dev environment)
`task dev:up` runs Core, JobService, RegistryCtl, Trivy adapter, and supporting services in Docker with hot reload. The Portal can run natively with HMR or in a container; service ports and profiling endpoints are described in `devenv/README.md`.
