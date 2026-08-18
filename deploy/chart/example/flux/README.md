# FluxCD (GitOps)

End-to-end Flux setup for the `harbor-next` chart: `GitRepository` source,
`HelmRelease` with drift detection, and the secret-pinning values that make
rendering **byte-for-byte deterministic** — the property GitOps engines
need to avoid perpetual drift and surprise rollouts.

## Why pinning matters

By default the chart auto-generates secret material it needs (encryption
key, per-component identity secrets, CSRF key, token-signing CA, registry
htpasswd). Generated values are random per render:

- **Argo CD** templates client-side on every sync — `lookup` returns
  nothing there, so every sync would rotate all generated secrets and
  roll every workload through the `checksum/secret` annotations.
- **Flux** runs real Helm upgrades, where `lookup` persists most values —
  but any path that still generates (and `helm diff`-style tooling)
  benefits from pinning all of them.

This example sets `autoGenSecrets: false`, which turns every would-be
generation into a **render-time failure naming the value to pin**. With
all identities pinned, two renders of the chart are byte-identical (CI
enforces this — `task helm:gitops-determinism`).

## What gets pinned

| Value | Secret (key) | Consumed by |
|---|---|---|
| `existingSecretAdminPassword` | `harbor-admin` (`HARBOR_ADMIN_PASSWORD`) | core |
| `existingSecretSecretKey` | `harbor-encryption` (`secretKey`) | core (credential encryption) |
| `core.existingSecret` | `harbor-core-identity` (`secret`) | all components (service auth) |
| `core.existingXsrfSecret` | `harbor-core-identity` (`CSRF_KEY`) | core |
| `core.tokenSecretName` | `harbor-token-cert` (`tls.key`, `tls.crt`) | core (registry token signing) |
| `registry.existingSecret` | `harbor-registry-identity` (`REGISTRY_HTTP_SECRET`) | registry |
| `registry.credentials.existingSecret` | `harbor-registry-credentials` (`REGISTRY_CREDENTIAL_PASSWORD` + `REGISTRY_HTPASSWD`) | core/jobservice ↔ registry |
| `jobservice.existingSecret` | `harbor-jobservice-identity` (`JOBSERVICE_SECRET`) | jobservice |
| `database.existingSecret` | `harbor-database` (`POSTGRESQL_PASSWORD`) | core/jobservice/exporter |
| `ingress.autoGenCert: false` + `ingress.tls` | `harbor-tls` (cert-manager) | ingress |

## Usage

1. Generate real values for `identity-secrets.yaml` (commands in the file
   header), then **encrypt it with [SOPS](https://fluxcd.io/flux/guides/mozilla-sops/)**
   or replace it with ExternalSecrets / SealedSecrets manifests. Never
   commit plaintext secrets.
2. Adjust `helmrelease.yaml`: `externalURL`, ingress host/issuer, external
   PostgreSQL coordinates, storage size.
3. Commit this directory to the repository Flux watches, and reference it
   from a Flux `Kustomization`:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: harbor
  namespace: flux-system
spec:
  interval: 10m
  path: ./deploy/chart/example/flux
  prune: true
  sourceRef:
    kind: GitRepository
    name: flux-system
  decryption:          # when using SOPS
    provider: sops
    secretRef:
      name: sops-age
```

## Rotation

Pinned secrets are yours, so rotation is yours too: update the Secret
(re-encrypt with SOPS or rotate in the external store). Kubernetes does
**not** restart pods when a referenced Secret changes — trigger a rollout
(`kubectl rollout restart deployment -n harbor -l app.kubernetes.io/instance=harbor`)
or run [Reloader](https://github.com/stakater/Reloader) and annotate the
deployments via `<component>.podAnnotations`/chart `extraManifests`.

Note: rotating `harbor-encryption` (`secretKey`) re-keys nothing
retroactively — credentials already stored in the database stay encrypted
with the old key and become unreadable. Treat it as fixed for the life of
the instance.

## Argo CD

The same pinned values work unchanged as an Argo CD `Application`:
deterministic rendering means no perpetual `OutOfSync`, and
`autoGenSecrets: false` guarantees a loud render failure instead of silent
secret rotation if a pin is ever removed. No `ignoreDifferences`
workarounds needed.
