#!/usr/bin/env bash
set -euo pipefail

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: required command not found in agent image: $1" >&2
    exit 1
  fi
}

safe_ref_fragment() {
  printf '%s' "$1" | tr -c 'A-Za-z0-9._/-' '-' | tr -s '-' | sed 's#^/##; s#/$##'
}

list_forbidden_changes() {
  local path
  while IFS= read -r -d '' path; do
    case "${path}" in
      8gcr-ee/patches/*) ;;
      *) printf '%s\n' "${path}"; return 0 ;;
    esac
  done < <(git diff --name-only -z HEAD --)

  while IFS= read -r -d '' path; do
    case "${path}" in
      8gcr-ee/patches/*) ;;
      *) printf '%s\n' "${path}"; return 0 ;;
    esac
  done < <(git ls-files --others --exclude-standard -z)

  return 1
}

create_or_update_pr() {
  local final_message_file=$1
  local basic_auth

  git diff "origin/${TARGET_BRANCH}..HEAD" --binary > /agent-run/pr.diff
  git log --oneline "origin/${TARGET_BRANCH}..HEAD" > /agent-run/commits.txt
  basic_auth=$(printf 'x-access-token:%s' "${GITHUB_TOKEN}" | base64 | tr -d '\n')
  git -c "http.https://github.com/.extraheader=AUTHORIZATION: basic ${basic_auth}" push --force-with-lease -u origin "${agent_branch}"

  cat > /agent-run/pr-body.md <<EOF
Automated upstream sync conflict resolution.

Failed workflow: ${failed_run_url}
Target branch: ${TARGET_BRANCH}
Agent branch: ${agent_branch}

The agent run uploaded its handoff bundle, logs, final diff, validation log, and structured report as workflow artifacts.

## Agent Summary

$(cat "${final_message_file}" 2>/dev/null || true)
EOF

  pr_url=$(gh pr list \
    --repo "${GITHUB_REPOSITORY}" \
    --head "${agent_branch}" \
    --json url \
    --jq '.[0].url')

  if [ -z "${pr_url}" ]; then
    pr_url=$(gh pr create \
      --repo "${GITHUB_REPOSITORY}" \
      --base "${TARGET_BRANCH}" \
      --head "${agent_branch}" \
      --title "fix: Resolve upstream sync conflict" \
      --body-file /agent-run/pr-body.md \
      --draft)
  else
    gh pr edit "${pr_url}" --body-file /agent-run/pr-body.md
  fi

  printf '%s\n' "${pr_url}" > /agent-run/pr-url.txt
}

restore_target_path() {
  local pre_merge_sha=$1
  local path=$2

  if git cat-file -e "${pre_merge_sha}:${path}" 2>/dev/null; then
    git checkout --ours -- "${path}"
    git add "${path}"
  else
    git rm -f -- "${path}"
  fi
}

resolve_sync_merge_conflict() {
  local pre_merge_sha conflicts unsupported path sync_subject

  git config rerere.enabled true
  git config rerere.autoUpdate true
  git config merge.ours.driver true
  rr_cache_dir="$(git rev-parse --git-path rr-cache)"
  if [ -d "8gcr-ee/.rr-cache" ]; then
    mkdir -p "${rr_cache_dir}"
    cp -r 8gcr-ee/.rr-cache/* "${rr_cache_dir}"/ 2>/dev/null || true
  fi

  pre_merge_sha=$(git rev-parse HEAD)
  sync_subject="chore: sync ${upstream_remote}/${upstream_branch}"
  if git merge --squash "${upstream_remote}/${upstream_branch}"; then
    if ! git diff --cached --quiet; then
      git commit -s -m "${sync_subject}"
    fi
  else
    conflicts=$(git diff --name-only --diff-filter=U)
    unsupported=""
    while IFS= read -r path; do
      [ -n "${path}" ] || continue
      case "${path}" in
        CLAUDE.md|.github/workflows/*|.github/scripts/format-release-notes.mjs|release-please-config.json|8gcr-ee/.rr-cache/*)
          restore_target_path "${pre_merge_sha}" "${path}"
          ;;
        Taskfile.yml)
          git checkout --ours -- Taskfile.yml
          git add Taskfile.yml
          ;;
        *)
          unsupported="${unsupported}${path}"$'\n'
          ;;
      esac
    done <<EOF
${conflicts}
EOF

    if [ -n "${unsupported}" ]; then
      printf 'ERROR: unsupported merge conflicts for deterministic agent resolver:\n%s' "${unsupported}" >&2
      exit 28
    fi

    if git diff --name-only --diff-filter=U | grep -q .; then
      echo "ERROR: agent merge conflict resolution left unresolved files" >&2
      git diff --name-only --diff-filter=U >&2
      exit 28
    fi

    git commit -s -m "${sync_subject}"
  fi

  if ! git diff --quiet "${pre_merge_sha}" HEAD -- .github/workflows/ 2>/dev/null; then
    git checkout "${pre_merge_sha}" -- .github/workflows/ 2>/dev/null || true
    git diff --name-only --diff-filter=A "${pre_merge_sha}" HEAD -- .github/workflows/ | while read -r path; do
      git rm -f "${path}" 2>/dev/null || true
    done
    git commit --amend --no-edit
  fi

  cd "${patch_worktree}"
  if stg series | grep -q '^- '; then
    echo "ERROR: deterministic merge resolver cannot export an unapplied StGit stack" >&2
    exit 26
  fi
  if ! stg export -d "${repo_dir}/8gcr-ee/patches/" -O --abbrev=7 > /agent-run/merge-stg-export.log 2>&1; then
    echo "ERROR: stg export failed after deterministic merge; see merge-stg-export.log" >&2
    exit 27
  fi

  cd "${repo_dir}"
  git add -A 8gcr-ee/patches/
  if ! git diff --cached --quiet -- 8gcr-ee/patches/; then
    git commit -s -m "fix: Rebase patches on upstream main"
  fi

  if ! task sync:sanity-check > /agent-run/merge-sanity-check.log 2>&1; then
    cat > /agent-run/deterministic-merge-fallback.md <<EOF
Deterministic upstream merge conflict resolution completed, but sync:sanity-check failed.

Continue with the Codex path. Inspect /agent-run/merge-sanity-check.log, fix the repository state, and leave the patch stack ready for export and validation.
EOF
    return 1
  fi

  if ! task sync:verify-patches > /agent-run/merge-verify-patches.log 2>&1; then
    cat > /agent-run/deterministic-merge-fallback.md <<EOF
Deterministic upstream merge conflict resolution completed, but sync:verify-patches failed.

Continue with the Codex path. Inspect /agent-run/merge-verify-patches.log, resolve the patch-stack validation failure, and leave the patch stack ready for export and validation.
EOF
    return 1
  fi

  if ! task sync:save-rerere > /agent-run/merge-save-rerere.log 2>&1; then
    cat > /agent-run/deterministic-merge-fallback.md <<EOF
Deterministic upstream merge conflict resolution completed, but sync:save-rerere failed.

Continue with the Codex path. Inspect /agent-run/merge-save-rerere.log, fix the repository state, and leave the patch stack ready for export and validation.
EOF
    return 1
  fi

  if ! git diff --quiet "${pre_merge_sha}" HEAD -- .github/workflows/ 2>/dev/null; then
    echo "ERROR: merge-resolution PR would modify .github/workflows" >&2
    exit 29
  fi

  cat > /agent-run/final-message.md <<EOF
Resolved upstream sync merge conflicts in the agent workflow.

- Preserved target-branch GitHub workflow files so GITHUB_TOKEN does not push workflow changes.
- Preserved target-branch release automation and rerere cache conflicts.
- Preserved target-branch CLAUDE.md instructions.
- Preserved target-branch Taskfile private includes.
- Exported the rebased StGit patch stack before validating patch application.
- Ran task sync:sanity-check, task sync:verify-patches, and task sync:save-rerere.
EOF

  create_or_update_pr /agent-run/final-message.md

  jq -n \
    --arg status "pr_created" \
    --arg mode "sync_merge_conflict" \
    --arg repository "${GITHUB_REPOSITORY}" \
    --arg target_branch "${TARGET_BRANCH}" \
    --arg agent_branch "${agent_branch}" \
    --arg failed_run_url "${failed_run_url}" \
    --arg pr_url "${pr_url}" \
    '{status: $status, mode: $mode, repository: $repository, target_branch: $target_branch, agent_branch: $agent_branch, failed_run_url: $failed_run_url, pr_url: $pr_url}' \
    > /agent-run/agent-report.json

  echo "Created PR: ${pr_url}"
}

render_prompt() {
  local prompt
  prompt=$(< /agent-prompts/upstream-sync.md)
  prompt=${prompt//\{\{TARGET_BRANCH\}\}/${TARGET_BRANCH}}
  prompt=${prompt//\{\{AGENT_BRANCH\}\}/${agent_branch}}
  prompt=${prompt//\{\{FAILED_RUN_URL\}\}/${failed_run_url}}
  prompt=${prompt//\{\{UPSTREAM_REMOTE\}\}/${upstream_remote}}
  prompt=${prompt//\{\{UPSTREAM_URL\}\}/${upstream_url}}
  prompt=${prompt//\{\{UPSTREAM_BRANCH\}\}/${upstream_branch}}
  printf '%s\n' "${prompt}"
}

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SERVER_URL:?GITHUB_SERVER_URL is required}"
: "${GITHUB_TOKEN:?GITHUB_TOKEN is required}"
: "${TARGET_BRANCH:?TARGET_BRANCH is required}"
: "${FAILED_RUN_ID:?FAILED_RUN_ID is required}"

require git
require gh
require jq
require codex
require task
require stg

export GH_TOKEN=${GH_TOKEN:-${GITHUB_TOKEN}}

repo_dir=/workspace/repo
patch_worktree=/workspace/patch-rebase
validation_worktree=/workspace/patch-validate
context_dir=/agent-run/repo-context
branch_fragment=$(safe_ref_fragment "${TARGET_BRANCH}")
agent_branch="${AGENT_BRANCH_PREFIX:-agent/upstream-sync-patches}/${branch_fragment}/${FAILED_RUN_ID}"
failed_run_url="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${FAILED_RUN_ID}"
upstream_remote=${UPSTREAM_REMOTE:-harbor-next}
upstream_url=${UPSTREAM_URL:-https://github.com/container-registry/harbor-next.git}
upstream_branch=${UPSTREAM_BRANCH:-main}
mkdir -p "${context_dir}"

test -f /agent-prompts/upstream-sync.md || { echo "ERROR: /agent-prompts/upstream-sync.md missing" >&2; exit 1; }

git config --global user.name "${AGENT_GIT_NAME:-8gcr-ai-agent[bot]}"
git config --global user.email "${AGENT_GIT_EMAIL:-8gcr-ai-agent[bot]@users.noreply.github.com}"
gh auth setup-git --hostname github.com

git clone "https://github.com/${GITHUB_REPOSITORY}.git" "${repo_dir}"
cd "${repo_dir}"
git fetch origin "${TARGET_BRANCH}"
git checkout -B "${agent_branch}" "origin/${TARGET_BRANCH}"
git remote add "${upstream_remote}" "${upstream_url}" 2>/dev/null || git remote set-url "${upstream_remote}" "${upstream_url}"
git fetch "${upstream_remote}" "${upstream_branch}"

git worktree add -B "wip/agent-rebase-${FAILED_RUN_ID}" "${patch_worktree}" "${upstream_remote}/${upstream_branch}"
cd "${patch_worktree}"
stg init
set +e
stg import -3 -S "${repo_dir}/8gcr-ee/patches/series" > "${context_dir}/patch-import.log" 2>&1
set -e
git status --short --branch > "${context_dir}/patch-worktree-status.txt" || true
stg series > "${context_dir}/stg-series-after-import.txt" 2>&1 || true

cd "${repo_dir}"

git remote -v > "${context_dir}/remotes.txt"
git status --short --branch > "${context_dir}/initial-status.txt"
git rev-parse HEAD > "${context_dir}/target-head.txt"
git rev-parse "${upstream_remote}/${upstream_branch}" > "${context_dir}/upstream-head.txt"
git log --oneline --decorate -n 30 > "${context_dir}/target-log.txt"
git log --oneline --decorate -n 30 "${upstream_remote}/${upstream_branch}" > "${context_dir}/upstream-log.txt"
git rev-list --left-right --count "HEAD...${upstream_remote}/${upstream_branch}" > "${context_dir}/target-vs-upstream-count.txt"
task --list-all > "${context_dir}/task-list.txt"
{
  git --version
  gh --version | head -n 1
  task --version
  stg --version
  codex --version
} > "${context_dir}/tool-versions.txt" 2>&1 || true

if grep -q "ERROR: Merge conflict with ${upstream_remote}/${upstream_branch}" /agent-run/failed-workflow.log 2>/dev/null; then
  if resolve_sync_merge_conflict; then
    exit 0
  fi
fi

render_prompt > /agent-run/agent-prompt.md

cd "${patch_worktree}"
set +e
codex exec \
  --sandbox "${AGENT_CODEX_SANDBOX:-danger-full-access}" \
  --json \
  --output-last-message /agent-run/final-message.md \
  - < /agent-run/agent-prompt.md \
  > /agent-run/agent-output.jsonl \
  2> /agent-run/agent-stderr.log
codex_status=$?
set -e

if [ "${codex_status}" -ne 0 ]; then
  jq -n \
    --arg status "codex_failed" \
    --argjson codex_exit_code "${codex_status}" \
    --arg repository "${GITHUB_REPOSITORY}" \
    --arg target_branch "${TARGET_BRANCH}" \
    --arg agent_branch "${agent_branch}" \
    --arg failed_run_url "${failed_run_url}" \
    '{status: $status, codex_exit_code: $codex_exit_code, repository: $repository, target_branch: $target_branch, agent_branch: $agent_branch, failed_run_url: $failed_run_url}' \
    > /agent-run/agent-report.json
  exit "${codex_status}"
fi

if stg series | grep -q '^- '; then
  echo "ERROR: agent left unapplied StGit patches" >&2
  exit 26
fi

if ! stg export -d "${repo_dir}/8gcr-ee/patches/" -O --abbrev=7 > /agent-run/stg-export.log 2>&1; then
  echo "ERROR: stg export failed; see stg-export.log in the agent artifacts" >&2
  exit 27
fi

cd "${repo_dir}"
git status --porcelain=v1 > /agent-run/git-status-after-codex.txt || true
git diff --binary > /agent-run/final.diff || true

if forbidden_path=$(list_forbidden_changes); then
  git diff --binary > /agent-run/forbidden-non-patch-diff.patch || true
  printf '%s\n' "${forbidden_path}" > /agent-run/forbidden-non-patch-paths.txt
  echo "ERROR: agent modified files outside 8gcr-ee/patches, refusing to continue" >&2
  exit 23
fi

if git diff --quiet && git diff --cached --quiet; then
  if [ -f /agent-run/deterministic-merge-fallback.md ] && [ "$(git rev-list --count "origin/${TARGET_BRANCH}..HEAD")" -gt 0 ]; then
    :
  else
    echo "ERROR: agent produced no patch artifact changes" >&2
    exit 24
  fi
fi

git worktree add -B "wip/agent-validate-${FAILED_RUN_ID}" "${validation_worktree}" "${upstream_remote}/${upstream_branch}"
cd "${validation_worktree}"
stg init
if ! stg import -3 -S "${repo_dir}/8gcr-ee/patches/series" > /agent-run/patch-validation.log 2>&1; then
  echo "ERROR: exported patches do not apply cleanly to ${upstream_remote}/${upstream_branch}" >&2
  exit 25
fi

cd "${repo_dir}"

if ! git diff --quiet || ! git diff --cached --quiet; then
  git add -A
  git commit -s -m "fix: Resolve upstream sync conflict"
fi

if [ -f /agent-run/deterministic-merge-fallback.md ]; then
  if ! task sync:verify-patches > /agent-run/post-codex-merge-verify-patches.log 2>&1; then
    echo "ERROR: post-Codex merge validation failed; see post-codex-merge-verify-patches.log" >&2
    exit 30
  fi
  if ! task sync:save-rerere > /agent-run/post-codex-merge-save-rerere.log 2>&1; then
    echo "ERROR: post-Codex rerere save failed; see post-codex-merge-save-rerere.log" >&2
    exit 31
  fi
fi

ahead_count=$(git rev-list --count "origin/${TARGET_BRANCH}..HEAD")
if [ "${ahead_count}" -eq 0 ]; then
  echo "ERROR: agent produced no commits" >&2
  exit 24
fi

validation_status=0
if [ -n "${AGENT_VALIDATION_COMMAND:-}" ]; then
  set +e
  bash -lc "${AGENT_VALIDATION_COMMAND}" > /agent-run/validation.log 2>&1
  validation_status=$?
  set -e
fi

if [ "${validation_status}" -ne 0 ]; then
  echo "ERROR: validation failed; refusing to push PR branch" >&2
  exit "${validation_status}"
fi

create_or_update_pr /agent-run/final-message.md

jq -n \
  --arg status "pr_created" \
  --arg repository "${GITHUB_REPOSITORY}" \
  --arg target_branch "${TARGET_BRANCH}" \
  --arg agent_branch "${agent_branch}" \
  --arg failed_run_url "${failed_run_url}" \
  --arg pr_url "${pr_url}" \
  --arg validation_command "${AGENT_VALIDATION_COMMAND:-}" \
  '{status: $status, repository: $repository, target_branch: $target_branch, agent_branch: $agent_branch, failed_run_url: $failed_run_url, pr_url: $pr_url, validation_command: $validation_command}' \
  > /agent-run/agent-report.json

echo "Created PR: ${pr_url}"
