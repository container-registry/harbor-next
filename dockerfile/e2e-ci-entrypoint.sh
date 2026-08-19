#!/usr/bin/env bash
set -euo pipefail

unset DOCKER_TLS_VERIFY DOCKER_CERT_PATH
export DOCKER_HOST=unix:///var/run/docker.sock

if [ ! -S /var/run/docker.sock ]; then
  mkdir -p /workspace/reports
  dockerd_log=/workspace/reports/e2e-dockerd.log
  dockerd-entrypoint.sh dockerd > "$dockerd_log" 2>&1 &
  dockerd_pid=$!

  for _ in $(seq 1 60); do
    if docker info >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "$dockerd_pid" >/dev/null 2>&1; then
      cat "$dockerd_log" >&2 || true
      exit 1
    fi
    sleep 2
  done

  if ! docker info >/dev/null 2>&1; then
    cat "$dockerd_log" >&2 || true
    exit 1
  fi
fi

exec "$@"
