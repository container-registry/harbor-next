#!/usr/bin/env bash
# Tests previous-stable-tag.sh against a throwaway repository, so the tag
# selection is pinned by something other than the release that depends on it.
#
# Usage: .github/scripts/previous-stable-tag_test.sh
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/previous-stable-tag.sh"
repo="$(mktemp -d)"
trap 'rm -rf "${repo}"' EXIT

git -c init.defaultBranch=main init --quiet "${repo}"
cd "${repo}"
git -c user.name=test -c user.email=test@example.test \
  commit --quiet --allow-empty -m "root"

# Lightweight tags regardless of the caller's git config: a signing default
# would otherwise demand a tag message and a key.
tag() { git -c tag.gpgSign=false -c tag.forceSignAnnotated=false tag "$1"; }

failures=0

check() {
  local label="$1" version="$2" want="$3" got
  got="$("${script}" "${version}")"
  if [ "${got}" = "${want}" ]; then
    printf 'ok   %s\n' "${label}"
  else
    printf 'FAIL %s: %s -> %s, want %s\n' "${label}" "${version}" "${got:-<empty>}" "${want:-<empty>}"
    failures=$((failures + 1))
  fi
}

check "no tags at all" 2.16.0 ""

for name in v2.14.0 v2.15.0 v2.15.9 v2.15.10 v2.16.0; do
  tag "${name}"
done
tag v2.16.0-nightly-20260918
tag v2.16.0-rc.1
tag chart-v2.0.0
tag not-a-release

# The newest stable below the target, prereleases and other lines ignored.
check "prerelease answers with its release's predecessor" v2.16.0-nightly-20260918 v2.15.10
check "release ignores its own prereleases" v2.16.0 v2.15.10
check "numeric, not lexical, ordering" 2.15.10 v2.15.9
check "a patch release picks the patch below it" 2.15.9 v2.15.0
check "the target itself is never its own predecessor" 2.15.0 v2.14.0
check "nothing below the oldest release" 2.14.0 ""
check "a version below every tag" 1.0.0 ""
check "a version above every tag" 3.0.0 v2.16.0
check "the v prefix is optional" v2.15.9 v2.15.0

if ! "${script}" "not-a-version" >/dev/null 2>&1; then
  printf 'ok   %s\n' "a non-version argument is rejected"
else
  printf 'FAIL %s\n' "a non-version argument was accepted"
  failures=$((failures + 1))
fi

if [ "${failures}" -ne 0 ]; then
  printf '\n%d check(s) failed\n' "${failures}" >&2
  exit 1
fi
printf '\nall checks passed\n'
