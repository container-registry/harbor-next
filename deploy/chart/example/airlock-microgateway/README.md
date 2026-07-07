# Harbor Next behind Airlock Microgateway 5.1

Front Harbor with [Airlock Microgateway](https://github.com/airlock/microgateway)
5.1 using the Kubernetes **Gateway API**. Airlock 5.x dropped the sidecar model
(removed in 5.0) and is now a Gateway API implementation: it terminates TLS,
routes, and — with a Premium license — applies WAAP protection.

This example runs on the free **Community Edition** (no license). In CE the Microgateway
is a plain Gateway API reverse proxy in front of Harbor and handles image push/pull,
including large layers, unchanged. See [`waf-premium.yaml`](waf-premium.yaml) for the
optional Premium WAF add-on.

Files:
- [`values.yaml`](values.yaml) — Harbor chart values (ingress off, `gateway.enabled`,
  TLS off internally). The chart emits the HTTPRoute.
- [`airlock-gateway.yaml`](airlock-gateway.yaml) — `GatewayParameters` + `Gateway`
  (HTTPS listener, TLS terminate).
- [`postgres.yaml`](postgres.yaml) — throwaway PostgreSQL.
- [`waf-premium.yaml`](waf-premium.yaml) — optional, Premium-only WAF + `Limits`.

## Prerequisites

Kubernetes >= 1.30, `helm` >= 3.8, and a cluster where you can reach a
`LoadBalancer` service (k3d/kind/minikube all work — use `kubectl port-forward` if
your loadbalancer IP is not routable).

### 1. Gateway API CRDs + Airlock Microgateway operator

```bash
kubectl apply --server-side -f \
  https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.6.0/standard-install.yaml

kubectl create namespace airlock-microgateway-system
helm install airlock-microgateway \
  oci://quay.io/airlockcharts/microgateway --version '5.1.1' \
  -n airlock-microgateway-system --wait

# GatewayClass 'airlock-microgateway' should report ACCEPTED=True
kubectl get gatewayclass
```

## Deploy Harbor

```bash
kubectl create namespace harbor

# TLS cert for the Gateway listener (self-signed for the example)
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout tls.key -out tls.crt \
  -subj "/CN=harbor.mgw.local" -addext "subjectAltName=DNS:harbor.mgw.local"
kubectl -n harbor create secret tls harbor-mgw-tls --cert=tls.crt --key=tls.key

# Database + Gateway
kubectl apply -f postgres.yaml
kubectl apply -f airlock-gateway.yaml

# Harbor (from the chart root: deploy/chart)
helm install harbor . -n harbor -f example/airlock-microgateway/values.yaml

# Wait for the operator to program the Gateway (PROGRAMMED=True) and provision Envoy
kubectl -n harbor get gateway harbor -w
```

The operator provisions an Envoy Deployment and a `LoadBalancer` Service named
`harbor` in the `harbor` namespace. The chart's HTTPRoute (`harbor-harbor-next`)
attaches to it and routes `/api/`, `/service/`, `/v2/`, `/c/`, `/chartrepo/` and `/`
to `harbor-core`.

## Test push / pull

Point `harbor.mgw.local` at the Gateway. On a k3d/kind cluster the simplest path is
`kubectl port-forward` plus a client that speaks HTTP itself (crane/oras) so the
Docker daemon's networking is out of the loop:

```bash
kubectl -n harbor port-forward svc/harbor 8443:443
echo "127.0.0.1 harbor.mgw.local" | sudo tee -a /etc/hosts

crane auth login harbor.mgw.local:8443 -u admin -p Harbor12345 --insecure
crane copy alpine:3.20 harbor.mgw.local:8443/library/alpine:3.20 --insecure
crane ls harbor.mgw.local:8443/library/alpine --insecure
```

`docker login/push` also works once `harbor.mgw.local:8443` is trusted (add the cert
to the daemon, or use an `insecure-registries` entry).

## Premium WAF (optional)

`kubectl apply -f waf-premium.yaml` — **requires a Premium license**. Without one the
`ContentSecurityPolicy` is rejected and the Gateway returns 500 for the route. The
manifest turns the WAF on for the Harbor route with uploads unrestricted, so image
pushes are unaffected.

On Community Edition, a portal-only `Content-Security-Policy` can instead be set on
the HTTPRoute itself via `gateway.routeOverrides.default.filters` (commented block in
[`values.yaml`](values.yaml)), which needs no license and leaves the registry API path
untouched.

## Notes

- `externalURL` must be the hostname clients use through the Gateway; it drives
  registry redirects and the token realm.
- The example registry and PostgreSQL use `emptyDir`. A rolling upgrade wipes them —
  use real storage (S3/PVC + managed DB) for anything persistent.
- Airlock emits rich Envoy access logs (ECS JSON) — `kubectl -n harbor logs deploy/harbor`.
