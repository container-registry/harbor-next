#!/usr/bin/env bash

set -euo pipefail

if ! command -v pg_ctlcluster >/dev/null 2>&1 || ! command -v redis-server >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  sudo apt-get update
  sudo apt-get install -y --no-install-recommends postgresql redis-server
fi

pg_version="$(find /etc/postgresql -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort -V | tail -n1)"
if [ -z "${pg_version}" ]; then
  echo "Failed to detect installed PostgreSQL version"
  exit 1
fi

sudo pg_ctlcluster "${pg_version}" main start
sudo -u postgres psql -d postgres -c "ALTER USER postgres WITH PASSWORD 'root123';"
if ! sudo -u postgres psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'registry'" | grep -q 1; then
  sudo -u postgres createdb registry
fi
sudo -u postgres psql -d postgres -c "ALTER SYSTEM SET max_connections = 300;"
sudo pg_ctlcluster "${pg_version}" main restart

for _ in $(seq 1 30); do
  if PGPASSWORD=root123 pg_isready -h 127.0.0.1 -p 5432 -U postgres >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! PGPASSWORD=root123 pg_isready -h 127.0.0.1 -p 5432 -U postgres >/dev/null 2>&1; then
  echo "PostgreSQL failed to become ready"
  exit 1
fi

if command -v redis-cli >/dev/null 2>&1; then
  redis-cli -h 127.0.0.1 -p 6379 shutdown >/dev/null 2>&1 || true
fi

redis-server --daemonize yes --save "" --appendonly no --bind 127.0.0.1 --port 6379

for _ in $(seq 1 30); do
  if redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1; then
  echo "Redis failed to become ready"
  exit 1
fi
