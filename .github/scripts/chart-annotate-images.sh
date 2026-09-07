#!/usr/bin/env bash
# Append the per-release artifacthub.io/images annotation to a chart's
# Chart.yaml. The annotation is not committed because its image tags follow
# the chart's appVersion, and it is appended rather than templated because
# Chart.yaml keeps `annotations:` as its last top-level key on purpose.
#
# Usage: chart-annotate-images.sh <chart-dir> <registry-address> <registry-project>
set -euo pipefail

chart_dir="${1:?chart directory is required}"
registry_address="${2:?registry address is required}"
registry_project="${3:?registry project is required}"
chart_yaml="${chart_dir}/Chart.yaml"

# The append is only correct while `annotations:` is the final top-level key.
# Any key added after it would swallow the block, and a map-valued key would
# swallow it without a YAML error — so fail loudly here instead.
last_key=$(grep -E '^[A-Za-z]' "${chart_yaml}" | tail -1 | cut -d: -f1)
if [[ "${last_key}" != "annotations" ]]; then
  echo "Chart.yaml must keep 'annotations' as its last top-level key (found '${last_key}')." >&2
  exit 1
fi

# Both quote styles are valid YAML here, and a leftover quote would sail
# through into every image tag, so validate the result rather than trust it.
app_version="$(awk -F'[:[:space:]]+' '$1 == "appVersion" { gsub(/["'"'"']/, "", $2); print $2; exit }' "${chart_yaml}")"
if [[ ! "${app_version}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  echo "Unusable appVersion read from ${chart_yaml}: '${app_version}'" >&2
  exit 1
fi

# The Trivy adapter is not built in this repo and does not follow appVersion
# or REGISTRY_ADDRESS/REGISTRY_PROJECT: built-in mode deploys the released
# harbor-scanner-trivy image from the 8gcr preset of the harbor.image.sourceMap
# helper (registry, repos.trivy, tags.trivy; the tag is kept current by
# Renovate). Read all three so the annotation matches what the chart deploys.
scanner_image="$(awk '
  /define "harbor.image.sourceMap"/ { in_map = 1 }
  in_map && /^8gcr:/ { in_8gcr = 1; next }
  in_8gcr && /^[^[:space:]]/ { exit }
  in_8gcr && /^  registry:/ { registry = $2 }
  in_8gcr && /^    trivy: v[0-9]/ { tag = $2; next }
  in_8gcr && /^    trivy:/ { repo = $2 }
  END { print registry "/" repo ":" tag }
' "${chart_dir}/templates/_helpers.tpl")"
if [[ ! "${scanner_image}" =~ ^[a-z0-9.-]+/[a-z0-9._/-]+:v[0-9][A-Za-z0-9._-]*$ ]]; then
  echo "Unusable harbor-scanner-trivy image read from ${chart_dir}/templates/_helpers.tpl: '${scanner_image}'" >&2
  exit 1
fi

{
  echo "  artifacthub.io/images: |"
  for component in core jobservice registryctl exporter portal registry; do
    image_name="harbor-${component}"
    echo "    - name: ${image_name}"
    echo "      image: ${registry_address}/${registry_project}/${image_name}:${app_version}"
  done
  echo "    - name: harbor-scanner-trivy"
  echo "      image: ${scanner_image}"
} >> "${chart_yaml}"

echo "Annotated ${chart_yaml} with images for appVersion ${app_version}"
