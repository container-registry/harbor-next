#!/usr/bin/env bash
set -euo pipefail

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: required command not found: $1" >&2
    exit 1
  fi
}

write_env_file() {
  local env_file=$1
  : > "${env_file}"
  chmod 600 "${env_file}"
  {
    printf 'GITHUB_REPOSITORY=%s\n' "${GITHUB_REPOSITORY}"
    printf 'GITHUB_SERVER_URL=%s\n' "${GITHUB_SERVER_URL}"
    printf 'GITHUB_TOKEN=%s\n' "${AGENT_GITHUB_TOKEN}"
    printf 'GH_TOKEN=%s\n' "${AGENT_GITHUB_TOKEN}"
    printf 'TARGET_BRANCH=%s\n' "${TARGET_BRANCH}"
    printf 'FAILED_RUN_ID=%s\n' "${FAILED_RUN_ID}"
    printf 'FAILED_HEAD_SHA=%s\n' "${FAILED_HEAD_SHA:-}"
    printf 'UPSTREAM_REMOTE=%s\n' "${UPSTREAM_REMOTE:-harbor-next}"
    printf 'UPSTREAM_URL=%s\n' "${UPSTREAM_URL:-https://github.com/container-registry/harbor-next.git}"
    printf 'UPSTREAM_BRANCH=%s\n' "${UPSTREAM_BRANCH:-main}"
    printf 'AGENT_BRANCH_PREFIX=%s\n' "${AGENT_BRANCH_PREFIX:-agent/upstream-sync-patches}"
    printf 'AGENT_CODEX_SANDBOX=%s\n' "${AGENT_CODEX_SANDBOX:-danger-full-access}"
    printf 'AGENT_VALIDATION_COMMAND=%s\n' "${AGENT_VALIDATION_COMMAND:-}"
    printf 'AGENT_GIT_NAME=%s\n' "${AGENT_GIT_NAME:-8gcr-ai-agent[bot]}"
    printf 'AGENT_GIT_EMAIL=%s\n' "${AGENT_GIT_EMAIL:-8gcr-ai-agent[bot]@users.noreply.github.com}"
    printf 'HOME=%s\n' "/agent-home"
    printf 'CODEX_HOME=%s\n' "/codex-home"
    if [ -n "${CODEX_API_KEY:-}" ]; then
      printf 'CODEX_API_KEY=%s\n' "${CODEX_API_KEY}"
    fi
  } >> "${env_file}"
}

: "${AGENT_RUN_DIR:?AGENT_RUN_DIR is required}"
: "${AGENT_GITHUB_TOKEN:?AGENT_GITHUB_TOKEN is required}"
: "${AGENT_IMAGE:?Set vars.AGENT_IMAGE to the OCI image that contains git, gh, task, stg, and codex}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${TARGET_BRANCH:?TARGET_BRANCH is required}"
: "${FAILED_RUN_ID:?FAILED_RUN_ID is required}"

backend=${AGENT_RUNTIME_BACKEND:-docker}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "${script_dir}/../../.." && pwd)
prompt_dir="${repo_root}/.github/agents/prompts"
runtime_dir="${AGENT_RUN_DIR}.runtime"
env_file="${runtime_dir}/agent.env"
codex_home="${runtime_dir}/codex-home"
agent_home="${runtime_dir}/home"
workspace_dir="${runtime_dir}/workspace"
mkdir -p "${codex_home}" "${agent_home}" "${workspace_dir}"
chmod 700 "${codex_home}" "${agent_home}" "${workspace_dir}"
trap 'rm -rf "${runtime_dir}"' EXIT

if [ -n "${CODEX_AUTH_JSON:-}" ]; then
  printf '%s' "${CODEX_AUTH_JSON}" > "${codex_home}/auth.json"
  chmod 600 "${codex_home}/auth.json"
fi

write_env_file "${env_file}"
if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
  echo "::add-mask::${AGENT_GITHUB_TOKEN}"
fi

case "${backend}" in
  podman)
    require podman
    podman run --rm \
      --name "agent-upstream-sync-${FAILED_RUN_ID}-${GITHUB_RUN_ATTEMPT:-1}" \
      --userns=keep-id \
      --user "$(id -u):$(id -g)" \
      --cap-drop=ALL \
      --security-opt=no-new-privileges \
      --pids-limit="${AGENT_PIDS_LIMIT:-2048}" \
      --memory="${AGENT_MEMORY:-8g}" \
      --cpus="${AGENT_CPUS:-4}" \
      --network="${AGENT_PODMAN_NETWORK:-slirp4netns}" \
      --env-file "${env_file}" \
      -v "${AGENT_RUN_DIR}:/agent-run:Z" \
      -v "${agent_home}:/agent-home:Z" \
      -v "${codex_home}:/codex-home:Z" \
      -v "${workspace_dir}:/workspace:Z" \
      -v "${script_dir}:/agent-scripts:ro,Z" \
      -v "${prompt_dir}:/agent-prompts:ro,Z" \
      -w /workspace \
      "${AGENT_IMAGE}" \
      /agent-scripts/agent-entrypoint.sh
    ;;
  docker)
    require docker
    container_id=$(docker create \
      --name "agent-upstream-sync-${FAILED_RUN_ID}-${GITHUB_RUN_ATTEMPT:-1}" \
      --user "$(id -u):$(id -g)" \
      --cap-drop=ALL \
      --security-opt=no-new-privileges \
      --pids-limit="${AGENT_PIDS_LIMIT:-2048}" \
      --memory="${AGENT_MEMORY:-8g}" \
      --cpus="${AGENT_CPUS:-4}" \
      --network="${AGENT_DOCKER_NETWORK:-bridge}" \
      --env-file "${env_file}" \
      "${AGENT_IMAGE}" \
      bash -lc 'mkdir -p /workspace && /agent-scripts/agent-entrypoint.sh')
    trap 'docker rm -f "${container_id}" >/dev/null 2>&1 || true; rm -rf "${runtime_dir}"' EXIT

    docker cp "${AGENT_RUN_DIR}/." "${container_id}:/agent-run"
    docker cp "${agent_home}/." "${container_id}:/agent-home"
    docker cp "${codex_home}/." "${container_id}:/codex-home"
    docker cp "${workspace_dir}/." "${container_id}:/workspace"
    docker cp "${script_dir}/." "${container_id}:/agent-scripts"
    docker cp "${prompt_dir}/." "${container_id}:/agent-prompts"

    if docker start --attach "${container_id}"; then
      docker cp "${container_id}:/agent-run/." "${AGENT_RUN_DIR}"
    else
      status=$?
      docker cp "${container_id}:/agent-run/." "${AGENT_RUN_DIR}" >/dev/null 2>&1 || true
      exit "${status}"
    fi
    ;;
  *)
    echo "ERROR: unsupported AGENT_RUNTIME_BACKEND=${backend}. Supported: podman, docker" >&2
    exit 1
    ;;
esac
