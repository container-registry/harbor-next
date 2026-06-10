# Migrating from goharbor/harbor-helm (v1.x)

This guide covers migrating from the legacy
[goharbor/harbor-helm](https://github.com/goharbor/harbor-helm/blob/main/values.yaml)
chart, referred to as "2.x" below (chart v1.x, Harbor app v2.x). The
value-by-value mapping tables live in
[MIGRATION-REFERENCE.md](MIGRATION-REFERENCE.md); an automated translator is
described in the next section.

This chart is a ground-up redesign, **not a drop-in replacement**. You cannot
`helm upgrade` an existing release in place — the resource names, labels,
selectors, and configuration model all differ. Plan a new installation plus a
data migration.

## Automated translation: harbor-migrate.ys

[`harbor-migrate.ys`](harbor-migrate.ys) (in this directory) converts a 2.x
values file to this chart's format and prints an advisory report of
everything that changes, gets dropped, or will not work as expected. It needs
[YAMLScript](https://yamlscript.org) (`brew install yamlscript`):

```bash
# from the chart directory
ys docs/harbor-migrate.ys old-values.yaml new-values.yaml

# or via Task
task migrate -- old-values.yaml new-values.yaml

# report only (values to /dev/null)
ys docs/harbor-migrate.ys old-values.yaml > /dev/null
```

The migrated values go to `new-values.yaml` (or stdout); the report goes to
stderr with three severity levels:

| Level | Meaning |
|---|---|
| `ERROR` | Will not work — manual action required before install (internal database, Notary/ChartMuseum, CloudFront key Secret, inline GCS keys) |
| `WARN` | Migrated, but semantics changed — review (publicly known default credentials, fixed Redis DB indexes, dropped inline component secrets, probe tuning) |
| `INFO` | Mapped automatically — listed so nothing changes silently |

What it does for you: applies every rename/restructure from the
[MIGRATION-REFERENCE.md](MIGRATION-REFERENCE.md) tables,
converts storage backends to the `registry.config` passthrough (wiring
`storageCredentials` with the legacy Secret key names), expands
`jobservice.jobLoggers` into full logger structures, splits
`redis.external.addr` into host/port, flips `expose.type` to the per-method
flags, translates ingress TLS, and adds the `jobservice.config.metric` block
when metrics are enabled. Values that match the 2.x defaults of unchanged
settings are omitted instead of restated.

It also coerces quoted scalars (`"true"`, `"5432"`) to native booleans and
integers — the legacy chart tolerated them, but this chart's
`values.schema.json` rejects them at install time.

What it cannot do: move data (database contents, images), create Kubernetes
Secrets, carry over inline component secret strings, or make an in-place
`helm upgrade` possible. Always read the report and run `helm template` with
the output before installing — the script's golden tests
(`task test:migrate`) do exactly that with two fixtures:
[`tests/migrate/values-2x.yaml`](../tests/migrate/values-2x.yaml) (the worked
example below) and
[`tests/migrate/values-dedicated-2x.yaml`](../tests/migrate/values-dedicated-2x.yaml),
a sanitized real-world tenant values file from the dedicated-container-registry
deployment engine (S3 storage, external PG/Redis, cert-manager ingress,
string-typed booleans). Their expected outputs and advisory reports sit next
to them (`values*-next.yaml`, `report*.txt`).

## Breaking changes at a glance

| Change | 2.x | This chart | What you must do |
|---|---|---|---|
| Internal PostgreSQL | `database.type: internal` (StatefulSet) | **Removed** — external DB only | Provision PostgreSQL (managed PG, CloudNativePG, etc.) and `pg_dump`/restore if you keep your data |
| Internal Redis | `redis.type: internal` | Valkey subchart (`valkey.enabled: true`, default) or `externalRedis` | Nothing — Redis holds caches and job queues; starting fresh is safe |
| nginx proxy | `nginx.*` deployment in front of core/portal | **Removed** — Ingress/Gateway/Service points at core directly | Drop all `nginx.*` values |
| Required values | Defaults for everything | `externalURL`, `database.host`, and `harborAdminPassword` (or `existingSecretAdminPassword`) are **required** — install fails without them | Set them explicitly |
| Admin password | Defaults to `Harbor12345` | No default | Set a strong password |
| `secretKey` | Defaults to `not-a-secure-key` | Auto-generated if empty | **Carry over your old value if you reuse the database** (see below) |
| Persistence | `persistence.enabled: true` | Per-component, **default `false`** | Enable `registry.persistence` (and `jobservice.persistence`) if you use filesystem storage |
| Trivy | `trivy.enabled: true` | Default `false` | Set `trivy.enabled: true` if you scan images |
| TLS | `expose.tls.enabled: true` | `tls.enabled: false` (terminate at ingress) | Revisit the [TLS section](MIGRATION-REFERENCE.md#tls) |
| Internal TLS | `internalTLS.*` | **Not supported** | Use a service mesh for pod-to-pod TLS |
| Images | `docker.io/goharbor/*` | `8gears.container-registry.com/8gcr/harbor-*` (Harbor Next builds) | Nothing, unless you mirror images |
| Probes | Fully configurable per component | Fully configurable via `<component>.probes.{startup,liveness,readiness}` (full K8s probe specs) | Move probe overrides into `<component>.probes` |
| Notary / ChartMuseum | Removed in later 2.x releases too | Not supported | Use cosign/OCI artifacts |

## Choosing a migration path

**A. Replicate.** Install this chart as a new Harbor instance, then use
Harbor's replication feature (or re-push from CI) to copy projects and
artifacts. Users, robot accounts, and settings must be recreated (or
scripted via the Harbor API). No secret carry-over needed.

**B. Reuse the existing database and storage backend.** Possible when your
2.x install already used (or you first migrate to) an external PostgreSQL and
object storage. Point this chart at the same database and the same
`registry.config.storage` backend. You **must** carry over:

- `secretKey` — encrypts credentials stored in the database (replication
  endpoints, scanner credentials, …). With a different key Harbor cannot
  decrypt them.

  ```bash
  kubectl get secret <old-release>-core -o jsonpath='{.data.secretKey}' | base64 -d
  ```

- The database itself — `pg_dump` from the internal `harbor-database` pod
  into your external PostgreSQL if you were on `database.type: internal`.
- The target Harbor app version must be **>=** the source version (schema
  migrations only run forward).

Admin password hashes, users, and projects live in the database and survive
the move. `core.secret`, `jobservice.secret`, and `REGISTRY_HTTP_SECRET` are
inter-component/transient secrets — letting the chart regenerate them is fine
(in-flight uploads are invalidated, nothing else).

---

## Value-by-value reference

The complete mapping tables — every 2.x value, its new location and
semantics, what is dropped, and the workarounds — live in
[MIGRATION-REFERENCE.md](MIGRATION-REFERENCE.md), the prose counterpart of
the `harbor-migrate.ys` script. Use it when translating a values file by
hand (or with an LLM) and when reviewing the script's advisory report.

## Worked example

A typical 2.x production values file and its translation:

**2.x (`goharbor/harbor-helm`):**

```yaml
expose:
  type: ingress
  tls:
    enabled: true
    certSource: secret
    secret:
      secretName: harbor-tls
  ingress:
    hosts:
      core: harbor.example.com
    className: nginx
externalURL: https://harbor.example.com
harborAdminPassword: "changeme"
secretKey: "0123456789abcdef"
persistence:
  enabled: true
  imageChartStorage:
    type: s3
    s3:
      region: eu-central-1
      bucket: my-harbor
      existingSecret: harbor-s3-creds
database:
  type: external
  external:
    host: pg.example.com
    coreDatabase: registry
    username: harbor
    existingSecret: harbor-db        # key: password
redis:
  type: external
  external:
    addr: redis.example.com:6379
    existingSecret: harbor-redis     # key: REDIS_PASSWORD
trivy:
  enabled: true
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

**This chart:**

```yaml
externalURL: https://harbor.example.com
harborAdminPassword: "changeme"      # better: existingSecretAdminPassword
secretKey: "0123456789abcdef"        # MUST match the old install (reused DB)

ingress:
  enabled: true                      # default
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "0"
  tls:
    - secretName: harbor-tls
      hosts:
        - harbor.example.com

registry:
  config:
    storage:
      s3:
        region: eu-central-1
        bucket: my-harbor
      cache:
        layerinfo: redis
  storageCredentials:
    s3:
      existingSecret: harbor-s3-creds  # same keys as 2.x — reused as-is

database:
  host: pg.example.com
  database: registry
  username: harbor
  existingSecret: harbor-db
  existingSecretKey: password        # 2.x key name

valkey:
  enabled: false
externalRedis:
  host: redis.example.com
  port: 6379
  existingSecret: harbor-redis       # same key REDIS_PASSWORD

trivy:
  enabled: true

metrics:
  enabled: true
  serviceMonitor:
    enabled: true

jobservice:
  config:
    metric:                          # metrics.enabled does not reach jobservice
      enabled: true
      path: /metrics
      port: 8001
```

## Verifying the migration

1. `helm template` with your translated values — template-time validation
   catches missing required values, multiple expose methods, and HPA
   misconfiguration before anything reaches the cluster.
2. Install into a throwaway namespace and check every pod becomes Ready.
3. Log in with the admin account; confirm projects/users are present (path B)
   or set up replication (path A).
4. `docker login` + push + pull against `externalURL`.
5. If Trivy is enabled, trigger a scan and confirm it completes.
6. Confirm replication endpoints and scanner credentials still decrypt
   (path B — this is the `secretKey` carry-over test).
