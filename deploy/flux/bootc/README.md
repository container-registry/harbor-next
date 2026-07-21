# bootc Harbor FluxCD Bundle

Continuous-deploy bundle for `https://bootc.8gears.container-registry.dev` on
the **hz-hopper** cluster. It runs beside the existing 8gcr deployment in the
isolated `8gcr-dev-bootc` namespace.

## Deployment flow

1. Flux image automation watches the mutable `latest` tag for every bootc
   component in `8gears.container-registry.com/8gcr-dev`.
2. Digest changes update annotations in `helmrelease.yaml` on the `bootc`
   branch.
3. `.github/workflows/bootc-rolling-flux.yml` publishes this directory to
   `oci://8gears.container-registry.com/ops/hz-hopper/bootc-rolling:latest`.
4. The root `harbor-bootc` Flux Kustomization on hz-hopper applies the bundle.
5. Helm rolls pods because the digest annotations changed, while containers
   continue to use the requested `latest` tags with `imagePullPolicy: Always`.

## Images

| Component | Repository |
|---|---|
| Core | `8gcr-dev/harbor-core-bootc:latest` |
| Jobservice | `8gcr-dev/harbor-jobservice-bootc:latest` |
| Registry | `8gcr-dev/harbor-registry-bootc:latest` |
| Registryctl | `8gcr-dev/harbor-registryctl-bootc:latest` |
| Portal | `8gcr-dev/harbor-portal-bootc:latest` |
| Exporter | `8gcr-dev/harbor-exporter-bootc:latest` |
| Trivy | `8gcr-dev/harbor-trivy-adapter-bootc:latest` |
| Grype | `8gcr-dev/harbor-grype-scanner-bootc:latest` |

Grype is deployed as a separate scanner adapter Service at
`http://harbor-bootc-grype:8080`. Register it in Harbor with
`use_internal_addr: true`; its metadata identifies it as a
`bootc-native-package-vulnerability-scanner`.

## Credentials

Plaintext credentials never live in Git. `harbor-bootc-pull` is stored as two
SOPS-encrypted Secrets:

- `flux-system/harbor-bootc-pull`: Flux image scans.
- `8gcr-dev-bootc/harbor-bootc-pull`: chart and workload image pulls.

The root OCIRepository intentionally uses the pre-existing
`flux-system/harbor-system-pull`, because the 8gcr-dev robot can read component
images and the chart but cannot read the `ops` project.

## One-time bootstrap

After the bootc workflow publishes its first bundle:

```sh
kubectl --kubeconfig ~/.kube/hz-hopper.kubeconfig.yaml \
  apply -f deploy/flux/bootc/bootstrap.yaml
```

ExternalDNS on hz-hopper manages `container-registry.dev` and creates the DNS
record from the Ingress. Cert-manager issues `harbor-tls` with the
`letsencrypt-prod` ClusterIssuer.

## Local validation

```sh
kustomize build deploy/flux/bootc
yq '.spec.values' deploy/flux/bootc/helmrelease.yaml >/tmp/bootc-values.yaml
helm template harbor-bootc deploy/chart \
  --namespace 8gcr-dev-bootc \
  -f /tmp/bootc-values.yaml
```
