#!/usr/bin/env bash
# Usage: chart-package.sh <chart-dir> <version> <dest-dir>
# Packages the chart as `8gcr` and, while users move off the old name, a
# second time as `harbor` so charts/harbor keeps receiving releases. Only the
# Chart.yaml name differs; templates, values and subcharts are the same bytes.
set -euo pipefail
chart_dir="$1" version="$2" dest="$3"
helm package "${chart_dir}" --version "${version}" --destination "${dest}"
stage="$(mktemp -d "${TMPDIR:-/tmp}/chart-legacy.XXXXXX")"
trap 'rm -rf "${stage}"' EXIT
legacy="${stage}/harbor"
cp -R "${chart_dir}" "${legacy}"
sed -i.bak 's/^name: 8gcr$/name: harbor/' "${legacy}/Chart.yaml" && rm "${legacy}/Chart.yaml.bak"
grep -qx 'name: harbor' "${legacy}/Chart.yaml" || { echo "legacy chart name patch failed" >&2; exit 1; }
helm package "${legacy}" --version "${version}" --destination "${dest}"
