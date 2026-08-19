#!/usr/bin/env bash
# pr-harbor-next.sh — Create a PR to harbor-next with private-file guard.
#
# Usage:
#   8gcr-ee/pr-harbor-next.sh [gh pr create flags...]
#
# Examples:
#   8gcr-ee/pr-harbor-next.sh --title "feat: Add proxy cache TTL" --body "..."
#   8gcr-ee/pr-harbor-next.sh --fill
#   8gcr-ee/pr-harbor-next.sh --draft

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GUARD="$SCRIPT_DIR/hooks/harbor-next-guard.sh"

HARBOR_NEXT_REPO="container-registry/harbor-next"

# Resolve which ref we're about to PR
current_branch=$(git branch --show-current)
if [[ -z "$current_branch" ]]; then
  echo "ERROR: Detached HEAD. Check out a branch first." >&2
  exit 1
fi

echo "Checking branch '$current_branch' for private files..."
echo ""

if ! bash "$GUARD" HEAD; then
  echo ""
  echo "PR creation aborted."
  exit 1
fi

echo "Guard passed. Creating PR to $HARBOR_NEXT_REPO..."
echo ""

exec gh pr create \
  --repo "$HARBOR_NEXT_REPO" \
  --base main \
  "$@"
