#!/usr/bin/env bash
set -euo pipefail

context_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tag="${1:-harbor-homebrew-compat:brew6}"

docker build \
  -f "${context_dir}/Containerfile" \
  -t "${tag}" \
  "${context_dir}"
