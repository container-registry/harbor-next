#!/usr/bin/env bash

set -euo pipefail

mode="${1:?usage: list-go-test-packages.sh <pure|db-tagged> [exclude-regex]}"
exclude_regex="${2:-^$}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}/src"

case "${mode}" in
  pure)
    go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... \
      | awk 'NF' \
      | awk -v re="${exclude_regex}" '$0 !~ re'
    ;;
  db-tagged)
    pkg_dirs=()

    if command -v rg >/dev/null 2>&1; then
      mapfile -t pkg_dirs < <(
        { rg -l '^//go:build db$' . --glob '**/*_test.go' || true; } \
          | sed 's#^\./##; s#/[^/]*$##' \
          | sort -u
      )
    else
      mapfile -t pkg_dirs < <(
        { find . -type f -name '*_test.go' -print0 \
          | xargs -0 -r grep -l '^//go:build db$' || true; } \
          | sed 's#^\./##; s#/[^/]*$##' \
          | sort -u
      )
    fi

    if [ "${#pkg_dirs[@]}" -eq 0 ]; then
      exit 0
    fi

    go list -tags db "${pkg_dirs[@]/#/./}" \
      | awk 'NF' \
      | awk -v re="${exclude_regex}" '$0 !~ re'
    ;;
  *)
    echo "unknown mode: ${mode}" >&2
    exit 1
    ;;
esac
