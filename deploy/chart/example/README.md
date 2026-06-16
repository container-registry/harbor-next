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

Platform walkthroughs (cluster prep, database setup, the reasoning behind
the values) live in [`docs/guide/`](../docs/guide/) — K3S, Rancher/RKE2,
OpenShift, and Nutanix NKP.

## Usage

```bash
# Deploy with an example values file
helm install harbor . -n harbor --create-namespace -f example/k3d-local/values.yaml
```

## Prerequisites

Each example has its own prerequisites — see the comments in its values
file or the README in its directory. All scenarios need an external
PostgreSQL (the chart ships none); `k3d-local/` and `openshift/` show two
ways to deploy one alongside Harbor.
