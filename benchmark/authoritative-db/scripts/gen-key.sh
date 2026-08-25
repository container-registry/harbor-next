#!/usr/bin/env bash
# Generates the token signing key the compose stack mounts (gitignored, regenerate per checkout).
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KEY="$(dirname "$HERE")/compose/token_service_key.pem"
[ -f "$KEY" ] && exit 0
openssl genpkey -algorithm RSA -outform PEM -pkeyopt rsa_keygen_bits:4096 | openssl rsa -traditional -out "$KEY"
chmod 644 "$KEY"
echo "Generated $KEY"
