#!/usr/bin/env bash
# setup.sh — Provision AWS infrastructure for Harbor Next with IRSA
#
# Creates: EKS cluster, S3 bucket, IAM policy/role (IRSA), Aurora PostgreSQL,
#          IAM DB user, and deploys Harbor via Helm.
#
# Usage: ./setup.sh
#
# Prerequisites: aws, eksctl, helm, kubectl, jq

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Configuration ---
AWS_REGION="${AWS_REGION:-eu-central-1}"
CLUSTER_NAME="${CLUSTER_NAME:-harbor-next-irsa}"
NAMESPACE="${NAMESPACE:-harbor}"
RELEASE_NAME="${RELEASE_NAME:-harbor-next}"

DB_CLUSTER_ID="${DB_CLUSTER_ID:-harbor-next-aurora}"
DB_INSTANCE_ID="${DB_INSTANCE_ID:-harbor-next-aurora-1}"
DB_MASTER_USER="${DB_MASTER_USER:-postgres}"
DB_MASTER_PASSWORD="${DB_MASTER_PASSWORD:-$(openssl rand -base64 16)}"
DB_NAME="${DB_NAME:-registry}"
DB_IAM_USER="${DB_IAM_USER:-harbor_iam_user}"
DB_ENGINE_VERSION="${DB_ENGINE_VERSION:-16.6}"

IAM_POLICY_NAME="${IAM_POLICY_NAME:-harbor-next-irsa}"
IAM_ROLE_NAME="${IAM_ROLE_NAME:-harbor-next-irsa}"

CHART_VERSION="${CHART_VERSION:-3.0.0}"
CHART_REF="${CHART_REF:-oci://8gears.container-registry.com/harbor-next/chart/harbor}"

KUBECONFIG_PATH="${KUBECONFIG_PATH:-${HOME}/.kube/${CLUSTER_NAME}.yaml}"
export KUBECONFIG="${KUBECONFIG_PATH}"

# --- Preflight ---
echo "=== Preflight checks ==="
for cmd in aws eksctl helm kubectl jq; do
  command -v "$cmd" >/dev/null || { echo "ERROR: $cmd not found"; exit 1; }
done

AWS_ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
echo "AWS Account: ${AWS_ACCOUNT_ID}"
echo "Region:      ${AWS_REGION}"

BUCKET_NAME="${BUCKET_NAME:-harbor-next-irsa-${AWS_ACCOUNT_ID}}"

# --- Phase 1: EKS Cluster ---
echo ""
echo "=== Phase 1: EKS Cluster ==="
if eksctl get cluster --name "${CLUSTER_NAME}" --region "${AWS_REGION}" &>/dev/null; then
  echo "Cluster ${CLUSTER_NAME} already exists, skipping creation."
else
  echo "Creating EKS cluster ${CLUSTER_NAME}..."
  eksctl create cluster -f "${SCRIPT_DIR}/cluster.yaml"
fi
eksctl utils write-kubeconfig --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" --kubeconfig "${KUBECONFIG_PATH}"
echo "Kubeconfig: ${KUBECONFIG_PATH}"
kubectl get nodes

# --- Phase 2: S3 Bucket ---
echo ""
echo "=== Phase 2: S3 Bucket ==="
if aws s3api head-bucket --bucket "${BUCKET_NAME}" 2>/dev/null; then
  echo "Bucket ${BUCKET_NAME} already exists."
else
  echo "Creating S3 bucket ${BUCKET_NAME}..."
  aws s3 mb "s3://${BUCKET_NAME}" --region "${AWS_REGION}"
fi

# --- Phase 3: IAM Policy ---
echo ""
echo "=== Phase 3: IAM Policy ==="

# Get or create Aurora cluster to know the DB resource ID (needed for rds-db:connect ARN).
# We create the policy with a placeholder first and update after Aurora is ready.
POLICY_DOC=$(sed \
  -e "s|BUCKET_NAME|${BUCKET_NAME}|g" \
  -e "s|ACCOUNT_ID|${AWS_ACCOUNT_ID}|g" \
  -e "s|DB_RESOURCE_ID|*|g" \
  "${SCRIPT_DIR}/iam-policy.json")

POLICY_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${IAM_POLICY_NAME}"
if aws iam get-policy --policy-arn "${POLICY_ARN}" &>/dev/null; then
  echo "IAM policy ${IAM_POLICY_NAME} already exists."
else
  echo "Creating IAM policy ${IAM_POLICY_NAME}..."
  aws iam create-policy \
    --policy-name "${IAM_POLICY_NAME}" \
    --policy-document "${POLICY_DOC}" \
    --query 'Policy.Arn' --output text
fi

# --- Phase 4: IRSA Role ---
echo ""
echo "=== Phase 4: IRSA Role ==="
OIDC_ISSUER="$(aws eks describe-cluster \
  --name "${CLUSTER_NAME}" \
  --region "${AWS_REGION}" \
  --query 'cluster.identity.oidc.issuer' --output text)"
OIDC_PROVIDER="${OIDC_ISSUER#https://}"

TRUST_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringLike": {
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:${NAMESPACE}:*"
        }
      }
    }
  ]
}
EOF
)

ROLE_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${IAM_ROLE_NAME}"
if aws iam get-role --role-name "${IAM_ROLE_NAME}" &>/dev/null; then
  echo "IAM role ${IAM_ROLE_NAME} already exists."
else
  echo "Creating IAM role ${IAM_ROLE_NAME}..."
  aws iam create-role \
    --role-name "${IAM_ROLE_NAME}" \
    --assume-role-policy-document "${TRUST_POLICY}" \
    --query 'Role.Arn' --output text
  aws iam attach-role-policy \
    --role-name "${IAM_ROLE_NAME}" \
    --policy-arn "${POLICY_ARN}"
fi
echo "Role ARN: ${ROLE_ARN}"

# --- Phase 5: Aurora PostgreSQL Serverless v2 ---
echo ""
echo "=== Phase 5: Aurora PostgreSQL ==="

# Get VPC and subnets from EKS
VPC_ID="$(aws eks describe-cluster \
  --name "${CLUSTER_NAME}" --region "${AWS_REGION}" \
  --query 'cluster.resourcesVpcConfig.vpcId' --output text)"
VPC_CIDR="$(aws ec2 describe-vpcs --vpc-ids "${VPC_ID}" --region "${AWS_REGION}" \
  --query 'Vpcs[0].CidrBlock' --output text)"
SUBNET_IDS="$(aws ec2 describe-subnets --region "${AWS_REGION}" \
  --filters "Name=vpc-id,Values=${VPC_ID}" "Name=map-public-ip-on-launch,Values=false" \
  --query 'Subnets[*].SubnetId' --output text | tr '\t' ',')"
# Fallback: use all subnets if no private subnets found
if [ -z "${SUBNET_IDS}" ]; then
  SUBNET_IDS="$(aws ec2 describe-subnets --region "${AWS_REGION}" \
    --filters "Name=vpc-id,Values=${VPC_ID}" \
    --query 'Subnets[*].SubnetId' --output text | tr '\t' ',')"
fi

# DB subnet group
DB_SUBNET_GROUP="${CLUSTER_NAME}-db"
if aws rds describe-db-subnet-groups --db-subnet-group-name "${DB_SUBNET_GROUP}" --region "${AWS_REGION}" &>/dev/null; then
  echo "DB subnet group ${DB_SUBNET_GROUP} already exists."
else
  echo "Creating DB subnet group..."
  aws rds create-db-subnet-group \
    --db-subnet-group-name "${DB_SUBNET_GROUP}" \
    --db-subnet-group-description "Harbor Next Aurora" \
    --subnet-ids ${SUBNET_IDS//,/ } \
    --region "${AWS_REGION}"
fi

# Security group
SG_NAME="${CLUSTER_NAME}-aurora"
SG_ID="$(aws ec2 describe-security-groups --region "${AWS_REGION}" \
  --filters "Name=group-name,Values=${SG_NAME}" "Name=vpc-id,Values=${VPC_ID}" \
  --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null || true)"
if [ "${SG_ID}" = "None" ] || [ -z "${SG_ID}" ]; then
  echo "Creating security group ${SG_NAME}..."
  SG_ID="$(aws ec2 create-security-group --region "${AWS_REGION}" \
    --group-name "${SG_NAME}" \
    --description "Harbor Next Aurora access" \
    --vpc-id "${VPC_ID}" \
    --query 'GroupId' --output text)"
  aws ec2 authorize-security-group-ingress --region "${AWS_REGION}" \
    --group-id "${SG_ID}" \
    --protocol tcp --port 5432 \
    --cidr "${VPC_CIDR}"
else
  echo "Security group ${SG_NAME} already exists (${SG_ID})."
fi

# Aurora cluster
if aws rds describe-db-clusters --db-cluster-identifier "${DB_CLUSTER_ID}" --region "${AWS_REGION}" &>/dev/null; then
  echo "Aurora cluster ${DB_CLUSTER_ID} already exists."
else
  echo "Creating Aurora Serverless v2 cluster..."
  aws rds create-db-cluster \
    --db-cluster-identifier "${DB_CLUSTER_ID}" \
    --engine aurora-postgresql \
    --engine-version "${DB_ENGINE_VERSION}" \
    --master-username "${DB_MASTER_USER}" \
    --master-user-password "${DB_MASTER_PASSWORD}" \
    --database-name "${DB_NAME}" \
    --db-subnet-group-name "${DB_SUBNET_GROUP}" \
    --vpc-security-group-ids "${SG_ID}" \
    --serverless-v2-scaling-configuration MinCapacity=0.5,MaxCapacity=2 \
    --enable-iam-database-authentication \
    --region "${AWS_REGION}"
fi

# Aurora instance
if aws rds describe-db-instances --db-instance-identifier "${DB_INSTANCE_ID}" --region "${AWS_REGION}" &>/dev/null; then
  echo "Aurora instance ${DB_INSTANCE_ID} already exists."
else
  echo "Creating Aurora Serverless v2 instance..."
  aws rds create-db-instance \
    --db-instance-identifier "${DB_INSTANCE_ID}" \
    --db-cluster-identifier "${DB_CLUSTER_ID}" \
    --db-instance-class db.serverless \
    --engine aurora-postgresql \
    --region "${AWS_REGION}"
fi

echo "Waiting for Aurora cluster to become available..."
aws rds wait db-cluster-available \
  --db-cluster-identifier "${DB_CLUSTER_ID}" \
  --region "${AWS_REGION}"
echo "Waiting for Aurora instance to become available..."
aws rds wait db-instance-available \
  --db-instance-identifier "${DB_INSTANCE_ID}" \
  --region "${AWS_REGION}"

DB_ENDPOINT="$(aws rds describe-db-clusters \
  --db-cluster-identifier "${DB_CLUSTER_ID}" --region "${AWS_REGION}" \
  --query 'DBClusters[0].Endpoint' --output text)"
DB_RESOURCE_ID="$(aws rds describe-db-clusters \
  --db-cluster-identifier "${DB_CLUSTER_ID}" --region "${AWS_REGION}" \
  --query 'DBClusters[0].DbClusterResourceId' --output text)"
echo "Aurora endpoint: ${DB_ENDPOINT}"
echo "Aurora resource ID: ${DB_RESOURCE_ID}"

# Update IAM policy with actual DB resource ID
echo "Updating IAM policy with DB resource ID..."
POLICY_DOC_FINAL=$(sed \
  -e "s|BUCKET_NAME|${BUCKET_NAME}|g" \
  -e "s|ACCOUNT_ID|${AWS_ACCOUNT_ID}|g" \
  -e "s|DB_RESOURCE_ID|${DB_RESOURCE_ID}|g" \
  "${SCRIPT_DIR}/iam-policy.json")
aws iam create-policy-version \
  --policy-arn "${POLICY_ARN}" \
  --policy-document "${POLICY_DOC_FINAL}" \
  --set-as-default

# --- Phase 6: Create IAM DB user ---
echo ""
echo "=== Phase 6: IAM DB User ==="
echo "Creating IAM database user via temporary postgres pod..."

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

kubectl run pg-setup --rm -i --restart=Never \
  -n "${NAMESPACE}" \
  --image=postgres:16-alpine \
  --env="PGPASSWORD=${DB_MASTER_PASSWORD}" \
  -- psql -h "${DB_ENDPOINT}" -U "${DB_MASTER_USER}" -d "${DB_NAME}" <<SQL
CREATE USER ${DB_IAM_USER} WITH LOGIN;
GRANT rds_iam TO ${DB_IAM_USER};
GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_IAM_USER};
GRANT ALL ON SCHEMA public TO ${DB_IAM_USER};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${DB_IAM_USER};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${DB_IAM_USER};
SQL

echo "IAM DB user ${DB_IAM_USER} created."

# --- Phase 7: Deploy Harbor ---
echo ""
echo "=== Phase 7: Deploy Harbor ==="
helm install "${RELEASE_NAME}" "${CHART_REF}" \
  --version "${CHART_VERSION}" \
  -n "${NAMESPACE}" --create-namespace \
  -f "${SCRIPT_DIR}/values-aws-irsa.yaml" \
  --set "database.host=${DB_ENDPOINT}" \
  --set "registry.storage.s3.bucket=${BUCKET_NAME}" \
  --set "core.serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn=${ROLE_ARN}" \
  --set "jobservice.serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn=${ROLE_ARN}" \
  --set "registry.serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn=${ROLE_ARN}"

echo ""
echo "Waiting for pods to be ready..."
kubectl wait -n "${NAMESPACE}" --for=condition=ready pod -l app.kubernetes.io/instance="${RELEASE_NAME}" --timeout=300s

echo ""
echo "=== Setup complete ==="
echo ""
echo "Access Harbor:"
echo "  export KUBECONFIG=${KUBECONFIG_PATH}"
echo "  kubectl port-forward -n ${NAMESPACE} svc/${RELEASE_NAME}-core 8443:443 &"
echo "  docker login localhost:8443 -u admin -p Harbor12345"
echo ""
echo "Run verify.sh to test push/pull."
echo ""
echo "--- Saved values ---"
echo "KUBECONFIG=${KUBECONFIG_PATH}"
echo "DB_ENDPOINT=${DB_ENDPOINT}"
echo "DB_MASTER_PASSWORD=${DB_MASTER_PASSWORD}"
echo "BUCKET_NAME=${BUCKET_NAME}"
echo "ROLE_ARN=${ROLE_ARN}"
