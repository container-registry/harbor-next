# Harbor-Next Chart: Remaining Feature Gaps

Compared against harbor-helm upstream `values.yaml`. Items already implemented are not listed.

## Expose Section

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 1.4 | Ingress controller type | Low | Switch logic for gce/ncp/alb/f5-bigip specific annotations |
| 1.6 | Ingress kubeVersionOverride | Trivial | Override API version detection for ingress |

## Persistence

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 2.1 | Global persistence toggle | Low | `persistence.enabled` master switch |
| 2.3 | Registry PVC subPath | Trivial | Already has `filesystem.subPath` but may not align with harbor-helm's PVC subPath |
| 2.4 | Storage caBundleSecretName | Low | Mount CA bundle into registry for self-signed storage backend certs |
| 2.5 | Swift storage backend | Low | Add swift/OpenStack config block |
| 2.6 | Storage existingSecret per backend | Medium | s3/azure/gcs/oss existing secret references for credentials |

## Security

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 3.1 | Internal TLS | High | Inter-component TLS with auto/manual/secret cert sources per component |
| 3.4 | caSecretName | Low | CA cert download link |
| 3.5 | Global caBundleSecretName | Medium | Inject CA bundle into core/jobservice/registry/trivy |
| 3.6 | UAA secret | Trivial | Add uaaSecretName value |

## Registry

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 6.1 | CloudFront middleware | Low | Registry middleware config for CDN |
| 6.3 | registryctl extraEnvVars | Trivial | Separate env list for registryctl container |

## Redis

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 10.1 | Redis database index config | Low | Move hardcoded DB indices (0-7) to values |
| 10.3 | External Redis caBundleSecretName | Low | TLS CA bundle for Redis |

## Global / Cross-cutting

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 11.3 | podSecurityContext.enabled toggle | Low | Global toggle to disable PSC for OpenShift |
| 11.5 | ipFamily dual-stack | Low | `ipFamily.policy`/`families` on services |
| 11.7 | enableMigrateHelmHook | Medium | Database migration Job via helm hook |

## Metrics

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 12.1 | Per-component metrics ports | Low | Individual metrics port config per component |
| 12.2 | ServiceMonitor relabelings | Trivial | Add metricRelabelings/relabelings to ServiceMonitor |

## Priority Recommendation

1. **Trivial wins** — 2.3, 3.6, 6.3, 12.2 (< 30 min total)
2. **Storage existingSecrets** (2.6) — production requirement for secret management
3. **Redis DB indices** (10.1) — easy, removes hardcoded magic numbers
4. **ServiceMonitor relabelings** (12.2) — needed for real monitoring setups
5. **Internal TLS** (3.1) — high effort, important for zero-trust
6. **Helm migration hook** (11.7) — important for upgrades
