#!/usr/bin/env bash
# Lock and mirror the public-image workload without storing layers locally.
# `crane copy` streams source registry -> Harbor and is deliberately sequential
# to stay within public-registry limits.
set -euo pipefail

mode="${1:?usage: issue478-mirror-images.sh <lock|mirror>}"
manifest="${MANIFEST:-bench/issue478-images.csv}"
lock_file="${LOCK_FILE:-bench/issue478-images.lock.csv}"
platform="${PLATFORM:-linux/amd64}"
registry="${HARBOR_REGISTRY:-localhost:18080}"
project="${HARBOR_PROJECT:-issue478}"
tmp_dir="$(mktemp -d -t issue478-mirror.XXXXXX)"
trap 'rm -rf "$tmp_dir"' EXIT

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

contains_rate_limit() {
  rg -qi 'toomanyrequests|rate.?limit|status.?429|http.?429' "$1"
}

resolve_reference() {
  local source_reference="$1"
  local mirror_reference="mirror.gcr.io/${source_reference#docker.io/}"
  local candidate error_file digest

  # mirror.gcr.io mirrors Docker Hub content and keeps the benchmark from
  # consuming Docker Hub's unauthenticated pull allowance. Docker Hub is a
  # single fallback; a failure after that is intentionally terminal.
  for candidate in "$mirror_reference" "$source_reference"; do
    error_file="$tmp_dir/resolve-error"
    if digest="$(crane digest --platform "$platform" "$candidate" 2>"$error_file")"; then
      printf '%s,%s\n' "$candidate" "$digest"
      return 0
    fi

    if contains_rate_limit "$error_file"; then
      printf 'Rate limited while resolving %s; trying the next source if available.\n' "$candidate" >&2
    else
      printf 'Unable to resolve %s: %s\n' "$candidate" "$(tr '\n' ' ' <"$error_file")" >&2
    fi
  done

  die "No source could resolve $source_reference; stopping as requested."
}

initialize_lock_file() {
  if [[ ! -e "$lock_file" ]]; then
    mkdir -p "$(dirname "$lock_file")"
    printf '%s\n' 'source_reference,source_used,platform_digest,destination' >"$lock_file"
    return
  fi

  # A timed-out caller can leave a resumed run racing with the original. Keep
  # the first successful record for each source reference before proceeding.
  awk -F, 'NR == 1 || !seen[$1]++' "$lock_file" >"$tmp_dir/lock.normalized"
  mv "$tmp_dir/lock.normalized" "$lock_file"
}

lock_images() {
  [[ -f "$manifest" ]] || die "Manifest not found: $manifest"
  initialize_lock_file

  declare -A locked=()
  while IFS=, read -r source_reference _; do
    [[ "$source_reference" == 'source_reference' ]] && continue
    locked["$source_reference"]=1
  done <"$lock_file"

  local image tag source_reference _source repository destination source_used digest locked_count
  locked_count="${#locked[@]}"
  while IFS=, read -r image tag source_reference _source; do
    [[ "$image" == 'image' ]] && continue
    if [[ -n "${locked[$source_reference]:-}" ]]; then
      continue
    fi

    repository="${image#docker.io/}"
    destination="$registry/$project/issue478-${repository##*/}:$tag"
    IFS=, read -r source_used digest < <(resolve_reference "$source_reference")
    printf '%s,%s,%s,%s\n' "$source_reference" "$source_used" "$digest" "$destination" >>"$lock_file"
    locked["$source_reference"]=1
    ((locked_count += 1))
    if ((locked_count % 25 == 0)); then
      printf 'Locked %s image manifests; latest: %s\n' "$locked_count" "$source_reference"
    fi
  done <"$manifest"

  local total
  total="$(( $(wc -l <"$lock_file") - 1 ))"
  printf 'Locked %s Linux/amd64 image manifests in %s\n' "$total" "$lock_file"
}

ensure_project_and_login() {
  local password status
  password="$(kubectl --context kind-issue478 -n issue478 get secret harbor-core -o jsonpath='{.data.HARBOR_ADMIN_PASSWORD}' | base64 --decode)"
  [[ -n "$password" ]] || die 'Harbor administrator password is empty'

  status="$(curl --silent --output "$tmp_dir/project-response" --write-out '%{http_code}' \
    --user "admin:$password" --header 'Content-Type: application/json' \
    --request POST --data '{"project_name":"issue478","metadata":{"public":"true"}}' \
    "http://$registry/api/v2.0/projects")"
  case "$status" in
    201|409) ;;
    *) die "Could not create Harbor project $project (HTTP $status): $(tr '\n' ' ' <"$tmp_dir/project-response")" ;;
  esac

  # crane follows Docker's config-directory convention. Keep the short-lived
  # Harbor credential out of the developer's normal Docker config.
  export DOCKER_CONFIG="$tmp_dir/docker-config"
  mkdir -p "$DOCKER_CONFIG"
  printf '%s' "$password" | crane auth login --insecure --username admin --password-stdin "$registry" >/dev/null
}

mirror_images() {
  [[ -f "$lock_file" ]] || die "Digest lock file not found: run '$0 lock' first"
  initialize_lock_file
  ensure_project_and_login

  local source_reference source_used digest destination destination_digest copied=0 skipped=0
  while IFS=, read -r source_reference source_used digest destination; do
    [[ "$source_reference" == 'source_reference' ]] && continue
    if destination_digest="$(crane digest --insecure --platform "$platform" "$destination" 2>/dev/null)"; then
      if [[ "$destination_digest" == "$digest" ]]; then
        ((skipped += 1))
        continue
      fi
      die "Destination digest mismatch for $destination: expected $digest, found $destination_digest"
    fi
    if ! crane copy --insecure --platform "$platform" --jobs 1 "$source_used@$digest" "$destination"; then
      die "Copy failed for $source_reference; stopping as requested."
    fi
    ((copied += 1))
    if ((copied % 10 == 0)); then
      printf 'Copied %s images; latest: %s\n' "$copied" "$destination"
    fi
  done <"$lock_file"

  printf 'Copied %s and verified/skipped %s locked images in Harbor project %s\n' "$copied" "$skipped" "$project"
}

case "$mode" in
  lock) lock_images ;;
  mirror) mirror_images ;;
  *) die "Unknown mode: $mode (expected lock or mirror)" ;;
esac
