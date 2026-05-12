# 8gcr-dev FluxCD Bundle

Self-contained continuous-deploy bundle for `https://8gcr.container-registry.dev` on the **hz-hopper** cluster. Everything required to run this Harbor instance lives in this directory — no private repo, no out-of-band secrets store. SOPS encryption protects credential material so the bundle stays safe in a public repo.

## Layout

| File | Purpose |
|------|---------|
| `namespace.yaml` | Namespace `8gcr-dev-main` (the cluster runs multiple Harbor instances; namespace is deployment-specific). |
| `ocirepository.yaml` | Anonymous pull of the chart from `oci://8gears.container-registry.com/8gcr-dev/chart/harbor-next:latest`, 5-min interval. |
| `helmrelease.yaml` | Renders the chart with the dev-environment values; references the secrets below; provisions a CloudNativePG `Cluster` via the chart's `extraManifests` escape hatch. |
| `secrets.sops.yaml` | SOPS-encrypted `Secret` resources for the admin password, the Harbor↔Postgres password, and the CNPG bootstrap credentials. Decrypted on the cluster by Flux using the existing `flux-system/sops-age` key. |
| `kustomization.yaml` | Plain Kustomize index, lets the Flux Kustomization controller reconcile this directory. |
| `bootstrap.yaml` | One-time `kubectl apply` target that creates the `OCIRepository` + `Kustomization` in `flux-system` to fetch and apply this bundle. Required only the first time per cluster. |

## How it gets to the cluster

1. **Chart + bundle publish** (`.github/workflows/chart-publish.yml`, every push to `main`):
   - Packages `deploy/chart/` and pushes to `oci://8gears.container-registry.com/8gcr-dev/chart/harbor-next:<version>` and `:latest`.
   - Bundles this directory (everything except `bootstrap.yaml`) and pushes to `oci://8gears.container-registry.com/8gcr-dev/deploy:<version>` and `:latest` via `flux push artifact`.
2. **Component images** — maintained out-of-band by an existing buildah pipeline that publishes to `8gears.container-registry.com/8gcr-dev/harbor-<component>:latest`. This bundle consumes those `:latest` tags.
3. **Reconcile** — Flux on hz-hopper pulls the published deploy bundle every 5 min, decrypts `secrets.sops.yaml`, applies everything.

The Harbor project `8gcr-dev` is public, so every pull above is anonymous: no `imagePullSecrets`, no `OCIRepository.secretRef`.

## One-time bootstrap

The cluster needs an initial pointer that says "follow this OCI artifact". That pointer can't itself live inside the artifact (chicken-and-egg). Apply it once per cluster:

```sh
kubectl apply -f \
  https://raw.githubusercontent.com/container-registry/harbor-next/main/deploy/flux/8gcr-dev/bootstrap.yaml
```

This creates an `OCIRepository` and a `Kustomization` in `flux-system` that reconcile the published bundle. After this single `kubectl apply`, every subsequent change reaches the cluster via OCI → Flux. The bootstrap file is idempotent — re-applying it is a no-op.

Equivalent with the Flux CLI:

```sh
flux create source oci harbor-8gcr-dev \
  --url=oci://8gears.container-registry.com/8gcr-dev/deploy \
  --tag=latest --interval=5m

flux create kustomization harbor-8gcr-dev \
  --source=OCIRepository/harbor-8gcr-dev \
  --path=./ --prune --wait --interval=5m --timeout=10m \
  --decryption-provider=sops --decryption-secret=sops-age \
  --depends-on=infrastructure-cert-manager \
  --depends-on=infrastructure-ingress-nginx
```

## Cluster prerequisites (provisioned outside this bundle)

Everything else this bundle needs is satisfied by hz-hopper's standing infrastructure:

| Requirement | Source |
|---|---|
| FluxCD v2 (`source-controller`, `kustomize-controller`, `helm-controller`) | Pre-installed on hz-hopper. |
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

The three `Secret` resources in the file:

| Name | Type | Keys | Used by |
|---|---|---|---|
| `harbor-admin` | `Opaque` | `HARBOR_ADMIN_PASSWORD` | Harbor Core (`existingSecretAdminPassword`). |
| `harbor-database` | `Opaque` | `POSTGRESQL_PASSWORD` | Harbor Core/Jobservice runtime DB client (`database.existingSecret`). |
| `harbor-db-password` | `kubernetes.io/basic-auth` | `username` (`harbor`), `password` | CNPG bootstrap (`spec.bootstrap.initdb.secret.name`). |

`harbor-database/POSTGRESQL_PASSWORD` and `harbor-db-password/password` MUST hold the same value — CNPG initdb seeds Postgres with one, the chart connects with the other.

## Release vs dev

| Trigger | Registry project | Tag | Consumer |
|---------|------------------|-----|----------|
| push to `main` (chart + bundle) | `8gcr-dev` | `latest`, `<base>-main.<sha7>` | this bundle (8gcr-dev environment) |
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
