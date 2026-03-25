# Deploy Harbor on Nutanix Kubernetes Platform

## Deploy a Local CNPG System

Install CNPG:

```bash
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.28/releases/cnpg-1.28.1.yaml
```

A new PostgreSQL database will be deployed alongside Harbor in the next step.

## Deploy a Monitoring Stack

### Loki

Deploy Loki to collect and store logs:

```bash
helm install loki grafana/loki-stack \
    --namespace monitoring \
    --set grafana.enabled=false \
    --set promtail.enabled=true \
    --set loki.persistence.enabled=true \
    --set loki.persistence.size=10Gi
```

### Alloy

Create a file to override the default values (see [here](https://grafana.com/docs/alloy/latest/collect/logs-in-kubernetes/#pods-logs) for more):

```bash
cat > alloy-values.yaml <<EOF
alloy:
  configMap:
    content: |
      discovery.kubernetes "pods" {
        role = "pod"
      }

      discovery.relabel "filtered_pods" {
        targets = discovery.kubernetes.pods.targets

        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          regex         = "harbor|test-1-alex"
          action        = "keep"
        }

        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          target_label  = "namespace"
        }

        rule {
          source_labels = ["__meta_kubernetes_pod_name"]
          target_label  = "pod"
        }

        rule {
          source_labels = ["__meta_kubernetes_pod_container_name"]
          target_label  = "container"
        }

        rule {
          source_labels = ["__meta_kubernetes_pod_uid", "__meta_kubernetes_pod_container_name"]
          target_label  = "__path__"
          separator     = "/"
          replacement   = "/var/log/pods/*$1/*.log"
        }
      }

      loki.source.kubernetes "pods" {
        targets    = discovery.relabel.filtered_pods.output
        forward_to = [loki.write.default.receiver]
      }

      loki.write "default" {
        endpoint {
          url = "http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push"
        }
      }

  controller:
    type: "daemonset"

  alloy:
    mounts:
      varlog: true
      dockercontainers: true
EOF
```

Install Alloy with Helm:

```bash
helm install alloy grafana/alloy \
    --namespace monitoring \
    --create-namespace \
    -f alloy-values.yaml
```

## Install an Ingress Controller

### Nginx

Install an Nginx ingress controller with Helm:

```bash
helm install nginx-ingress ingress-nginx/ingress-nginx \
    --namespace ingress-nginx \
    --create-namespace \
    --set controller.service.type=LoadBalancer
```

### Traefik

Traefik is installed by default, so no additional steps are required if it is used.

## Deploy Harbor

### Retrieve the Harbor Helm Chart

Pull the Helm chart from the OCI repository. First, log in to the registry if authentication is required:

```bash
helm registry login 8gears.container-registry.com
```

Pull the chart using the OCI reference:

```bash
helm pull oci://8gears.container-registry.com/8gcr/charts/harbor-next
```

Decompress the downloaded chart:

```bash
tar xzvf harbor-*.tgz
```

### Edit Values

#### Monitoring

Enable monitoring with Prometheus by setting the `metrics` section in your values file:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    namespace: ""
    labels:
      prometheus.kommander.d2iq.io/select: "true"
    interval: 30s
    scrapeTimeout: 10s
    honorLabels: true
```

#### Ingress

Configure the Nginx ingress:

```yaml
ingress:
  autoGenCert: true
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "0"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-request-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-next-upstream-timeout: "30"
    nginx.ingress.kubernetes.io/proxy-next-upstream-tries: "5"
  core: "harbor1.example.com"
  hosts:
    - host: harbor1.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: harbor-tls
      hosts:
        - harbor1.example.com
```

Alternatively, if using the default Traefik ingress:

```yaml
ingress:
  autoGenCert: true
  enabled: true
  className: "traefik"
  annotations:
    traefik.ingress.kubernetes.io/router.tls: "true"
```

#### Database

Configure Harbor to use the external PostgreSQL database deployed with CNPG:

```yaml
database:
  host: "harbor-db-rw"
  port: 5432
  username: "harbor"
  password: ""
  database: registry
  sslmode: disable
  maxIdleConns: 100
  maxOpenConns: 900
  connMaxIdleTime: "0"
  connMaxLifetime: "0"
  existingSecret: "harbor-db-app"
  existingSecretKey: "password"
```

#### Add a Manifest to Deploy a PostgreSQL Database

Use `extraManifests` to deploy the database in the same namespace:

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

#### Other Settings

Set the replica count per component as required. For example:

```yaml
core:
  replicas: 3
registry:
  replicas: 3
portal:
  replicas: 2
```

> **Note:** `externalURL` is required. Set it to the URL users will access Harbor at (e.g., `externalURL: "https://harbor1.example.com"`).

### Apply Manifests

Deploy Harbor into a new `harbor` namespace:

```bash
helm upgrade \
  --namespace harbor \
  --create-namespace \
  --install test-1 .
```

# Debugging

## Check Loki works

```bash
curl -k -G -s "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={namespace="harbor"}' \
  --data-urlencode 'limit=100' \
  --data-urlencode "start=$(date -u -d '1 hour ago' +%s)000000000"
```

## Check Events

```bash
kubectl get events -n ingress-nginx --sort-by='.metadata.creationTimestamp'
```

## Ingress

If issues arise with the ingress controller, restart it with:

```bash
kubectl rollout restart \
  -n ingress-nginx deployment nginx-ingress-ingress-nginx-controller
```
