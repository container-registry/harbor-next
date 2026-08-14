# Architecture

Harbor is a set of cooperating services behind a single Core gateway. Use `gopls-mcp` for code navigation; this file only documents what the code doesn't tell you.

## Services

| Service | Port | Entry | Role |
|---------|------|-------|------|
| Core | 8080 | `src/core/main.go` | API gateway, auth, Docker V2 proxy, serves Portal in prod |
| JobService | 8888 | `src/jobservice/main.go` | Async jobs (gocraft/work fork) |
| RegistryCtl | 8080 (internal) | `src/registryctl/main.go` | Direct storage deletion via `storage.Vacuum` |
| Registry | 5000 | `goharbor/registry-photon` | Docker distribution V2 |
| PostgreSQL | 5432 | — | Metadata |
| Redis/Valkey | 6379 | — | Cache + job queue |
| Portal | 4200 (dev) | `src/portal/` | Angular UI |

Local ports are SLOT-offset: `SLOT=N` adds `N*100`. Container names prefixed by `${PROJECT_PREFIX:-harbor}`.

## Inter-service auth (the non-obvious bit)

| Edge | Secret | Client |
|------|--------|--------|
| Core → JobService | `CORE_SECRET` | `src/pkg/task/` |
| Core/JobService → RegistryCtl | `JOBSERVICE_SECRET` | `src/common/registryctl/client.go` |
| Core/JobService → Registry | Basic/Token | `src/pkg/registry/client.go` |
| Portal → Core | Session/Token | Angular HttpClient |

## Why RegistryCtl exists

Docker Registry V2 has no clean deletion API for GC. RegistryCtl wraps `docker/distribution`'s `storage.Vacuum` to remove blobs/manifests directly from the storage backend.

## Layered code

```
src/server/v2.0/{handler,restapi}/   ← auto-generated from OpenAPI; do not edit
src/controller/<domain>/             ← business logic
src/pkg/<domain>/{,dao/}             ← data access + domain models
src/lib/{orm,cache,log,config,errors}/   ← infrastructure
```

Patterns: repository (DAO interfaces), controller, middleware chain (`src/server/middleware/`), plugin (scanners, auth providers, storage drivers).

## Flow gotchas worth knowing

- **Pull path:** Core checks Redis manifest cache before hitting Registry. Cache misses go to Registry and write back.
- **GC is two-phase:** MARK in Postgres (find unreferenced blobs/manifests) → SWEEP via RegistryCtl (`DeleteBlob`/`DeleteManifest` → `storage.Vacuum`).
- **Auto-scan** is a JobService job submitted by Core after manifest PUT (if enabled).
- **Quotas** are enforced in Core middleware before forwarding to Registry on push.

## Job framework specifics

- Job types: Generic (immediate), Scheduled (delayed), Periodic (cron).
- Status vocabulary includes `cancelled` and `stopped` (Harbor extensions over upstream).
- Implement `job.Interface` (`MaxFails`, `MaxCurrency`, `ShouldRetry`, `Validate`, `Run`) in `src/jobservice/job/impl/<name>/`.
- Logger backends: `STD_OUTPUT`, `FILE`, `DB`.

## Auth providers

Pluggable: DB, LDAP, OIDC, UAA, AuthProxy. RBAC is project-scoped + system-scoped via Casbin.

## Configuration runtime

`make/harbor.yml` → prepare script → runtime config. Categories: DB, Redis, registry storage, LDAP/OIDC, certs.
