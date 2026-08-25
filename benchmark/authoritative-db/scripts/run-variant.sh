#!/usr/bin/env bash
# Runs one full benchmark pass for a variant against the 3-core compose stack.
# Usage: run-variant.sh <variant> <core-image-tag>
set -euo pipefail

VARIANT="${1:?variant name required}"
CORE_TAG="${2:?core image tag required}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="$(dirname "$HERE")"
COMPOSE_DIR="$BENCH_DIR/compose"
OUT="$BENCH_DIR/results/$VARIANT"
export DOCKER_HOST="${DOCKER_HOST:-unix:///run/user/$(id -u)/podman/podman.sock}"
COMPOSE="podman compose -f $COMPOSE_DIR/docker-compose.bench.yml"

mkdir -p "$OUT"
echo "=== [$VARIANT] tearing down any previous stack ==="
CORE_TAG="$CORE_TAG" $COMPOSE down -v >/dev/null 2>&1 || true

echo "=== [$VARIANT] starting stack (image tag: $CORE_TAG) ==="
START_TS=$(date +%s.%N)
CORE_TAG="$CORE_TAG" $COMPOSE up -d

echo "=== [$VARIANT] waiting for all 3 core replicas to become healthy ==="
echo "replica,seconds_to_ready" > "$OUT/cold-start-per-pod.csv"
for name in core-0 core-1 core-2; do
  cid="compose_${name}_1"
  until podman inspect --format '{{.State.Health.Status}}' "$cid" 2>/dev/null | grep -q healthy; do
    sleep 0.2
  done
  ready_ts=$(date +%s.%N)
  elapsed=$(echo "$ready_ts - $START_TS" | bc)
  echo "$name,$elapsed" >> "$OUT/cold-start-per-pod.csv"
done

echo "=== [$VARIANT] extracting migration phase durations from logs ==="
echo "replica,phase,duration" > "$OUT/migration-phase-durations.csv"
for name in core-0 core-1 core-2; do
  podman logs "compose_${name}_1" 2>&1 \
    | { grep -oE 'migration phase "[a-z]+" took [0-9.]+(µs|ms|s)' || true; } \
    | while read -r line; do
        phase=$(echo "$line" | sed -E 's/.*"([a-z]+)".*/\1/')
        dur=$(echo "$line" | sed -E 's/.*took ([0-9.]+(µs|ms|s)).*/\1/')
        echo "$name,$phase,$dur" >> "$OUT/migration-phase-durations.csv"
      done
done

echo "=== [$VARIANT] starting DB stat sampler ==="
"$HERE/pg-stat-sampler.sh" localhost 15432 "$OUT" &
SAMPLER_PID=$!

echo "=== [$VARIANT] cold k6 burst ==="
CORE_PORTS=18080,18081,18082 VUS=10 DURATION_SEC=20 \
  k6 run --summary-trend-stats="avg,min,med,max,p(95),p(99)" --summary-export="$OUT/k6-cold.json" "$BENCH_DIR/load/login.js" 2>&1 | tail -30 || true

echo "=== [$VARIANT] warm load + mid-run pod-kill/restart ==="
( CORE_PORTS=18080,18081,18082 VUS=20 DURATION_SEC=180 \
  k6 run --summary-trend-stats="avg,min,med,max,p(95),p(99)" --summary-export="$OUT/k6-warm.json" "$BENCH_DIR/load/login.js" 2>&1 | tail -40 ) &
K6_PID=$!

sleep 45
VICTIM="compose_core-1_1"
echo "=== [$VARIANT] killing $VICTIM mid-load ==="
KILL_TS=$(date +%s.%N)
# podman-compose's `restart: unless-stopped` isn't actively supervised in a
# rootless session (no dockerd-style daemon) — restart explicitly, which is
# closer to a real k8s pod reschedule (fresh process, cold migration) anyway.
# Retry: the podman socket can hiccup transiently under concurrent k6+sampler load.
for attempt in 1 2 3 4 5; do
  podman kill "$VICTIM" && break || { echo "kill attempt $attempt failed, retrying..."; sleep 2; }
done
sleep 1
for attempt in 1 2 3 4 5; do
  podman start "$VICTIM" && break || { echo "start attempt $attempt failed, retrying..."; sleep 2; }
done
wait_deadline=$(($(date +%s) + 60))
until podman inspect --format '{{.State.Health.Status}}' "$VICTIM" 2>/dev/null | grep -q healthy; do
  [ "$(date +%s)" -lt "$wait_deadline" ] || { echo "$VICTIM never became healthy after restart, giving up"; break; }
  sleep 0.2
done
RECOVER_TS=$(date +%s.%N)
echo "victim,kill_ts,recover_ts,recovery_seconds" > "$OUT/pod-kill-recovery.csv"
echo "$VICTIM,$KILL_TS,$RECOVER_TS,$(echo "$RECOVER_TS - $KILL_TS" | bc)" >> "$OUT/pod-kill-recovery.csv"
podman logs "$VICTIM" --since "$(date -u -d "@${KILL_TS%.*}" +%Y-%m-%dT%H:%M:%SZ)" 2>&1 \
  | { grep -oE 'migration phase "[a-z]+" took [0-9.]+(µs|ms|s)' || true; } \
  | while read -r line; do echo "$VICTIM (post-kill),$line" >> "$OUT/migration-phase-durations.csv"; done

wait "$K6_PID" || true
kill "$SAMPLER_PID" 2>/dev/null || true
wait "$SAMPLER_PID" 2>/dev/null || true

echo "=== [$VARIANT] tearing down ==="
CORE_TAG="$CORE_TAG" $COMPOSE down -v >/dev/null 2>&1 || true
echo "=== [$VARIANT] done, results in $OUT ==="
