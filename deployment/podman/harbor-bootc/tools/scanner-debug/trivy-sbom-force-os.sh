#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

IMAGE_NAME="${IMAGE_NAME:-bluefin/bluefin-bootc:latest-20260708}"
HARBOR="${HARBOR:-http://100.100.156.26:18085}"
PROJECT="${PROJECT:-bluefin}"
REPO="${REPO:-bluefin-bootc}"
ARTIFACT="${ARTIFACT:-latest-20260708}"
USERPASS="${HARBOR_AUTH:-${USERPASS:-admin:Harbor12345}}"
OS_NAME="${OS_NAME:-fedora}"
OS_VERSION="${OS_VERSION:-42}"
PURL_NAMESPACE="${PURL_NAMESPACE:-}"

OUT="${WORK_DIR:-$REPO_ROOT/temp/trivy-sbom-force-os}"
mkdir -p "$OUT" "$REPO_ROOT/temp/trivy-tmp" "$REPO_ROOT/temp/trivy-cache" "$REPO_ROOT/temp/xdg-cache"

echo "== image =="
echo "$IMAGE_NAME"

echo "== fetch Harbor SPDX SBOM =="
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
  > "${OUT}/harbor-spdx.json"

echo "== SPDX package summary =="
jq '{
  spdxVersion,
  package_count:(.packages|length),
  rpm_purls:([.packages[] | .externalRefs[]? | select(.referenceType=="purl" and (.referenceLocator|startswith("pkg:rpm/")))] | length)
}' "${OUT}/harbor-spdx.json"

echo "== rewrite SPDX RPM packages to CycloneDX with explicit OS =="
echo "forced OS: ${OS_NAME} ${OS_VERSION}"
if [[ -n "$PURL_NAMESPACE" ]]; then
  echo "forced RPM PURL namespace: ${PURL_NAMESPACE}"
fi
jq --arg image "$IMAGE_NAME" --arg os_name "$OS_NAME" --arg os_version "$OS_VERSION" --arg purl_namespace "$PURL_NAMESPACE" '
  def purl:
    [.externalRefs[]? | select(.referenceType=="purl" and (.referenceLocator|startswith("pkg:rpm/"))) | .referenceLocator][0]
    | if . == null or $purl_namespace == "" then .
      else sub("^pkg:rpm/[^/]+/"; "pkg:rpm/" + $purl_namespace + "/")
      end;

  {
    bomFormat: "CycloneDX",
    specVersion: "1.5",
    serialNumber: "urn:uuid:00000000-0000-4000-8000-000000000044",
    version: 1,
    metadata: {
      timestamp: (.creationInfo.created // null),
      tools: [{vendor: "harbor", name: "spdx-to-trivy-os-sbom", version: "one-time"}],
      component: {
        type: "container",
        name: $image,
        bomRef: "container:" + $image
      }
    },
    components: (
      [
        {
          type: "operating-system",
          name: $os_name,
          version: $os_version,
          bomRef: ("os:" + $os_name + "-" + $os_version),
          purl: ("pkg:generic/" + $os_name + "@" + $os_version)
        }
      ]
      +
      [
        .packages[]?
        | select(purl != null)
        | {
            type: "library",
            name: .name,
            version: (.versionInfo // "0"),
            bomRef: ("pkg:" + .SPDXID),
            purl: purl,
            properties: [
              {name: "syft:package:type", value: "rpm"},
              {name: "aquasecurity:trivy:PkgType", value: "rpm"}
            ]
          }
      ]
    )
  }
' "${OUT}/harbor-spdx.json" > "${OUT}/forced-fedora-cyclonedx.json"

echo "== CycloneDX package summary =="
jq '{
  bomFormat,
  specVersion,
  os_components:([.components[] | select(.type=="operating-system")] | length),
  components:(.components|length),
  rpm_purls:([.components[] | select((.purl // "") | startswith("pkg:rpm/"))] | length),
  sample:[.components[] | select((.purl // "") | startswith("pkg:rpm/")) | {name, version, type, purl}][0:5]
}' "${OUT}/forced-fedora-cyclonedx.json"

echo "== trivy sbom scan against rewritten CycloneDX =="
TMPDIR="$REPO_ROOT/temp/trivy-tmp" \
XDG_CACHE_HOME="$REPO_ROOT/temp/xdg-cache" \
TRIVY_CACHE_DIR="$REPO_ROOT/temp/trivy-cache" \
trivy sbom \
  --format json \
  --output "${OUT}/trivy-from-forced-cyclonedx.json" \
  --scanners vuln \
  --pkg-types os \
  "${OUT}/forced-fedora-cyclonedx.json"

echo "== vulnerability summary =="
jq '{
  artifact_name:.ArtifactName,
  os:.Metadata.OS,
  targets:[.Results[]? | {target:.Target, class:.Class, type:.Type, vulns:((.Vulnerabilities // [])|length)}],
  total:([.Results[]?.Vulnerabilities[]?] | length),
  fixed:([.Results[]?.Vulnerabilities[]? | select(.FixedVersion != null and .FixedVersion != "")] | length),
  by_severity:([.Results[]?.Vulnerabilities[]?] | group_by(.Severity) | map({severity:.[0].Severity,total:length})),
  top_packages:(
    [.Results[]?.Vulnerabilities[]? | {name:.PkgName, installed:.InstalledVersion, fixed:.FixedVersion, severity:.Severity, id:.VulnerabilityID}]
    | group_by(.name)
    | map({name:.[0].name, installed:.[0].installed, total:length, severities:([.[].severity] | unique), fixes:([.[].fixed | select(. != null and . != "")] | unique)})
    | sort_by(-.total, .name)
    | .[0:25]
  )
}' "${OUT}/trivy-from-forced-cyclonedx.json"

echo "== outputs =="
echo "${OUT}/harbor-spdx.json"
echo "${OUT}/forced-fedora-cyclonedx.json"
echo "${OUT}/trivy-from-forced-cyclonedx.json"
