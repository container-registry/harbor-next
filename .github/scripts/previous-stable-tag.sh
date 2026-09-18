#!/usr/bin/env bash
# Prints the greatest STABLE release tag below the given version, or nothing
# when there is none.
#
# Used wherever a release's predecessor has to be pinned rather than inferred.
# GitHub's own inference counts prereleases, so once a nightly is published it
# would become the predecessor of the next real release, and that release's
# notes would cover a single day. The e2e upgrade check wants the same answer
# for the same reason: the release a user is actually upgrading from.
#
# A prerelease shares its predecessor with the release it leads up to, so
# v2.16.0-nightly-20260918 and v2.16.0 both answer with the newest stable
# release below 2.16.0.
#
# Usage: previous-stable-tag.sh 2.16.0
#        previous-stable-tag.sh v2.16.0-nightly-20260918
set -euo pipefail

version="${1:?usage: previous-stable-tag.sh <version>}"
target="${version#v}"
target="${target%%-*}"

if [[ ! "${target}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "not a version: ${version}" >&2
  exit 1
fi

# Listed first so a git failure is still an error; the filter below may
# legitimately match nothing, which is not.
tags=$(git tag --list 'v[0-9]*' --sort=-v:refname)

printf '%s\n' "${tags}" \
  | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
  | awk -v target="${target}" '
      function lower(a, b,   x, y, i) {
        split(a, x, "."); split(b, y, ".")
        for (i = 1; i <= 3; i++) {
          if (x[i] + 0 < y[i] + 0) return 1
          if (x[i] + 0 > y[i] + 0) return 0
        }
        return 0
      }
      # Descending order, so the first tag below the target is the greatest one.
      { tag = $0; sub(/^v/, "", tag); if (lower(tag, target)) { print $0; exit } }
    ' || true
