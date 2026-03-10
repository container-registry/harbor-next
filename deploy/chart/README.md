# Harbor Next

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

This chart is a ground-up redesign of the Harbor Helm chart with modern Kubernetes best practices, built for [Harbor Next](https://github.com/container-registry/harbor-next) and Harbor.

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
![Diagram](https://kroki.io/svgbob/svg/eNp7NKXn0ZSG4YImcD2a0qSABSh55qUXpRYXK-gruCeWpJYnVio4BngqKeAAQFOAJk0ZRiEzg0uBAAD5GZlHebrYQ4X4RHLRtD3Y3Y1bnItEj0wgw_Nk6YE4DJRSA_KLShJzFOB8JJZzflEqjP9oegu6KdN2gdUFpaZnFpcUVSJiEKJdSSPUU1MJm8FKGsCkD5ZCk1HS8MxNTE8tBsrBjSI-D8yAG4VVzxqq6SEmJRORwtGk8MTeFsKxT9B8LIkUTQybblITIukJF7utTQpe-UnFqUVlmcmpsPAEk64VBcAEm1qEFMq4TAAmp5DE4mxQakIyQUnDN7WkKDMZmsjwmEBaKiI9peJNDU00KSu3EKdsDcUlPK4ykcJykfR6AamkC0vMyU6txAheCCe4JL8IWPQooJViShpBqSmZSEkIoUlJIyDMWT_YWF8PKktueUWiNvSQRk0JpIct9kqhuATYXAkO9EEEhGsFMNvlAauKRw1TFIpSC0szi1JTNJW4SHc-zBMATXrWZQ==)

## Prerequisites

- Kubernetes 1.28+ (we follow [endoflife.date/kubernetes](https://endoflife.date/kubernetes) for supported versions)
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

## Example Values Files

The [`example/`](example/) directory contains ready-to-use values files for common environments:

| File | Description |
|------|-------------|
| [`k3d-local.yaml`](example/k3d-local.yaml) | Local development with k3d cluster |
| [`rke2-rancher.yaml`](example/rke2-rancher.yaml) | RKE2/Rancher deployment |
| [`openshift/`](example/openshift/) | OpenShift deployment with edge-terminated routes |

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
| core.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-core","tag":""}` | Core image settings |
| core.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-core"` | Core image repository |
| core.image.tag | string | `""` | Core image tag (defaults to appVersion) |
| core.initContainers | list | `[]` | Init containers (run before main containers) |
| core.nodeSelector | object | `{}` | Node selector for Core pods |
| core.pdb | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget for Core |
| core.pdb.enabled | bool | `false` | Enable PodDisruptionBudget |
| core.pdb.minAvailable | int | `1` | Minimum available pods (can be integer or percentage) |
| core.podAnnotations | object | `{}` | Additional pod annotations for Core |
| core.podLabels | object | `{}` | Additional pod labels for Core |
| core.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Core |
| core.quotaUpdateProvider | string | `"db"` | Quota update provider (db or redis) |
| core.replicas | int | `1` | Number of Core replicas |
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
| database.connMaxIdleTime | string | `"0"` | Maximum idle time for connections (0 = no limit) |
| database.connMaxLifetime | string | `"0"` | Maximum lifetime of connections (0 = no limit) |
| database.database | string | `"registry"` | Database name |
| database.existingSecret | string | `""` | Existing secret containing database credentials Default secret key: POSTGRESQL_PASSWORD |
| database.existingSecretKey | string | `""` |  |
| database.host | string | `""` | Database host (required) |
| database.maxIdleConns | int | `100` | Maximum idle connections |
| database.maxOpenConns | int | `900` | Maximum open connections |
| database.password | string | `""` | Database password (ignored if existingSecret is set) |
| database.port | int | `5432` | Database port |
| database.sslmode | string | `"disable"` | SSL mode for database connection |
| database.username | string | `""` | Database username |
| existingSecretAdminPassword | string | `""` | Existing secret containing the admin password (overrides harborAdminPassword) |
| existingSecretAdminPasswordKey | string | `"HARBOR_ADMIN_PASSWORD"` | Key in the existing secret for admin password |
| existingSecretSecretKey | string | `""` | Existing secret containing the encryption key (overrides secretKey) |
| exporter.affinity | object | `{}` | Affinity rules for Exporter pods |
| exporter.config | object | {} | Exporter application config (converted to env vars in ConfigMap) |
| exporter.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| exporter.enabled | bool | `true` | Enable Harbor exporter for Prometheus metrics |
| exporter.extraEnv | list | [] | Extra environment variables with valueFrom support |
| exporter.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-exporter","tag":""}` | Exporter image settings |
| exporter.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-exporter"` | Exporter image repository |
| exporter.image.tag | string | `""` | Exporter image tag (defaults to appVersion) |
| exporter.initContainers | list | `[]` | Init containers (run before main containers) |
| exporter.nodeSelector | object | `{}` | Node selector for Exporter pods |
| exporter.pdb | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget for Exporter |
| exporter.pdb.enabled | bool | `false` | Enable PodDisruptionBudget |
| exporter.pdb.minAvailable | int | `1` | Minimum available pods (can be integer or percentage) |
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
| externalRedis.existingSecret | string | `""` | Existing secret containing Redis password Must have key: REDIS_PASSWORD |
| externalRedis.host | string | `""` | External Redis host |
| externalRedis.password | string | `""` | External Redis password |
| externalRedis.port | int | `6379` | External Redis port |
| externalRedis.sentinelMasterSet | string | `""` | Sentinel master set name (for Redis Sentinel) |
| externalRedis.tlsOptions | object | `{"enable":false}` | TLS options for external Redis |
| externalRedis.tlsOptions.enable | bool | `false` | Enable TLS for external Redis connection |
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
| harborAdminPassword | string | "Harbor12345" | Harbor admin password (initial setup) |
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
| jobservice.config | object | {} | Jobservice application config (converted to env vars in ConfigMap) |
| jobservice.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| jobservice.existingSecret | string | `""` | Use existing secret for Jobservice secret |
| jobservice.existingSecretKey | string | `"JOBSERVICE_SECRET"` | Key in existing secret containing the Jobservice secret |
| jobservice.extraEnv | list | [] | Extra environment variables with valueFrom support |
| jobservice.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-jobservice","tag":""}` | Jobservice image settings |
| jobservice.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-jobservice"` | Jobservice image repository |
| jobservice.image.tag | string | `""` | Jobservice image tag (defaults to appVersion) |
| jobservice.initContainers | list | `[]` | Init containers (run before main containers) |
| jobservice.jobLoggers | list | `["file"]` | Job loggers: file, database, or stdout |
| jobservice.loggerSweeperDuration | int | `14` | Logger sweeper duration in days (ignored if logger is stdout) |
| jobservice.max_job_workers | int | `4` |  |
| jobservice.nodeSelector | object | `{}` | Node selector for Jobservice pods |
| jobservice.notification | object | `{"webhook_job_http_client_timeout":3,"webhook_job_max_retry":3}` | Notification settings |
| jobservice.pdb | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget for Jobservice |
| jobservice.pdb.enabled | bool | `false` | Enable PodDisruptionBudget |
| jobservice.pdb.minAvailable | int | `1` | Minimum available pods (can be integer or percentage) |
| jobservice.persistence | object | `{"accessModes":["ReadWriteOnce"],"annotations":{},"enabled":false,"existingClaim":"","resourcePolicy":"keep","size":"1Gi","storageClass":""}` | Jobservice persistence settings |
| jobservice.persistence.accessModes | list | `["ReadWriteOnce"]` | PVC access modes |
| jobservice.persistence.annotations | object | `{}` | Annotations for PVC |
| jobservice.persistence.enabled | bool | `false` | Enable persistence for jobservice |
| jobservice.persistence.existingClaim | string | `""` | Existing PVC name (disables dynamic provisioning) |
| jobservice.persistence.resourcePolicy | string | `"keep"` | Resource policy: "keep" prevents PVC deletion on helm uninstall |
| jobservice.persistence.size | string | `"1Gi"` | PVC size |
| jobservice.persistence.storageClass | string | `""` | Storage class for PVC |
| jobservice.podAnnotations | object | `{}` | Additional pod annotations for Jobservice |
| jobservice.podLabels | object | `{}` | Additional pod labels for Jobservice |
| jobservice.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Jobservice |
| jobservice.reaper | object | `{"max_dangling_hours":168,"max_update_hours":24}` | Reaper settings |
| jobservice.replicas | int | `1` | Number of Jobservice replicas |
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
| portal.config | object | {} | Portal application config (converted to env vars in ConfigMap) |
| portal.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| portal.extraEnv | list | [] | Extra environment variables with valueFrom support |
| portal.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-portal","tag":""}` | Portal image settings |
| portal.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-portal"` | Portal image repository |
| portal.image.tag | string | `""` | Portal image tag (defaults to appVersion) |
| portal.initContainers | list | `[]` | Init containers (run before main containers) |
| portal.nodeSelector | object | `{}` | Node selector for Portal pods |
| portal.pdb | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget for Portal |
| portal.pdb.enabled | bool | `false` | Enable PodDisruptionBudget |
| portal.pdb.minAvailable | int | `1` | Minimum available pods (can be integer or percentage) |
| portal.podAnnotations | object | `{}` | Additional pod annotations for Portal |
| portal.podLabels | object | `{}` | Additional pod labels for Portal |
| portal.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Portal |
| portal.replicas | int | `1` | Number of Portal replicas |
| portal.resources | object | `{"limits":{"memory":"256Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Portal resource requests and limits |
| portal.secret | object | {} | Sensitive config for Portal (converted to env vars in Secret) |
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
| registry.config | object | `{}` |  |
| registry.controller | object | `{"image":{"repository":"8gears.container-registry.com/8gcr/harbor-registryctl","tag":""}}` | Registryctl image settings |
| registry.controller.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-registryctl"` | Registryctl image repository |
| registry.controller.image.tag | string | `""` | Registryctl image tag (defaults to appVersion) |
| registry.credentials.existingSecret | string | `""` |  |
| registry.credentials.htpasswdString | string | `""` |  |
| registry.credentials.password | string | `""` |  |
| registry.credentials.username | string | `"harbor_registry_user"` |  |
| registry.deploymentStrategy | object | {} | Deployment strategy (empty = K8s default RollingUpdate) |
| registry.extraEnv | list | [] | Extra environment variables with valueFrom support |
| registry.image | object | `{"repository":"8gears.container-registry.com/8gcr/harbor-registry","tag":""}` | Registry image settings |
| registry.image.repository | string | `"8gears.container-registry.com/8gcr/harbor-registry"` | Registry image repository |
| registry.image.tag | string | `""` | Registry image tag (defaults to appVersion) |
| registry.initContainers | list | `[]` | Init containers (run before main containers) |
| registry.nodeSelector | object | `{}` | Node selector for Registry pods |
| registry.pdb | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget for Registry |
| registry.pdb.enabled | bool | `false` | Enable PodDisruptionBudget |
| registry.pdb.minAvailable | int | `1` | Minimum available pods (can be integer or percentage) |
| registry.persistence | object | `{"accessModes":["ReadWriteOnce"],"annotations":{},"enabled":false,"existingClaim":"","resourcePolicy":"keep","size":"10Gi","storageClass":""}` | Registry persistence settings |
| registry.persistence.accessModes | list | `["ReadWriteOnce"]` | PVC access modes |
| registry.persistence.annotations | object | `{}` | Annotations for PVC |
| registry.persistence.enabled | bool | `false` | Enable persistence for registry |
| registry.persistence.existingClaim | string | `""` | Existing PVC name (disables dynamic provisioning) |
| registry.persistence.resourcePolicy | string | `"keep"` | Resource policy: "keep" prevents PVC deletion on helm uninstall |
| registry.persistence.size | string | `"10Gi"` | PVC size |
| registry.persistence.storageClass | string | `""` | Storage class for PVC |
| registry.podAnnotations | object | `{}` | Additional pod annotations for Registry |
| registry.podLabels | object | `{}` | Additional pod labels for Registry |
| registry.podSecurityContext | object | `{"fsGroup":10000,"fsGroupChangePolicy":"OnRootMismatch"}` | Pod security context for Registry |
| registry.relativeurls | bool | `false` | If true, the registry returns relative URLs in Location headers |
| registry.replicas | int | `1` | Number of Registry replicas |
| registry.resources | object | `{"limits":{"memory":"512Mi"},"requests":{"cpu":"100m","memory":"256Mi"}}` | Registry resource requests and limits |
| registry.secret | object | {} | Sensitive config for Registry (converted to env vars in Secret) |
| registry.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":10000,"runAsNonRoot":true,"runAsUser":10000,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for Registry container |
| registry.serviceAccount | object | `{"annotations":{},"automountServiceAccountToken":false,"create":true,"name":""}` | Service account settings for Registry |
| registry.serviceAccount.automountServiceAccountToken | bool | `false` | Automount service account token |
| registry.storage | object | `{"azure":{},"disableredirect":false,"filesystem":{"rootdirectory":"/storage","subPath":""},"gcs":{},"oss":{},"s3":{},"type":"filesystem"}` | Registry storage configuration |
| registry.storage.azure | object | `{}` | Azure Blob storage settings |
| registry.storage.filesystem | object | `{"rootdirectory":"/storage","subPath":""}` | Filesystem storage settings |
| registry.storage.gcs | object | `{}` | Google Cloud Storage settings |
| registry.storage.oss | object | `{}` | Alibaba Cloud OSS settings |
| registry.storage.s3 | object | `{}` | S3 storage settings |
| registry.storage.type | string | `"filesystem"` | Storage type: filesystem, s3, azure, gcs, oss |
| registry.tolerations | list | `[]` | Tolerations for Registry pods |
| registry.topologySpreadConstraints | list | `[]` | Topology spread constraints for pod scheduling |
| registry.upload_purging | object | {} | Registry application config (converted to env vars in ConfigMap) |
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
| trivy.dbRepository[0] | string | `"mirror.gcr.io/aquasec/trivy-db"` |  |
| trivy.dbRepository[1] | string | `"ghcr.io/aquasecurity/trivy-db"` |  |
| trivy.debugMode | bool | `false` | Debug mode for more verbose scanning log |
| trivy.enabled | bool | `false` | Enable Trivy scanner |
| trivy.gitHubToken | string | `""` | GitHub token to download Trivy DB (optional) |
| trivy.ignoreUnfixed | bool | `false` | Skip unfixed vulnerabilities |
| trivy.image.repository | string | `"8gears.container-registry.com/8gcr/trivy-adapter"` | Trivy adapter image repository |
| trivy.image.tag | string | `""` | Trivy adapter image tag (defaults to appVersion) |
| trivy.initContainers | list | `[]` | Init containers (run before main containers) |
| trivy.insecure | bool | `false` | Skip verifying registry certificate |
| trivy.javaDBRepository[0] | string | `"mirror.gcr.io/aquasec/trivy-java-db"` |  |
| trivy.javaDBRepository[1] | string | `"ghcr.io/aquasecurity/trivy-java-db"` |  |
| trivy.nodeSelector | object | `{}` | Node selector for Trivy pods |
| trivy.offlineScan | bool | `false` | Enable offline scan mode |
| trivy.pdb | object | `{"enabled":false,"minAvailable":1}` | PodDisruptionBudget for Trivy |
| trivy.pdb.enabled | bool | `false` | Enable PodDisruptionBudget |
| trivy.pdb.minAvailable | int | `1` | Minimum available pods |
| trivy.persistence | object | `{"accessModes":["ReadWriteOnce"],"annotations":{},"enabled":false,"existingClaim":"","size":"5Gi","storageClass":""}` | Trivy persistence settings - used for cache |
| trivy.persistence.accessModes | list | `["ReadWriteOnce"]` | PVC access modes |
| trivy.persistence.annotations | object | `{}` | Annotations for PVC |
| trivy.persistence.enabled | bool | `false` | Enable persistence for registry |
| trivy.persistence.existingClaim | string | `""` | Existing PVC name (disables dynamic provisioning) |
| trivy.persistence.size | string | `"5Gi"` | PVC size |
| trivy.persistence.storageClass | string | `""` | Storage class for PVC |
| trivy.podAnnotations | object | `{}` | Additional pod annotations for Trivy |
| trivy.podLabels | object | `{}` | Additional pod labels for Trivy |
| trivy.podSecurityContext | object | `{"fsGroup":10000}` | Pod security context for Trivy |
| trivy.replicas | int | `1` | Number of Trivy replicas |
| trivy.resources | object | `{"limits":{"cpu":1,"memory":"1Gi"},"requests":{"cpu":"200m","memory":"512Mi"}}` | Trivy resource requests and limits |
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
| `persistence.imageChartStorage.*` | `registry.storage.*` |
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
