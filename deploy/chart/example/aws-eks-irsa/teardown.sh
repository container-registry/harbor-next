#!/usr/bin/env bash
# teardown.sh — Remove all AWS resources created by setup.sh
#
# Usage: ./teardown.sh
#
# Deletes: Helm release, Aurora cluster, S3 bucket, IAM role/policy, EKS cluster.
# Each step is idempotent — safe to re-run on partial cleanup.

set -euo pipefail

AWS_REGION="${AWS_REGION:-eu-central-1}"
CLUSTER_NAME="${CLUSTER_NAME:-harbor-next-irsa}"
NAMESPACE="${NAMESPACE:-harbor}"
RELEASE_NAME="${RELEASE_NAME:-harbor-next}"
DB_CLUSTER_ID="${DB_CLUSTER_ID:-harbor-next-aurora}"
DB_INSTANCE_ID="${DB_INSTANCE_ID:-harbor-next-aurora-1}"
IAM_POLICY_NAME="${IAM_POLICY_NAME:-harbor-next-irsa}"
IAM_ROLE_NAME="${IAM_ROLE_NAME:-harbor-next-irsa}"

KUBECONFIG_PATH="${KUBECONFIG_PATH:-${HOME}/.kube/${CLUSTER_NAME}.yaml}"
export KUBECONFIG="${KUBECONFIG_PATH}"

AWS_ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
BUCKET_NAME="${BUCKET_NAME:-harbor-next-irsa-${AWS_ACCOUNT_ID}}"
POLICY_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${IAM_POLICY_NAME}"
DB_SUBNET_GROUP="${CLUSTER_NAME}-db"
SG_NAME="${CLUSTER_NAME}-aurora"

echo "=== Teardown: Harbor Next AWS EKS IRSA ==="
echo "Account: ${AWS_ACCOUNT_ID}, Region: ${AWS_REGION}"
echo ""

# --- Helm release ---
echo "--- Helm release ---"
if helm status "${RELEASE_NAME}" -n "${NAMESPACE}" &>/dev/null; then
  helm uninstall "${RELEASE_NAME}" -n "${NAMESPACE}"
  echo "Helm release ${RELEASE_NAME} uninstalled."
else
  echo "Helm release ${RELEASE_NAME} not found, skipping."
fi

# --- Aurora instance ---
echo ""
echo "--- Aurora instance ---"
if aws rds describe-db-instances --db-instance-identifier "${DB_INSTANCE_ID}" --region "${AWS_REGION}" &>/dev/null; then
  aws rds delete-db-instance \
    --db-instance-identifier "${DB_INSTANCE_ID}" \
    --skip-final-snapshot --region "${AWS_REGION}" 2>/dev/null || true
  echo "Deleting Aurora instance ${DB_INSTANCE_ID}..."
  aws rds wait db-instance-deleted --db-instance-identifier "${DB_INSTANCE_ID}" --region "${AWS_REGION}" 2>/dev/null || true
else
  echo "Aurora instance ${DB_INSTANCE_ID} not found, skipping."
fi

# --- Aurora cluster ---
echo ""
echo "--- Aurora cluster ---"
if aws rds describe-db-clusters --db-cluster-identifier "${DB_CLUSTER_ID}" --region "${AWS_REGION}" &>/dev/null; then
  aws rds delete-db-cluster \
    --db-cluster-identifier "${DB_CLUSTER_ID}" \
    --skip-final-snapshot --region "${AWS_REGION}" 2>/dev/null || true
  echo "Deleting Aurora cluster ${DB_CLUSTER_ID}..."
  aws rds wait db-cluster-deleted --db-cluster-identifier "${DB_CLUSTER_ID}" --region "${AWS_REGION}" 2>/dev/null || true
else
  echo "Aurora cluster ${DB_CLUSTER_ID} not found, skipping."
fi

# --- DB subnet group ---
echo ""
echo "--- DB subnet group ---"
if aws rds describe-db-subnet-groups --db-subnet-group-name "${DB_SUBNET_GROUP}" --region "${AWS_REGION}" &>/dev/null; then
  aws rds delete-db-subnet-group --db-subnet-group-name "${DB_SUBNET_GROUP}" --region "${AWS_REGION}"
  echo "Deleted DB subnet group ${DB_SUBNET_GROUP}."
else
  echo "DB subnet group ${DB_SUBNET_GROUP} not found, skipping."
fi

# --- Security group ---
echo ""
echo "--- Security group ---"
VPC_ID="$(aws eks describe-cluster --name "${CLUSTER_NAME}" --region "${AWS_REGION}" \
  --query 'cluster.resourcesVpcConfig.vpcId' --output text 2>/dev/null || true)"
if [ -n "${VPC_ID}" ] && [ "${VPC_ID}" != "None" ]; then
  SG_ID="$(aws ec2 describe-security-groups --region "${AWS_REGION}" \
    --filters "Name=group-name,Values=${SG_NAME}" "Name=vpc-id,Values=${VPC_ID}" \
    --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null || true)"
  if [ -n "${SG_ID}" ] && [ "${SG_ID}" != "None" ]; then
    aws ec2 delete-security-group --group-id "${SG_ID}" --region "${AWS_REGION}"
    echo "Deleted security group ${SG_ID}."
  else
    echo "Security group ${SG_NAME} not found, skipping."
  fi
fi

# --- S3 bucket ---
echo ""
echo "--- S3 bucket ---"
if aws s3api head-bucket --bucket "${BUCKET_NAME}" 2>/dev/null; then
  aws s3 rb "s3://${BUCKET_NAME}" --force
  echo "Deleted S3 bucket ${BUCKET_NAME}."
else
  echo "S3 bucket ${BUCKET_NAME} not found, skipping."
fi

# --- IAM role ---
echo ""
echo "--- IAM role ---"
if aws iam get-role --role-name "${IAM_ROLE_NAME}" &>/dev/null; then
  # Detach all policies
  for arn in $(aws iam list-attached-role-policies --role-name "${IAM_ROLE_NAME}" \
    --query 'AttachedPolicies[*].PolicyArn' --output text); do
    aws iam detach-role-policy --role-name "${IAM_ROLE_NAME}" --policy-arn "${arn}"
  done
  aws iam delete-role --role-name "${IAM_ROLE_NAME}"
  echo "Deleted IAM role ${IAM_ROLE_NAME}."
else
  echo "IAM role ${IAM_ROLE_NAME} not found, skipping."
fi

# --- IAM policy ---
echo ""
echo "--- IAM policy ---"
if aws iam get-policy --policy-arn "${POLICY_ARN}" &>/dev/null; then
  # Delete non-default versions first
  for vid in $(aws iam list-policy-versions --policy-arn "${POLICY_ARN}" \
    --query 'Versions[?!IsDefaultVersion].VersionId' --output text); do
    aws iam delete-policy-version --policy-arn "${POLICY_ARN}" --version-id "${vid}"
  done
  aws iam delete-policy --policy-arn "${POLICY_ARN}"
  echo "Deleted IAM policy ${IAM_POLICY_NAME}."
else
  echo "IAM policy ${IAM_POLICY_NAME} not found, skipping."
fi

# --- EKS cluster ---
echo ""
echo "--- EKS cluster ---"
if eksctl get cluster --name "${CLUSTER_NAME}" --region "${AWS_REGION}" &>/dev/null; then
  echo "Deleting EKS cluster ${CLUSTER_NAME} (this takes ~10 minutes)..."
  eksctl delete cluster --name "${CLUSTER_NAME}" --region "${AWS_REGION}"
else
  echo "EKS cluster ${CLUSTER_NAME} not found, skipping."
fi

# --- Kubeconfig ---
echo ""
echo "--- Kubeconfig ---"
if [ -f "${KUBECONFIG_PATH}" ]; then
  rm "${KUBECONFIG_PATH}"
  echo "Deleted ${KUBECONFIG_PATH}."
else
  echo "${KUBECONFIG_PATH} not found, skipping."
fi

echo ""
echo "=== Teardown complete ==="
