#!/usr/bin/env bash
# Bringing the disposable Harbor stack up and down.
# shellcheck shell=bash

# Both docker compose and podman-compose take the project directory from the
# first -f file, so deploy/compose stays the anchor for the base file's
# relative mounts. podman-compose has no --project-directory to pass.
compose() { # compose ARGS...
  # shellcheck disable=SC2086
  $E2E_COMPOSE \
    -f "${E2E_REPO_ROOT}/deploy/compose/docker-compose.yaml" \
    -f "${E2E_REPO_ROOT}/tests/e2e/compose.e2e.yaml" \
    --env-file "${E2E_ENV_FILE}" \
    "$@"
}

write_env_file() { # write_env_file TAG
  cat > "$E2E_ENV_FILE" <<EOF
COMPOSE_PROJECT_NAME=${E2E_PROJECT}
IMAGE_REPO=${E2E_IMAGE_REPO}
HARBOR_TAG=${1}
EXT_ENDPOINT=http://127.0.0.1:${E2E_PORT}
HARBOR_ADMIN_PASSWORD=${E2E_ADMIN_PASSWORD}
CORE_SECRET=${E2E_CORE_SECRET}
JOBSERVICE_SECRET=${E2E_JOBSERVICE_SECRET}
DB_PASSWORD=${E2E_DB_PASSWORD}
LOG_LEVEL=${E2E_LOG_LEVEL}
PORT_HTTP=${E2E_PORT}
PORT_HTTPS=${E2E_PORT_HTTPS}
TLS_CONF=/dev/null
TLS_CERT=/dev/null
TLS_KEY=/dev/null
E2E_NETWORK=${E2E_NETWORK}
E2E_DESTREG_IMAGE=${E2E_DESTREG_IMAGE}
E2E_JOBSERVICE_MEMORY=${E2E_JOBSERVICE_MEMORY}
E2E_JOBSERVICE_GOMEMLIMIT=${E2E_JOBSERVICE_GOMEMLIMIT}
EOF
}

# --- the scan-scale lane ----------------------------------------------------
# A jobservice that was OOM-killed comes back with its queue intact and retries
# the same conversion, so the symptom a caller sees is "Scan All had errors" or
# a timeout. Reading the container state names the real failure instead.
# Resolve the container directly: podman-compose's `ps` takes no service argument, so
# `compose ps -q jobservice` is an argument error there rather than a container id. Both
# compose implementations name it <project><sep>jobservice<sep>1.
jobservice_cid() {
  $E2E_ENGINE ps --filter "name=^${E2E_PROJECT}[_-]jobservice" --format '{{.ID}}' 2>/dev/null | head -1
}

jobservice_oom_count() {
  local cid; cid=$(jobservice_cid)
  [ -n "$cid" ] || { echo 0; return; }
  $E2E_ENGINE inspect --format '{{if .State.OOMKilled}}1{{else}}0{{end}}' "$cid" 2>/dev/null || echo 0
}

jobservice_restarts() {
  local cid; cid=$(jobservice_cid)
  [ -n "$cid" ] || { echo 0; return; }
  $E2E_ENGINE inspect --format '{{.RestartCount}}' "$cid" 2>/dev/null || echo 0
}

# Peak RSS as the cgroup accounted it, which is the number the OOM killer acts
# on. memory.peak is cgroup v2 only; fall back to the engine's own reading.
jobservice_peak_bytes() {
  local cid path; cid=$(jobservice_cid)
  [ -n "$cid" ] || { echo 0; return; }
  path=$($E2E_ENGINE inspect --format '{{.State.CgroupPath}}' "$cid" 2>/dev/null || true)
  if [ -n "$path" ] && [ -r "/sys/fs/cgroup${path}/memory.peak" ]; then
    cat "/sys/fs/cgroup${path}/memory.peak"
    return
  fi
  echo 0
}

ensure_token_key() {
  local key="${E2E_REPO_ROOT}/deploy/compose/config/token_service_key.pem"
  [ -f "$key" ] && return 0
  # config/ is runtime state, so a clean checkout does not have it yet.
  mkdir -p "$(dirname "$key")"
  log "generating token signing key"
  openssl genpkey -algorithm RSA -outform PEM -pkeyopt rsa_keygen_bits:4096 2>/dev/null \
    | openssl rsa -traditional -out "$key" 2>/dev/null
  # The container runs as UID 10000, which never matches the host user.
  chmod 644 "$key"
}

port_is_free() {
  ! (exec 3<>"/dev/tcp/127.0.0.1/${E2E_PORT}") 2>/dev/null
}

preflight() {
  command -v curl >/dev/null || die "curl not found"
  command -v jq >/dev/null || die "jq not found — install jq (https://jqlang.github.io/jq/)"
  command -v openssl >/dev/null || die "openssl not found"
  port_is_free || die "port ${E2E_PORT} is already in use — set E2E_PORT to something else"

  pull_images "$FIRST_TAG"
  if [ -n "$E2E_UPGRADE_FROM" ]; then
    pull_images "$E2E_TAG"
    write_env_file "$FIRST_TAG"
  fi
  $E2E_ENGINE pull -q "$E2E_CRANE_IMAGE" >/dev/null 2>&1 \
    || die "could not pull ${E2E_CRANE_IMAGE}"
}

# Pulling up front, rather than letting `up` do it, keeps the private-registry
# failure legible instead of buried in a compose error.
pull_images() { # pull_images TAG
  log "pulling images (${E2E_IMAGE_REPO}*:${1})"
  write_env_file "$1"
  if compose pull >/dev/null 2>"${E2E_WORKDIR}/pull.err"; then
    return 0
  fi
  sed -n '1,20p' "${E2E_WORKDIR}/pull.err" >&2 || true
  die "could not pull the Harbor images ${E2E_IMAGE_REPO}*:${1}.

These are the private 8gcr builds and the harness deliberately does not fall
back to upstream goharbor images — the point is testing *our* build. Log in to
the registry and retry:

    podman login ${E2E_IMAGE_REPO%%/*}

(podman's default auth store has worked where docker's config did not.)"
}

stack_up() { # stack_up TAG [COMPOSE_ARGS...]
  local tag=$1; shift
  write_env_file "$tag"
  if compose up -d "$@" >>"${E2E_WORKDIR}/compose.log" 2>&1; then
    return 0
  fi
  tail -n 20 "${E2E_WORKDIR}/compose.log" >&2
  return 1
}

components_healthy() {
  local body
  body=$(curl -sS -m 5 -u "admin:${E2E_ADMIN_PASSWORD}" "${E2E_API}/health" 2>/dev/null) || return 1
  [ "$(printf '%s' "$body" | jq -r '.status // "unknown"')" = "healthy" ]
}

# A core that cannot start its migrations restarts forever, so poll for that
# too rather than burning the whole readiness timeout on a dead stack.
core_crash_looping() {
  [ "$(compose logs --no-color --tail 60 core 2>/dev/null | grep -c '\[FATAL\]')" -ge 5 ]
}

first_fatal() {
  compose logs --no-color --tail 200 core 2>/dev/null \
    | grep -m1 -E '\[FATAL\]|\[ERROR\]' | cut -c1-200
}

wait_healthy() { # wait_healthy TIMEOUT
  local timeout=${1:-240} start i=0
  start=$(now)
  while :; do
    components_healthy && return 0
    i=$((i + 1))
    if [ $((i % 5)) -eq 0 ] && core_crash_looping; then
      warn "core is crash-looping"
      return 1
    fi
    if [ $(( $(now) - start )) -ge "$timeout" ]; then
      warn "timed out after ${timeout}s waiting for the components to report healthy"
      return 1
    fi
    sleep 2
  done
}

unhealthy_components() {
  curl -sS -m 5 -u "admin:${E2E_ADMIN_PASSWORD}" "${E2E_API}/health" 2>/dev/null \
    | jq -r '[.components[]? | select(.status != "healthy") | .name] | join(",")' 2>/dev/null
}

schema_version() {
  compose exec -T postgresql psql -U postgres -d registry -tAc \
    'select version, dirty from schema_migrations;' 2>/dev/null \
    | tr -d ' \r' | tail -1
}

dump_logs() {
  local dest="${E2E_LOG_DIR}"
  mkdir -p "$dest"
  local svc
  for svc in core jobservice registry registryctl portal trivy-adapter exporter postgresql; do
    compose logs --no-color --tail 400 "$svc" > "${dest}/${svc}.log" 2>&1 || true
  done
  warn "component logs written to ${dest}"
}

stack_down() {
  [ "${E2E_KEEP}" = "1" ] && { warn "E2E_KEEP=1 — leaving the stack up at ${E2E_URL} (admin/${E2E_ADMIN_PASSWORD})"; return 0; }
  log "tearing down"
  compose down -v --remove-orphans >/dev/null 2>&1 || true
}
