# Examples

Each example lives in its own directory with a `values.yaml` (plus any
scenario-specific manifests, scripts, or docs). Only this README sits at
the top level.

## Directories

| Directory | Description |
|-----------|-------------|
| [`k3d-local/`](k3d-local/) | Local development with k3d cluster |
| [`rke2-rancher/`](rke2-rancher/) | RKE2/Rancher deployment |
| [`private-ca/`](private-ca/) | Private-CA / mTLS scenario: PG with verify-full + Redis over TLS + shared CA for S3/OIDC |
| [`openshift/`](openshift/) | OpenShift deployment with ttl.sh images and edge-terminated routes |
| [`aws-eks-irsa/`](aws-eks-irsa/) | AWS EKS with IRSA for S3 storage and RDS IAM Auth (Aurora PostgreSQL) |
| [`flux/`](flux/) | FluxCD GitOps setup: HelmRelease with drift detection + fully pinned secrets (`autoGenSecrets: false`) for deterministic rendering — works for Argo CD too |

Every `example/*/values*.yaml` is render-checked in CI
(`task helm:examples`) — new examples are picked up automatically.

## Usage

```bash
# Deploy with an example values file
helm install harbor . -n harbor --create-namespace -f example/k3d-local/values.yaml
```

## Prerequisites

Each example may have specific prerequisites. See the comments in each
values file (or the directory's README) for details.

### k3d-local

Requires a PostgreSQL database. Deploy one first:

```bash
kubectl create namespace harbor

kubectl apply -n harbor -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
stringData:
  POSTGRES_PASSWORD: harbordbpass
  POSTGRES_USER: postgres
  POSTGRES_DB: registry
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15
        ports:
        - containerPort: 5432
        envFrom:
        - secretRef:
            name: postgres-secret
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
EOF

# Wait for postgres to be ready
kubectl wait -n harbor --for=condition=ready pod -l app=postgres --timeout=120s

# Then install Harbor
helm install harbor . -n harbor -f example/k3d-local/values.yaml
```
