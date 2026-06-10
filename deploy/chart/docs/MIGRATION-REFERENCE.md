# Migration reference: goharbor/harbor-helm (2.x) → this chart

The normative, value-by-value mapping reference for migrating from the
legacy [goharbor/harbor-helm](https://github.com/goharbor/harbor-helm)
chart ("2.x" below) to this chart. It is the **prose version of
[`harbor-migrate.ys`](harbor-migrate.ys)** — the script implements exactly
these tables, and its `ERROR`/`WARN`/`INFO` advisories correspond to the
*N/A* rows, the Notes columns, and the removed-settings table here.

Written for humans and LLMs alike: when translating a values file by hand
(or prompting a model to do it), work through the tables top to bottom —
every 2.x value is either listed with its new location and semantics, or
maps 1:1 unchanged.

For the migration *guide* — breaking changes overview, migration paths,
secret carry-over, the worked example, and verification steps — see
[MIGRATION.md](MIGRATION.md).

## Value mapping tables

Legend: **same** = identical path and semantics. *N/A* = no equivalent (see
[Removed settings](#removed-settings-and-workarounds)).

### Exposure

The 2.x `expose.type` selector is replaced by per-method `enabled` flags.
Exactly one may be enabled (template validation enforces this). Ingress is
the default.

| 2.x | This chart | Notes |
|---|---|---|
| `expose.type: ingress` | `ingress.enabled: true` | Default |
| `expose.type: clusterIP` | `expose.clusterIP.enabled: true` + `ingress.enabled: false` | |
| `expose.type: nodePort` | `expose.nodePort.enabled: true` + `ingress.enabled: false` | |
| `expose.type: loadBalancer` | `expose.loadBalancer.enabled: true` + `ingress.enabled: false` | |
| `expose.type: route` (Gateway API HTTPRoute) | `gateway.enabled: true` + `ingress.enabled: false` | 2.x reused the name "route" for Gateway API. Here `expose.route` is an **OpenShift Route** instead |
| `expose.ingress.hosts.core` | `externalURL` hostname (and optionally `ingress.hosts[].host`) | Ingress host defaults to the `externalURL` hostname |
| `expose.ingress.className` | `ingress.className` | |
| `expose.ingress.annotations` | `ingress.annotations` | ssl-redirect / proxy-body-size annotations are no longer pre-set — add the ones your controller needs |
| `expose.ingress.labels` | `ingress.labels` | |
| `expose.ingress.controller` | *N/A* | No controller-specific template logic remains |
| `expose.ingress.kubeVersionOverride` | *N/A* | |
| `expose.route.parentRefs` | `gateway.parentRefs` | |
| `expose.route.hosts` | `gateway.hostnames` | |
| `expose.clusterIP.name` | `expose.clusterIP.name` | Empty now defaults to the release fullname (2.x default was `harbor`) |
| `expose.clusterIP.staticClusterIP` | `expose.clusterIP.staticClusterIP` | |
| `expose.clusterIP.ports.httpPort` / `httpsPort` | `expose.clusterIP.ports.http` / `https` | Key rename |
| `expose.nodePort.*` | `expose.nodePort.*` | Same shape |
| `expose.loadBalancer.IP` | `expose.loadBalancer.IP` | |
| `expose.loadBalancer.ports.httpPort` / `httpsPort` | `expose.loadBalancer.ports.http` / `https` | Key rename |
| `expose.loadBalancer.sourceRanges` | `expose.loadBalancer.sourceRanges` | |
| `expose.*.annotations` / `labels` | same | |

### TLS

| 2.x | This chart | Notes |
|---|---|---|
| `expose.tls.enabled` | `tls.enabled` | Default flipped: now `false` (terminate TLS at the ingress controller / LB) |
| `expose.tls.certSource` (`auto`/`secret`/`none`) | `tls.certSource` | Same values; additionally `tls.certManager.*` for cert-manager-issued certificates |
| `expose.tls.auto.commonName` | `ingress.core` | Hostname override for the auto-generated certificate |
| `expose.tls.secret.secretName` | `tls.customSecrets.core` (and `tls.customSecrets.registry`) | |
| — | `ingress.autoGenCert` | New: toggle for the auto-generated ingress certificate |
| `internalTLS.*` | *N/A* | Component-to-component TLS is not supported; use a service mesh |

### Persistence and registry storage

Per-component `persistence` blocks replace the central
`persistence.persistentVolumeClaim` tree. **Defaults flipped from `true` to
`false`** — enable explicitly if you store images on a PVC.

| 2.x | This chart | Notes |
|---|---|---|
| `persistence.enabled` | `registry.persistence.enabled`, `jobservice.persistence.enabled`, `trivy.persistence.enabled` | Per component, default `false` |
| `persistence.resourcePolicy` | `registry.persistence.resourcePolicy`, `jobservice.persistence.resourcePolicy` | Same `"keep"` semantics |
| `persistence.persistentVolumeClaim.registry.{existingClaim,storageClass,size,annotations}` | `registry.persistence.{existingClaim,storageClass,size,annotations}` | |
| `persistence.persistentVolumeClaim.registry.accessMode` | `registry.persistence.accessModes` | Now a **list** |
| `persistence.persistentVolumeClaim.registry.subPath` | *N/A* | |
| `persistence.persistentVolumeClaim.jobservice.jobLog.*` | `jobservice.persistence.*` | Same renames as registry |
| `persistence.persistentVolumeClaim.database.*` | *N/A* | No internal database |
| `persistence.persistentVolumeClaim.redis.*` | `valkey.dataStorage` (subchart) | Redis data is disposable |
| `persistence.persistentVolumeClaim.trivy.*` | `trivy.persistence.*` | |

Registry storage backends are no longer chart-templated fields — the full
[distribution `config.yml`](https://distribution.github.io/distribution/about/configuration/)
passes through verbatim under `registry.config`. Set exactly one driver key
under `storage:` — the chart default ships no driver, so your `s3:` simply
becomes the active driver (filesystem is injected only when no driver is set;
two or more drivers fail at `helm install` time):

| 2.x | This chart | Notes |
|---|---|---|
| `persistence.imageChartStorage.type: <backend>` | `registry.config.storage.<backend>: {...}` | Any distribution storage driver/field works |
| `persistence.imageChartStorage.disableredirect: true` | `registry.config.storage.redirect.disable: true` | Native distribution syntax |
| `persistence.imageChartStorage.caBundleSecretName` | `externalRedis.tlsOptions.existingCaSecret` | Despite the name, this Secret is mounted on **every** component with `SSL_CERT_DIR` set — it covers private CAs for S3, OIDC, LDAP, and Redis alike. See `example/private-ca.yaml` |
| `...filesystem.rootdirectory` | `registry.config.storage.filesystem.rootdirectory` | |
| `...s3.{region,bucket,regionendpoint,...}` | `registry.config.storage.s3.{...}` | All distribution s3 fields pass through |
| `...s3.existingSecret` | `registry.storageCredentials.s3.existingSecret` | Same key names (`REGISTRY_STORAGE_S3_ACCESSKEY` / `REGISTRY_STORAGE_S3_SECRETKEY`) — your existing Secret works as-is |
| `...s3.accesskey` / `secretkey` (inline) | `registry.secret.REGISTRY_STORAGE_S3_ACCESSKEY` / `..._SECRETKEY` | Or better: a pre-created Secret via `storageCredentials` |
| `...azure.{accountname,container,realm}` | `registry.config.storage.azure.{...}` | |
| `...azure.accountkey` / `existingSecret` | `registry.storageCredentials.azure.existingSecret` | 2.x Secret used key `AZURE_STORAGE_ACCESS_KEY` — reuse it by setting `storageCredentials.azure.existingSecretKey: AZURE_STORAGE_ACCESS_KEY` |
| `...gcs.bucket` / `rootdirectory` / `chunksize` | `registry.config.storage.gcs.{...}` | |
| `...gcs.encodedkey` / `existingSecret` | `registry.storageCredentials.gcs.existingSecret` | Keyfile is mounted at `/etc/registry/gcs/key.json`; set `registry.config.storage.gcs.keyfile` to that path. 2.x Secret used key `GCS_KEY_DATA` — set `storageCredentials.gcs.existingSecretKey: GCS_KEY_DATA` to reuse it |
| `...gcs.useWorkloadIdentity` | Leave `storageCredentials.gcs.existingSecret` empty + set `registry.serviceAccount.annotations` | |
| `...swift.*` | `registry.config.storage.swift.{...}` + credentials via `registry.secret` (`REGISTRY_STORAGE_SWIFT_PASSWORD`, …) | No `storageCredentials` shortcut for swift |
| `...oss.*` | `registry.config.storage.oss.{...}` | |
| `...oss.existingSecret` | `registry.storageCredentials.oss.existingSecret` | Same key (`REGISTRY_STORAGE_OSS_ACCESSKEYSECRET`) |

### Global settings

| 2.x | This chart | Notes |
|---|---|---|
| `externalURL` | `externalURL` | Now required, no default |
| `harborAdminPassword` | `harborAdminPassword` | Now required (or `existingSecretAdminPassword`), no default |
| `existingSecretAdminPassword` / `...Key` | same | |
| `secretKey` | `secretKey` | Auto-generated if empty. **Copy your 2.x value when reusing the database** |
| `existingSecretSecretKey` | `existingSecretSecretKey` | Secret must hold the key under both `SECRET_KEY` and `secretKey` (or set the new `existingSecretSecretKeyKey`) |
| `logLevel` | `logLevel` | Does **not** propagate to jobservice loggers — set those in `jobservice.config.loggers[].level` / `job_loggers[].level` |
| `imagePullPolicy` | `image.pullPolicy` | |
| `imagePullSecrets` | `imagePullSecrets` | New alternative: `imageCredentials` creates the pull Secret for you |
| `updateStrategy.type` | `core.deploymentStrategy`, `registry.deploymentStrategy`, … per component | Full Deployment strategy spec, e.g. `deploymentStrategy.type: Recreate` for RWO volumes |
| `proxy.*` | `proxy.*` | Identical |
| `cache.*` | `cache.*` | Identical |
| `trace.*` | `trace.*` | Identical structure |
| `ipFamily.ipv4.enabled` / `ipv6.enabled` | same | |
| `ipFamily.policy` / `ipFamily.families` | *N/A* | |
| `containerSecurityContext` (one global block) | `<component>.securityContext` + `<component>.podSecurityContext` | Defaults already comply with PSS Restricted |
| `<component>.revisionHistoryLimit` | `global.revisionHistoryLimit` | One global knob (default 3) |
| `<component>.priorityClassName` | `global.priorityClassName` | One global knob |
| `caBundleSecretName` | `externalRedis.tlsOptions.existingCaSecret` | Generic extra-CA mount on all components (`SSL_CERT_DIR`) |
| `uaaSecretName` | `externalRedis.tlsOptions.existingCaSecret` | Same mechanism |
| `caSecretName` (portal CA download link) | *N/A* | |
| `enableMigrateHelmHook` | *N/A* | Schema migrations run at core startup (2.x default behavior) |

### Core

| 2.x | This chart | Notes |
|---|---|---|
| `core.image.repository` / `tag` | `core.image.repository` / `tag` | Different default registry; tag defaults to chart `appVersion` |
| `core.replicas` | `core.replicas` | New: `core.autoscaling` (HPA) — when enabled, `replicas` is ignored |
| `core.serviceAccountName` | `core.serviceAccount.name` | Chart creates an SA by default (`create: true`); set `create: false` to bring your own |
| `core.automountServiceAccountToken` | `core.serviceAccount.automountServiceAccountToken` | |
| `core.podDisruptionBudget.*` | `core.pdb.*` | Set exactly one of `minAvailable` / `maxUnavailable` |
| `core.startupProbe.enabled` / `initialDelaySeconds` | same | Other probe fields are fixed |
| `core.livenessProbe.*` / `readinessProbe.*` | *N/A* | Probes are not configurable |
| `core.extraEnvVars` | `core.extraEnv` | Same shape (supports `valueFrom`) |
| `core.nodeSelector` / `tolerations` / `affinity` / `topologySpreadConstraints` | same | |
| `core.podAnnotations` / `podLabels` / `serviceAnnotations` / `initContainers` | same | |
| `core.configureUserSettings` | same | |
| `core.quotaUpdateProvider` | same | |
| `core.secret` (string — inter-component secret) | auto-generated, or `core.existingSecret` / `core.existingSecretKey` | ⚠️ `core.secret` in this chart is a **map** of extra Secret env vars (`toEnvVars`), not a string. An inline string value is not supported |
| `core.existingSecret` | `core.existingSecret` | Same (key default `secret`) |
| `core.secretName` (token cert Secret) | `core.tokenSecretName` | Rename; keys `tls.crt` / `tls.key` unchanged |
| `core.tokenKey` / `tokenCert` | same | |
| `core.xsrfKey`, `core.existingXsrfSecret` / `...Key` | same | |
| `core.artifactPullAsyncFlushDuration` | same | |
| `core.gdpr.deleteUser` / `auditLogsCompliant` | same | |
| — | `core.config` / `core.secret` (maps) | New: any Harbor core env-config without chart changes |

### Jobservice

Jobservice config moved into a verbatim `config.yml` passthrough
(`jobservice.config`) plus an env map (`jobservice.env`, keys flattened to
`UPPER_SNAKE_CASE`).

| 2.x | This chart | Notes |
|---|---|---|
| `jobservice.maxJobWorkers` | `jobservice.config.worker_pool.workers` | |
| `jobservice.jobLoggers: [file, ...]` | `jobservice.config.job_loggers` / `loggers` | Full logger structures instead of name list — see the default in `values.yaml` |
| `jobservice.loggerSweeperDuration` | `jobservice.config.job_loggers[].sweeper.duration` | |
| `jobservice.notification.webhook_job_max_retry` | `jobservice.env.jobservice_webhook_job_max_retry` | Renders as `JOBSERVICE_WEBHOOK_JOB_MAX_RETRY` |
| `jobservice.notification.webhook_job_http_client_timeout` | `jobservice.env.jobservice_webhook_job_http_client_timeout` | |
| `jobservice.registryHttpClientTimeout` | `jobservice.env.registry_http_client_timeout` | Renders as `REGISTRY_HTTP_CLIENT_TIMEOUT` |
| `jobservice.reaper.*` | `jobservice.config.reaper.*` | |
| `jobservice.secret` (string) | auto-generated, or `jobservice.existingSecret` / `...Key` | Same caveat as `core.secret`: the `secret` key is now a map |
| `jobservice.image`, `replicas`, `podDisruptionBudget`→`pdb`, `extraEnvVars`→`extraEnv`, scheduling/annotation fields | same pattern as core | |

### Registry

| 2.x | This chart | Notes |
|---|---|---|
| `registry.registry.image.*` | `registry.image.*` | One nesting level removed |
| `registry.controller.image.*` | `registry.controller.image.*` | Same |
| `registry.registry.resources` / `registry.controller.resources` | `registry.resources` | Single block for both containers |
| `registry.registry.extraEnvVars` / `controller.extraEnvVars` | `registry.extraEnv` | Single list |
| `registry.relativeurls` | `registry.config.http.relativeurls` | Native distribution syntax |
| `registry.upload_purging.*` | `registry.config.storage.maintenance.uploadpurging.*` | Already in the chart default `config` |
| `registry.middleware.cloudFront.*` | `registry.config.middleware` (passthrough) | ⚠️ The CloudFront **private key Secret mount** has no supported equivalent yet (no `extraVolumes`) — open an issue if you need it |
| `registry.secret` (string — `REGISTRY_HTTP_SECRET`) | auto-generated, or `registry.existingSecret` / `...Key` | `registry.secret` is now a map injected via `envFrom` |
| `registry.credentials.{username,password,existingSecret,htpasswdString}` | same | |
| `registry.replicas`, `podDisruptionBudget`→`pdb`, scheduling/annotation fields | same pattern as core | |

### Portal and nginx

| 2.x | This chart | Notes |
|---|---|---|
| `nginx.*` | *N/A* | The nginx reverse proxy is gone; Ingress/Gateway/expose Services route to core, which proxies the portal |
| `portal.*` (standard fields) | `portal.*` | Same pattern as core (`extraEnvVars`→`extraEnv`, `podDisruptionBudget`→`pdb`, `serviceAccountName`→`serviceAccount.name`) |
| — | `portal.existingConfigMap` | New: bring your own `nginx.conf` for the portal's static file server |

### Trivy

| 2.x | This chart | Notes |
|---|---|---|
| `trivy.enabled` | `trivy.enabled` | Default flipped: now `false` |
| `trivy.image`, `replicas`, `resources`, `podDisruptionBudget`→`pdb` | same pattern | StatefulSet in both charts; new: `trivy.autoscaling` |
| `trivy.{debugMode,vulnType,severity,ignoreUnfixed,insecure,gitHubToken,skipUpdate,skipJavaDBUpdate,dbRepository,javaDBRepository,offlineScan,securityCheck,timeout}` | identical | |
| `persistence.persistentVolumeClaim.trivy.*` | `trivy.persistence.*` | |
| — | `trivy.config` / `trivy.secret` | New: any `SCANNER_*` adapter env without chart changes |

### Database

| 2.x | This chart | Notes |
|---|---|---|
| `database.type: internal` + `database.internal.*` | *N/A* | **External PostgreSQL required.** `pg_dump` the internal DB and restore it externally, or start fresh |
| `database.external.host` / `port` / `username` / `password` | `database.host` / `port` / `username` / `password` | Flattened; `port` is now a number |
| `database.external.coreDatabase` | `database.database` | Rename |
| `database.external.existingSecret` | `database.existingSecret` | 2.x required key `password`; default key here is `POSTGRESQL_PASSWORD` — set `database.existingSecretKey: password` to reuse your existing Secret |
| `database.external.sslmode` | `database.sslmode` | |
| `database.maxIdleConns` / `maxOpenConns` | same | New: `connMaxIdleTime`, `connMaxLifetime` |
| `database.podAnnotations` / `podLabels` | *N/A* | No database pods |
| — | `database.existingTlsSecret`, `database.clientCertEnabled` | New: `verify-ca`/`verify-full` with a private CA, optional mTLS |

### Redis / Valkey

| 2.x | This chart | Notes |
|---|---|---|
| `redis.type: internal` | `valkey.enabled: true` (default) | Bitnami-style Valkey subchart; `redis.internal.*` fields map to subchart values (`valkey.architecture`, `valkey.auth`, `valkey.dataStorage`, …) |
| `redis.type: external` | `valkey.enabled: false` + `externalRedis.*` | |
| `redis.external.addr` (`host:port`) | `externalRedis.host` + `externalRedis.port` | Split into two fields |
| `redis.external.sentinelMasterSet` | `externalRedis.sentinelMasterSet` | |
| `redis.external.username` / `password` | `externalRedis.username` / `password` | |
| `redis.external.existingSecret` | `externalRedis.existingSecret` | Same default key `REDIS_PASSWORD`, configurable via `existingSecretKey` |
| `redis.external.tlsOptions.enable` | `externalRedis.tlsOptions.enable` | New: `existingCaSecret` for private CAs |
| `redis.*.{coreDatabaseIndex,jobserviceDatabaseIndex,registryDatabaseIndex,trivyAdapterIndex,harborDatabaseIndex,cacheLayerDatabaseIndex}` | *N/A* — fixed | DB indexes are hardcoded: core `0`, jobservice `1`, registry `2`, trivy `5`, harbor `6`, cache layer `7`. Point Harbor at a Redis logical-DB range you don't share with other apps |

### Exporter and metrics

| 2.x | This chart | Notes |
|---|---|---|
| (exporter deployed when `metrics.enabled`) | `exporter.enabled` | Explicit toggle, default `true` |
| `exporter.cacheDuration` | `exporter.config.cache_time` | |
| `exporter.cacheCleanInterval` | `exporter.config.cache_clean_interval` | |
| `exporter.image`, `replicas`, `podDisruptionBudget`→`pdb`, `extraEnvVars`→`extraEnv`, scheduling fields | same pattern as core | |
| `metrics.enabled` | `metrics.enabled` | Does **not** propagate to jobservice — also set `jobservice.config.metric.{enabled,path,port}` |
| `metrics.{core,registry,jobservice,exporter}.path` / `port` | *N/A* — fixed | Always `/metrics` on port `8001` |
| `metrics.serviceMonitor.enabled` | same | Requires `metrics.enabled: true` (validated at template time) |
| `metrics.serviceMonitor.additionalLabels` | `metrics.serviceMonitor.labels` | Rename |
| `metrics.serviceMonitor.interval` | same | New: `scrapeTimeout`, `honorLabels`, `namespace` |
| `metrics.serviceMonitor.metricRelabelings` / `relabelings` | *N/A* | |

## Removed settings and workarounds

| 2.x setting | Why it's gone | Workaround |
|---|---|---|
| `database.internal.*` | Running stateful PostgreSQL inside the app chart is an operational liability | Managed PG or an operator (CloudNativePG example in the chart README) |
| `nginx.*` | Modern ingress controllers make the extra proxy hop redundant | `ingress.annotations` for body-size/ssl-redirect tuning; `portal.existingConfigMap` for custom portal nginx config |
| `internalTLS.*` | ~30-value surface that overlaps with service-mesh transparent TLS | Istio/Linkerd |
| Probe tuning (`livenessProbe`/`readinessProbe` per component) | Sane fixed defaults; tuning probes usually masks real problems | `core.startupProbe` covers the slow-boot case |
| `metrics.<comp>.port`/`path` | Fixed `8001`/`/metrics` simplifies the ServiceMonitor | — |
| Redis DB index selection | Fixed indexes keep the URL helpers simple | Dedicated Redis/Valkey instance or logical-DB separation on your side |
| `enableMigrateHelmHook` | Migrations run at core startup | — |
| `caSecretName` (portal CA download link) | Niche; CA distribution belongs outside Harbor | Distribute your CA via your platform tooling |
| `expose.ingress.controller` (gce/ncp/alb special-casing) | Controller-specific quirks belong in annotations | `ingress.annotations` |

