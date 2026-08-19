#!/usr/bin/env bash
set -euo pipefail

harbor="${HARBOR_URL:-http://100.100.156.26:18085}"
auth="${HARBOR_AUTH:-admin:Harbor12345}"
deadline_seconds="${DEADLINE_SECONDS:-14400}"
poll_seconds="${POLL_SECONDS:-20}"
max_scan_seconds="${MAX_SCAN_SECONDS:-3600}"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
work_dir="${WORK_DIR:-$repo_root/temp/scan-check}"
results_dir="$work_dir/results"

mkdir -p "$results_dir"

targets=(
  "bluefin bluefin-bootc latest-20260708"
  "bazzite bazzite-bootc stable"
  "dakota dakota-bootc stable"
  "arch-bootc arch-bootc sbom-bluefin-f3b991d"
)

declare -A successes
for target in "${targets[@]}"; do
  read -r project repo ref <<<"$target"
  successes["$project/$repo:$ref"]=0
done

log() {
  printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

artifact_json() {
  local project="$1" repo="$2" ref="$3" out="$4"
  curl -fsS -u "$auth" \
    "$harbor/api/v2.0/projects/$project/repositories/$repo/artifacts/$ref?with_tag=true&with_scan_overview=true" \
    -o "$out"
}

scan_status() {
  jq -r '(.scan_overview // {}) | to_entries[0].value.scan_status // "none"' "$1"
}

scan_total() {
  jq -r '(.scan_overview // {}) | to_entries[0].value.summary.total // 0' "$1"
}

scan_fixable() {
  jq -r '(.scan_overview // {}) | to_entries[0].value.summary.fixable // 0' "$1"
}

scan_severity() {
  jq -r '(.scan_overview // {}) | to_entries[0].value.severity // ""' "$1"
}

scan_end_time() {
  jq -r '(.scan_overview // {}) | to_entries[0].value.end_time // ""' "$1"
}

scan_report_id() {
  jq -r '(.scan_overview // {}) | to_entries[0].value.report_id // ""' "$1"
}

save_vulnerabilities() {
  local project="$1" repo="$2" ref="$3" prefix="$4"
  curl -fsS -u "$auth" \
    "$harbor/api/v2.0/projects/$project/repositories/$repo/artifacts/$ref/additions/vulnerabilities" \
    -o "$results_dir/$prefix-vulnerabilities.json" || return 0
}

summarize_vulnerabilities() {
  local file="$1"
  if [ ! -s "$file" ]; then
    echo "vulnerability_addition=missing"
    return
  fi
  jq -r '
    to_entries[0].value.vulnerabilities // [] |
    {
      vulnerabilities: length,
      package_names: ([.[].package] | unique | length),
      top_packages: ([.[].package] | group_by(.) | map({pkg: .[0], count: length}) | sort_by(-.count) | .[:8])
    }
  ' "$file"
}

validate_vulnerabilities() {
  local project="$1" file="$2"
  if [ ! -s "$file" ]; then
    return 1
  fi
  case "$project" in
    bluefin|bazzite)
      jq -e '
        [.[].vulnerabilities[]? |
          select(.vendor_attributes.package_type == "rpm" and
                 (.vendor_attributes.purl | startswith("pkg:rpm/")))] |
        length > 0
      ' "$file" >/dev/null
      ;;
    dakota)
      jq -e '
        [.[].vulnerabilities[]? |
          select(.vendor_attributes.package_type == "binary" and
                 .vendor_attributes.namespace == "nvd:cpe" and
                 .vendor_attributes.matcher == "stock-matcher" and
                 (.vendor_attributes.purl | startswith("pkg:generic/")) and
                 (.package == "python" or .package == "util-linux" or .package == "ffmpeg"))] |
        length > 0
      ' "$file" >/dev/null
      ;;
    arch-bootc)
      jq -e '
        [.[].vulnerabilities[]? |
          select(.id | startswith("CVE-")) |
          select(.vendor_attributes.package_type == "alpm" and
                 .vendor_attributes.namespace == "arch:distro:archlinux:rolling" and
                 (.vendor_attributes.purl | startswith("pkg:alpm/")))] |
        length > 0
      ' "$file" >/dev/null
      ;;
  esac
}

trigger_scan() {
  local project="$1" repo="$2" ref="$3"
  local code
  code="$(curl -sS -u "$auth" -o "$results_dir/$project-$ref-trigger.out" -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -X POST --data '{"scan_type":"vulnerability"}' \
    "$harbor/api/v2.0/projects/$project/repositories/$repo/artifacts/$ref/scan")"
  if [ "$code" != "202" ]; then
    log "$project/$repo:$ref trigger failed http=$code body=$(cat "$results_dir/$project-$ref-trigger.out")"
    return 1
  fi
  log "$project/$repo:$ref trigger accepted http=$code"
}

run_one_scan() {
  local project="$1" repo="$2" ref="$3" round="$4"
  local key="$project/$repo:$ref"
  local prefix="${project}-${repo}-${ref}-round${round}"
  local artifact="$results_dir/$prefix-artifact.json"
  local before_end before_report_id end_time report_id start now elapsed status total fixable severity

  artifact_json "$project" "$repo" "$ref" "$artifact"
  before_end="$(scan_end_time "$artifact")"
  before_report_id="$(scan_report_id "$artifact")"

  trigger_scan "$project" "$repo" "$ref" || return 1
  start="$(date +%s)"

  while true; do
    artifact_json "$project" "$repo" "$ref" "$artifact"
    status="$(scan_status "$artifact")"
    total="$(scan_total "$artifact")"
    fixable="$(scan_fixable "$artifact")"
    severity="$(scan_severity "$artifact")"
    end_time="$(scan_end_time "$artifact")"
    report_id="$(scan_report_id "$artifact")"
    log "$key round=$round status=$status severity=${severity:-none} total=$total fixable=$fixable report_id=${report_id:-none} end_time=${end_time:-none}"

    case "$status" in
      Success)
        if [ -z "$report_id" ] || [ "$report_id" = "$before_report_id" ] || [ -z "$end_time" ] || [ "$end_time" = "$before_end" ]; then
          sleep "$poll_seconds"
          continue
        fi
        save_vulnerabilities "$project" "$repo" "$ref" "$prefix"
        summarize_vulnerabilities "$results_dir/$prefix-vulnerabilities.json" \
          | tee "$results_dir/$prefix-summary.json"
        if [ "$total" -gt 0 ] && validate_vulnerabilities "$project" "$results_dir/$prefix-vulnerabilities.json"; then
          printf '%s\n' "$report_id" >"$results_dir/$prefix-report-id.txt"
          successes["$key"]=$((successes["$key"] + 1))
          log "$key round=$round PASSED success_count=${successes[$key]}"
          return 0
        fi
        log "$key round=$round scan succeeded but required Linux package provenance is missing"
        return 1
        ;;
      Error|Stopped)
        log "$key round=$round FAILED status=$status"
        return 1
        ;;
    esac

    now="$(date +%s)"
    elapsed=$((now - start))
    if [ "$elapsed" -ge "$max_scan_seconds" ]; then
      log "$key round=$round timed out after ${elapsed}s"
      return 1
    fi
    sleep "$poll_seconds"
  done
}

deadline=$(( $(date +%s) + deadline_seconds ))
attempt=0

while [ "$(date +%s)" -lt "$deadline" ]; do
  attempt=$((attempt + 1))
  log "attempt=$attempt starting"
  all_done=1

  for target in "${targets[@]}"; do
    read -r project repo ref <<<"$target"
    key="$project/$repo:$ref"
    if [ "${successes[$key]}" -ge 2 ]; then
      log "$key already passed twice"
      continue
    fi
    all_done=0
    run_one_scan "$project" "$repo" "$ref" "$((successes[$key] + 1))" || true
  done

  all_done=1
  for target in "${targets[@]}"; do
    read -r project repo ref <<<"$target"
    key="$project/$repo:$ref"
    if [ "${successes[$key]}" -lt 2 ]; then
      all_done=0
      break
    fi
  done

  if [ "$all_done" -eq 1 ]; then
    log "all targets passed twice"
    exit 0
  fi

  log "current success counts:"
  for target in "${targets[@]}"; do
    read -r project repo ref <<<"$target"
    key="$project/$repo:$ref"
    log "  $key=${successes[$key]}/2"
  done
  sleep 30
done

log "deadline reached before all targets passed twice"
exit 1
