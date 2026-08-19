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
  local home_dir=$2
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
    printf 'GITHUB_RUN_ID=%s\n' "${GITHUB_RUN_ID}"
    printf 'GITHUB_RUN_ATTEMPT=%s\n' "${GITHUB_RUN_ATTEMPT}"
    printf 'HOME=%s\n' "${home_dir}"
  } >> "${env_file}"
}

: "${AGENT_RUN_DIR:?AGENT_RUN_DIR is required}"
: "${AGENT_GITHUB_TOKEN:?AGENT_GITHUB_TOKEN is required}"
: "${AGENT_IMAGE:?Set vars.AGENT_IMAGE to the OCI image that contains gh and jq}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SERVER_URL:?GITHUB_SERVER_URL is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
: "${GITHUB_RUN_ATTEMPT:?GITHUB_RUN_ATTEMPT is required}"
: "${TARGET_BRANCH:?TARGET_BRANCH is required}"
: "${FAILED_RUN_ID:?FAILED_RUN_ID is required}"

backend=${AGENT_RUNTIME_BACKEND:-docker}
container_home=/agent-home
if [ "${backend}" = "docker" ]; then
  container_home=/tmp/agent-home
fi
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runtime_dir="${AGENT_RUN_DIR}.handoff-runtime"
env_file="${runtime_dir}/handoff.env"
agent_home="${runtime_dir}/home"
mkdir -p "${AGENT_RUN_DIR}" "${agent_home}"
chmod 700 "${agent_home}"
trap 'rm -rf "${runtime_dir}"' EXIT

write_env_file "${env_file}" "${container_home}"
if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
  echo "::add-mask::${AGENT_GITHUB_TOKEN}"
fi

case "${backend}" in
  podman)
    require podman
    podman run --rm \
      --name "agent-handoff-${FAILED_RUN_ID}-${GITHUB_RUN_ATTEMPT}" \
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
      -v "${script_dir}:/agent-scripts:ro,Z" \
      "${AGENT_IMAGE}" \
      /agent-scripts/prepare-agent-handoff.sh /agent-run
    ;;
  docker)
    require docker
    container_id=$(docker create \
      --name "agent-handoff-${FAILED_RUN_ID}-${GITHUB_RUN_ATTEMPT}" \
      --user 0:0 \
      --cap-drop=ALL \
      --security-opt=no-new-privileges \
      --pids-limit="${AGENT_PIDS_LIMIT:-2048}" \
      --memory="${AGENT_MEMORY:-8g}" \
      --cpus="${AGENT_CPUS:-4}" \
      --network="${AGENT_DOCKER_NETWORK:-bridge}" \
      --env-file "${env_file}" \
      "${AGENT_IMAGE}" \
      bash -lc 'mkdir -p /tmp/agent-home && /prepare-agent-handoff.sh /tmp/agent-run')
    trap 'docker rm -f "${container_id}" >/dev/null 2>&1 || true; rm -rf "${runtime_dir}"' EXIT

    docker cp "${script_dir}/prepare-agent-handoff.sh" "${container_id}:/prepare-agent-handoff.sh"
    if docker start --attach "${container_id}"; then
      docker cp "${container_id}:/tmp/agent-run/." "${AGENT_RUN_DIR}"
    else
      status=$?
      docker cp "${container_id}:/tmp/agent-run/." "${AGENT_RUN_DIR}" >/dev/null 2>&1 || true
      exit "${status}"
    fi
    ;;
  *)
    echo "ERROR: unsupported AGENT_RUNTIME_BACKEND=${backend}. Supported: podman, docker" >&2
    exit 1
    ;;
esac
