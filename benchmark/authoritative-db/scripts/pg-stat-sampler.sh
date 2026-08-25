#!/usr/bin/env bash
# Polls pg_stat_activity/pg_locks every 0.5s until killed. Usage:
#   pg-stat-sampler.sh <pghost> <pgport> <outdir>
set -euo pipefail

PGHOST="${1:?pghost required}"
PGPORT="${2:?pgport required}"
OUT="${3:?outdir required}"
export PGPASSWORD="${PGPASSWORD:-root123}"

mkdir -p "$OUT"
echo "ts,total_conns,active_conns,lock_wait_conns" > "$OUT/conn-samples.csv"
echo "ts,mode,granted,count" > "$OUT/lock-samples.csv"

while true; do
  ts=$(date +%s.%N)
  psql -h "$PGHOST" -p "$PGPORT" -U postgres -d registry -Atc "
    SELECT '$ts,' || count(*) || ',' || count(*) FILTER (WHERE state='active') || ',' ||
           count(*) FILTER (WHERE wait_event_type='Lock')
    FROM pg_stat_activity WHERE datname='registry';" >> "$OUT/conn-samples.csv" 2>/dev/null || true
  psql -h "$PGHOST" -p "$PGPORT" -U postgres -d registry -Atc "
    SELECT '$ts,' || mode || ',' || granted || ',' || count(*)
    FROM pg_locks WHERE relation IS NOT NULL
    GROUP BY mode, granted;" >> "$OUT/lock-samples.csv" 2>/dev/null || true
  sleep 0.5
done
