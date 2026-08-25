#!/usr/bin/env bash
# Normalizes each variant's raw results into results/summary.csv. Usage:
#   collect-results.sh <variant> [<variant> ...]
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="$(dirname "$HERE")"
RESULTS="$BENCH_DIR/results"

echo "variant,cold_p95_ms,cold_p99_ms,cold_start_max_pod_s,warm_p95_ms,warm_p99_ms,kill_recover_s,max_db_conns,max_lock_wait_conns" > "$RESULTS/summary.csv"

for variant in "$@"; do
  dir="$RESULTS/$variant"
  [ -d "$dir" ] || { echo "skipping $variant: no results dir" >&2; continue; }

  # k6 --summary-export already reports http_req_duration in milliseconds
  cold_p95=$(jq -r '.metrics.http_req_duration["p(95)"] // ""' "$dir/k6-cold.json" 2>/dev/null)
  cold_p99=$(jq -r '.metrics.http_req_duration["p(99)"] // ""' "$dir/k6-cold.json" 2>/dev/null)
  warm_p95=$(jq -r '.metrics.http_req_duration["p(95)"] // ""' "$dir/k6-warm.json" 2>/dev/null)
  warm_p99=$(jq -r '.metrics.http_req_duration["p(99)"] // ""' "$dir/k6-warm.json" 2>/dev/null)

  cold_max_pod=$(tail -n +2 "$dir/cold-start-per-pod.csv" 2>/dev/null | awk -F, '{print $2}' | sort -n | tail -1)
  kill_recover=$(tail -n +2 "$dir/pod-kill-recovery.csv" 2>/dev/null | awk -F, '{print $4}')
  max_conns=$(tail -n +2 "$dir/conn-samples.csv" 2>/dev/null | awk -F, '{print $2}' | sort -n | tail -1)
  max_lock_wait=$(tail -n +2 "$dir/conn-samples.csv" 2>/dev/null | awk -F, '{print $4}' | sort -n | tail -1)

  echo "$variant,$cold_p95,$cold_p99,$cold_max_pod,$warm_p95,$warm_p99,$kill_recover,$max_conns,$max_lock_wait" >> "$RESULTS/summary.csv"
done

echo "Wrote $RESULTS/summary.csv"
cat "$RESULTS/summary.csv"
