#!/usr/bin/env bash
# harbor-next-guard.sh — Blocks private 8gcr files from reaching harbor-next.
#
# Usage:
#   harbor-next-guard.sh <ref>        Check <ref> against harbor-next/main
#   harbor-next-guard.sh              Check HEAD against harbor-next/main
#
# Exit codes: 0 = clean, 1 = private files found, 2 = setup error

set -euo pipefail

HARBOR_NEXT_BASE="harbor-next/main"

# Paths that must never appear in a harbor-next PR.
# Patterns are matched as prefixes against file paths.
PRIVATE_PATHS=(
  ".claude/"
  ".mcp.json"
  ".gopls-mcp.json"
  ".gsd/"
  ".playwright-mcp/"
  "8gcr-ee/"
  "CLAUDE.md"
)

ref="${1:-HEAD}"

# Verify the base ref exists
if ! git rev-parse --verify "$HARBOR_NEXT_BASE" >/dev/null 2>&1; then
  echo "ERROR: $HARBOR_NEXT_BASE not found. Run: git fetch harbor-next" >&2
  exit 2
fi

merge_base=$(git merge-base "$HARBOR_NEXT_BASE" "$ref" 2>/dev/null) || {
  echo "ERROR: Cannot find merge base between $HARBOR_NEXT_BASE and $ref" >&2
  exit 2
}

changed_files=$(git diff --name-only "$merge_base".."$ref")

if [[ -z "$changed_files" ]]; then
  exit 0
fi

blocked=()
while IFS= read -r file; do
  for pattern in "${PRIVATE_PATHS[@]}"; do
    if [[ "$file" == "$pattern"* || "$file" == "$pattern" ]]; then
      blocked+=("$file")
      break
    fi
  done
done <<< "$changed_files"

if [[ ${#blocked[@]} -gt 0 ]]; then
  echo ""
  echo "BLOCKED: ${#blocked[@]} private file(s) would leak into harbor-next:"
  echo ""
  for f in "${blocked[@]}"; do
    echo "  - $f"
  done
  echo ""
  echo "These files exist in 8gcr but must not reach harbor-next."
  echo "Create your PR branch from harbor-next/main, or remove these files from the branch."
  exit 1
fi

exit 0
