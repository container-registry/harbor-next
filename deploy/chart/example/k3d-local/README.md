# Local development on k3d

Minimal local setup: Traefik ingress (k3d default) with auto-generated
TLS, internal Valkey, Trivy and metrics off, reduced resource requests.
[`values.yaml`](values.yaml) carries the full configuration.

The chart needs an external PostgreSQL. For local development a throwaway
Deployment is enough — for a production-grade in-cluster database see the
CNPG approach in the [K3S guide](../../docs/guide/k3s.md).

## Deploy

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

# Then install Harbor (from the chart directory)
helm install harbor . -n harbor -f example/k3d-local/values.yaml
```

Harbor answers at `https://harbor.localhost` (self-signed certificate —
`tls.certSource: auto`).
