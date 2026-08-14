#!/usr/bin/env bash
# setup.sh — Install the harbor-next pre-push guard.
#
# What it does:
#   1. Creates lefthook-local.yml with the pre-push guard
#   2. Runs lefthook install to activate it
#
# Run this after a fresh clone or when switching to an 8gcr branch.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

LEFTHOOK_LOCAL="$REPO_ROOT/lefthook-local.yml"

# Check if lefthook is available
if ! command -v lefthook >/dev/null 2>&1; then
  echo "lefthook not found. Falling back to direct hook install."
  cp 8gcr-ee/hooks/pre-push .git/hooks/pre-push
  chmod +x .git/hooks/pre-push
  echo "Installed .git/hooks/pre-push directly."
  exit 0
fi

cat > "$LEFTHOOK_LOCAL" << 'EOF'
pre-push:
  commands:
    harbor-next-guard:
      run: bash 8gcr-ee/hooks/pre-push {args}
EOF

lefthook install
echo "harbor-next pre-push guard installed via lefthook-local.yml"
