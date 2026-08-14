#!/usr/bin/env bash
set -euo pipefail

run_dir=${1:?usage: prepare-agent-handoff.sh <run-dir>}
mkdir -p "${run_dir}"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: required command not found: $1" >&2
    exit 1
  fi
}

require gh
require jq

: "${FAILED_RUN_ID:?FAILED_RUN_ID is required}"
: "${TARGET_BRANCH:?TARGET_BRANCH is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SERVER_URL:?GITHUB_SERVER_URL is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
: "${GITHUB_RUN_ATTEMPT:?GITHUB_RUN_ATTEMPT is required}"

failed_run_url="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${FAILED_RUN_ID}"
agent_run_url="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
correlation_id="agent-${FAILED_RUN_ID}-${GITHUB_RUN_ATTEMPT}"

gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${FAILED_RUN_ID}" > "${run_dir}/failed-run.json"
gh api --paginate "repos/${GITHUB_REPOSITORY}/actions/runs/${FAILED_RUN_ID}/jobs" > "${run_dir}/failed-jobs.json"
gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${FAILED_RUN_ID}/artifacts" > "${run_dir}/failed-artifacts.json"

if ! gh run view "${FAILED_RUN_ID}" \
  --repo "${GITHUB_REPOSITORY}" \
  --json conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,jobs,name,status,updatedAt,url,workflowName \
  > "${run_dir}/failed-run-view.json"; then
  echo "{}" > "${run_dir}/failed-run-view.json"
fi

if ! gh run view "${FAILED_RUN_ID}" --repo "${GITHUB_REPOSITORY}" --log > "${run_dir}/failed-workflow.log"; then
  echo "gh run view --log failed; preserving metadata only" > "${run_dir}/failed-workflow.log"
fi

if ! gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${FAILED_RUN_ID}/logs" > "${run_dir}/failed-workflow-logs.zip"; then
  rm -f "${run_dir}/failed-workflow-logs.zip"
fi

jq -n \
  --arg correlation_id "${correlation_id}" \
  --arg repository "${GITHUB_REPOSITORY}" \
  --arg target_branch "${TARGET_BRANCH}" \
  --arg failed_run_id "${FAILED_RUN_ID}" \
  --arg failed_run_url "${failed_run_url}" \
  --arg failed_head_sha "${FAILED_HEAD_SHA:-}" \
  --arg agent_run_id "${GITHUB_RUN_ID}" \
  --arg agent_run_attempt "${GITHUB_RUN_ATTEMPT}" \
  --arg agent_run_url "${agent_run_url}" \
  '{
    correlation_id: $correlation_id,
    repository: $repository,
    target_branch: $target_branch,
    failed_run_id: $failed_run_id,
    failed_run_url: $failed_run_url,
    failed_head_sha: $failed_head_sha,
    agent_run_id: $agent_run_id,
    agent_run_attempt: $agent_run_attempt,
    agent_run_url: $agent_run_url
  }' > "${run_dir}/handoff.json"

cat > "${run_dir}/README.md" <<EOF
# Upstream Sync Agent Handoff

- Correlation ID: \`${correlation_id}\`
- Repository: \`${GITHUB_REPOSITORY}\`
- Target branch: \`${TARGET_BRANCH}\`
- Failed run: ${failed_run_url}
- Agent run: ${agent_run_url}

Files:

- \`handoff.json\`: machine-readable run context.
- \`failed-run.json\`: GitHub Actions workflow run metadata.
- \`failed-run-view.json\`: GitHub CLI run view with jobs and step summaries where available.
- \`failed-jobs.json\`: GitHub Actions jobs metadata.
- \`failed-artifacts.json\`: Failed run artifact metadata.
- \`failed-workflow.log\`: failed workflow log output when available.
- \`failed-workflow-logs.zip\`: raw downloaded logs archive when available.
EOF

echo "Prepared handoff bundle at ${run_dir}"
