# 8gcr-dev FluxCD Bundle

Continuous-deploy bundle for `https://8gcr.container-registry.dev` on the **hz-hopper** cluster.

## What this is

A flat directory of Flux v2 manifests:

| File | Purpose |
|------|---------|
| `namespace.yaml` | Namespace `8gcr-dev-main` (deployment-specific; the cluster runs multiple Harbor instances). |
| `ocirepository.yaml` | Pulls the Harbor Next chart from `oci://8gears.container-registry.com/8gcr-dev/chart/harbor-next:main-latest` every 5 minutes. |
| `helmrelease.yaml` | Renders the chart into the `8gcr-dev-main` namespace with the dev-environment values (image tags pinned to `8gears.container-registry.com/8gcr-dev/harbor-*:main-latest`). |
| `kustomization.yaml` | Plain Kustomize index, lets the Flux Kustomization controller reconcile this directory. |

## How it gets to the cluster

1. **Build & publish** — `.github/workflows/chart-publish.yml`, on every push to `main`:
   - Packages `deploy/chart/` and pushes to `oci://8gears.container-registry.com/8gcr-dev/chart/harbor-next:<version>` (also moves the `main-latest` tag).
   - Bundles this directory and pushes it to `oci://8gears.container-registry.com/8gcr-dev/deploy:<version>` via `flux push artifact` (also moves `main-latest`).
2. **Build & publish images** — `.github/workflows/images-dev.yml`, on every push to `main`:
   - Builds all Harbor component images (core, jobservice, registry, registryctl, portal, exporter, trivy-adapter) for `linux/amd64` and `linux/arm64`.
   - Pushes to `8gears.container-registry.com/8gcr-dev/harbor-<component>:main-<sha7>` and `:main-latest`.
3. **Reconcile** — a Flux `OCIRepository` + `Kustomization` defined in [`container-registry/k8s/apps`](https://github.com/container-registry/k8s/apps) (component `harbor_8gcr_dev.py`) on hz-hopper points at `oci://8gears.container-registry.com/8gcr-dev/deploy:main-latest` and applies this directory.

## Cluster prerequisites (provisioned via `k8s/apps`)

In namespace `8gcr-dev-main`:

- `Secret/harbor-admin` — admin password (key `HARBOR_ADMIN_PASSWORD`).
- `Secret/harbor-database` — PostgreSQL password (key `POSTGRESQL_PASSWORD`).
- `Secret/harbor-oci-pull` — `dockerconfigjson` to pull the chart and component images from `8gears.container-registry.com`.
- `Certificate/harbor-tls` — issued by `letsencrypt-prod` ClusterIssuer for `8gcr.container-registry.dev`.
- A reachable PostgreSQL at `postgres.8gcr-dev-main.svc.cluster.local:5432` with a `registry` database owned by user `harbor` (chart does not embed Postgres).

DNS: `8gcr.container-registry.dev` A/AAAA records point at the hz-hopper ingress LoadBalancer.

## Release vs dev

| Trigger | Registry project | Tag | Consumer |
|---------|------------------|-----|----------|
| push to `main` | `8gcr-dev` | `main-latest`, `main-<sha7>` | this bundle (8gcr-dev environment) |
| release-please tag | `8gcr` | `<semver>` | future production overlays (pin to semver) |

## Local validation

```sh
# render the chart with the values in helmrelease.yaml
yq '.spec.values' deploy/flux/8gcr-dev/helmrelease.yaml > /tmp/values.yaml
helm dependency update deploy/chart
helm template harbor deploy/chart -f /tmp/values.yaml --namespace 8gcr-dev-main | less
```
