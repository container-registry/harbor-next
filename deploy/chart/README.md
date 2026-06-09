# harbor-next

![Version: 3.0.0](https://img.shields.io/badge/Version-3.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v2.15.0](https://img.shields.io/badge/AppVersion-v2.15.0-informational?style=flat-square)

A modern, production-ready Helm chart for [Harbor Next](https://github.com/container-registry/harbor-next) - the cloud native container registry for Kubernetes.

## TL;DR

```bash
# Provide DB credentials via a Secret instead of --set (process list + shell history leak)
kubectl create secret generic my-harbor-db \
  --from-literal=POSTGRESQL_PASSWORD='your-strong-password'

helm install my-harbor oci://8gears.container-registry.com/8gcr/charts/harbor-next \
  --set externalURL=https://harbor.example.com \
  --set database.host=my-postgres.example.com \
  --set database.existingSecret=my-harbor-db
```

> **⚠️ Security:** Harbor's default admin credentials (`admin` / `Harbor12345`) are
> publicly known. Set a strong password via `harborAdminPassword` (or, better, a
> Kubernetes Secret referenced by `existingSecretAdminPassword`) and rotate it after
> the first login.

## Why This Chart?

This chart is a ground-up redesign of the Harbor Helm chart with modern Kubernetes best practices, built for [Harbor Next](https://github.com/container-registry/harbor-next):

| Feature | Legacy harbor-helm | This Chart |
|---------|-------------------|------------|
| **Configuration** | 70+ helper templates, hardcoded env vars | `toEnvVars` pattern — any config works |
| **Database** | Built-in PostgreSQL StatefulSet | External only (production best practice) |
| **Redis** | Built-in Redis Deployment | Valkey subchart or external |
| **Ingress** | nginx reverse proxy + Ingress | Direct Ingress/Gateway API |
| **Security** | Basic security context | PSS Restricted profile compliant |
| **Validation** | None | JSON Schema validation |
| **Resource Defaults** | None (comments only) | Sensible defaults for all components |
| **PodDisruptionBudget** | Not available | Per-component PDB support |
| **Templates** | 48 files, 607-line helpers | 43 files, ~800-line helpers |
| **values.yaml** | 1,116 lines | ~1,150 lines (317 helm-docs annotations) |

## Key Features

### Future-Proof Configuration with `toEnvVars`

The chart uses a unique `toEnvVars` pattern that converts nested YAML configuration to flat environment variables. This means **any Harbor configuration option works without chart updates**:

```yaml
core:
  config:
    # These become CONFIG_KEY and NESTED_VALUE env vars
    config_key: "value"
    nested:
      value: "something"
  secret:
    # These become secrets (base64 encoded)
    sensitive_data: "secret-value"
```

### Production-Ready Security

All containers run with Pod Security Standards (PSS) **Restricted** profile:

- `runAsNonRoot: true` — No root containers
- `readOnlyRootFilesystem: true` — Immutable container filesystem
- `allowPrivilegeEscalation: false` — No privilege escalation
- `capabilities.drop: ["ALL"]` — No Linux capabilities
- `seccompProfile.type: RuntimeDefault` — Seccomp filtering enabled

### High Availability Ready

- **PodDisruptionBudgets** — Ensure availability during node maintenance
- **Resource requests/limits** — Guaranteed QoS with sensible defaults
- **Affinity/Anti-affinity** — Control pod placement
- **Multiple replicas** — Scale any component horizontally

### Flexible Ingress Options

1. **Standard Kubernetes Ingress** (default)
2. **Gateway API HTTPRoute** (modern alternative)
3. **extraManifests** for custom routing (Traefik IngressRoute, etc.)

### Schema Validation

Built-in `values.schema.json` provides:
- IDE autocompletion and validation
- Required field enforcement (`externalURL`, `database.host`)
- Type checking and enum validation
- Immediate feedback on configuration errors

## Architecture

<!--
```SVGBob
┌───────────────────────────────────────────────────────────────────┐
│                    "Ingress / Gateway API"                        │
└───────────────────────────────────────────────────────────────────┘
                                │
          ┌─────────────────────┼─────────────────────┐
          ▼                     ▼                     ▼
   ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
   │   Portal    │       │    Core     │◄─────►│  Registry   │
   │   "(UI)"    │       │   "(API)"   │       │  "(Images)" │
   └─────────────┘       └──────┬──────┘       └──────┬──────┘
                                │                     │
                       ┌────────┴────────┐            │
                       ▼                 ▼            │
                ┌─────────────┐   ┌─────────────┐     │
                │ Jobservice  │   │  Exporter   │     │
                │  "(Tasks)"  │   │ "(Metrics)" │     │
                └──────┬──────┘   └─────────────┘     │
                       │                              │
          ┌────────────┴────────────┬─────────────────┘
          ▼                         ▼
   ┌─────────────┐          ┌──────────────┐
   │   Valkey    │          │   Storage    │
   │  "(Redis)"  │          │"(PVC/S3/.)"  │
   └─────────────┘          └──────────────┘
          │
          ▼
   ┌────────────────┐
   │   PostgreSQL   │  "(External — required)"
   └────────────────┘
```
-->

## Prerequisites

- Kubernetes 1.28+ (we follow [endoflife.date/kubernetes](https://endoflife.date/kubernetes) for supported versions)
- Helm 3.x
- **External PostgreSQL database** (required)
- PV provisioner (for filesystem storage)

## Installing the Chart

### Basic Installation

```bash
# Pre-create the DB credential and admin Secrets so passwords never appear on the CLI
kubectl create namespace harbor
kubectl -n harbor create secret generic my-harbor-db \
  --from-literal=POSTGRESQL_PASSWORD='your-strong-db-password'
kubectl -n harbor create secret generic my-harbor-admin \
  --from-literal=HARBOR_ADMIN_PASSWORD='your-strong-admin-password'

helm install my-harbor oci://8gears.container-registry.com/8gcr/charts/harbor-next \
  --namespace harbor \
  --set externalURL=https://harbor.example.com \
  --set database.host=postgres.example.com \
  --set database.existingSecret=my-harbor-db \
  --set existingSecretAdminPassword=my-harbor-admin
```

### With Values File

```bash
helm install my-harbor oci://8gears.container-registry.com/8gcr/charts/harbor-next \
  --namespace harbor \
  --create-namespace \
  -f values-production.yaml
```

## Configuration

### Required Values

| Key | Description |
|-----|-------------|
| `externalURL` | Public URL for Harbor (e.g., `https://harbor.example.com`) |
| `database.host` | PostgreSQL host |
| `database.password` | PostgreSQL password (or use `database.existingSecret`) |

### Component Configuration

Each Harbor component (core, portal, registry, jobservice, exporter) supports:

| Key | Description |
|-----|-------------|
| `<component>.replicas` | Number of replicas (ignored when `autoscaling.enabled=true`) |
| `<component>.autoscaling` | `enabled`/`minReplicas`/`maxReplicas`/CPU+memory targets — see [HPA section](#hpa--autoscaling) |
| `<component>.resources` | CPU/memory requests and limits |
| `<component>.config` | Application config (becomes ConfigMap env vars) |
| `<component>.secret` | Sensitive config (becomes Secret env vars) |
| `<component>.extraEnv` | Additional env vars with `valueFrom` support |
| `<component>.lifecycle` | Container `preStop`/`postStart` hook spec — see [Lifecycle hooks](#lifecycle-hooks-prestop-drain) |
| `<component>.hostAliases` | `/etc/hosts` entries (list of `{ip, hostnames}`) for private DNS targets |
| `<component>.pdb.enabled` | Enable PodDisruptionBudget |
| `<component>.pdb.minAvailable` | Minimum available pods during disruption |
| `<component>.affinity` | Pod affinity rules |
| `<component>.nodeSelector` | Node selection constraints |
| `<component>.tolerations` | Pod tolerations |
| `<component>.securityContext` | Container security context |
| `<component>.podSecurityContext` | Pod security context |
| `<component>.serviceAccount.create` | Create dedicated ServiceAccount |

### Default Resource Allocations

| Component | CPU Request | Memory Request | Memory Limit |
|-----------|-------------|----------------|--------------|
| Core | 100m | 256Mi | 512Mi |
| Registry | 100m | 256Mi | 512Mi |
| Portal | 100m | 128Mi | 256Mi |
| Jobservice | 100m | 256Mi | 512Mi |
| Exporter | 100m | 128Mi | 256Mi |

## Example Values Files

The [`example/`](example/) directory contains ready-to-use values files for common environments:

| File | Description |
|------|-------------|
| [`k3d-local.yaml`](example/k3d-local.yaml) | Local development with k3d cluster |
| [`rke2-rancher.yaml`](example/rke2-rancher.yaml) | RKE2/Rancher deployment |
| [`private-ca.yaml`](example/private-ca.yaml) | Private-CA / mTLS: PG with `verify-full` + Redis over TLS + shared CA for S3/OIDC |
| [`openshift/`](example/openshift/) | OpenShift deployment with edge-terminated routes |
| [`aws-eks-irsa/`](example/aws-eks-irsa/) | AWS EKS with IRSA for S3 storage and RDS IAM Auth |

```bash
helm install harbor . -n harbor --create-namespace -f example/k3d-local.yaml
```

## Configuration Examples

### Production Setup with High Availability

```yaml
externalURL: https://harbor.example.com

# External database (required)
database:
  host: postgres.example.com
  password: your-db-password
  sslmode: require

# HA: PDB + autoscaling. `replicas:` is ignored once autoscaling kicks
# in — the HPA owns the count and the Deployment template OMITS the
# field so helm upgrade does not fight the HPA on every roll-out.
core:
  pdb:
    enabled: true
    minAvailable: 1
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
  resources:
    requests:
      cpu: 200m
      memory: 512Mi
    limits:
      memory: 1Gi

portal:
  pdb:
    enabled: true
    minAvailable: 1
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 6
    targetCPUUtilizationPercentage: 70

registry:
  pdb:
    enabled: true
    minAvailable: 1
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 8
    targetCPUUtilizationPercentage: 70

# Ingress with TLS
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: harbor.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: harbor-tls
      hosts:
        - harbor.example.com

# Use Valkey for Redis
valkey:
  enabled: true
  architecture: standalone
```

### Registry configuration (YAML passthrough)

`registry.config` accepts the full [distribution/distribution `config.yml` schema](https://distribution.github.io/distribution/about/configuration/) verbatim — every key distribution supports works without chart changes. The chart wholesale renders this block into `/etc/registry/config.yml`, merging only a small set of chart-managed runtime values (`redis`, `log.level`, `http.addr/debug`) on top via Helm `mustMergeOverwrite`. User keys always win on collision.

Three customization tiers:

1. **Default** — ship-shape filesystem backend out of the box, no override needed.
2. **Inline** — edit `registry.config` in your values file.
3. **External ConfigMap** — own `config.yml` outside the chart entirely.

**Switching storage backend** — Helm deep-merges values, so adding `s3:` alongside the default `filesystem:` produces a config.yml with BOTH backends defined. Explicitly null out the chart default:

```yaml
registry:
  config:
    storage:
      filesystem: null    # remove chart default
      s3:
        region: us-east-1
        bucket: my-harbor-bucket
        # forcepathstyle: true  # MinIO / Ceph RGW / SeaweedFS
      cache:
        layerinfo: redis
```

**Credentials never go in `registry.config`** (they'd be visible in the rendered ConfigMap). Use one of:

- **BYO Secret (recommended)** — `registry.storageCredentials.<backend>.existingSecret` references a pre-existing Secret. The chart injects `REGISTRY_STORAGE_<BACKEND>_<KEY>` env vars on both the registry container and the registryctl sidecar (registryctl runs garbage collection and needs the same creds). Distribution honors these env overrides for any config.yml field.
- **Inline (dev)** — `registry.secret: { REGISTRY_STORAGE_S3_ACCESSKEY: <plain> }` gets b64-encoded into the chart-generated Secret and injected via `envFrom`.

### S3 Storage Backend

```yaml
registry:
  config:
    storage:
      filesystem: null
      s3:
        region: us-east-1
        bucket: my-harbor-bucket
        regionendpoint: https://s3.us-east-1.amazonaws.com
        encrypt: true
        secure: true
        # forcepathstyle: true   # MinIO / Ceph RGW / SeaweedFS — virtual-host
        #                        # style is the libS3 default and most non-AWS
        #                        # S3-compatible backends only accept path-style.
      cache:
        layerinfo: redis
  storageCredentials:
    s3:
      existingSecret: my-s3-creds                      # BYO Kubernetes Secret
      existingSecretAccessKeyKey: REGISTRY_STORAGE_S3_ACCESSKEY
      existingSecretSecretKeyKey: REGISTRY_STORAGE_S3_SECRETKEY
  persistence:
    enabled: false  # No PVC needed for S3
```

For AWS EKS with IRSA (no static credentials), omit `storageCredentials` entirely — the AWS SDK credential chain picks up the projected service account token. Annotate the SAs with the role ARN: `core.serviceAccount.annotations`, `jobservice.serviceAccount.annotations`, `registry.serviceAccount.annotations`.

### Azure Blob Storage

```yaml
registry:
  config:
    storage:
      filesystem: null
      azure:
        accountname: mystorageaccount
        container: harbor
      cache:
        layerinfo: redis
  storageCredentials:
    azure:
      existingSecret: my-azure-creds                    # Secret with the accountkey
      existingSecretKey: REGISTRY_STORAGE_AZURE_ACCOUNTKEY
  persistence:
    enabled: false
```

### Google Cloud Storage

```yaml
registry:
  config:
    storage:
      filesystem: null
      gcs:
        bucket: my-harbor-bucket
        keyfile: /etc/registry/gcs/key.json    # mounted from existingSecret
      cache:
        layerinfo: redis
  storageCredentials:
    gcs:
      existingSecret: my-gcs-keyfile           # Secret with the JSON keyfile
      existingSecretKey: gcs-key.json
  persistence:
    enabled: false
```

For GCS with Workload Identity (no keyfile), omit `storageCredentials.gcs` and `config.storage.gcs.keyfile`. Annotate the SAs with the GSA mapping.

### Externally-managed `config.yml` (existingConfigMap)

For kustomize overlays, GitOps pipelines, or external config generators:

```yaml
registry:
  existingConfigMap: my-registry-config       # ConfigMap with config.yml + ctl-config.yml
  storageCredentials:
    s3:
      existingSecret: my-s3-creds             # still wired even with external ConfigMap
  persistence:
    enabled: false
```

The chart skips generating its own registry ConfigMap and mounts `my-registry-config` instead. Chart-managed credentials and runtime env-var overrides still apply — the Deployment doesn't know or care what's inside the external ConfigMap.

Same pattern available for `jobservice.existingConfigMap` and `portal.existingConfigMap`.

### Gateway API Instead of Ingress

```yaml
ingress:
  enabled: false

gateway:
  enabled: true
  parentRefs:
    - name: my-gateway
      namespace: default
  hostnames:
    - harbor.example.com
```

### External Redis (Instead of Valkey)

```yaml
valkey:
  enabled: false

externalRedis:
  host: redis.example.com
  port: 6379
  password: redis-password
  # For Redis Sentinel:
  # sentinelMasterSet: mymaster
```

### cert-manager Integration

```yaml
tls:
  certManager:
    enabled: true
    issuerRef:
      name: letsencrypt-prod
      kind: ClusterIssuer
    duration: 2160h    # 90 days
    renewBefore: 360h  # 15 days
```

### HPA / Autoscaling

Per-component HPAs target the matching Deployment (or the trivy StatefulSet) and the controller template OMITS `replicas:` whenever `autoscaling.enabled=true` — otherwise each `helm upgrade` would reset the count and fight the HPA. Available on `core`, `jobservice`, `registry`, `portal`, and `trivy`.

```yaml
core:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80   # optional
    # Raw HPAv2 pass-through:
    # metrics: []     # external / object / pods metrics
    # behavior: {}    # scaleUp/scaleDown stabilization windows
```

The chart fails fast at `helm install` / `helm template` time if `maxReplicas` is missing or `minReplicas > maxReplicas`.

### Lifecycle hooks (preStop drain)

Behind AWS NLB, GCP load balancers, or any setup with eventually-consistent LB deregistration, in-flight requests during a rolling upgrade hit a SIGTERM'd pod and surface as `504 Gateway Timeout` or `EOF`. A `preStop` sleep gives the LB time to deregister before the container exits:

```yaml
core:
  lifecycle:
    preStop:
      exec:
        command: ["/bin/sh", "-c", "sleep 15"]

registry:           # applied to both registry + registryctl containers
  lifecycle:
    preStop:
      exec:
        command: ["/bin/sh", "-c", "sleep 30"]
```

Available on every component. `httpGet` and `tcpSocket` hook handlers are accepted too.

### External Redis with a private CA

For managed Redis hosting (self-hosted with cert-manager, some on-prem managed offerings) where the server cert is signed by a private CA, set `externalRedis.tlsOptions.existingCaSecret` to a Secret holding the CA bundle. The chart mounts that Secret on every Harbor component and sets `SSL_CERT_DIR` so Go's `crypto/x509` reads from the mount **in addition to** the default system trust bundle.

```yaml
valkey:
  enabled: false

externalRedis:
  host: redis.private.example.com
  port: 6379
  password: redis-password
  tlsOptions:
    enable: true
    existingCaSecret: my-redis-ca            # Secret with `ca.crt` key
    existingCaSecretKey: ca.crt              # default, override if needed
```

Because the mount affects the system trust pool, the **same Secret also covers private CAs used for S3 endpoints, OIDC providers, LDAP**, etc. — not just Redis. The plumbing is skipped automatically when `valkey.enabled=true` (in-cluster Redis uses cluster-internal trust, no custom CA involved).

### PostgreSQL TLS / mTLS

For managed PostgreSQL that requires `verify-full` sslmode (RDS-with-custom-CA, GCP CloudSQL, on-prem with internal PKI), point `database.existingTlsSecret` at a Secret containing the cert files. The chart mounts it read-only at `/etc/harbor/db-tls` and auto-constructs `POSTGRESQL_URL` env on core and jobservice (Harbor's runtime DB pool returns `cfg.URL` verbatim when set).

```yaml
database:
  host: pg.example.com
  username: harbor
  database: registry
  sslmode: verify-full
  existingSecret: my-harbor-db               # POSTGRESQL_PASSWORD
  existingTlsSecret: my-pg-tls               # ca.crt (+ tls.crt + tls.key for mTLS)
  clientCertEnabled: false                   # set true for client-cert auth
```

Expected keys in `existingTlsSecret` follow cert-manager convention:

| Key | When |
|-----|------|
| `ca.crt` | Always (server-cert verification) |
| `tls.crt` | When `clientCertEnabled: true` |
| `tls.key` | When `clientCertEnabled: true` |

**Caveats:**
- **Exporter** does not yet plumb `POSTGRESQL_URL` through its viper config, so its DB connection ignores the URL env. Works fine for `sslmode=verify-ca` with a publicly-trusted CA; for mTLS the exporter will fail until a backend-side patch lands.
- **Schema migrations** use Harbor's `NewMigrator`, which builds its own DSN from individual fields and ignores `cfg.URL`. Servers that **require** client certificates will reject the migrator. Use `sslmode=verify-ca` (CA-only verification) or coordinate the initial migration via an external trusted client until the migrator honors `URL`.

A combined example covering DB TLS + private Redis CA + cert-manager-managed ingress lives at [`example/private-ca.yaml`](example/private-ca.yaml).

### Custom Configuration via `toEnvVars`

```yaml
core:
  config:
    # Any Harbor Core config option
    token_expiration: 30
    robot_token_duration: 30
    # Nested config becomes NESTED_KEY_HERE env var
    nested:
      key_here: value
  secret:
    # Sensitive values (stored in Secret)
    csrf_key: "your-csrf-key"

jobservice:
  config:
    max_job_workers: 20
    job_loggers: "file,stdout"
```

### Prometheus ServiceMonitor

```yaml
metrics:
  serviceMonitor:
    enabled: true
    namespace: monitoring  # Optional, defaults to release namespace
    interval: 30s
    labels:
      release: prometheus
```

### CloudNativePG via extraManifests

```yaml
extraManifests:
  - apiVersion: postgresql.cnpg.io/v1
    kind: Cluster
    metadata:
      name: harbor-db
    spec:
      instances: 3
      storage:
        size: 10Gi
```

## Uninstalling the Chart

```bash
helm uninstall my-harbor --namespace harbor
```

> **Note**: PersistentVolumeClaims are not deleted automatically. Remove them manually if needed.

## Requirements

Kubernetes: `>=1.28.0-0`

| Repository | Name | Version |
|------------|------|---------|
| oci://ghcr.io/valkey-io/valkey-helm | valkey | 0.9.3 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| cache | object | `{"enabled":false,"expireHours":24}` | Cache configuration (Redis-based caching for manifests) |
| cache.enabled | bool | `false` | Enable Redis caching |
| cache.expireHours | int | `24` | Cache expiration in hours |
| core.affinity | object | `{}` | Affinity rules for Core pods |
| core.artifactPullAsyncFlushDuration | string | `""` | Artifact pull async flush duration |
| core.autoscaling | object | See [values.yaml](values.yaml) | HorizontalPodAutoscaler configuration. When enabled the chart OMITS the static `replicas:` field on the Deployment so HPA owns the replica count. `maxReplicas` is REQUIRED. Tracks upstream goharbor/harbor-helm#1068. |
| core.config | object | {} | Harbor Core application config (converted to env vars in ConfigMap) Any Harbor Core config can be set here without chart changes |
| core.configureUserSettings | string | `""` | Initial user settings JSON applied on first boot |
| core.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| core.existingSecret | string | `""` | Use existing secret for Core secret |
| core.existingSecretKey | string | `"secret"` | Key in existing secret containing the Core secret |
| core.existingXsrfSecret | string | `""` | Existing secret for XSRF key |
| core.existingXsrfSecretKey | string | `"CSRF_KEY"` | Key in existing XSRF secret |
| core.extraEnv | list | [] | Extra environment variables with valueFrom support |
| core.gdpr | object | `{"auditLogsCompliant":false,"deleteUser":false}` | GDPR settings |
| core.gdpr.auditLogsCompliant | bool | `false` | Enable audit logs GDPR compliance |
| core.gdpr.deleteUser | bool | `false` | Enable user deletion for GDPR compliance |
| core.hostAliases | list | [] | Host entries injected into /etc/hosts (PodSpec.hostAliases). Use for private DNS that does not exist in cluster DNS — service-mesh sidecars, legacy LDAP/SMTP/proxy targets, internal CAs, etc. Format matches the Kubernetes PodSpec: a list of `{ip, hostnames}` entries. |
| core.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-core","tag":""}` | Core image settings |
| core.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-core"` | Core image repository |
| core.image.tag | string | `""` | Core image tag (defaults to appVersion) |
| core.initContainers | list | `[]` | Init containers (run before main containers) |
| core.lifecycle | object | {} | Container `lifecycle` hook spec (preStop / postStart). Common use: preStop `sleep` so AWS/GCP LBs deregister the pod before SIGTERM, avoiding 504s on rolling upgrades. Both hook handler shapes are accepted (`exec`, `httpGet`, `tcpSocket`). Tracks upstream #1722/#1739/#2156/#2157 — all closed without merge, the gap was never closed there. |
| core.nodeSelector | object | `{}` | Node selector for Core pods |
| core.pdb | object | `{"enabled":false}` | PodDisruptionBudget for Core |
| core.pdb.enabled | bool | `false` | Enable PodDisruptionBudget. When true, exactly one of `minAvailable` or `maxUnavailable` must be set (Kubernetes rejects PDBs with both fields). |
| core.podAnnotations | object | `{}` | Additional pod annotations for Core |
| core.podLabels | object | `{}` | Additional pod labels for Core |
| core.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Core |
| core.quotaUpdateProvider | string | `"db"` | Quota update provider (db or redis) |
| core.replicas | int | `1` | Number of Core replicas (ignored when autoscaling.enabled=true) |
| core.resources | object | `{"limits":{"memory":"512Mi"},"requests":{"cpu":"100m","memory":"256Mi"}}` | Core resource requests and limits |
| core.secret | object | {} | Sensitive config for Core (converted to env vars in Secret) |
| core.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for Core container |
| core.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":true,"name":""}` | Service account settings for Core |
| core.serviceAccount.annotations | object | `{}` | Service account annotations |
| core.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| core.serviceAccount.create | bool | `true` | Create a service account for Core |
| core.serviceAccount.name | string | `""` | Service account name (auto-generated if empty) |
| core.serviceAnnotations | object | `{}` | Service annotations for Core service |
| core.startupProbe | object | `{"enabled":true,"initialDelaySeconds":10}` | Startup probe for Core |
| core.tokenCert | string | `""` | Token certificate (PEM, signed by tokenKey) |
| core.tokenKey | string | `""` | Token private key (PEM). Auto-generated if both tokenKey and tokenCert are empty and tokenSecretName is empty. |
| core.tokenSecretName | string | `""` | Existing secret for token service key/cert (must contain keys: tls.key, tls.crt) |
| core.tolerations | list | `[]` | Tolerations for Core pods |
| core.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| core.xsrfKey | string | `""` | XSRF key (auto-generated if empty) |
| database.clientCertEnabled | bool | `false` | Include client cert + key in the auto-built POSTGRESQL_URL. Set to true only when PG is configured to require client certificate authentication. libpq fails if `sslcert` is set but the file is missing. |
| database.connMaxIdleTime | string | `"0"` | Maximum idle time for connections (0 = no limit) |
| database.connMaxLifetime | string | `"0"` | Maximum lifetime of connections (0 = no limit) |
| database.database | string | `"registry"` | Database name |
| database.existingSecret | string | `""` | Existing secret containing database credentials Default secret key: POSTGRESQL_PASSWORD |
| database.existingSecretKey | string | `""` |  |
| database.existingTlsSecret | string | `""` | Secret holding the PEM-encoded CA bundle (and optionally client cert + key) for verifying / authenticating to PostgreSQL. Required for `verify-ca` / `verify-full` sslmode against managed PG with a private CA (RDS-with-custom-CA, GCP CloudSQL, on-prem with internal PKI). Tracks upstream goharbor/harbor-helm#1859.  Expected keys (cert-manager convention):   ca.crt   — CA bundle (always required when this Secret is set)   tls.crt  — client cert (only when clientCertEnabled=true)   tls.key  — client key  (only when clientCertEnabled=true)  When set, the chart mounts the Secret at /etc/harbor/db-tls and injects POSTGRESQL_URL env on core + jobservice. The runtime DB pool honors that env over the individual fields.  Caveats:   - Exporter does not yet plumb POSTGRESQL_URL into its viper config,     so its DB connection ignores client certs (it'll work fine for     sslmode=verify-ca with a publicly-trusted CA).   - Harbor's migration tool (NewMigrator) does not honor cfg.URL     either, so schema migrations against a server that REQUIRES     mTLS will fail. Use sslmode=verify-ca or migrate via an external     trusted client until that's fixed upstream. |
| database.host | string | `""` | Database host (required) |
| database.maxIdleConns | int | `100` | Maximum idle connections |
| database.maxOpenConns | int | `900` | Maximum open connections |
| database.password | string | `""` | Database password (ignored if existingSecret is set) |
| database.port | int | `5432` | Database port |
| database.sslmode | string | `"disable"` | SSL mode for database connection |
| database.username | string | `""` | Database username |
| existingSecretAdminPassword | string | `""` | Existing secret containing the admin password (overrides harborAdminPassword) |
| existingSecretAdminPasswordKey | string | `"HARBOR_ADMIN_PASSWORD"` | Key in the existing secret for admin password |
| existingSecretSecretKey | string | `""` | Existing secret containing the encryption key (overrides secretKey). The secret must hold the 16-char key under `SECRET_KEY` and `secretKey` (or override the key name via `existingSecretSecretKeyKey`). |
| existingSecretSecretKeyKey | string | `"secretKey"` | Key in `existingSecretSecretKey` that holds the encryption key. Used both for the `SECRET_KEY` env on core and the `secret-key` volume mount (which Harbor reads as `/etc/core/key`). |
| exporter.affinity | object | `{}` | Affinity rules for Exporter pods |
| exporter.config | object | {} | Exporter config as env vars. Exporter is env-driven (HARBOR_EXPORTER_*); nested maps flatten to UPPER_SNAKE_CASE via toEnvVars and are injected via envFrom. Any exporter setting works without chart changes. |
| exporter.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| exporter.enabled | bool | `true` | Enable Harbor exporter for Prometheus metrics |
| exporter.extraEnv | list | [] | Extra environment variables with valueFrom support |
| exporter.hostAliases | list | [] | Host entries injected into /etc/hosts (PodSpec.hostAliases). Use for private DNS that does not exist in cluster DNS — service-mesh sidecars, legacy LDAP/SMTP/proxy targets, internal CAs, etc. Format matches the Kubernetes PodSpec: a list of `{ip, hostnames}` entries. |
| exporter.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-exporter","tag":""}` | Exporter image settings |
| exporter.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-exporter"` | Exporter image repository |
| exporter.image.tag | string | `""` | Exporter image tag (defaults to appVersion) |
| exporter.initContainers | list | `[]` | Init containers (run before main containers) |
| exporter.lifecycle | object | {} | Container `lifecycle` hook spec (preStop / postStart). Common use: preStop `sleep` so AWS/GCP LBs deregister the pod before SIGTERM, avoiding 504s on rolling upgrades. Both hook handler shapes are accepted (`exec`, `httpGet`, `tcpSocket`). Tracks upstream #1722/#1739/#2156/#2157 — all closed without merge, the gap was never closed there. |
| exporter.nodeSelector | object | `{}` | Node selector for Exporter pods |
| exporter.pdb | object | `{"enabled":false}` | PodDisruptionBudget for Exporter |
| exporter.pdb.enabled | bool | `false` | Enable PodDisruptionBudget. When true, exactly one of `minAvailable` or `maxUnavailable` must be set (Kubernetes rejects PDBs with both fields). |
| exporter.podAnnotations | object | `{}` | Additional pod annotations for Exporter |
| exporter.podLabels | object | `{}` | Additional pod labels for Exporter |
| exporter.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Exporter |
| exporter.replicas | int | `1` | Number of Exporter replicas |
| exporter.resources | object | `{"limits":{"memory":"256Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Exporter resource requests and limits |
| exporter.secret | object | {} | Sensitive config for Exporter (converted to env vars in Secret) |
| exporter.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for Exporter container |
| exporter.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":true,"name":""}` | Service account settings for Exporter |
| exporter.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| exporter.tolerations | list | `[]` | Tolerations for Exporter pods |
| exporter.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| expose | object | `{"clusterIP":{"annotations":{},"enabled":false,"labels":{},"name":"","ports":{"http":80,"https":443},"staticClusterIP":""},"loadBalancer":{"IP":"","annotations":{},"enabled":false,"labels":{},"name":"","ports":{"http":80,"https":443},"sourceRanges":[]},"nodePort":{"annotations":{},"enabled":false,"labels":{},"name":"","ports":{"http":{"nodePort":30002,"port":80},"https":{"nodePort":30003,"port":443}}},"route":{"annotations":{},"enabled":false,"host":"","labels":{},"tls":{"insecureEdgeTerminationPolicy":"Redirect","termination":"edge"}}}` | Direct service exposure (ClusterIP / NodePort / LoadBalancer) Creates a front-door service pointing to core (which proxies portal requests). Internal per-component services are always created regardless of this setting. |
| expose.clusterIP | object | `{"annotations":{},"enabled":false,"labels":{},"name":"","ports":{"http":80,"https":443},"staticClusterIP":""}` | ClusterIP service for direct exposure |
| expose.clusterIP.annotations | object | `{}` | Service annotations |
| expose.clusterIP.enabled | bool | `false` | Enable ClusterIP expose service |
| expose.clusterIP.labels | object | `{}` | Service labels |
| expose.clusterIP.name | string | `""` | Service name override (defaults to fullname) |
| expose.clusterIP.ports.http | int | `80` | HTTP port |
| expose.clusterIP.ports.https | int | `443` | HTTPS port (only used when tls.enabled=true) |
| expose.clusterIP.staticClusterIP | string | `""` | Static ClusterIP (empty = auto-assigned) |
| expose.loadBalancer | object | `{"IP":"","annotations":{},"enabled":false,"labels":{},"name":"","ports":{"http":80,"https":443},"sourceRanges":[]}` | LoadBalancer service for cloud provider external access |
| expose.loadBalancer.IP | string | `""` | Static LoadBalancer IP (empty = auto-assigned, cloud-provider dependent) |
| expose.loadBalancer.annotations | object | `{}` | Service annotations |
| expose.loadBalancer.enabled | bool | `false` | Enable LoadBalancer expose service |
| expose.loadBalancer.labels | object | `{}` | Service labels |
| expose.loadBalancer.name | string | `""` | Service name override (defaults to fullname) |
| expose.loadBalancer.ports.http | int | `80` | HTTP port |
| expose.loadBalancer.ports.https | int | `443` | HTTPS port |
| expose.loadBalancer.sourceRanges | list | `[]` | Allowed source IP ranges |
| expose.nodePort | object | `{"annotations":{},"enabled":false,"labels":{},"name":"","ports":{"http":{"nodePort":30002,"port":80},"https":{"nodePort":30003,"port":443}}}` | NodePort service for external access via node IPs |
| expose.nodePort.annotations | object | `{}` | Service annotations |
| expose.nodePort.enabled | bool | `false` | Enable NodePort expose service |
| expose.nodePort.labels | object | `{}` | Service labels |
| expose.nodePort.name | string | `""` | Service name override (defaults to fullname) |
| expose.nodePort.ports.http.nodePort | int | `30002` | NodePort for HTTP (30000-32767, empty = auto-assigned) |
| expose.nodePort.ports.http.port | int | `80` | Service port for HTTP |
| expose.nodePort.ports.https.nodePort | int | `30003` | NodePort for HTTPS (30000-32767, empty = auto-assigned) |
| expose.nodePort.ports.https.port | int | `443` | Service port for HTTPS |
| expose.route | object | `{"annotations":{},"enabled":false,"host":"","labels":{},"tls":{"insecureEdgeTerminationPolicy":"Redirect","termination":"edge"}}` | OpenShift Route for external access |
| expose.route.annotations | object | `{}` | Route annotations |
| expose.route.enabled | bool | `false` | Enable OpenShift Route |
| expose.route.host | string | `""` | Route hostname (empty = auto-generated by OpenShift) |
| expose.route.labels | object | `{}` | Route labels |
| expose.route.tls.insecureEdgeTerminationPolicy | string | `"Redirect"` | Insecure edge termination policy: Allow, Disable, Redirect |
| expose.route.tls.termination | string | `"edge"` | TLS termination type: edge, passthrough, or reencrypt |
| externalRedis.existingSecret | string | `""` | Existing secret containing Redis password |
| externalRedis.existingSecretKey | string | `"REDIS_PASSWORD"` | Key in the existing secret that holds the Redis password |
| externalRedis.host | string | `""` | External Redis host |
| externalRedis.password | string | `""` | External Redis password |
| externalRedis.port | int | `6379` | External Redis port |
| externalRedis.sentinelMasterSet | string | `""` | Sentinel master set name (for Redis Sentinel) |
| externalRedis.tlsOptions | object | `{"enable":false,"existingCaSecret":"","existingCaSecretKey":"ca.crt"}` | TLS options for external Redis. For managed Redis with a private CA (self-hosted Redis, some on-prem managed offerings, custom certs via cert-manager), set `existingCaSecret` to a Secret holding the CA bundle. The chart mounts it on every Harbor component at `/etc/harbor/extra-ca` and sets `SSL_CERT_DIR` so Go's TLS adds it to the system trust pool — so the same Secret also covers private CAs used for S3 endpoints, OIDC, etc. Tracks upstream goharbor/harbor-helm#549. |
| externalRedis.tlsOptions.enable | bool | `false` | Enable TLS for external Redis connection |
| externalRedis.tlsOptions.existingCaSecret | string | `""` | Name of an existing Secret containing the CA bundle. Leave empty to use only the cluster's default trust store. |
| externalRedis.tlsOptions.existingCaSecretKey | string | `"ca.crt"` | Key inside `existingCaSecret` that holds the PEM-encoded CA bundle. Default `ca.crt` is the convention cert-manager uses. |
| externalRedis.username | string | `""` | External Redis username |
| externalURL | string | "" | External URL for Harbor (REQUIRED) This is the URL users will use to access Harbor (e.g., https://harbor.example.com) |
| extraManifests | list | [] | Extra static manifests to deploy These are merged with chart labels and deployed as-is |
| extraTemplateManifests | list | [] | Extra templated manifests to deploy These can use .Values, .Release, and other template functions |
| fullnameOverride | string | `""` | Override the full name |
| gateway | object | `{"enabled":false,"hostnames":[],"parentRefs":[]}` | Gateway API configuration (alternative to ingress) |
| gateway.enabled | bool | `false` | Enable Gateway API HTTPRoute |
| gateway.hostnames | list | `[]` | Hostnames for the HTTPRoute |
| gateway.parentRefs | list | `[]` | Gateway parent references |
| global | object | `{"priorityClassName":"","revisionHistoryLimit":3}` | Global defaults inherited by all components |
| global.priorityClassName | string | `""` | Priority class name for all component pods |
| global.revisionHistoryLimit | int | `3` | Number of old ReplicaSets to retain (K8s default is 10) |
| harborAdminPassword | string | `""` | Harbor admin password (initial setup). **REQUIRED** unless `existingSecretAdminPassword` is set. Do not use the legacy default `Harbor12345` — it is publicly known. For production reference a pre-created Secret via `existingSecretAdminPassword` rather than passing the value here. Rotate from the Harbor UI after first login. |
| image | object | `{"pullPolicy":"IfNotPresent"}` | Global image settings |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for all Harbor components |
| imageCredentials | object | `{}` | Credentials to pull images imageCredentials:   registry: xyz.com   username: xxx   password: yyy   email: zzz@xyz.com |
| imagePullSecrets | list | [] | List of image pull secrets |
| ingress | object | `{"annotations":{},"autoGenCert":true,"className":"","core":"","enabled":true,"hosts":[],"labels":{},"tls":[]}` | Ingress configuration |
| ingress.annotations | object | `{}` | Ingress annotations |
| ingress.className | string | `""` | Ingress class name |
| ingress.core | string | `""` | Hostname override used by auto-generated ingress certificate Defaults to first ingress host, or externalURL hostname when ingress.hosts is empty |
| ingress.enabled | bool | `true` | Enable ingress |
| ingress.hosts | list | `[]` | Additional ingress hosts |
| ingress.labels | object | `{}` | Ingress labels |
| ingress.tls | list | `[]` | Ingress TLS configuration |
| ipFamily.ipv4.enabled | bool | `true` |  |
| ipFamily.ipv6.enabled | bool | `true` |  |
| jobservice.affinity | object | `{}` | Affinity rules for Jobservice pods |
| jobservice.autoscaling | object | See [values.yaml](values.yaml) | HorizontalPodAutoscaler. See `core.autoscaling` for full docs. |
| jobservice.config | object | See [values.yaml](values.yaml) | Full Harbor jobservice `config.yml` passed through verbatim. Used only when `existingConfigMap` is empty.  Chart-managed values injected via env-var override at runtime:   - `protocol`, `port` (from chart helpers)   - `worker_pool.backend`, `worker_pool.workers` (when JOB_SERVICE_*     env vars are wired by the chart)   - `worker_pool.redis_pool.redis_url` (chart sets the URL with auth     via JOB_SERVICE_POOL_REDIS_URL; you can leave a placeholder here)   - `worker_pool.redis_pool.namespace`  The following keys are NOT env-overridable (Harbor jobservice limitation — see src/jobservice/config/config.go). You MUST set them in this block; changing `.Values.logLevel` or `.Values.metrics.enabled` globally will NOT propagate here:   - `metric.enabled`, `metric.path`, `metric.port`   - `loggers[].level`, `job_loggers[].level`   - `job_loggers[].sweeper.*`   - `reaper.*`   - `max_retrieve_size_mb` |
| jobservice.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| jobservice.env | object | {} | Supplementary env vars for the jobservice container (and the jobservice ConfigMap-env). Nested maps flatten to `UPPER_SNAKE_CASE` keys via `harbor.toEnvVars`. Use for any setting Harbor reads from env but is not part of the YAML config (e.g. webhook tuning). |
| jobservice.existingConfigMap | string | `""` | Use an externally-managed ConfigMap containing `config.yml` instead of generating one from `config:` below. Semantics match `registry.existingConfigMap`. |
| jobservice.existingSecret | string | `""` | Use existing secret for Jobservice secret |
| jobservice.existingSecretKey | string | `"JOBSERVICE_SECRET"` | Key in existing secret containing the Jobservice secret |
| jobservice.extraEnv | list | [] | Extra environment variables with valueFrom support |
| jobservice.hostAliases | list | [] | Host entries injected into /etc/hosts (PodSpec.hostAliases). Use for private DNS that does not exist in cluster DNS — service-mesh sidecars, legacy LDAP/SMTP/proxy targets, internal CAs, etc. Format matches the Kubernetes PodSpec: a list of `{ip, hostnames}` entries. |
| jobservice.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-jobservice","tag":""}` | Jobservice image settings |
| jobservice.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-jobservice"` | Jobservice image repository |
| jobservice.image.tag | string | `""` | Jobservice image tag (defaults to appVersion) |
| jobservice.initContainers | list | `[]` | Init containers (run before main containers) |
| jobservice.lifecycle | object | {} | Container `lifecycle` hook spec (preStop / postStart). Common use: preStop `sleep` so AWS/GCP LBs deregister the pod before SIGTERM, avoiding 504s on rolling upgrades. Both hook handler shapes are accepted (`exec`, `httpGet`, `tcpSocket`). Tracks upstream #1722/#1739/#2156/#2157 — all closed without merge, the gap was never closed there. |
| jobservice.nodeSelector | object | `{}` | Node selector for Jobservice pods |
| jobservice.pdb | object | `{"enabled":false}` | PodDisruptionBudget for Jobservice |
| jobservice.pdb.enabled | bool | `false` | Enable PodDisruptionBudget. When true, exactly one of `minAvailable` or `maxUnavailable` must be set (Kubernetes rejects PDBs with both fields). |
| jobservice.persistence | object | `{"accessModes":["ReadWriteOnce"],"annotations":{},"enabled":false,"existingClaim":"","resourcePolicy":"keep","size":"1Gi"}` | Jobservice persistence settings |
| jobservice.persistence.accessModes | list | `["ReadWriteOnce"]` | PVC access modes |
| jobservice.persistence.annotations | object | `{}` | Annotations for PVC |
| jobservice.persistence.enabled | bool | `false` | Enable persistence for jobservice |
| jobservice.persistence.existingClaim | string | `""` | Existing PVC name (disables dynamic provisioning) |
| jobservice.persistence.resourcePolicy | string | `"keep"` | Resource policy: "keep" prevents PVC deletion on helm uninstall |
| jobservice.persistence.size | string | `"1Gi"` | PVC size |
| jobservice.podAnnotations | object | `{}` | Additional pod annotations for Jobservice |
| jobservice.podLabels | object | `{}` | Additional pod labels for Jobservice |
| jobservice.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Jobservice |
| jobservice.replicas | int | `1` | Number of Jobservice replicas (ignored when autoscaling.enabled=true) |
| jobservice.resources | object | `{"limits":{"memory":"512Mi"},"requests":{"cpu":"100m","memory":"256Mi"}}` | Jobservice resource requests and limits |
| jobservice.secret | object | {} | Sensitive config for Jobservice (converted to env vars in Secret) |
| jobservice.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for Jobservice container |
| jobservice.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":true,"name":""}` | Service account settings for Jobservice |
| jobservice.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| jobservice.tolerations | list | `[]` | Tolerations for Jobservice pods |
| jobservice.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| logLevel | string | `"info"` | Log level for all components (debug, info, warning, error, fatal) |
| metrics.enabled | bool | `false` | Enable metrics endpoints on all components |
| metrics.serviceMonitor | object | `{"enabled":false,"honorLabels":true,"interval":"30s","labels":{},"namespace":"","scrapeTimeout":"10s"}` | Enable Prometheus ServiceMonitor |
| metrics.serviceMonitor.enabled | bool | `false` | Create ServiceMonitor resource |
| metrics.serviceMonitor.honorLabels | bool | `true` | Honor labels |
| metrics.serviceMonitor.interval | string | `"30s"` | Scrape interval |
| metrics.serviceMonitor.labels | object | `{}` | Additional labels for ServiceMonitor |
| metrics.serviceMonitor.namespace | string | `""` | ServiceMonitor namespace (defaults to release namespace) |
| metrics.serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout |
| nameOverride | string | `""` | Override the chart name |
| portal.affinity | object | `{}` | Affinity rules for Portal pods |
| portal.autoscaling | object | See [values.yaml](values.yaml) | HorizontalPodAutoscaler. See `core.autoscaling` for full docs. |
| portal.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| portal.existingConfigMap | string | `""` | Use an externally-managed ConfigMap containing `nginx.conf` instead of the chart-generated one. When set, the chart skips ConfigMap generation and the Deployment mounts the named ConfigMap. Use for custom nginx configuration (TLS termination, custom headers, extra locations) without forking the chart. Semantics match `registry.existingConfigMap`. Portal serves static Angular assets via nginx and has no env/key config surface — to customize nginx.conf, point existingConfigMap at your own ConfigMap (there is no `config`/`secret` passthrough here). |
| portal.extraEnv | list | [] | Extra environment variables with valueFrom support |
| portal.hostAliases | list | [] | Host entries injected into /etc/hosts (PodSpec.hostAliases). Use for private DNS that does not exist in cluster DNS — service-mesh sidecars, legacy LDAP/SMTP/proxy targets, internal CAs, etc. Format matches the Kubernetes PodSpec: a list of `{ip, hostnames}` entries. |
| portal.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-portal","tag":""}` | Portal image settings |
| portal.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-portal"` | Portal image repository |
| portal.image.tag | string | `""` | Portal image tag (defaults to appVersion) |
| portal.initContainers | list | `[]` | Init containers (run before main containers) |
| portal.lifecycle | object | {} | Container `lifecycle` hook spec (preStop / postStart). Common use: preStop `sleep` so AWS/GCP LBs deregister the pod before SIGTERM, avoiding 504s on rolling upgrades. Both hook handler shapes are accepted (`exec`, `httpGet`, `tcpSocket`). Tracks upstream #1722/#1739/#2156/#2157 — all closed without merge, the gap was never closed there. |
| portal.nodeSelector | object | `{}` | Node selector for Portal pods |
| portal.pdb | object | `{"enabled":false}` | PodDisruptionBudget for Portal |
| portal.pdb.enabled | bool | `false` | Enable PodDisruptionBudget. When true, exactly one of `minAvailable` or `maxUnavailable` must be set (Kubernetes rejects PDBs with both fields). |
| portal.podAnnotations | object | `{}` | Additional pod annotations for Portal |
| portal.podLabels | object | `{}` | Additional pod labels for Portal |
| portal.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Portal |
| portal.replicas | int | `1` | Number of Portal replicas (ignored when autoscaling.enabled=true) |
| portal.resources | object | `{"limits":{"memory":"256Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Portal resource requests and limits |
| portal.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for Portal container |
| portal.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":true,"name":""}` | Service account settings for Portal |
| portal.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| portal.serviceAnnotations | object | `{}` | Service annotations for Portal service |
| portal.tolerations | list | `[]` | Tolerations for Portal pods |
| portal.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| proxy.components[0] | string | `"core"` |  |
| proxy.components[1] | string | `"jobservice"` |  |
| proxy.components[2] | string | `"trivy"` |  |
| proxy.httpProxy | string | `nil` |  |
| proxy.httpsProxy | string | `nil` |  |
| proxy.noProxy | string | `"127.0.0.1,localhost,.local,.internal"` |  |
| registry.affinity | object | `{}` | Affinity rules for Registry pods |
| registry.autoscaling | object | See [values.yaml](values.yaml) | HorizontalPodAutoscaler. See `core.autoscaling` for full docs. |
| registry.config | object | See [values.yaml](values.yaml) | Full [distribution/distribution](https://distribution.github.io/distribution/about/configuration/) `config.yml` passed through verbatim. Used only when `existingConfigMap` is empty. Replace this entire block to switch storage backends or add any field distribution supports (notifications, health, custom middleware, etc.).  Chart-managed values injected via env-var override at runtime (these ALWAYS win over what's in this block — do not duplicate here):   - `http.addr`, `http.secret`, `http.debug.prometheus.enabled`   - `redis.*` (addr, password, db, tls)   - `log.level` (from `.Values.logLevel`)   - storage credentials when `storageCredentials.<backend>.existingSecret` is set  See `storageCredentials` below for the BYO-Secret pattern. For inline credentials use `registry.secret` (b64-encoded into the generated Secret) — do not put plaintext credentials in this block, they would be visible in the ConfigMap. |
| registry.controller | object | `{"image":{"repository":"8gears.container-registry.com/8gcr/harbor-registryctl","tag":""}}` | Registryctl image settings |
| registry.controller.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-registryctl"` | Registryctl image repository |
| registry.controller.image.tag | string | `""` | Registryctl image tag (defaults to appVersion) |
| registry.credentials.existingSecret | string | `""` |  |
| registry.credentials.htpasswdString | string | `""` |  |
| registry.credentials.password | string | `""` |  |
| registry.credentials.username | string | `"harbor_registry_user"` |  |
| registry.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| registry.existingConfigMap | string | `""` | Use an externally-managed ConfigMap containing `config.yml` and `ctl-config.yml` instead of generating one from `config:` below. When set, the chart skips ConfigMap generation and the Deployment mounts the named ConfigMap at /etc/registry. Chart-managed runtime values (redis URL with auth, HTTP secret, storage credentials from `storageCredentials`, log level, ports) are still injected via env-var overrides on the Deployment, so they take precedence over whatever is in the external ConfigMap. Use this for kustomize/GitOps workflows where `config.yml` is owned by a separate manifest pipeline. |
| registry.existingSecret | string | `""` | Existing Secret that supplies `REGISTRY_HTTP_SECRET`. When set, the generated registry Secret omits `REGISTRY_HTTP_SECRET` and the deployment reads it from this Secret via env. Independent of `storageCredentials`. |
| registry.existingSecretKey | string | `"REGISTRY_HTTP_SECRET"` | Key in `registry.existingSecret` that holds `REGISTRY_HTTP_SECRET`. |
| registry.extraEnv | list | [] | Extra environment variables with valueFrom support |
| registry.hostAliases | list | [] | Host entries injected into /etc/hosts (PodSpec.hostAliases). Use for private DNS that does not exist in cluster DNS — service-mesh sidecars, legacy LDAP/SMTP/proxy targets, internal CAs, etc. Format matches the Kubernetes PodSpec: a list of `{ip, hostnames}` entries. |
| registry.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-registry","tag":""}` | Registry image settings |
| registry.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-registry"` | Registry image repository |
| registry.image.tag | string | `""` | Registry image tag (defaults to appVersion) |
| registry.initContainers | list | `[]` | Init containers (run before main containers) |
| registry.lifecycle | object | {} | Container `lifecycle` hook spec (preStop / postStart). Common use: preStop `sleep` so AWS/GCP LBs deregister the pod before SIGTERM, avoiding 504s on rolling upgrades. Both hook handler shapes are accepted (`exec`, `httpGet`, `tcpSocket`). Tracks upstream #1722/#1739/#2156/#2157 — all closed without merge, the gap was never closed there. |
| registry.nodeSelector | object | `{}` | Node selector for Registry pods |
| registry.pdb | object | `{"enabled":false}` | PodDisruptionBudget for Registry |
| registry.pdb.enabled | bool | `false` | Enable PodDisruptionBudget. When true, exactly one of `minAvailable` or `maxUnavailable` must be set (Kubernetes rejects PDBs with both fields). |
| registry.persistence | object | `{"accessModes":["ReadWriteOnce"],"annotations":{},"enabled":false,"existingClaim":"","resourcePolicy":"keep","size":"10Gi"}` | Registry persistence settings |
| registry.persistence.accessModes | list | `["ReadWriteOnce"]` | PVC access modes |
| registry.persistence.annotations | object | `{}` | Annotations for PVC |
| registry.persistence.enabled | bool | `false` | Enable persistence for registry |
| registry.persistence.existingClaim | string | `""` | Existing PVC name (disables dynamic provisioning) |
| registry.persistence.resourcePolicy | string | `"keep"` | Resource policy: "keep" prevents PVC deletion on helm uninstall |
| registry.persistence.size | string | `"10Gi"` | PVC size |
| registry.podAnnotations | object | `{}` | Additional pod annotations for Registry |
| registry.podLabels | object | `{}` | Additional pod labels for Registry |
| registry.podSecurityContext | object | `{"fsGroup":10000,"fsGroupChangePolicy":"OnRootMismatch"}` | Pod security context for Registry |
| registry.replicas | int | `1` | Number of Registry replicas (ignored when autoscaling.enabled=true) |
| registry.resources | object | `{"limits":{"memory":"512Mi"},"requests":{"cpu":"100m","memory":"256Mi"}}` | Registry resource requests and limits |
| registry.secret | object | {} | Sensitive config for Registry. Each key is b64-encoded into the generated registry Secret and injected on both `registry` and `registryctl` containers via `envFrom`. Use for inline credentials, e.g.:      secret:       REGISTRY_STORAGE_S3_ACCESSKEY: <plaintext>       REGISTRY_STORAGE_S3_SECRETKEY: <plaintext>  Prefer `storageCredentials.<backend>.existingSecret` for production (External-Secrets-Operator / Vault / SealedSecrets workflows). |
| registry.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for Registry container |
| registry.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":true,"name":""}` | Service account settings for Registry |
| registry.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| registry.storageCredentials | object | See [values.yaml](values.yaml) | BYO Secret references for storage credentials. The chart injects the credential as an env var on both `registry` and `registryctl` containers; distribution honors `REGISTRY_STORAGE_<BACKEND>_<KEY>` env overrides for any backend. Set the entry matching the backend you configured in `config.storage`. Other entries stay empty.  Env injection is gated only on `existingSecret` being non-empty (not on which backend is active in `config:`). Setting credentials for an inactive backend is harmless — distribution ignores env vars not matching its active driver. |
| registry.storageCredentials.azure.existingSecret | string | `""` | Existing Secret containing the Azure storage account key. |
| registry.storageCredentials.azure.existingSecretKey | string | `"REGISTRY_STORAGE_AZURE_ACCOUNTKEY"` | Key in `existingSecret` holding the account key. |
| registry.storageCredentials.gcs.existingSecret | string | `""` | Existing Secret containing the GCS service-account keyfile. When set, mounted at /etc/registry/gcs/key.json — reference this path from `config.storage.gcs.keyfile`. Leave empty when using Workload Identity (set `serviceAccount.annotations` instead). |
| registry.storageCredentials.gcs.existingSecretKey | string | `"gcs-key.json"` | Key in `existingSecret` holding the keyfile JSON content. |
| registry.storageCredentials.oss.existingSecret | string | `""` | Existing Secret containing the Alibaba OSS access key secret. |
| registry.storageCredentials.oss.existingSecretKey | string | `"REGISTRY_STORAGE_OSS_ACCESSKEYSECRET"` | Key in `existingSecret` holding the access key secret. |
| registry.storageCredentials.s3.existingSecret | string | `""` | Existing Secret containing AWS S3 credentials. |
| registry.storageCredentials.s3.existingSecretAccessKeyKey | string | `"REGISTRY_STORAGE_S3_ACCESSKEY"` | Key in `existingSecret` holding the access key ID. |
| registry.storageCredentials.s3.existingSecretSecretKeyKey | string | `"REGISTRY_STORAGE_S3_SECRETKEY"` | Key in `existingSecret` holding the secret access key. |
| registry.tolerations | list | `[]` | Tolerations for Registry pods |
| registry.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| secretKey | string | auto-generated | Secret key for encryption (16 characters) Used for encrypting credentials stored in the database |
| tls.certManager.duration | string | `"2160h"` | Certificate duration |
| tls.certManager.enabled | bool | `false` | Enable cert-manager for TLS certificates |
| tls.certManager.issuerRef | object | `{}` | cert-manager issuer reference |
| tls.certManager.renewBefore | string | `"360h"` | Certificate renewal before expiry |
| tls.certSource | string | `"none"` | TLS certificate source: auto, secret, or none |
| tls.customSecrets | object | `{"core":"","registry":""}` | Custom TLS secrets (alternative to cert-manager) |
| tls.customSecrets.core | string | `""` | TLS secret for core/portal |
| tls.customSecrets.registry | string | `""` | TLS secret for registry |
| tls.enabled | bool | `false` | Enable TLS (set to false if terminating TLS at ingress/load balancer) |
| trace.enabled | bool | `false` |  |
| trace.jaeger.endpoint | string | `"http://hostname:14268/api/traces"` |  |
| trace.otel.compression | bool | `false` |  |
| trace.otel.endpoint | string | `"hostname:4318"` |  |
| trace.otel.insecure | bool | `true` |  |
| trace.otel.timeout | int | `10` |  |
| trace.otel.url_path | string | `"/v1/traces"` |  |
| trace.provider | string | `"jaeger"` |  |
| trace.sample_rate | int | `1` |  |
| trivy.affinity | object | `{}` | Affinity rules for Trivy pods |
| trivy.autoscaling | object | See [values.yaml](values.yaml) | HorizontalPodAutoscaler for the Trivy StatefulSet. See `core.autoscaling` for full docs. |
| trivy.config | object | {} | Trivy adapter config as env vars. Trivy is env-driven (SCANNER_*); nested maps flatten to UPPER_SNAKE_CASE via toEnvVars and are injected via envFrom. Any adapter setting works without chart changes. |
| trivy.dbRepository[0] | string | `"mirror.gcr.io/aquasec/trivy-db"` |  |
| trivy.dbRepository[1] | string | `"ghcr.io/aquasecurity/trivy-db"` |  |
| trivy.debugMode | bool | `false` | Debug mode for more verbose scanning log |
| trivy.enabled | bool | `false` | Enable Trivy scanner |
| trivy.extraEnv | list | [] | Extra environment variables with valueFrom support |
| trivy.gitHubToken | string | `""` | GitHub token to download Trivy DB (optional) |
| trivy.hostAliases | list | [] | Host entries injected into /etc/hosts (PodSpec.hostAliases). Use for private DNS that does not exist in cluster DNS — service-mesh sidecars, legacy LDAP/SMTP/proxy targets, internal CAs, etc. Format matches the Kubernetes PodSpec: a list of `{ip, hostnames}` entries. |
| trivy.ignoreUnfixed | bool | `false` | Skip unfixed vulnerabilities |
| trivy.image.repository | string | `"8gears.container-registry.com/8gcr/trivy-adapter"` | Trivy adapter image repository |
| trivy.image.tag | string | `""` | Trivy adapter image tag (defaults to appVersion) |
| trivy.initContainers | list | `[]` | Init containers (run before main containers) |
| trivy.insecure | bool | `false` | Skip verifying registry certificate |
| trivy.javaDBRepository[0] | string | `"mirror.gcr.io/aquasec/trivy-java-db"` |  |
| trivy.javaDBRepository[1] | string | `"ghcr.io/aquasecurity/trivy-java-db"` |  |
| trivy.lifecycle | object | {} | Container `lifecycle` hook spec (preStop / postStart). Common use: preStop `sleep` so AWS/GCP LBs deregister the pod before SIGTERM, avoiding 504s on rolling upgrades. Both hook handler shapes are accepted (`exec`, `httpGet`, `tcpSocket`). Tracks upstream #1722/#1739/#2156/#2157 — all closed without merge, the gap was never closed there. |
| trivy.nodeSelector | object | `{}` | Node selector for Trivy pods |
| trivy.offlineScan | bool | `false` | Enable offline scan mode |
| trivy.pdb | object | `{"enabled":false}` | PodDisruptionBudget for Trivy |
| trivy.pdb.enabled | bool | `false` | Enable PodDisruptionBudget. When true, exactly one of `minAvailable` or `maxUnavailable` must be set (Kubernetes rejects PDBs with both fields). |
| trivy.persistence | object | `{"accessModes":["ReadWriteOnce"],"annotations":{},"enabled":false,"existingClaim":"","size":"5Gi"}` | Trivy persistence settings - used for cache |
| trivy.persistence.accessModes | list | `["ReadWriteOnce"]` | PVC access modes |
| trivy.persistence.annotations | object | `{}` | Annotations for PVC |
| trivy.persistence.enabled | bool | `false` | Enable persistence for registry |
| trivy.persistence.existingClaim | string | `""` | Existing PVC name (disables dynamic provisioning) |
| trivy.persistence.size | string | `"5Gi"` | PVC size |
| trivy.podAnnotations | object | `{}` | Additional pod annotations for Trivy |
| trivy.podLabels | object | `{}` | Additional pod labels for Trivy |
| trivy.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Trivy |
| trivy.replicas | int | `1` | Number of Trivy replicas (ignored when autoscaling.enabled=true) |
| trivy.resources | object | `{"limits":{"cpu":1,"memory":"1Gi"},"requests":{"cpu":"200m","memory":"512Mi"}}` | Trivy resource requests and limits |
| trivy.secret | object | {} | Sensitive Trivy adapter config (converted to env vars in a Secret). |
| trivy.securityCheck | string | `"vuln"` |  |
| trivy.securityContext | object | `{"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000}` | Security context for Trivy container |
| trivy.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":false,"name":""}` | Service account settings for Trivy |
| trivy.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| trivy.severity | string | `"UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"` | Severity levels to check |
| trivy.skipJavaDBUpdate | bool | `false` | Skip Java DB updates |
| trivy.skipUpdate | bool | `false` | Skip Trivy DB updates |
| trivy.timeout | string | `"5m0s"` | Timeout for scanning |
| trivy.tolerations | list | `[]` | Tolerations for Trivy pods |
| trivy.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| trivy.vulnType | string | `"os,library"` | Vulnerability types to scan (os,library) |
| valkey.architecture | string | `"standalone"` | Valkey architecture: standalone or replication |
| valkey.auth | object | `{"enabled":false,"password":""}` | Valkey authentication settings |
| valkey.dataStorage | object | `{"enabled":false}` | Valkey persistence configuration |
| valkey.enabled | bool | `true` | Enable Valkey subchart |
| valkey.fullnameOverride | string | `"valkey"` |  |
| valkey.podSecurityContext.fsGroup | int | `1000` |  |
| valkey.podSecurityContext.runAsGroup | int | `1000` |  |
| valkey.podSecurityContext.runAsUser | int | `1000` |  |
| valkey.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| valkey.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| valkey.securityContext.runAsNonRoot | bool | `true` |  |
| valkey.securityContext.runAsUser | int | `1000` |  |

## Migrating from Legacy Harbor Chart

This chart is a redesign, not a drop-in replacement. Migration steps:

1. **Backup your Harbor data** using Harbor's built-in backup or database dumps
2. **Export your projects and artifacts** if needed
3. **Deploy this chart** as a new installation
4. **Migrate data** using one of:
   - Harbor's replication feature (recommended)
   - Database migration with external tooling
   - Re-push images from your CI/CD pipeline

### Legacy-to-New Settings Mapping

| Legacy Setting | New Setting |
|---------------|-------------|
| `expose.type: ingress` | `ingress.enabled: true` |
| `expose.ingress.*` | `ingress.*` |
| `database.type: internal` | Not supported — use external DB |
| `database.external.*` | `database.*` |
| `redis.type: internal` | `valkey.enabled: true` |
| `redis.external.*` | `externalRedis.*` |
| `persistence.imageChartStorage.*` | `registry.config.storage.*` (YAML passthrough — see "Registry configuration" below) |
| `nginx.*` | Not applicable — no nginx proxy |
| `notary.*` | Not supported — Notary deprecated |
| `chartmuseum.*` | Not supported — use OCI artifacts |

## Troubleshooting

### Schema Validation Errors

If you see validation errors, check:
- `externalURL` must be a valid URL starting with `http://` or `https://`
- `database.host` is required
- Resource values must be valid Kubernetes quantities

### Pods CrashLooping

Check logs with:
```bash
kubectl logs -n harbor deploy/my-harbor-core
kubectl logs -n harbor deploy/my-harbor-registry
```

Common issues:
- Database connection failed — verify `database.host` and credentials
- Redis connection failed — verify Valkey is running or `externalRedis` config

### Permission Denied Errors

The chart uses `readOnlyRootFilesystem: true`. If a component needs to write:
- Check if a writable volume mount is needed
- Volumes for `/tmp` and other writable paths are pre-configured

## Development

### Run Tests

```bash
helm unittest deploy/chart
```

### Lint Chart

```bash
helm lint deploy/chart \
  --set externalURL=https://example.com \
  --set database.host=db \
  --set harborAdminPassword=changeme123
```

### Generate Docs

```bash
helm-docs --chart-search-root deploy/chart
```

### Render Templates

```bash
helm template test deploy/chart \
  --set externalURL=https://example.com \
  --set database.host=db \
  --set harborAdminPassword=changeme123 \
  --debug
```

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
