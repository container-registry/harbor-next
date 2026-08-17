#!/usr/bin/env bash
set -euo pipefail

harbor="${HARBOR_URL:-http://100.100.156.26:18085}"
auth="${HARBOR_AUTH:-admin:Harbor12345}"
poll_seconds="${POLL_SECONDS:-5}"
max_scan_seconds="${MAX_SCAN_SECONDS:-1800}"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
results_dir="${WORK_DIR:-$repo_root/temp/scan-check}/sbom-results"

mkdir -p "$results_dir"

targets=(
  "bluefin bluefin-bootc latest-20260708 rpm 1500"
  "bazzite bazzite-bootc stable rpm 2500"
  "dakota dakota-bootc stable generic 3"
  "arch-bootc arch-bootc sbom-bluefin-f3b991d alpm 500"
)

log() {
  printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

artifact_json() {
  local project="$1" repo="$2" ref="$3" out="$4"
  curl -fsS -u "$auth" \
    "$harbor/api/v2.0/projects/$project/repositories/$repo/artifacts/$ref?with_sbom_overview=true" \
    -o "$out"
}

trigger_sbom() {
  local project="$1" repo="$2" ref="$3" out="$4" code
  code="$(curl -sS -u "$auth" -o "$out" -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -X POST --data '{"scan_type":"sbom"}' \
    "$harbor/api/v2.0/projects/$project/repositories/$repo/artifacts/$ref/scan")"
  if [ "$code" != "202" ]; then
    log "$project/$repo:$ref SBOM trigger failed http=$code body=$(cat "$out")"
    return 1
  fi
}

validate_sbom() {
  local project="$1" package_type="$2" minimum="$3" file="$4"
  jq -e --arg prefix "pkg:$package_type/" --argjson minimum "$minimum" '
    (.spdxVersion | startswith("SPDX-")) and
    .SPDXID == "SPDXRef-DOCUMENT" and
    ([.packages[]? | select(any(.externalRefs[]?;
      (.referenceLocator // "") | startswith($prefix)))] | length) >= $minimum
  ' "$file" >/dev/null

  if [ "$project" = "dakota" ]; then
    jq -e '
      [.packages[]? | select(any(.externalRefs[]?;
        (.referenceLocator // "") | startswith("pkg:generic/"))) | .name]
      | sort == ["ffmpeg", "python", "util-linux"]
    ' "$file" >/dev/null
  fi
}

for target in "${targets[@]}"; do
  read -r project repo ref package_type minimum <<<"$target"
  prefix="$results_dir/$project-$ref"
  artifact_json "$project" "$repo" "$ref" "$prefix-before.json"
  before="$(jq -r '.sbom_overview.report_id // ""' "$prefix-before.json")"
  trigger_sbom "$project" "$repo" "$ref" "$prefix-trigger.out"
  start="$(date +%s)"

  while true; do
    artifact_json "$project" "$repo" "$ref" "$prefix-artifact.json"
    scan_status="$(jq -r '.sbom_overview.scan_status // "Pending"' "$prefix-artifact.json")"
    report_id="$(jq -r '.sbom_overview.report_id // ""' "$prefix-artifact.json")"
    log "$project/$repo:$ref status=$scan_status report_id=${report_id:-none}"

    if [ "$scan_status" = "Success" ] && [ -n "$report_id" ] && [ "$report_id" != "$before" ]; then
      sbom_digest="$(jq -r '.sbom_overview.sbom_digest' "$prefix-artifact.json")"
      curl -fsS -u "$auth" -H 'Accept: application/json' \
        "$harbor/api/v2.0/projects/$project/repositories/$repo/artifacts/$sbom_digest/additions/sbom" \
        -o "$prefix.spdx.json"
      validate_sbom "$project" "$package_type" "$minimum" "$prefix.spdx.json"
      jq -n \
        --arg project "$project" --arg repo "$repo" --arg ref "$ref" \
        --arg report_id "$report_id" --arg sbom_digest "$sbom_digest" \
        --arg package_type "$package_type" \
        --argjson packages "$(jq '.packages | length' "$prefix.spdx.json")" \
        --argjson native_packages "$(jq --arg prefix "pkg:$package_type/" \
          '[.packages[]? | select(any(.externalRefs[]?; (.referenceLocator // "") | startswith($prefix)))] | length' \
          "$prefix.spdx.json")" \
        '{project:$project,repository:$repo,reference:$ref,report_id:$report_id,
          sbom_digest:$sbom_digest,packages:$packages,package_type:$package_type,
          native_packages:$native_packages}' \
        | tee "$prefix-summary.json"
      break
    fi
    if [ "$scan_status" = "Error" ] || [ "$scan_status" = "Stopped" ]; then
      log "$project/$repo:$ref SBOM failed status=$scan_status"
      exit 1
    fi
    if [ "$(( $(date +%s) - start ))" -ge "$max_scan_seconds" ]; then
      log "$project/$repo:$ref SBOM timed out"
      exit 1
    fi
    sleep "$poll_seconds"
  done
done

log "all Harbor SBOM checks passed"
