#!/usr/bin/env bash
# verify.sh — Verify Harbor Next deployment on AWS EKS with IRSA
#
# Tests: port-forward, docker login, push, pull, S3 storage, IAM auth logs.
#
# Usage: ./verify.sh

set -euo pipefail

AWS_REGION="${AWS_REGION:-eu-central-1}"
CLUSTER_NAME="${CLUSTER_NAME:-harbor-next-irsa}"
NAMESPACE="${NAMESPACE:-harbor}"
RELEASE_NAME="${RELEASE_NAME:-harbor-next}"
LOCAL_PORT="${LOCAL_PORT:-8443}"

KUBECONFIG_PATH="${KUBECONFIG_PATH:-${HOME}/.kube/${CLUSTER_NAME}.yaml}"
export KUBECONFIG="${KUBECONFIG_PATH}"

AWS_ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
BUCKET_NAME="${BUCKET_NAME:-harbor-next-irsa-${AWS_ACCOUNT_ID}}"

PASS=0
FAIL=0

check() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "  PASS: ${desc}"
    ((PASS++))
  else
    echo "  FAIL: ${desc}"
    ((FAIL++))
  fi
}

echo "=== Harbor Next IRSA Verification ==="
echo ""

# --- Pods ---
echo "--- Pod status ---"
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""

check "All pods Running" kubectl wait -n "${NAMESPACE}" \
  --for=condition=ready pod \
  -l "app.kubernetes.io/instance=${RELEASE_NAME}" \
  --timeout=60s

# --- Port-forward ---
echo ""
echo "--- Port-forward ---"
# Kill any existing port-forward
pkill -f "port-forward.*${LOCAL_PORT}" 2>/dev/null || true
sleep 1
kubectl port-forward -n "${NAMESPACE}" "svc/${RELEASE_NAME}-core" "${LOCAL_PORT}:443" &
PF_PID=$!
sleep 3

check "Port-forward active" kill -0 "${PF_PID}"

# --- Docker login ---
echo ""
echo "--- Docker login ---"
check "Docker login" docker login "localhost:${LOCAL_PORT}" -u admin -p Harbor12345

# --- Push ---
echo ""
echo "--- Push image ---"
docker pull alpine:latest 2>/dev/null || true
docker tag alpine:latest "localhost:${LOCAL_PORT}/library/alpine:irsa-test"
check "Docker push" docker push "localhost:${LOCAL_PORT}/library/alpine:irsa-test"

# --- Pull ---
echo ""
echo "--- Pull image ---"
docker rmi "localhost:${LOCAL_PORT}/library/alpine:irsa-test" 2>/dev/null || true
check "Docker pull" docker pull "localhost:${LOCAL_PORT}/library/alpine:irsa-test"

# --- S3 verification ---
echo ""
echo "--- S3 storage ---"
S3_OBJECTS="$(aws s3 ls "s3://${BUCKET_NAME}/" --recursive 2>/dev/null | head -5)"
if [ -n "${S3_OBJECTS}" ]; then
  echo "  PASS: S3 bucket contains registry data"
  echo "${S3_OBJECTS}" | sed 's/^/    /'
  ((PASS++))
else
  echo "  FAIL: S3 bucket is empty"
  ((FAIL++))
fi

# --- IAM auth logs ---
echo ""
echo "--- IAM auth (core logs) ---"
CORE_LOGS="$(kubectl logs -n "${NAMESPACE}" -l "app.kubernetes.io/component=core" --tail=50 2>/dev/null)"
if echo "${CORE_LOGS}" | grep -qi "iam\|IAMAuth"; then
  echo "  PASS: Core logs mention IAM auth"
  ((PASS++))
else
  echo "  WARN: No IAM auth mention in recent core logs (may need to check earlier logs)"
fi

# --- No static AWS creds ---
echo ""
echo "--- Credential check ---"
SECRETS_JSON="$(kubectl get secret -n "${NAMESPACE}" -o json 2>/dev/null)"
if echo "${SECRETS_JSON}" | grep -q "REGISTRY_STORAGE_S3_ACCESSKEY"; then
  echo "  FAIL: Static S3 credentials found in secrets"
  ((FAIL++))
else
  echo "  PASS: No static S3 credentials in secrets"
  ((PASS++))
fi

# --- Cleanup port-forward ---
kill "${PF_PID}" 2>/dev/null || true

# --- Summary ---
echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
if [ "${FAIL}" -gt 0 ]; then
  exit 1
fi
