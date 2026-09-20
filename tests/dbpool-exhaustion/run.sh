#!/usr/bin/env bash
# Drive Harbor's core middleware chain into DB connection exhaustion.
#
#   ./run.sh              # default pool size 4
#   REPRO_MAX_CONNS=10 ./run.sh
#   REPRO_KEEP_PG=1 ./run.sh   # leave PostgreSQL up after the run
#
# Needs Docker Compose v2 (the `docker compose` plugin, with `--wait`).
# See README.md for what each phase proves.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

export REPRO_PG_PORT="${REPRO_PG_PORT:-55432}"
export REPRO_MAX_CONNS="${REPRO_MAX_CONNS:-4}"
export REPRO_OBSERVE="${REPRO_OBSERVE:-20s}"

if ! [[ "$REPRO_MAX_CONNS" =~ ^[0-9]+$ ]] || [ "$REPRO_MAX_CONNS" -lt 2 ]; then
  echo "REPRO_MAX_CONNS must be an integer >= 2 (got '$REPRO_MAX_CONNS')" >&2
  exit 2
fi

# Give PostgreSQL headroom over the pool under test — the pool itself, the
# observer connection, and the superuser reserve — so the server's own limit can
# never be what the repro measures.
REPRO_PG_MAX_CONNECTIONS="${REPRO_PG_MAX_CONNECTIONS:-$(( REPRO_MAX_CONNS * 2 + 50 ))}"
if [ "$REPRO_PG_MAX_CONNECTIONS" -lt 200 ]; then
  REPRO_PG_MAX_CONNECTIONS=200
fi
export REPRO_PG_MAX_CONNECTIONS

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose (v2 plugin) is required; 'docker compose version' failed." >&2
  echo "The legacy docker-compose v1 binary does not support --wait." >&2
  exit 2
fi

compose() { docker compose -p dbpool-repro -f "$here/docker-compose.yml" "$@"; }

cleanup() {
  if [ "${REPRO_KEEP_PG:-0}" = "1" ]; then
    echo
    echo "PostgreSQL left running on 127.0.0.1:$REPRO_PG_PORT (REPRO_KEEP_PG=1)."
    echo "  Note: the wedged sessions are gone. They belong to the test process,"
    echo "  so PostgreSQL tears them down when it exits — inspect pg_stat_activity"
    echo "  from a second terminal *while* the run is in its observation windows."
    echo "  stop it with: docker compose -p dbpool-repro -f $here/docker-compose.yml down -v"
    return
  fi
  compose down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Report whether the port is still bound. Linux has ss, macOS has netstat;
# with neither, say so rather than silently skipping the wait below.
port_in_use() {
  if command -v ss >/dev/null 2>&1; then
    ss -ltn 2>/dev/null | grep -q ":$REPRO_PG_PORT "
  elif command -v netstat >/dev/null 2>&1; then
    netstat -an 2>/dev/null | grep -q "[.:]$REPRO_PG_PORT .*LISTEN"
  else
    return 1
  fi
}

echo "==> starting PostgreSQL on 127.0.0.1:$REPRO_PG_PORT (max_connections=$REPRO_PG_MAX_CONNECTIONS)"
# Clear anything a previous run left behind, and give the port a moment to come
# back: rootless Podman holds it briefly after the container is gone.
compose down -v >/dev/null 2>&1 || true
if command -v ss >/dev/null 2>&1 || command -v netstat >/dev/null 2>&1; then
  for _ in $(seq 20); do
    port_in_use || break
    sleep 1
  done
else
  echo "    (no ss or netstat; skipping the port-release wait)"
fi
compose up -d --wait

echo "==> running the repro with a pool of $REPRO_MAX_CONNS connections"
cd "$repo/src"
POSTGRES_MIGRATION_SCRIPTS_PATH="$repo/make/migrations/postgresql/" \
  go test -tags dbpool_repro -count=1 -v -timeout 10m \
  -run TestConnectionExhaustion ./lib/dbpool/
