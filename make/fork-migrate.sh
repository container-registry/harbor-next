#!/usr/bin/env bash
# fork-migrate.sh — Helper script for managing fork migrations during
# upstream merges, fork rollbacks, and daily operations.
#
# This script wraps the fork-migrator CLI and provides safe compound
# operations for common workflows.
#
# Usage:
#   ./make/fork-migrate.sh <command>
#
# Commands:
#   up                  Apply all pending fork migrations
#   down                Roll back ALL fork migrations
#   status              Show current fork migration state
#   version             Show current fork migration version
#   steps <N>           Apply N steps (negative = rollback)
#   pre-upstream-merge  Roll back fork migrations before merging upstream
#   post-upstream-merge Apply upstream + fork migrations after merging upstream
#   switch-to-upstream  Fully remove fork schema changes (return to upstream-only)
#   switch-to-fork      Apply fork schema changes on top of upstream
#   force <V>           Force version (recover from dirty state)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Build fork-migrator if not already built
FORK_MIGRATOR="${REPO_ROOT}/src/cmd/fork-migrator/fork-migrator"

build_migrator() {
    if [ ! -f "$FORK_MIGRATOR" ]; then
        echo "Building fork-migrator..."
        (cd "$REPO_ROOT/src" && go build -o "$FORK_MIGRATOR" ./cmd/fork-migrator)
    fi
}

# Default migration path if not overridden
export FORK_MIGRATION_SCRIPTS_PATH="${FORK_MIGRATION_SCRIPTS_PATH:-${REPO_ROOT}/make/migrations/postgresql/fork/}"

run_fork_migrator() {
    build_migrator
    "$FORK_MIGRATOR" "$@"
}

case "${1:-help}" in
    up)
        echo "==> Applying fork migrations..."
        run_fork_migrator up
        ;;

    down)
        echo "==> Rolling back ALL fork migrations..."
        run_fork_migrator down
        ;;

    status)
        run_fork_migrator status
        ;;

    version)
        run_fork_migrator version
        ;;

    steps)
        shift
        run_fork_migrator steps "$@"
        ;;

    force)
        shift
        run_fork_migrator force "$@"
        ;;

    pre-upstream-merge)
        echo "==> PRE-UPSTREAM-MERGE: Rolling back fork migrations..."
        echo "    This returns the database to upstream-only state."
        echo ""
        run_fork_migrator down
        echo ""
        echo "==> Fork migrations rolled back. You can now safely merge upstream."
        echo "    After merging, run: $0 post-upstream-merge"
        ;;

    post-upstream-merge)
        echo "==> POST-UPSTREAM-MERGE: Applying upstream + fork migrations..."
        echo ""
        echo "--- Step 1/2: Applying upstream migrations ---"
        UPSTREAM_MIGRATOR="${REPO_ROOT}/src/cmd/standalone-db-migrator/standalone-db-migrator"
        if [ ! -f "$UPSTREAM_MIGRATOR" ]; then
            echo "Building standalone-db-migrator..."
            (cd "$REPO_ROOT/src" && go build -o "$UPSTREAM_MIGRATOR" ./cmd/standalone-db-migrator)
        fi
        "$UPSTREAM_MIGRATOR"
        echo ""
        echo "--- Step 2/2: Re-applying fork migrations ---"
        run_fork_migrator up
        echo ""
        echo "==> Upstream merge complete. Both upstream and fork migrations applied."
        ;;

    switch-to-upstream)
        echo "==> SWITCH TO UPSTREAM: Removing all fork schema changes..."
        run_fork_migrator down
        echo "==> Database is now at upstream-only state."
        ;;

    switch-to-fork)
        echo "==> SWITCH TO FORK: Applying fork schema changes on top of upstream..."
        run_fork_migrator up
        echo "==> Fork schema changes applied."
        ;;

    help|--help|-h)
        cat <<'USAGE'
fork-migrate.sh — Manage fork-specific database schema migrations

Usage:
  ./make/fork-migrate.sh <command>

Commands:
  up                    Apply all pending fork migrations
  down                  Roll back ALL fork migrations
  status                Show current fork migration state
  version               Show current fork migration version
  steps <N>             Apply N steps (negative = rollback)
  pre-upstream-merge    Roll back fork migrations (safe for upstream merge)
  post-upstream-merge   Apply upstream migrations then re-apply fork migrations
  switch-to-upstream    Remove fork changes, return to upstream-only schema
  switch-to-fork        Apply fork changes on top of upstream schema
  force <V>             Force version (recover from dirty state)

Upstream Merge Workflow:
  1. ./make/fork-migrate.sh pre-upstream-merge
  2. git fetch upstream && git merge upstream/main
  3. ./make/fork-migrate.sh post-upstream-merge

Environment:
  POSTGRESQL_HOST, POSTGRESQL_PORT, POSTGRESQL_USERNAME,
  POSTGRESQL_PASSWORD, POSTGRESQL_DATABASE, POSTGRESQL_SSLMODE
  FORK_MIGRATION_SCRIPTS_PATH (default: make/migrations/postgresql/fork/)
USAGE
        ;;

    *)
        echo "Unknown command: $1"
        echo "Run '$0 help' for usage."
        exit 1
        ;;
esac
