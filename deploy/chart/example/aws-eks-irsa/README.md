# AWS EKS with IRSA — Harbor Next Example

Deploy Harbor Next on AWS EKS using IAM Roles for Service Accounts (IRSA) for zero-static-credential access to S3 (registry storage) and Aurora PostgreSQL (RDS IAM Auth).

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  EKS Cluster (harbor-next-irsa)                         │
│                                                         │
│  ┌──────────┐  ┌────────────┐  ┌──────────┐             │
│  │   Core   │  │ Job Service│  │ Registry │             │
│  │          │  │            │  │          │             │
│  │ IAM Auth │  │  IAM Auth  │  │ S3 IRSA  │             │
│  └────┬─────┘  └─────┬──────┘  └────┬─────┘             │
│       │              │              │                   │
│  "IRSA projected token (AWS_WEB_IDENTITY_TOKEN_FILE)"   │
│       │              │              │                   │
└───────┼──────────────┼──────────────┼───────────────────┘
        │              │              │
   ┌────▼──────────────▼────┐    ┌────▼───────┐
   │  Aurora PostgreSQL     │    │  S3 Bucket │
   │  "(Serverless v2)"     │    │            │
   │  IAM DB Auth enabled   │    │  Registry  │
   └────────────────────────┘    │  blobs     │
                                 └────────────┘
```

**Key points:**
- No static AWS credentials stored in Kubernetes Secrets
- IRSA injects short-lived tokens via projected service account volumes
- RDS IAM Auth generates fresh DB tokens per connection (15-min expiry)
- S3 SDK picks up IRSA credentials via default credential chain

## Prerequisites

- AWS CLI v2, configured for account `163500494166` (or your target account)
- `eksctl` >= 0.170
- `helm` >= 3.14
- `kubectl`
- `docker` (for push/pull verification)
- Harbor Next images with RDS IAM Auth support (from `aws-iam-auth` branch)

### IAM Auth Commits

The RDS IAM Auth code is on the `aws-iam-auth` branch. Cherry-pick these commits onto your build branch before building images:

```bash
git cherry-pick 702be52951  # feat: add support for AWS IAM auth
git cherry-pick 31f5ead5f1  # fix: add the metadata entries for the new config keys
git cherry-pick fce902463f  # fix: JobService database authentication with IAM
```

Then build images:

```bash
task images PLATFORMS=linux/amd64
```

## Environment Variables

All scripts accept these overrides:

| Variable | Default | Description |
|----------|---------|-------------|
| `AWS_REGION` | `eu-central-1` | AWS region |
| `CLUSTER_NAME` | `harbor-next-irsa` | EKS cluster name |
| `NAMESPACE` | `harbor` | Kubernetes namespace |
| `RELEASE_NAME` | `harbor-next` | Helm release name |
| `BUCKET_NAME` | `harbor-next-irsa-<ACCOUNT_ID>` | S3 bucket name |
| `DB_CLUSTER_ID` | `harbor-next-aurora` | Aurora cluster identifier |
| `DB_MASTER_PASSWORD` | (random) | Aurora master password |
| `CHART_VERSION` | `3.0.0` | Helm chart version |
| `CHART_REF` | `oci://8gears.container-registry.com/8gcr/charts/harbor-next` | Chart OCI reference |
| `KUBECONFIG_PATH` | `~/.kube/<CLUSTER_NAME>.yaml` | Kubeconfig file path |

## Quick Start

```bash
# 1. Run setup (creates EKS, S3, Aurora, IAM, deploys Harbor)
./setup.sh

# 2. Verify
./verify.sh

# 3. Cleanup when done
./teardown.sh
```

Total setup time: ~20 minutes (EKS cluster creation dominates).

## Step-by-Step

### 1. Create EKS Cluster

```bash
eksctl create cluster -f cluster.yaml
```

Creates a single `t3.medium` node cluster with OIDC provider enabled (required for IRSA).

### 2. Create S3 Bucket

```bash
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
aws s3 mb "s3://harbor-next-irsa-${ACCOUNT_ID}" --region eu-central-1
```

### 3. Create IAM Policy

Edit `iam-policy.json` — replace `BUCKET_NAME`, `ACCOUNT_ID`, and `DB_RESOURCE_ID` with actual values. Then:

```bash
aws iam create-policy \
  --policy-name harbor-next-irsa \
  --policy-document file://iam-policy.json
```

### 4. Create IRSA Role

```bash
# Get OIDC provider
OIDC=$(aws eks describe-cluster --name harbor-next-irsa \
  --query 'cluster.identity.oidc.issuer' --output text)
OIDC_PROVIDER="${OIDC#https://}"

# Create role with trust policy scoped to harbor namespace SAs
aws iam create-role --role-name harbor-next-irsa \
  --assume-role-policy-document '...'  # see setup.sh for full trust policy

aws iam attach-role-policy --role-name harbor-next-irsa \
  --policy-arn "arn:aws:iam::${ACCOUNT_ID}:policy/harbor-next-irsa"
```

The trust policy uses `StringLike` with `system:serviceaccount:harbor:*` to allow all Harbor service accounts in the namespace to assume the role.

### 5. Create Aurora PostgreSQL

```bash
aws rds create-db-cluster \
  --db-cluster-identifier harbor-next-aurora \
  --engine aurora-postgresql --engine-version 16.6 \
  --serverless-v2-scaling-configuration MinCapacity=0.5,MaxCapacity=2 \
  --enable-iam-database-authentication \
  ...
```

### 6. Create IAM DB User

Connect to Aurora and run:

```sql
CREATE USER harbor_iam_user WITH LOGIN;
GRANT rds_iam TO harbor_iam_user;
GRANT ALL PRIVILEGES ON DATABASE registry TO harbor_iam_user;
GRANT ALL ON SCHEMA public TO harbor_iam_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO harbor_iam_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO harbor_iam_user;
```

### 7. Deploy Harbor

```bash
helm install harbor-next oci://8gears.container-registry.com/8gcr/charts/harbor-next \
  --version 3.0.0 -n harbor --create-namespace \
  -f values-aws-irsa.yaml \
  --set database.host=<AURORA_ENDPOINT> \
  --set registry.storage.s3.bucket=<BUCKET_NAME> \
  --set core.serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=<ROLE_ARN> \
  --set jobservice.serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=<ROLE_ARN> \
  --set registry.serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=<ROLE_ARN>
```

### 8. Verify

```bash
# Port-forward
kubectl port-forward -n harbor svc/harbor-next-core 8443:443 &

# Push/pull
docker login localhost:8443 -u admin -p Harbor12345
docker pull alpine:latest
docker tag alpine:latest localhost:8443/library/alpine:test
docker push localhost:8443/library/alpine:test

# Check S3
aws s3 ls "s3://${BUCKET_NAME}/" --recursive | head

# Check IAM auth in logs
kubectl logs -n harbor -l app.kubernetes.io/component=core --tail=20
```

## Troubleshooting

### Pods stuck in CrashLoopBackOff

Check logs:
```bash
kubectl logs -n harbor -l app.kubernetes.io/component=core --previous
```

Common causes:
- Aurora not reachable: check security group allows 5432 from EKS VPC CIDR
- IAM auth not enabled: verify `--enable-iam-database-authentication` on Aurora cluster
- Missing IAM grants: verify `GRANT rds_iam TO harbor_iam_user` was executed

### "ExpiredTokenException" in registry logs

IRSA token refresh issue. Verify:
```bash
kubectl describe sa -n harbor  # should show eks.amazonaws.com/role-arn annotation
kubectl exec -n harbor <registry-pod> -- env | grep AWS  # should show AWS_WEB_IDENTITY_TOKEN_FILE
```

### S3 "AccessDenied"

1. Check the IAM policy is attached to the role
2. Verify bucket name matches
3. Check IRSA is working: `kubectl exec <pod> -- aws sts get-caller-identity`

### "password authentication failed" for harbor_iam_user

The IAM auth code (`HARBOR_DATABASE_IAM_AUTH=true`) is required. Ensure images are built from the `aws-iam-auth` branch or have the cherry-picked commits.

### automountServiceAccountToken

IRSA requires the projected service account token volume. All Harbor components that need AWS credentials must have `automountServiceAccountToken: true` in their values. The chart defaults to `false`.

## Cleanup

```bash
./teardown.sh
```

This removes (in order): Helm release, Aurora instance + cluster, DB subnet group, security group, S3 bucket (including all objects), IAM role + policy, EKS cluster.

## Files

| File | Purpose |
|------|---------|
| `cluster.yaml` | eksctl ClusterConfig (EKS + OIDC) |
| `values-aws-irsa.yaml` | Helm values (S3, Aurora, IRSA, IAM Auth) |
| `iam-policy.json` | IAM policy template (S3 + rds-db:connect) |
| `setup.sh` | Full provisioning script |
| `teardown.sh` | Full cleanup script |
| `verify.sh` | Push/pull/S3 verification |
