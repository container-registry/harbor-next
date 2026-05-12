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
| ~~2.4~~ | ~~Storage caBundleSecretName~~ | ~~Low~~ | **Done** — covered by `externalRedis.tlsOptions.existingCaSecret` (mounts the bundle on every component, including registry) |
| 2.5 | Swift storage backend | Low | Add swift/OpenStack config block |
| ~~2.6~~ | ~~Storage existingSecret per backend~~ | ~~Medium~~ | **Done** — `registry.storage.{s3,azure,oss}.existingSecret` |

## Security

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 3.1 | Internal TLS (inter-component mTLS) | High | See "Decisions" below — deferred |
| 3.4 | caSecretName | Low | CA cert download link |
| ~~3.5~~ | ~~Global caBundleSecretName~~ | ~~Medium~~ | **Done** — `externalRedis.tlsOptions.existingCaSecret` is honored by every Harbor component's TLS layer via `SSL_CERT_DIR`, not just Redis (S3, OIDC, LDAP all benefit) |
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
| ~~10.3~~ | ~~External Redis caBundleSecretName~~ | ~~Low~~ | **Done** — `externalRedis.tlsOptions.existingCaSecret` |

## Global / Cross-cutting

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 11.3 | podSecurityContext.enabled toggle | Low | Global toggle to disable PSC for OpenShift |
| 11.5 | ipFamily dual-stack | Low | `ipFamily.policy`/`families` on services |
| 11.7 | enableMigrateHelmHook | Medium | See "Decisions" below — deferred |

## Metrics

| # | Feature | Complexity | Notes |
|---|---------|-----------|-------|
| 12.1 | Per-component metrics ports | Low | Individual metrics port config per component |
| 12.2 | ServiceMonitor relabelings | Trivial | Add metricRelabelings/relabelings to ServiceMonitor |

## Decisions on deferred items

### Migration hook (11.7 / upstream #1178) — deferred

Upstream filed and re-filed this; #1178 closed `NOT_PLANNED` after 14
comments. The reason is the same here: **Harbor core runs database
migrations automatically on startup**
(`src/common/dao/pgsql.go#UpgradeSchema`), so a pre-install helm Job
that runs migrations separately is redundant and would race the core
container in `helm upgrade` if not gated correctly.

The community ask is real (people want an opt-in pre-install hook),
but solving it properly requires either:
  (a) An `init: true` mode in `harbor-core` that runs migrations and
      exits without starting the server — currently no such flag.
  (b) Reusing the core image as a hook Job with a custom entrypoint
      that calls only the migrator — fragile across image rebuilds.

Status: **out of chart scope until (a) is addressed in Harbor core**.

### Internal TLS (3.1 / upstream gap, weak community signal) — deferred

Upstream `internalTLS.*` ships ~30 values controlling auto/manual/secret
cert sources per component, mounting paths, CA propagation, and per-
component TLS toggles. Roughly equivalent to a full sub-chart of work.

Community signal is **weak** — no specific high-reaction OPEN issue;
listed only in REMAINING-GAPS.md and production-criticality judgement.
Harbor in cluster-internal pod-to-pod paths is typically secured by
the cluster's underlying network policy / service mesh (Istio mTLS,
Linkerd transparent TLS), which gives the same property at the network
layer without coupling Harbor to a particular cert-management pipeline.

Status: **deferred**. The chart's existing cert-manager integration
(`tls.certManager.*`) and the new `existingCaSecret` plumbing (which
already mounts CAs on every component) cover the highest-pain TLS
paths — public ingress, external Redis, external S3, external PG.
Inter-component TLS is best deferred to a follow-up that aligns with
either upstream's internalTLS shape or service-mesh handoff.

### HA verification (8 / upstream #435) — verified

Walked through the chart's existing posture:
  - `topologySpreadConstraints` block is wired on all 6 components and
    propagates via the `harbor.podScheduling` helper.
  - `PodDisruptionBudget` is opt-in per component with mutual-exclusion
    fail-fast on minAvailable + maxUnavailable (pdbs.yaml).
  - `revisionHistoryLimit` is per-component with a global fallback.
  - PVC accessModes default to `ReadWriteOnce` (matches harbor-helm).
    Users with multiple replicas + filesystem registry storage MUST
    switch to `ReadWriteMany` or external storage — the chart can't
    auto-detect this.
  - With HPA now available (item 4), production users have all the
    primitives needed for HA: spread + PDB + autoscale + multi-AZ
    node selection via tolerations/affinity.

End-to-end failover testing belongs in CI integration tests, not unit
tests. **No code changes needed.**
