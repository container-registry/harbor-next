# Create Deployment on K3S

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
   helm pull oci://8gears.container-registry.com/8gcr/chart/harbor-next
   ```

2. Decompress the downloaded chart:

   ```bash
   tar xzvf harbor-*.tgz
   ```

## Add Database Manifest

Use `extraManifests` to deploy the PostgreSQL database within the same namespace.

```yaml
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
```

## Configure Harbor Database Settings

Update the Harbor configuration to use the external database and the password stored in the secret.

```yaml
database:
  host: "harbor-db-rw"
  port: 5432
  username: "harbor"
  existingSecret: "harbor-db-app"
  existingSecretKey: "password"
```

## Configure Traefik Ingress

Enable and configure the ingress using Traefik.

```yaml
ingress:
  enabled: true
  className: "traefik"
  autoGenCert: true
  annotations:
    traefik.ingress.kubernetes.io/router.tls: "true"
```

## Deploy Harbor

Install or upgrade Harbor in the default namespace.

```bash
helm upgrade --install test-1 .
```