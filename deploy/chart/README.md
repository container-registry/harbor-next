# Harbor Next Helm Chart

![Version: 3.0.0](https://img.shields.io/badge/Version-3.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v2.14.1](https://img.shields.io/badge/AppVersion-v2.14.1-informational?style=flat-square)

A modern, production-ready Helm chart for [Harbor Next](https://github.com/container-registry/harbor-next) - the cloud native container registry for Kubernetes.

## TL;DR

```bash
helm install my-harbor oci://8gears.container-registry.com/8gcr/chart/harbor-next \
  --set externalURL=https://harbor.example.com \
  --set database.host=my-postgres.example.com \
  --set database.password=secret
```

## Why This Chart?

This chart is a ground-up redesign of the Harbor Helm chart with modern Kubernetes best practices, built for [Harbor Next](https://github.com/container-registry/harbor-next):

| Feature | Legacy harbor-helm | This Chart |
|---------|-------------------|------------|
| **Configuration** | 70+ helper templates, hardcoded env vars | `toEnvVars` pattern - any config works |
| **Database** | Built-in PostgreSQL StatefulSet | External only (production best practice) |
| **Redis** | Built-in Redis Deployment | Valkey subchart or external |
| **Ingress** | nginx reverse proxy + Ingress | Direct Ingress/Gateway API |
| **Security** | Basic security context | PSS Restricted profile compliant |
| **Validation** | None | JSON Schema validation |
| **Resource Defaults** | None (comments only) | Sensible defaults for all components |
| **PodDisruptionBudget** | Not available | Per-component PDB support |
| **Templates** | 48 files, 607-line helpers | ~28 files, 443-line helpers |
| **values.yaml** | 1,116 lines | ~700 lines |

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

- `runAsNonRoot: true` - No root containers
- `readOnlyRootFilesystem: true` - Immutable container filesystem
- `allowPrivilegeEscalation: false` - No privilege escalation
- `capabilities.drop: ["ALL"]` - No Linux capabilities
- `seccompProfile.type: RuntimeDefault` - Seccomp filtering enabled

### High Availability Ready

- **PodDisruptionBudgets** - Ensure availability during node maintenance
- **Resource requests/limits** - Guaranteed QoS with sensible defaults
- **Affinity/Anti-affinity** - Control pod placement
- **Multiple replicas** - Scale any component horizontally

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

```
┌─────────────────────────────────────────────────────────────┐
│                     Ingress / Gateway API                    │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
   ┌─────────┐          ┌──────────┐          ┌──────────┐
   │  Portal │          │   Core   │◄────────►│ Registry │
   │  (UI)   │          │  (API)   │          │ (Images) │
   └─────────┘          └────┬─────┘          └────┬─────┘
                             │                     │
                    ┌────────┴────────┐            │
                    ▼                 ▼            │
              ┌───────────┐    ┌──────────┐       │
              │Jobservice │    │ Exporter │       │
              │ (Tasks)   │    │(Metrics) │       │
              └─────┬─────┘    └──────────┘       │
                    │                             │
        ┌───────────┴───────────┬─────────────────┘
        ▼                       ▼
   ┌─────────┐            ┌──────────┐
   │ Valkey  │            │ Storage  │
   │ (Redis) │            │(PVC/S3/..)│
   └─────────┘            └──────────┘
        │
        ▼
   ┌──────────────┐
   │  PostgreSQL  │ (External - required)
   └──────────────┘
```

## Prerequisites

- Kubernetes 1.33+ (we follow [endoflife.date/kubernetes](https://endoflife.date/kubernetes) for supported versions)
- Helm 3.x
- **External PostgreSQL database** (required)
- PV provisioner (for filesystem storage)

## Installing the Chart

### Basic Installation

```bash
helm install my-harbor oci://8gears.container-registry.com/8gcr/chart/harbor-next \
  --namespace harbor \
  --create-namespace \
  --set externalURL=https://harbor.example.com \
  --set database.host=postgres.example.com \
  --set database.password=your-password
```

### With Values File

```bash
helm install my-harbor oci://8gears.container-registry.com/8gcr/chart/harbor-next \
  --namespace harbor \
  --create-namespace \
  -f values-production.yaml
```

## Uninstalling the Chart

```bash
helm uninstall my-harbor --namespace harbor
```

> **Note**: PersistentVolumeClaims are not deleted automatically. Remove them manually if needed.

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://valkey.io/valkey-helm/ | valkey | 0.9.x |

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
| `<component>.replicas` | Number of replicas |
| `<component>.resources` | CPU/memory requests and limits |
| `<component>.config` | Application config (becomes ConfigMap env vars) |
| `<component>.secret` | Sensitive config (becomes Secret env vars) |
| `<component>.extraEnv` | Additional env vars with `valueFrom` support |
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

## Configuration Examples

### Production Setup with High Availability

```yaml
externalURL: https://harbor.example.com

# External database (required)
database:
  host: postgres.example.com
  password: your-db-password
  sslmode: require

# HA: Multiple replicas with PDB
core:
  replicas: 2
  pdb:
    enabled: true
    minAvailable: 1
  resources:
    requests:
      cpu: 200m
      memory: 512Mi
    limits:
      memory: 1Gi

portal:
  replicas: 2
  pdb:
    enabled: true
    minAvailable: 1

registry:
  replicas: 2
  pdb:
    enabled: true
    minAvailable: 1

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

### S3 Storage Backend

```yaml
registry:
  storage:
    type: s3
    s3:
      region: us-east-1
      bucket: my-harbor-bucket
      accesskey: AKIAIOSFODNN7EXAMPLE
      secretkey: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
      # Optional settings
      regionendpoint: https://s3.us-east-1.amazonaws.com
      encrypt: true
      secure: true
  persistence:
    enabled: false  # Disable PVC when using S3
```

### Azure Blob Storage

```yaml
registry:
  storage:
    type: azure
    azure:
      accountname: mystorageaccount
      accountkey: base64-encoded-key
      container: harbor
  persistence:
    enabled: false
```

### Google Cloud Storage

```yaml
registry:
  storage:
    type: gcs
    gcs:
      bucket: my-harbor-bucket
      keyfile: |
        {
          "type": "service_account",
          ...
        }
  persistence:
    enabled: false
```

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

## Migrating from Legacy Harbor Chart

This chart is a redesign, not a drop-in replacement. Migration steps:

1. **Backup your Harbor data** using Harbor's built-in backup or database dumps
2. **Export your projects and artifacts** if needed
3. **Deploy this chart** as a new installation
4. **Migrate data** using one of:
   - Harbor's replication feature (recommended)
   - Database migration with external tooling
   - Re-push images from your CI/CD pipeline

### Key Migration Differences

| Legacy Setting | New Setting |
|---------------|-------------|
| `expose.type: ingress` | `ingress.enabled: true` |
| `expose.ingress.*` | `ingress.*` |
| `database.type: internal` | Not supported - use external DB |
| `database.external.*` | `database.*` |
| `redis.type: internal` | `valkey.enabled: true` |
| `redis.external.*` | `externalRedis.*` |
| `persistence.imageChartStorage.*` | `registry.storage.*` |
| `nginx.*` | Not applicable - no nginx proxy |
| `notary.*` | Not supported - Notary deprecated |
| `chartmuseum.*` | Not supported - use OCI artifacts |

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
- Database connection failed - verify `database.host` and credentials
- Redis connection failed - verify Valkey is running or `externalRedis` config

### Permission Denied Errors

The chart uses `readOnlyRootFilesystem: true`. If a component needs to write:
- Check if a writable volume mount is needed
- Volumes for `/tmp` and other writable paths are pre-configured

## Development

### Run Tests

```bash
helm unittest .
```

### Lint Chart

```bash
helm lint . --set externalURL=https://example.com --set database.host=db
```

### Render Templates

```bash
helm template test . \
  --set externalURL=https://example.com \
  --set database.host=db \
  --debug
```

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
