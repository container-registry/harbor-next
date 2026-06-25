#!/usr/bin/env bash
set -euo pipefail

BASE_BRANCH="${BASE_BRANCH:-main}"
BOOTSTRAP_SHA="${BOOTSTRAP_SHA:-c500bbb758ee41a7c8a6ae382583016804a4129f}"
DRY_RUN="${DRY_RUN:-false}"
TARGET_REMOTE="${TARGET_REMOTE:-origin}"
UPSTREAM_BRANCH="${UPSTREAM_BRANCH:-main}"
UPSTREAM_REMOTE="${UPSTREAM_REMOTE:-upstream}"
UPSTREAM_REPO="${UPSTREAM_REPO:-goharbor/harbor}"
GH_REPO="${GH_REPO:-${GITHUB_REPOSITORY:-}}"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "::error::$1 is required."
    exit 1
  fi
}

short_sha() {
  git rev-parse --short=9 "$1"
}

tmp_file() {
  local tmp_dir
  tmp_dir="${RUNNER_TEMP:-$(git rev-parse --git-path tmp)}"
  mkdir -p "${tmp_dir}"
  mktemp "${tmp_dir}/upstream-cherry-pick.XXXXXX"
}

remote_branch_exists() {
  git ls-remote --exit-code --heads "${TARGET_REMOTE}" "$1" >/dev/null 2>&1
}

search_pr_marker() {
  local sha=$1
  gh api -X GET search/issues \
    -f "q=repo:${GH_REPO} is:pr in:body \"Upstream-Commit: ${sha}\"" \
    --jq '.total_count'
}

stage_cherry_pick_paths() {
  local sha=$1
  local path
  local paths=()

  git add -u
  mapfile -d '' -t paths < <(git diff-tree --no-commit-id --name-only -r -z "${sha}")
  for path in "${paths[@]}"; do
    if [ -e "${path}" ] || [ -L "${path}" ]; then
      git add -- "${path}"
    fi
  done
}

handled_in_git_log() {
  local sha=$1
  git log "${TARGET_REMOTE}/${BASE_BRANCH}" -F --grep="${sha}" --format=%H -n 1 | grep -q .
}

is_handled() {
  local branch=$1
  local sha=$2
  local pr_count

  if remote_branch_exists "${branch}"; then
    return 0
  fi

  pr_count=$(search_pr_marker "${sha}")
  if [ "${pr_count}" -gt 0 ]; then
    return 0
  fi

  handled_in_git_log "${sha}"
}

single_parent_commit() {
  local parents
  parents=$(git show -s --format=%P "$1")
  [ -n "${parents}" ] && [[ "${parents}" != *" "* ]]
}

upstream_pr_number() {
  local subject=$1
  if [[ "${subject}" =~ \(\#([0-9]+)\)$ ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
  fi
}

strip_pr_suffix() {
  local subject=$1
  local pr_number=$2
  local suffix

  if [ -n "${pr_number}" ]; then
    suffix=" (#${pr_number})"
    subject="${subject%${suffix}}"
  fi

  printf '%s\n' "${subject}"
}

trim_body() {
  local body=$1
  local max=4000

  if [ "${#body}" -le "${max}" ]; then
    printf '%s\n' "${body}"
    return
  fi

  printf '%s\n\n[Upstream PR body truncated.]\n' "${body:0:${max}}"
}

load_upstream_pr() {
  local pr_number=$1

  if [ -z "${pr_number}" ]; then
    return 1
  fi

  gh pr view "${pr_number}" \
    --repo "${UPSTREAM_REPO}" \
    --json author,body,mergedBy,reviews,title,url
}

jq_value() {
  local json=$1
  local filter=$2

  if [ -z "${json}" ]; then
    return 0
  fi

  jq -r "${filter}" <<<"${json}"
}

build_commit_message() {
  local file=$1
  local sha=$2
  local subject=$3
  local original_body=$4
  local status=$5
  local pr_number=$6
  local author=$7

  {
    printf 'upstream: %s\n\n' "${subject}"
    if [ -n "${original_body}" ]; then
      printf '%s\n\n' "${original_body}"
    fi
    printf '(cherry picked from commit %s)\n\n' "${sha}"
    printf 'Upstream-Commit: %s\n' "${sha}"
    if [ -n "${pr_number}" ]; then
      printf 'Upstream-PR: %s#%s\n' "${UPSTREAM_REPO}" "${pr_number}"
    fi
    printf 'Upstream-Author: %s\n' "${author}"
    printf 'Cherry-Pick-Status: %s\n' "${status}"
  } >"${file}"
}

build_pr_body() {
  local file=$1
  local sha=$2
  local upstream_short=$3
  local pr_number=$4
  local upstream_title=$5
  local author=$6
  local merged_by=$7
  local approvals=$8
  local upstream_url=$9
  local status=${10}
  local description=${11}
  local run_url="${GITHUB_SERVER_URL:-https://github.com}/${GH_REPO}/actions/runs/${GITHUB_RUN_ID:-local}"

  {
    if [ "${status}" = "conflicted" ]; then
      printf '> This cherry-pick had conflicts and was opened as a draft. Conflict markers may be present in the diff and must be resolved before marking ready for review.\n\n'
    fi

    printf '## Summary\n\n'
    if [ -n "${pr_number}" ]; then
      printf 'Cherry-picks upstream Harbor commit `%s` from %s#%s.\n\n' "${upstream_short}" "${UPSTREAM_REPO}" "${pr_number}"
    else
      printf 'Cherry-picks upstream Harbor commit `%s`.\n\n' "${upstream_short}"
    fi

    printf '## Upstream Context\n\n'
    if [ -n "${pr_number}" ]; then
      printf -- '- Upstream PR: %s#%s\n' "${UPSTREAM_REPO}" "${pr_number}"
    else
      printf -- '- Upstream PR: unavailable\n'
    fi
    printf -- '- Upstream commit: %s\n' "${sha}"
    printf -- '- Upstream title: %s\n' "${upstream_title}"
    printf -- '- Upstream author: %s\n' "${author}"
    printf -- '- Upstream merged by: %s\n' "${merged_by:-unavailable}"
    printf -- '- Upstream approved by: %s\n' "${approvals:-unavailable}"
    if [ -n "${upstream_url}" ]; then
      printf -- '- Upstream PR URL: %s\n' "${upstream_url}"
    fi
    printf -- '- Upstream commit URL: https://github.com/%s/commit/%s\n\n' "${UPSTREAM_REPO}" "${sha}"

    printf '## Cherry-Pick Status\n\n'
    printf -- '- Status: %s\n' "${status}"
    printf -- '- Base branch: %s\n' "${BASE_BRANCH}"
    printf -- '- Workflow run: %s\n\n' "${run_url}"

    printf '## Upstream Description\n\n'
    if [ -n "${description}" ]; then
      trim_body "${description}"
    else
      printf 'No upstream PR description available.\n'
    fi
    printf '\n## Review Notes\n\n'
    printf -- '- Generated by the upstream cherry-pick workflow.\n'
    printf -- '- One upstream commit maps to one Harbor Next PR.\n'
    printf -- '- If this PR is closed without merge, the workflow will not recreate it.\n\n'
    printf 'Upstream-Commit: %s\n' "${sha}"
    if [ -n "${pr_number}" ]; then
      printf 'Upstream-PR: %s#%s\n' "${UPSTREAM_REPO}" "${pr_number}"
    fi
    printf 'Cherry-Pick-Status: %s\n' "${status}"
  } >"${file}"
}

open_pr() {
  local branch=$1
  local title=$2
  local body_file=$3
  local status=$4
  local pr_url

  if [ "${DRY_RUN}" = "true" ]; then
    echo "dry-run: would push ${branch} and open PR: ${title}"
    return 0
  fi

  git push "${TARGET_REMOTE}" "${branch}"

  if [ "${status}" = "conflicted" ]; then
    pr_url=$(gh pr create --base "${BASE_BRANCH}" --head "${branch}" --title "${title}" --body-file "${body_file}" --draft)
  else
    pr_url=$(gh pr create --base "${BASE_BRANCH}" --head "${branch}" --title "${title}" --body-file "${body_file}")
  fi

  for label in upstream automated; do
    gh pr edit "${pr_url}" --add-label "${label}" >/dev/null 2>&1 || true
  done
  if [ "${status}" = "conflicted" ]; then
    gh pr edit "${pr_url}" --add-label needs-conflict-resolution >/dev/null 2>&1 || true
  fi

  echo "opened ${pr_url}"
}

cherry_pick_commit() {
  local sha=$1
  local subject
  local pr_number
  local upstream_subject
  local upstream_short
  local branch
  local pr_json=""
  local pr_title=""
  local pr_body=""
  local upstream_author=""
  local upstream_merged_by=""
  local upstream_approvals=""
  local upstream_url=""
  local git_author
  local status="clean"
  local commit_message
  local pr_body_file
  local pr_title_final

  subject=$(git show -s --format=%s "${sha}")
  pr_number=$(upstream_pr_number "${subject}")
  upstream_subject=$(strip_pr_suffix "${subject}" "${pr_number}")
  upstream_short=$(short_sha "${sha}")
  branch="upstream/cherry-pick-${pr_number:-no-pr}-${upstream_short}"

  if is_handled "${branch}" "${sha}"; then
    echo "handled: ${sha}"
    return 0
  fi

  if pr_json=$(load_upstream_pr "${pr_number}" 2>/dev/null); then
    pr_title=$(jq_value "${pr_json}" '.title // empty')
    pr_body=$(jq_value "${pr_json}" '.body // empty')
    upstream_author=$(jq_value "${pr_json}" 'if .author.login then "@" + .author.login else empty end')
    upstream_merged_by=$(jq_value "${pr_json}" 'if .mergedBy.login then "@" + .mergedBy.login else empty end')
    upstream_approvals=$(jq_value "${pr_json}" '[.reviews[]? | select(.state == "APPROVED") | .author.login] | unique | map("@" + .) | join(", ")')
    upstream_url=$(jq_value "${pr_json}" '.url // empty')
  fi

  git_author=$(git show -s --format='%an <%ae>' "${sha}")
  upstream_author="${upstream_author:-${git_author}}"
  upstream_subject="${pr_title:-${upstream_subject}}"

  if [ -n "${pr_number}" ]; then
    pr_title_final="upstream: Cherry-Pick ${upstream_subject} (${UPSTREAM_REPO}#${pr_number})"
  else
    pr_title_final="upstream: Cherry-Pick ${upstream_subject}"
  fi

  echo "candidate: ${sha} ${upstream_subject}"

  git switch -C "${branch}" "${TARGET_REMOTE}/${BASE_BRANCH}"

  if ! git cherry-pick --no-commit "${sha}"; then
    status="conflicted"
    stage_cherry_pick_paths "${sha}"
  fi

  if git diff --cached --quiet; then
    echo "empty: ${sha}"
    git cherry-pick --abort >/dev/null 2>&1 || true
    git cherry-pick --quit >/dev/null 2>&1 || true
    git switch -C "${BASE_BRANCH}" "${TARGET_REMOTE}/${BASE_BRANCH}"
    return 0
  fi

  commit_message=$(tmp_file)
  pr_body_file=$(tmp_file)
  build_commit_message "${commit_message}" "${sha}" "${upstream_subject}" "$(git show -s --format=%b "${sha}")" "${status}" "${pr_number}" "${upstream_author}"
  build_pr_body "${pr_body_file}" "${sha}" "${upstream_short}" "${pr_number}" "${upstream_subject}" "${upstream_author}" "${upstream_merged_by}" "${upstream_approvals}" "${upstream_url}" "${status}" "${pr_body}"

  git commit -s -F "${commit_message}"
  git cherry-pick --quit >/dev/null 2>&1 || true

  open_pr "${branch}" "${pr_title_final}" "${pr_body_file}" "${status}"

  rm -f "${commit_message}" "${pr_body_file}"
  git switch -C "${BASE_BRANCH}" "${TARGET_REMOTE}/${BASE_BRANCH}"
}

main() {
  local line
  local marker
  local sha
  local candidates=()

  require gh
  require git
  require grep
  require jq
  require mktemp

  if [ -z "${GH_REPO}" ]; then
    echo "::error::GH_REPO or GITHUB_REPOSITORY is required."
    exit 1
  fi

  git config user.name "github-actions[bot]"
  git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
  gh auth setup-git

  if git remote get-url "${UPSTREAM_REMOTE}" >/dev/null 2>&1; then
    git remote set-url "${UPSTREAM_REMOTE}" "https://github.com/${UPSTREAM_REPO}.git"
  else
    git remote add "${UPSTREAM_REMOTE}" "https://github.com/${UPSTREAM_REPO}.git"
  fi

  git fetch "${TARGET_REMOTE}" "${BASE_BRANCH}" --prune
  git fetch "${UPSTREAM_REMOTE}" "${UPSTREAM_BRANCH}" --prune
  git switch -C "${BASE_BRANCH}" "${TARGET_REMOTE}/${BASE_BRANCH}"

  while read -r marker sha _; do
    if [ "${marker}" != "+" ]; then
      continue
    fi
    candidates+=("${sha}")
  done < <(git cherry -v "${TARGET_REMOTE}/${BASE_BRANCH}" "${UPSTREAM_REMOTE}/${UPSTREAM_BRANCH}" "${BOOTSTRAP_SHA}")

  echo "candidates: ${#candidates[@]}"

  for sha in "${candidates[@]}"; do
    if ! single_parent_commit "${sha}"; then
      echo "skip merge commit: ${sha}"
      continue
    fi
    cherry_pick_commit "${sha}"
  done
}

main "$@"
