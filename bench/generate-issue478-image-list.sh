#!/usr/bin/env bash
# Generate a bounded, reproducible pool of official-image references for the
# Harbor #478 scan-memory benchmark.  Docker Hub's tag endpoint is the source
# of truth: every emitted tag was present when this script ran.
set -euo pipefail

output_file="${1:-bench/issue478-images.csv}"
tmp_dir="$(mktemp -d -t issue478-image-list.XXXXXX)"
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$(dirname "$output_file")"

# display-name|Docker-Hub-repository|quota. The quotas cap the pool at 620:
# large enough to apply sustained concurrent load while staying practical for a
# disposable cluster. Some image families expose fewer eligible tags in the
# sampled Docker Hub result pages.
images=(
  "golang|library/golang|130"
  "alpine|library/alpine|40"
  "busybox|library/busybox|60"
  "node|library/node|140"
  "nginx|library/nginx|90"
  "postgres|library/postgres|100"
  "valkey|valkey/valkey|60"
)

# Retain released version tags and common Linux variants.  This excludes
# Windows, release-candidate, test and digest-like tags that would add noise to
# a Linux-based benchmark.  The individual tags are still checked against the
# Docker Hub API below.
stable_tag_pattern='^(latest|[0-9]+(\.[0-9]+){0,2}(-(alpine([0-9]+(\.[0-9]+)?)?|bookworm|bullseye|trixie|slim|perl|mainline|stable|fpm|cli|glibc|musl|uclibc))?)$'

printf '%s\n' 'image,tag,reference,tag_source' >"$output_file"

for image_quota in "${images[@]}"; do
  IFS='|' read -r image repository quota <<<"$image_quota"
  raw_tags="$tmp_dir/$image.raw"

  # Sampling two pages by recency plus both lexical directions captures current
  # variants and old releases without downloading the entire, very large tag
  # catalog.
  for ordering in last_updated name -name; do
    for page in 1 2; do
      url="https://hub.docker.com/v2/repositories/$repository/tags?page_size=100&ordering=$ordering&page=$page"
      curl --fail --silent --show-error --location \
        --retry 3 --retry-all-errors --connect-timeout 15 --max-time 90 \
        --user-agent 'harbor-issue478-benchmark/1.0' "$url" \
        | jq -r '.results[].name' >>"$raw_tags"
    done
  done

  candidate_tags="$tmp_dir/$image.candidates"
  LC_ALL=C sort -u "$raw_tags" | grep -E "$stable_tag_pattern" | sort -V >"$candidate_tags" || true
  available="$(wc -l <"$candidate_tags" | tr -d ' ')"
  if ((available == 0)); then
    printf 'No usable official Docker Hub tags found for %s\n' "$image" >&2
    exit 1
  fi

  if ((available <= quota)); then
    selected_tags="$candidate_tags"
  else
    selected_tags="$tmp_dir/$image.selected"
    # Select evenly over version-sorted candidates rather than merely taking
    # the newest (or oldest) tags.
    awk -v selected="$quota" -v available="$available" \
      'int((NR - 1) * selected / available) != int(NR * selected / available)' \
      "$candidate_tags" >"$selected_tags"
  fi

  while IFS= read -r tag; do
    printf '%s,%s,%s,%s\n' \
      "docker.io/$repository" "$tag" "docker.io/$repository:$tag" \
      "https://hub.docker.com/r/$repository" >>"$output_file"
  done <"$selected_tags"
done

total="$(($(wc -l <"$output_file") - 1))"
if ((total < 500 || total > 800)); then
  printf 'Generated %s images; expected a 500-800 image workload\n' "$total" >&2
  exit 1
fi

printf 'Wrote %s validated official-image references to %s\n' "$total" "$output_file"
