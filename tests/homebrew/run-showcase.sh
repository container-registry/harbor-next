#!/usr/bin/env bash
set -euo pipefail

image="${HOMEBREW_COMPAT_IMAGE:-harbor-homebrew-compat:brew6}"
suite="${1:-all}"

docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e HARBOR_URL="${HARBOR_URL:-http://host.docker.internal:8080}" \
  -e HARBOR_REGISTRY="${HARBOR_REGISTRY:-host.docker.internal:8080}" \
  -e HARBOR_PROJECT="${HARBOR_PROJECT:-homebrew}" \
  -e HARBOR_USERNAME="${HARBOR_USERNAME:-admin}" \
  -e HARBOR_PASSWORD="${HARBOR_PASSWORD:-Harbor12345}" \
  "${image}" \
  "${suite}"
