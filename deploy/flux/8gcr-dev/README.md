# 8gcr FluxCD Bundle

Continuous-deploy bundle for `https://8gcr.container-registry.dev` on the **hz-hopper** cluster. The manifests are safe to keep in a public repo: credential material is SOPS-encrypted.

## Source of truth

`main` is both the reviewed source of truth and the live source: every push to `main` touching this directory republishes the OCI bundle (`.github/workflows/rolling-flux.yml`), which Flux reconciles within ~5 min. The former `8gcr-rolling` branch is retired; the published artifact keeps its `ops/hz-hopper/8gcr-rolling` repository name so the cluster bootstrap needs no change. One-shot operational manifests (debug/reset Jobs) now also land here via PR and are removed the same way once done.

A single environment doesn't justify a Kustomize base/overlay split yet; when a second tenant (e.g. a production overlay pinned to release semver instead of `latest`) appears, factor the shared pieces into `deploy/flux/base/` and keep per-tenant overlays like this directory.

## Layout

| File | Purpose |
|------|---------|
| `namespace.yaml` | Namespace `8gcr-dev-main` (the cluster runs multiple Harbor instances; namespace is deployment-specific). |
| `ocirepository.yaml` | Authenticated pull of the chart from `oci://8gears.container-registry.com/8gcr/charts/harbor-next`, tracking released versions via semver `>=2.0.0`, 5-min interval. |
| `helmrelease.yaml` | Renders the chart with the 8gcr environment values; references the secrets below; provisions a CloudNativePG `Cluster` via the chart's `extraManifests` escape hatch. |
| `image-tracking.yaml` | Flux image-reflector resources (`ImageRepository` + `ImagePolicy`) that resolve the digest behind each `8gcr/harbor-*:latest` tag in-cluster. No git write-back. |
| `rollout-sync.yaml` | CronJob (+ ServiceAccount/RBAC) that copies each reflected digest into the workload's pod-template annotation, rolling the pods when a `latest` digest moves. Registry to cluster directly — no fluxcdbot commits, no bundle republish per image update. |
| `pull-secret-flux-system.sops.yaml` | SOPS-encrypted `Secret` for `flux-system/harbor-system-pull`, used by Flux to pull `ops/hz-hopper/8gcr-rolling` and inspect `8gcr` images. |
| `pull-secret-8gcr-dev-main.sops.yaml` | SOPS-encrypted `Secret` for `8gcr-dev-main/harbor-system-pull`, used by Flux and Harbor pods to pull the chart and component images from `8gcr`. |
| `secrets.sops.yaml` | SOPS-encrypted `Secret` resources for the admin password, the Harbor↔Postgres password, and the CNPG bootstrap credentials. It does not contain registry pull credentials. Decrypted on the cluster by Flux using the existing `flux-system/sops-age` key. |
| `kustomization.yaml` | Plain Kustomize index, lets the Flux Kustomization controller reconcile this directory. |
| `bootstrap.yaml` | One-time `kubectl apply` target that creates the `OCIRepository` + `Kustomization` in `flux-system` to fetch and apply this bundle using the `harbor-system-pull` secret. Required only the first time per cluster. |

## How it gets to the cluster

1. **Component image publish** (`.github/workflows/main-images.yml`, runs after `Release Ready` succeeds on `main`, via the reusable `publish-images.yml`):
   - Applies the commercial patches, then builds all Harbor component images for `linux/amd64` and `linux/arm64`.
   - Moves `latest` for `8gears.container-registry.com/8gcr/harbor-<component>` (`trivy-adapter` keeps its existing `8gcr/trivy-adapter` repository name).
   - It does not publish Flux deployment configuration.
2. **Chart publish** (`.github/workflows/publish-chart.yml`, dispatched for a published `chart-v<semver>` release tag):
   - Packages `deploy/chart/` and pushes to `oci://8gears.container-registry.com/8gcr/charts/harbor-next:<version>`.
   - This bundle's chart `OCIRepository` follows those releases with a `>=2.0.0` semver range, so a new chart release rolls out without editing this directory.
   - It does not publish Flux deployment configuration.
3. **Rolling Flux config publish** (`.github/workflows/rolling-flux.yml`, every push to `main` that touches this directory):
   - Bundles this directory and pushes it to `oci://8gears.container-registry.com/ops/hz-hopper/8gcr-rolling:<branch>-<sha7>`.
   - Moves the `latest` tag on that OCI artifact.
   - Authenticates to the `ops` project with `secrets.OPS_REGISTRY_PASSWORD`.
4. **Latest image tracking (gitless)** — image-reflector on hz-hopper watches each `8gcr/harbor-*:latest` tag by digest (`image-tracking.yaml`). The `rollout-sync` CronJob compares each `ImagePolicy` digest with the workload's pod-template annotation and patches it on change, restarting the pods (`imagePullPolicy: Always` re-pulls `latest`). Image updates never touch git or republish the bundle.
5. **Reconcile** — Flux on hz-hopper pulls the published `ops/hz-hopper/8gcr-rolling:latest` bundle every 5 min, decrypts SOPS secrets, and applies everything. Chart releases roll out via the OCIRepository semver range; image updates roll out via `rollout-sync`.

The manifests intentionally do not store plaintext registry credentials. Pull access is provided by SOPS-managed cluster secrets:

| Namespace | Secret | Used by |
|---|---|---|
| `flux-system` | `harbor-system-pull` | Root `OCIRepository/harbor-8gcr` pulling `ops/hz-hopper/8gcr-rolling` and Flux image scanning for `8gcr/harbor-*`. |
| `8gcr-dev-main` | `harbor-system-pull` | Chart `OCIRepository/harbor-next-chart` and Harbor component image pulls. |

## One-time bootstrap

The cluster needs an initial pointer that says "follow this OCI artifact". That pointer can't itself live inside the artifact (chicken-and-egg). Apply it once per cluster:

```sh
kubectl apply -f \
  https://raw.githubusercontent.com/container-registry/harbor-next/main/deploy/flux/8gcr-dev/bootstrap.yaml
```

This creates an `OCIRepository` and a `Kustomization` in `flux-system` that reconcile the published bundle. After this single `kubectl apply`, every subsequent change reaches the cluster via OCI → Flux. The bootstrap file is idempotent — re-applying it is a no-op.

Equivalent with the Flux CLI:

```sh
flux create source oci harbor-8gcr \
  --url=oci://8gears.container-registry.com/ops/hz-hopper/8gcr-rolling \
  --tag=latest --interval=5m \
  --secret-ref=harbor-system-pull

flux create kustomization harbor-8gcr \
  --source=OCIRepository/harbor-8gcr \
  --path=./ --prune --wait --interval=5m --timeout=10m \
  --decryption-provider=sops --decryption-secret=sops-age \
  --depends-on=infrastructure-cert-manager \
  --depends-on=infrastructure-ingress-nginx
```

## Cluster prerequisites (provisioned outside this bundle)

Everything else this bundle needs is satisfied by hz-hopper's standing infrastructure:

| Requirement | Source |
|---|---|
| FluxCD v2 (`source-controller`, `kustomize-controller`, `helm-controller`, `image-reflector-controller`) | Pre-installed on hz-hopper. image-reflector is required for mutable `latest` digest tracking; image-automation-controller is NOT used (no git write-back). |
| `flux-system/harbor-system-pull` | SOPS-managed dockerconfigjson for pulling `ops/hz-hopper/8gcr-rolling` and scanning `8gcr/harbor-*` images. |
| `8gcr-dev-main/harbor-system-pull` | SOPS-managed dockerconfigjson for pulling the chart and component images from `8gcr`. |
| SOPS decryption key | `Secret/sops-age` in `flux-system` (already present; same key used by other Flux Kustomizations). Public key: `age18jfefmcak9zk6jrh7j59ap0rg3zxg577suvmlyrgm3sn0l28zq4slcu94r`. |
| CloudNativePG operator | Pre-installed in `cnpg-system`. The chart's `extraManifests` renders a `Cluster.postgresql.cnpg.io/v1`; CNPG reconciles it into a Postgres pod + `harbor-db-rw` Service that the chart's `database.host` value points at. |
| Cert-manager + `letsencrypt-prod` ClusterIssuer | Pre-installed; HelmRelease ingress is annotated to use it. |
| `nginx` IngressClass | Pre-installed. |
| DNS `8gcr.container-registry.dev` → `65.109.85.186` | Manual Cloudflare A record (external-dns on hz-hopper only manages `container-registry.com`, not `.dev`). |

## Editing secrets

The encrypted file is created and updated with [SOPS](https://github.com/getsops/sops) against the cluster's age public key:

```sh
export AGE_PUB=age18jfefmcak9zk6jrh7j59ap0rg3zxg577suvmlyrgm3sn0l28zq4slcu94r

# create or edit in-place
sops --age "$AGE_PUB" --encrypted-regex '^(data|stringData)$' \
  deploy/flux/8gcr-dev/secrets.sops.yaml

# rotate by re-encrypting a freshly-built plaintext file
sops --age "$AGE_PUB" --encrypted-regex '^(data|stringData)$' \
  -e plaintext-secrets.yaml > deploy/flux/8gcr-dev/secrets.sops.yaml
```

The two `Secret` resources in the file:

| Name | Type | Keys | Consumers |
|---|---|---|---|
| `harbor-admin` | `Opaque` | `HARBOR_ADMIN_PASSWORD` | Harbor Core (`existingSecretAdminPassword`). |
| `harbor-db-password` | `kubernetes.io/basic-auth` | `username` (`harbor`), `password` | (1) CNPG bootstrap — `Cluster.spec.bootstrap.initdb.secret.name`; (2) Harbor's chart — `database.existingSecret` + `existingSecretKey: password`. Same Secret, both consumers. |

One DB Secret, one source of truth: rotating `harbor-db-password.password` updates both CNPG-initdb (on a fresh Cluster) and Harbor's runtime client in one place. No risk of the runtime password drifting away from what was seeded into Postgres.

## Release vs rolling

| Trigger | Registry project | Tag | Consumer |
|---------|------------------|-----|----------|
| push to `main` (component images) | `8gcr` | `latest`, `main-<sha7>` | image-reflector + `rollout-sync` CronJob track the `latest` digest in-cluster |
| push to `main` (chart) | `8gcr` | `2.0.0`, `<base>-dev`, `<base>-main.<sha7>` | this bundle's chart source |
| push to `main` (Flux config) | `ops` | `latest`, `<branch>-<sha7>` | hz-hopper root Flux `OCIRepository` |
| release-please tag | `8gcr` | `<semver>` | future production overlays (pin to semver) |

## Local validation

```sh
# render the chart with the values in helmrelease.yaml
yq '.spec.values' deploy/flux/8gcr-dev/helmrelease.yaml > /tmp/values.yaml
helm dependency update deploy/chart
helm template harbor deploy/chart -f /tmp/values.yaml --namespace 8gcr-dev-main | less

# decrypt secrets locally (requires SOPS_AGE_KEY_FILE pointing at the cluster's private key)
sops -d deploy/flux/8gcr-dev/secrets.sops.yaml
```
