#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

IMAGE_NAME="${IMAGE_NAME:-bluefin/bluefin-bootc:latest-20260708}"
HARBOR="${HARBOR:-http://100.100.156.26:18085}"
PROJECT="${PROJECT:-bluefin}"
REPO="${REPO:-bluefin-bootc}"
ARTIFACT="${ARTIFACT:-latest-20260708}"
USERPASS="${HARBOR_AUTH:-${USERPASS:-admin:Harbor12345}}"

OUT="${WORK_DIR:-$REPO_ROOT/temp/grype-sbom-scan}"
mkdir -p "$OUT" "$REPO_ROOT/temp/grype-tmp" "$REPO_ROOT/temp/grype-cache" "$REPO_ROOT/temp/xdg-cache"

echo "== image =="
echo "$IMAGE_NAME"

echo "== fetch Harbor SBOM =="
SBOM_DIGEST="$(
  curl -fsS -u "$USERPASS" \
    "${HARBOR}/api/v2.0/projects/${PROJECT}/repositories/${REPO}/artifacts/${ARTIFACT}?with_sbom_overview=true" \
    | jq -r '.sbom_overview.sbom_digest'
)"

if [[ -z "$SBOM_DIGEST" || "$SBOM_DIGEST" == "null" ]]; then
  echo "No SBOM digest found. Generate SBOM in Harbor first." >&2
  exit 1
fi

echo "SBOM digest: $SBOM_DIGEST"

curl -fsS -u "$USERPASS" \
  -H 'Accept: application/json' \
  "${HARBOR}/api/v2.0/projects/${PROJECT}/repositories/${REPO}/artifacts/${SBOM_DIGEST}/additions/sbom" \
  > "${OUT}/harbor-sbom.json"

echo "== SBOM package summary =="
jq '{
  spdxVersion,
  package_count:(.packages|length),
  rpm_purls:([.packages[] | .externalRefs[]? | select(.referenceType=="purl" and (.referenceLocator|startswith("pkg:rpm/")))] | length)
}' "${OUT}/harbor-sbom.json"

echo "== grype SBOM scan =="
TMPDIR="$REPO_ROOT/temp/grype-tmp" \
XDG_CACHE_HOME="$REPO_ROOT/temp/xdg-cache" \
GRYPE_DB_CACHE_DIR="$REPO_ROOT/temp/grype-cache/db" \
GRYPE_CHECK_FOR_APP_UPDATE=false \
grype "sbom:${OUT}/harbor-sbom.json" -o json \
  > "${OUT}/grype-from-sbom.json"

echo "== vulnerability summary =="
jq '{
  matches:(.matches|length),
  fixed:([.matches[] | select((.vulnerability.fix.versions // []) | length > 0)] | length),
  by_type:(.matches | group_by(.artifact.type) | map({type:.[0].artifact.type,total:length,fixed:([.[] | select((.vulnerability.fix.versions // []) | length > 0)] | length)})),
  by_severity:(.matches | group_by(.vulnerability.severity) | map({severity:.[0].vulnerability.severity,total:length,fixed:([.[] | select((.vulnerability.fix.versions // []) | length > 0)] | length)})),
  top_packages:(
    [.matches[] | {name:.artifact.name, version:.artifact.version, type:.artifact.type, vuln:.vulnerability.id, severity:.vulnerability.severity, fix:.vulnerability.fix.versions}]
    | group_by(.name)
    | map({name:.[0].name, version:.[0].version, type:.[0].type, total:length, severities:([.[].severity] | unique), fixes:([.[].fix[]?] | unique)})
    | sort_by(-.total, .name)
    | .[0:25]
  )
}' "${OUT}/grype-from-sbom.json"

echo "== outputs =="
echo "${OUT}/harbor-sbom.json"
echo "${OUT}/grype-from-sbom.json"
