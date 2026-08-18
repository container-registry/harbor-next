# Create Deployment on K3S

This guide deploys Harbor with a CloudNativePG-managed database in the
same namespace — everything in one values file. For a plain local-dev
setup on k3d (same Traefik ingress, throwaway Postgres Deployment instead
of CNPG), see [`example/k3d-local/`](../../example/k3d-local/).

## Deploy Local CNPG System

To deploy the database in the same namespace, first install the CloudNativePG (CNPG) operator:

```bash
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.28/releases/cnpg-1.28.1.yaml
```

## Retrieve Harbor Helm Chart

Pull the Harbor Helm chart from the OCI registry.

1. Pull the chart:

   ```bash
   helm pull oci://8gears.container-registry.com/8gcr/charts/harbor-next
   ```

2. Decompress the downloaded chart:

   ```bash
   tar xzvf harbor-*.tgz
   cd harbor-next
   ```

## Prepare a Values File

Save the following as `values-k3s.yaml` in the extracted chart directory.
The three blocks below stack into a single file — `extraManifests` deploys
the database alongside Harbor, `database` wires Harbor to it via an
existing secret, and `ingress` exposes Harbor through Traefik.

```yaml
# values-k3s.yaml

# 1. Database manifest deployed in the same namespace as Harbor
extraManifests:
  - apiVersion: postgresql.cnpg.io/v1
    kind: Cluster
    metadata:
      name: harbor-db
    spec:
      instances: 1
      storage:
        size: 10Gi
      bootstrap:
        initdb:
          database: registry
          owner: harbor

# 2. Harbor → database wiring (CNPG creates `harbor-db-app` Secret)
database:
  host: "harbor-db-rw"
  port: 5432
  username: "harbor"
  existingSecret: "harbor-db-app"
  existingSecretKey: "password"

# 3. Traefik ingress with auto-generated TLS
ingress:
  enabled: true
  className: "traefik"
  autoGenCert: true
  annotations:
    traefik.ingress.kubernetes.io/router.tls: "true"
```

## Deploy Harbor

Install or upgrade Harbor in the default namespace using the values file
prepared above. `database.host` is already in the file, so we only set
the deploy-time values on the command line.

> **⚠️ Security:** Change `harborAdminPassword` below — `Harbor12345` is
> the publicly-documented default and must not be used outside throwaway
> environments. Better: create a Secret and reference it via
> `existingSecretAdminPassword` instead of `--set`.

```bash
helm upgrade --install test-1 . \
  -f values-k3s.yaml \
  --set externalURL=https://harbor.localhost \
  --set harborAdminPassword='change-me-strong-password'
```
