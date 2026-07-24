#!/usr/bin/env bash
# Fast, repeatable Harbor deployment for the Issue #478 benchmark.
set -euo pipefail

command="${1:?usage: deploy-issue478.sh <up|apply|fresh|down|status>}"
cluster="${KIND_CLUSTER_NAME:-issue478}"
context="kind-$cluster"
namespace="${HARBOR_NAMESPACE:-issue478}"
release="${HARBOR_RELEASE:-harbor}"
chart_version="${HARBOR_CHART_VERSION:-1.19.1}"
adapter_version="${SCANNER_ADAPTER_VERSION:-v0.39.1}"
scanner_image="${SCANNER_IMAGE:-localhost/issue478/trivy-adapter:$adapter_version}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require() {
  command -v "$1" >/dev/null || {
    printf 'Required command not found: %s\n' "$1" >&2
    exit 1
  }
}

cluster_exists() {
  kind get clusters | grep -Fxq "$cluster"
}

wait_for_harbor() {
  for _ in $(seq 1 60); do
    health="$(curl --fail --silent --show-error http://localhost:18080/api/v2.0/health || true)"
    if jq --exit-status '.status == "healthy"' >/dev/null <<<"$health"; then
      return
    fi
    sleep 2
  done
  printf 'Harbor pods became Ready, but the API health check did not become healthy.\n' >&2
  printf '%s\n' "$health" >&2
  exit 1
}

deploy() {
  if ! cluster_exists; then
    kind create cluster --name "$cluster" --config "$root_dir/bench/kind-issue478.yaml"
  elif ! kubectl --context "$context" get nodes >/dev/null 2>&1; then
    printf 'Kind cluster %s exists but is unavailable. Run the explicit fresh task to recreate it.\n' "$cluster" >&2
    exit 1
  fi

  if ! podman image exists "$scanner_image"; then
    printf 'Required scanner image is missing: %s\n' "$scanner_image" >&2
    printf 'Build the pinned adapter first, or override SCANNER_IMAGE with an equivalent local image.\n' >&2
    exit 1
  fi

  # This streams the existing image into the Kind node; no registry push is
  # needed and the chart values keep imagePullPolicy at IfNotPresent.
  kind load docker-image --name "$cluster" "$scanner_image"

  if helm --kube-context "$context" --namespace "$namespace" status "$release" >/dev/null 2>&1; then
    printf 'Existing Harbor release found; preserving it. Use deploy:apply to force chart values.\n'
    wait_for_harbor
    return
  fi

  apply_chart
  wait_for_harbor
  printf 'Harbor benchmark is ready at http://localhost:18080 (cluster=%s, scanner=%s)\n' \
    "$cluster" "$adapter_version"
}

apply_chart() {
  helm repo add --force-update harbor https://helm.goharbor.io >/dev/null
  helm repo update harbor >/dev/null
  helm upgrade --install "$release" harbor/harbor \
    --kube-context "$context" \
    --namespace "$namespace" \
    --create-namespace \
    --version "$chart_version" \
    --values "$root_dir/bench/harbor-issue478-values.yaml" \
    --set "trivy.image.tag=$adapter_version" \
    --wait --timeout 10m
}

apply() {
  if ! cluster_exists; then
    printf 'Kind cluster %s does not exist. Run deploy or deploy:fresh first.\n' "$cluster" >&2
    exit 1
  fi
  if ! podman image exists "$scanner_image"; then
    printf 'Required scanner image is missing: %s\n' "$scanner_image" >&2
    exit 1
  fi
  kind load docker-image --name "$cluster" "$scanner_image"
  apply_chart
  wait_for_harbor
  printf 'Harbor chart values applied at http://localhost:18080 (scanner=%s)\n' "$adapter_version"
}

case "$command" in
  up)
    require kind
    require kubectl
    require helm
    require podman
    require curl
    require jq
    deploy
    ;;
  fresh)
    require kind
    if cluster_exists; then
      kind delete cluster --name "$cluster"
    fi
    require kubectl
    require helm
    require podman
    require curl
    require jq
    deploy
    ;;
  apply)
    require kind
    require kubectl
    require helm
    require podman
    require curl
    require jq
    apply
    ;;
  down)
    require kind
    if cluster_exists; then
      kind delete cluster --name "$cluster"
    fi
    ;;
  status)
    require kubectl
    kubectl --context "$context" -n "$namespace" get pods
    curl --fail --silent --show-error http://localhost:18080/api/v2.0/health
    ;;
  *)
    printf 'Unknown command: %s\n' "$command" >&2
    exit 1
    ;;
esac
