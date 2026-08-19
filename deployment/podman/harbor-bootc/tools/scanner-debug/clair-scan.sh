#!/usr/bin/env bash
set -Eeuo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
WORK_DIR="${WORK_DIR:-${REPO_ROOT}/temp/clair-bluefin}"

export TMPDIR="${WORK_DIR}/tmp"
export XDG_CACHE_HOME="${WORK_DIR}/xdg-cache"

HARBOR_URL="${HARBOR_URL:-http://100.100.156.26:18085}"
HARBOR_USER="${HARBOR_USER:-admin}"
HARBOR_PASSWORD="${HARBOR_PASSWORD:-Harbor12345}"
HARBOR_PROJECT="${HARBOR_PROJECT:-bluefin}"
HARBOR_REPOSITORY="${HARBOR_REPOSITORY:-bluefin-bootc}"
HARBOR_TAG="${HARBOR_TAG:-latest-20260708}"
IMAGE_REF="${IMAGE_REF:-100.100.156.26:18085/bluefin/bluefin-bootc:latest-20260708}"
UPSTREAM_IMAGE_REF="${UPSTREAM_IMAGE_REF:-ghcr.io/ublue-os/bluefin:latest-20260708}"
AUTH_FILE="${AUTH_FILE:-${REPO_ROOT}/temp/podman-auth/test-images-auth.json}"
AUTH_COPY="${AUTH_COPY:-${WORK_DIR}/auth.json}"

CLAIR_IMAGE="${CLAIR_IMAGE:-quay.io/projectquay/clair:4.9.0}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-docker.io/library/postgres:16-alpine}"
CLAIR_PORT="${CLAIR_PORT:-19060}"
CLAIR_INTROSPECTION_PORT="${CLAIR_INTROSPECTION_PORT:-19089}"
CLAIR_UPDATE_WAIT_SECONDS="${CLAIR_UPDATE_WAIT_SECONDS:-180}"

NETWORK_NAME="${NETWORK_NAME:-clair-bluefin-net}"
DB_CONTAINER="${DB_CONTAINER:-clair-bluefin-db}"
CLAIR_CONTAINER="${CLAIR_CONTAINER:-clair-bluefin}"
PODMAN_ROOT="${PODMAN_ROOT:-${WORK_DIR}/podman-root}"
PODMAN_RUNROOT="${PODMAN_RUNROOT:-${WORK_DIR}/podman-runroot}"

CONFIG_FILE="${WORK_DIR}/config.yaml"
REGISTRIES_CONF="${WORK_DIR}/registries.conf"
SBOM_FILE="${WORK_DIR}/harbor-sbom.spdx.json"
SBOM_ATTEMPT_LOG="${WORK_DIR}/clair-sbom-attempt.log"
IMAGE_REPORT_JSON="${WORK_DIR}/clair-image-report.json"
IMAGE_REPORT_TEXT="${WORK_DIR}/clair-image-report.txt"
IMAGE_REPORT_REF_FILE="${WORK_DIR}/clair-image-report.ref"

log() {
  printf '[clair-bluefin] %s\n' "$*"
}

die() {
  printf '[clair-bluefin] ERROR: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

podman_() {
  podman --root "${PODMAN_ROOT}" --runroot "${PODMAN_RUNROOT}" "$@"
}

prepare_dirs() {
  mkdir -p \
    "${WORK_DIR}" \
    "${TMPDIR}" \
    "${XDG_CACHE_HOME}" \
    "${PODMAN_ROOT}" \
    "${PODMAN_RUNROOT}" \
    "${WORK_DIR}/postgres"
}

prepare_auth_file() {
  if [[ ! -f "${AUTH_FILE}" ]]; then
    log "Auth file not found at ${AUTH_FILE}; relying on anonymous registry access"
    return
  fi

  cp "${AUTH_FILE}" "${AUTH_COPY}"
  chmod 0644 "${AUTH_COPY}"
}

check_deps() {
  need curl
  need jq
  need podman
}

write_config() {
  cat >"${CONFIG_FILE}" <<'YAML'
---
log_level: info
introspection_addr: ":8089"
http_listen_addr: ":6060"
updaters:
  sets:
    - ubuntu
    - debian
    - rhel-vex
    - alpine
    - osv
auth:
  psk:
    key: "c2VjcmV0"
    iss:
      - clairctl
indexer:
  connstring: host=clair-bluefin-db port=5432 user=clair password=clair dbname=clair sslmode=disable
  scanlock_retry: 10
  layer_scan_concurrency: 1
  migrations: true
matcher:
  indexer_addr: http://127.0.0.1:6060/
  connstring: host=clair-bluefin-db port=5432 user=clair password=clair dbname=clair sslmode=disable
  migrations: true
matchers: {}
notifier:
  indexer_addr: http://127.0.0.1:6060/
  matcher_addr: http://127.0.0.1:6060/
  connstring: host=clair-bluefin-db port=5432 user=clair password=clair dbname=clair sslmode=disable
  migrations: true
  delivery_interval: 1m
  poll_interval: 5m
YAML
}

write_registries_conf() {
  cat >"${REGISTRIES_CONF}" <<'CONF'
unqualified-search-registries = ["docker.io"]

[[registry]]
location = "100.100.156.26:18085"
insecure = true

[[registry]]
location = "hetzner-bootc.tail6c5ea9.ts.net:18085"
insecure = true
CONF
}

fetch_harbor_sbom() {
  log "Fetching Harbor SBOM for ${HARBOR_PROJECT}/${HARBOR_REPOSITORY}:${HARBOR_TAG}"

  local artifact_json sbom_digest
  artifact_json="${WORK_DIR}/artifact-with-sbom.json"

  curl -fsS -u "${HARBOR_USER}:${HARBOR_PASSWORD}" \
    "${HARBOR_URL}/api/v2.0/projects/${HARBOR_PROJECT}/repositories/${HARBOR_REPOSITORY}/artifacts/${HARBOR_TAG}?with_sbom_overview=true" \
    >"${artifact_json}"

  sbom_digest="$(jq -r '.sbom_overview.sbom_digest // empty' "${artifact_json}")"
  [[ -n "${sbom_digest}" ]] || die "Harbor did not return .sbom_overview.sbom_digest for ${HARBOR_REPOSITORY}:${HARBOR_TAG}"

  curl -fsS -u "${HARBOR_USER}:${HARBOR_PASSWORD}" -H 'Accept: application/json' \
    "${HARBOR_URL}/api/v2.0/projects/${HARBOR_PROJECT}/repositories/${HARBOR_REPOSITORY}/artifacts/${sbom_digest}/additions/sbom" \
    >"${SBOM_FILE}"

  log "SBOM saved: ${SBOM_FILE}"
  jq -r '"SBOM format: \(.spdxVersion // .bomFormat // "unknown"), packages: \((.packages // .components // []) | length)"' "${SBOM_FILE}"
}

ensure_network() {
  if ! podman_ network exists "${NETWORK_NAME}" >/dev/null 2>&1; then
    podman_ network create "${NETWORK_NAME}" >/dev/null
  fi
}

start_postgres() {
  if podman_ container exists "${DB_CONTAINER}" >/dev/null 2>&1; then
    log "Postgres container already exists: ${DB_CONTAINER}"
    podman_ start "${DB_CONTAINER}" >/dev/null
    return
  fi

  log "Starting Postgres"
  podman_ run -d \
    --name "${DB_CONTAINER}" \
    --network "${NETWORK_NAME}" \
    -e POSTGRES_USER=clair \
    -e POSTGRES_PASSWORD=clair \
    -e POSTGRES_DB=clair \
    -v "${WORK_DIR}/postgres:/var/lib/postgresql/data:Z" \
    "${POSTGRES_IMAGE}" >/dev/null
}

wait_for_postgres() {
  log "Waiting for Postgres"
  for _ in $(seq 1 60); do
    if podman_ exec "${DB_CONTAINER}" pg_isready -U clair -d clair >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  podman_ logs "${DB_CONTAINER}" >&2 || true
  die "Postgres did not become ready"
}

start_clair() {
  if podman_ container exists "${CLAIR_CONTAINER}" >/dev/null 2>&1; then
    log "Clair container already exists: ${CLAIR_CONTAINER}"
    podman_ start "${CLAIR_CONTAINER}" >/dev/null
    return
  fi

  log "Starting Clair ${CLAIR_IMAGE}"
  local args=(
    run -d
    --name "${CLAIR_CONTAINER}"
    --network "${NETWORK_NAME}"
    -p "127.0.0.1:${CLAIR_PORT}:6060"
    -p "127.0.0.1:${CLAIR_INTROSPECTION_PORT}:8089"
    -e CLAIR_MODE=combo
    -e CLAIR_CONF=/config/config.yaml
    -v "${CONFIG_FILE}:/config/config.yaml:ro,Z"
    -v "${REGISTRIES_CONF}:/etc/containers/registries.conf:ro,Z"
  )

  if [[ -f "${AUTH_COPY}" ]]; then
    args+=(
      -e REGISTRY_AUTH_FILE=/auth/auth.json
      -v "${AUTH_COPY}:/auth/auth.json:ro,Z"
      -v "${AUTH_COPY}:/root/.docker/config.json:ro,Z"
    )
  fi

  args+=("${CLAIR_IMAGE}")
  podman_ "${args[@]}" >/dev/null
}

wait_for_clair() {
  log "Waiting for Clair API"
  for _ in $(seq 1 90); do
    if curl -fsS "http://127.0.0.1:${CLAIR_INTROSPECTION_PORT}/healthz" >/dev/null 2>&1; then
      log "Clair health endpoint is up"
      return
    fi
    if curl -fsS "http://127.0.0.1:${CLAIR_PORT}/" >/dev/null 2>&1; then
      log "Clair API port is answering"
      return
    fi
    sleep 2
  done
  podman_ logs "${CLAIR_CONTAINER}" >&2 || true
  die "Clair did not become ready"
}

wait_for_updaters() {
  if [[ "${CLAIR_UPDATE_WAIT_SECONDS}" == "0" ]]; then
    log "Skipping updater wait because CLAIR_UPDATE_WAIT_SECONDS=0"
    return
  fi

  log "Waiting ${CLAIR_UPDATE_WAIT_SECONDS}s for Clair updater data. Set CLAIR_UPDATE_WAIT_SECONDS=0 to skip."
  sleep "${CLAIR_UPDATE_WAIT_SECONDS}"
}

clairctl_container_args() {
  local args=(
    run --rm
    --network "${NETWORK_NAME}"
    -v "${CONFIG_FILE}:/config/config.yaml:ro,Z"
    -v "${REGISTRIES_CONF}:/etc/containers/registries.conf:ro,Z"
    -v "${WORK_DIR}:/work:Z"
  )

  if [[ -f "${AUTH_COPY}" ]]; then
    args+=(
      -e REGISTRY_AUTH_FILE=/auth/auth.json
      -v "${AUTH_COPY}:/auth/auth.json:ro,Z"
      -v "${AUTH_COPY}:/root/.docker/config.json:ro,Z"
    )
  else
    log "Auth file not found at ${AUTH_COPY}; relying on anonymous registry access"
  fi

  args+=(--entrypoint clairctl "${CLAIR_IMAGE}")
  printf '%s\n' "${args[@]}"
}

run_clairctl() {
  mapfile -t container_args < <(clairctl_container_args)
  podman_ "${container_args[@]}" -c /config/config.yaml "$@"
}

scan_sbom_best_effort() {
  log "Trying Clair against SBOM inputs"
  : >"${SBOM_ATTEMPT_LOG}"

  {
    echo "Clair/clairctl does not document SPDX or CycloneDX SBOM ingestion."
    echo "This script tries the obvious inputs anyway so the behavior is captured."
    echo
  } >>"${SBOM_ATTEMPT_LOG}"

  local target status
  for target in "sbom:/work/$(basename "${SBOM_FILE}")" "/work/$(basename "${SBOM_FILE}")"; do
    echo "## clairctl report target: ${target}" >>"${SBOM_ATTEMPT_LOG}"
    set +e
    run_clairctl report --host "http://${CLAIR_CONTAINER}:6060" --out json "${target}" >>"${SBOM_ATTEMPT_LOG}" 2>&1
    status=$?
    set -e
    echo "exit_status=${status}" >>"${SBOM_ATTEMPT_LOG}"
    echo >>"${SBOM_ATTEMPT_LOG}"
  done

  log "SBOM attempt log saved: ${SBOM_ATTEMPT_LOG}"
}

run_clair_image_reports() {
  local image_ref="$1"
  printf '%s\n' "${image_ref}" >"${IMAGE_REPORT_REF_FILE}"

  set +e
  run_clairctl report --host "http://${CLAIR_CONTAINER}:6060" --out json "${image_ref}" >"${IMAGE_REPORT_JSON}" 2>"${WORK_DIR}/clair-image-report.stderr"
  local json_status=$?
  set -e

  if [[ "${json_status}" -ne 0 ]]; then
    log "JSON report failed with exit ${json_status}; stderr saved: ${WORK_DIR}/clair-image-report.stderr"
  else
    log "JSON report saved: ${IMAGE_REPORT_JSON}"
  fi

  set +e
  run_clairctl report --host "http://${CLAIR_CONTAINER}:6060" --out text "${image_ref}" >"${IMAGE_REPORT_TEXT}" 2>"${WORK_DIR}/clair-image-report-text.stderr"
  local text_status=$?
  set -e

  if [[ "${text_status}" -ne 0 ]]; then
    log "Text report failed with exit ${text_status}; stderr saved: ${WORK_DIR}/clair-image-report-text.stderr"
  else
    log "Text report saved: ${IMAGE_REPORT_TEXT}"
  fi

  if [[ "${json_status}" -ne 0 && "${text_status}" -ne 0 ]]; then
    return 1
  fi
}

scan_image_direct() {
  log "Running Clair direct image scan: ${IMAGE_REF}"

  if run_clair_image_reports "${IMAGE_REF}"; then
    return
  fi

  if grep -q 'HTTP response to HTTPS client' "${WORK_DIR}/clair-image-report.stderr" "${WORK_DIR}/clair-image-report-text.stderr" 2>/dev/null &&
    [[ -n "${UPSTREAM_IMAGE_REF}" && "${UPSTREAM_IMAGE_REF}" != "${IMAGE_REF}" ]]; then
    mv "${WORK_DIR}/clair-image-report.stderr" "${WORK_DIR}/clair-image-report.harbor-http.stderr" 2>/dev/null || true
    mv "${WORK_DIR}/clair-image-report-text.stderr" "${WORK_DIR}/clair-image-report-text.harbor-http.stderr" 2>/dev/null || true
    log "Harbor image ref is plain HTTP and clairctl requires HTTPS. Retrying upstream image: ${UPSTREAM_IMAGE_REF}"
    run_clair_image_reports "${UPSTREAM_IMAGE_REF}"
    return
  fi

  return 1
}

summarize_outputs() {
  log "Outputs:"
  printf '  SBOM:              %s\n' "${SBOM_FILE}"
  printf '  SBOM attempt log:  %s\n' "${SBOM_ATTEMPT_LOG}"
  printf '  Image report JSON: %s\n' "${IMAGE_REPORT_JSON}"
  printf '  Image report text: %s\n' "${IMAGE_REPORT_TEXT}"
  if [[ -s "${IMAGE_REPORT_REF_FILE}" ]]; then
    printf '  Image report ref:  %s\n' "$(cat "${IMAGE_REPORT_REF_FILE}")"
  fi

  if [[ -s "${IMAGE_REPORT_JSON}" ]]; then
    jq -r '"  Clair summary: packages=\(.packages | length), vulnerabilities=\(.vulnerabilities | length), package_vulnerabilities=\(.package_vulnerabilities | length), distributions=\(.distributions | length)"' "${IMAGE_REPORT_JSON}" || true
  fi

  if [[ -s "${IMAGE_REPORT_TEXT}" ]]; then
    log "First report lines:"
    sed -n '1,40p' "${IMAGE_REPORT_TEXT}"
  fi
}

start_stack() {
  prepare_dirs
  check_deps
  prepare_auth_file
  write_config
  write_registries_conf
  ensure_network
  start_postgres
  wait_for_postgres
  start_clair
  wait_for_clair
}

stop_stack() {
  podman_ rm -f "${CLAIR_CONTAINER}" "${DB_CONTAINER}" >/dev/null 2>&1 || true
}

clean_all() {
  stop_stack
  podman_ network rm "${NETWORK_NAME}" >/dev/null 2>&1 || true
  rm -rf "${WORK_DIR}" 2>/dev/null || podman_ unshare rm -rf "${WORK_DIR}"
}

run_all() {
  start_stack
  fetch_harbor_sbom
  wait_for_updaters
  scan_sbom_best_effort
  scan_image_direct
  summarize_outputs
}

case "${1:-all}" in
  all)
    run_all
    ;;
  start)
    start_stack
    ;;
  sbom)
    start_stack
    fetch_harbor_sbom
    wait_for_updaters
    scan_sbom_best_effort
    ;;
  image)
    start_stack
    scan_image_direct
    summarize_outputs
    ;;
  stop)
    stop_stack
    ;;
  clean)
    clean_all
    ;;
  *)
    cat <<USAGE
Usage:
  $0 [all|start|sbom|image|stop|clean]

Default:
  $0 all

Useful overrides:
  IMAGE_REF=100.100.156.26:18085/bluefin/bluefin-bootc:latest-20260704
  HARBOR_URL=http://100.100.156.26:18085
  HARBOR_USER=admin
  HARBOR_PASSWORD=Harbor12345
  CLAIR_UPDATE_WAIT_SECONDS=600
  CLAIR_IMAGE=quay.io/projectquay/clair:4.9.0
USAGE
    exit 2
    ;;
esac
