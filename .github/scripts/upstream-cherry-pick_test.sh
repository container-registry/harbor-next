#!/usr/bin/env bash
# Globals set here are read by the sourced script.
# shellcheck disable=SC2034
# Tests for the pure parts of upstream-cherry-pick.sh, with gh stubbed out.
set -euo pipefail

# shellcheck source=SCRIPTDIR/upstream-cherry-pick.sh
. "$(dirname "$0")/upstream-cherry-pick.sh"

failures=0
work=$(mktemp -d)
trap 'rm -rf "${work}"' EXIT
export RUNNER_TEMP="${work}"
GH_REPO=example/repo

check() {
  local name=$1
  local want=$2
  local got=$3

  if [ "${got}" != "${want}" ]; then
    printf 'FAIL %s\n  want: %s\n  got:  %s\n' "${name}" "${want}" "${got}"
    failures=$((failures + 1))
  else
    printf 'ok   %s\n' "${name}"
  fi
}

reset_deferred() {
  deferred_shas=()
  deferred_branches=()
  deferred_reasons=()
  deferred_titles=()
}

long=$(printf 'x%.0s' {1..120})
suffix=" (goharbor/harbor#12345)"

check "fit: short subject unchanged" "fix: a (goharbor/harbor#1)" "$(fit_pr_subject "fix: a" " (goharbor/harbor#1)")"
check "fit: short subject without PR" "fix: a" "$(fit_pr_subject "fix: a" "")"
got=$(fit_pr_subject "${long}" "${suffix}")
check "fit: long subject keeps PR suffix" "${suffix}" "${got: -${#suffix}}"
check "fit: long subject is 100 chars" "100" "${#got}"
check "fit: long subject gets ellipsis" "..." "$(sed -E 's/.*(\.\.\.) \(.*/\1/' <<<"${got}")"
got=$(fit_pr_subject "${long}" "")
check "fit: long subject without PR is 100 chars" "100" "${#got}"
check "fit: long subject without PR ends in ellipsis" "..." "${got: -3}"
got=$(fit_pr_subject "${long}" " $(printf 'y%.0s' {1..99})")
check "fit: oversized suffix falls back to plain cut" "${long:0:100}" "${got}"

check "md_cell: escapes pipes and newlines" 'a \| b c' "$(md_cell $'a | b\nc')"

reset_deferred
DRY_RUN=true
defer_pick "upstream/a" "abc123" "upstream: fix a | b" "conflicted pick" >/dev/null
GITHUB_STEP_SUMMARY="${work}/summary.md"
report_deferred >/dev/null
check "dry-run: summary says would park" "Would park 1 upstream pick(s)" "$(head -1 "${work}/summary.md" | cut -d' ' -f1-5)"
check "dry-run: title pipe escaped in table" "1" "$(grep -c 'upstream: fix a \\| b' "${work}/summary.md")"

DRY_RUN=false
push_branch() { :; }
GITHUB_STEP_SUMMARY=/dev/null

gh() {
  printf '%s\n' "$*" >>"${work}/gh.log"
  case "$1 $2" in
    "issue list") [ "${GH_LIST_FAIL:-}" = "true" ] && return 1; printf '%s\n' "${GH_LIST_OUT:-}" ;;
    "issue create") printf 'https://github.com/example/repo/issues/77\n' ;;
    "issue comment") [ "${GH_COMMENT_FAIL:-}" = "true" ] && return 1; return 0 ;;
    *) return 0 ;;
  esac
}

run_report() {
  : >"${work}/gh.log"
  reset_deferred
  defer_pick "upstream/b" "def456" "upstream: fix b" "\`.github/\` only" >/dev/null
  report_deferred >/dev/null 2>&1 && echo 0 || echo 1
}

GH_LIST_FAIL=true
check "lookup failure: report fails" "1" "$(run_report)"
check "lookup failure: no issue created" "0" "$(grep -c '^issue create' "${work}/gh.log" || true)"
GH_LIST_FAIL=false

GH_LIST_OUT=""
check "no issue yet: report succeeds" "0" "$(run_report)"
check "no issue yet: issue created" "1" "$(grep -c '^issue create' "${work}/gh.log" || true)"
check "no issue yet: comment on new issue" "1" "$(grep -c '^issue comment 77 ' "${work}/gh.log" || true)"

GH_LIST_OUT="12"
check "existing issue: comment on it" "0" "$(run_report)"
check "existing issue: no new issue" "0" "$(grep -c '^issue create' "${work}/gh.log" || true)"

GH_COMMENT_FAIL=true
check "comment failure: report fails" "1" "$(run_report)"
GH_COMMENT_FAIL=false

if [ "${failures}" -ne 0 ]; then
  echo "${failures} failure(s)"
  exit 1
fi
echo "all passed"
