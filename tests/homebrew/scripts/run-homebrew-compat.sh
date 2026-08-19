#!/usr/bin/env bash
set -euo pipefail

suite="${1:-${SUITE:-all}}"

if [[ "${suite}" == "shell" ]]; then
  exec bash -l
fi

formula="${HOMEBREW_FORMULA:-harbor-cli}"
version="${HOMEBREW_FORMULA_VERSION:-0.0.23}"
project="${HARBOR_PROJECT:-homebrew}"
repository="${HOMEBREW_OCI_REPOSITORY:-core/${formula}}"
registry="${HARBOR_REGISTRY:-host.docker.internal:8080}"
harbor_url="${HARBOR_URL:-http://${registry}}"
username="${HARBOR_USERNAME:-admin}"
password="${HARBOR_PASSWORD:-Harbor12345}"
source_image="${HOMEBREW_SOURCE_IMAGE:-ghcr.io/homebrew/core/${formula}:${version}}"
target_image="${HOMEBREW_TARGET_IMAGE:-${registry}/${project}/${repository}:${version}}"
formula_file="${HOMEBREW_FORMULA_FILE:-/opt/homebrew-fixtures/Formula/${formula}.rb}"
tap_name="${HOMEBREW_TAP:-harbor/fixtures}"
tap_formula="${tap_name}/${formula}"
proxy_formula="${HOMEBREW_PROXY_FORMULA:-jq}"
proxy_cask="${HOMEBREW_PROXY_CASK:-firefox}"

log() {
  printf '\n==> %s\n' "$*"
}

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

basic_auth_token() {
  printf '%s:%s' "${username}" "${password}" | base64 -w0
}

ensure_project() {
  local api="${harbor_url%/}/api/v2.0/projects"
  local code

  code="$(curl -sS -o /tmp/homebrew-project.json -w '%{http_code}' \
    -u "${username}:${password}" \
    "${api}/${project}")"

  if [[ "${code}" == "200" ]]; then
    log "Harbor project ${project} already exists"
    return
  fi

  if [[ "${code}" != "404" ]]; then
    cat /tmp/homebrew-project.json >&2 || true
    echo "failed to inspect Harbor project ${project}: HTTP ${code}" >&2
    exit 1
  fi

  log "Creating Harbor project ${project}"
  code="$(curl -sS -o /tmp/homebrew-project-create.json -w '%{http_code}' \
    -u "${username}:${password}" \
    -H 'Content-Type: application/json' \
    -X POST "${api}" \
    -d "{\"project_name\":\"${project}\",\"public\":true}")"

  if [[ "${code}" != "201" && "${code}" != "409" ]]; then
    cat /tmp/homebrew-project-create.json >&2 || true
    echo "failed to create Harbor project ${project}: HTTP ${code}" >&2
    exit 1
  fi
}

push_package() {
  need skopeo
  ensure_project
  log "Copying ${source_image} to ${target_image}"
  skopeo copy \
    --all \
    --src-tls-verify=true \
    --dest-tls-verify=false \
    --dest-creds "${username}:${password}" \
    "docker://${source_image}" \
    "docker://${target_image}"
}

brew_env() {
  export HOMEBREW_NO_AUTO_UPDATE=1
  export HOMEBREW_ARTIFACT_DOMAIN="${harbor_url%/}"
  export HOMEBREW_ARTIFACT_DOMAIN_NO_FALLBACK=1
  export HOMEBREW_DOCKER_REGISTRY_BASIC_AUTH_TOKEN
  HOMEBREW_DOCKER_REGISTRY_BASIC_AUTH_TOKEN="$(basic_auth_token)"
}

prepare_tap() {
  need brew

  local tap_root
  tap_root="$(brew --repo "${tap_name}" 2>/dev/null || true)"
  if [[ -z "${tap_root}" ]]; then
    log "Creating local Homebrew tap ${tap_name}"
    brew tap-new "${tap_name}" >/dev/null
    tap_root="$(brew --repo "${tap_name}")"
  fi

  mkdir -p "${tap_root}/Formula"
  cp "${formula_file}" "${tap_root}/Formula/${formula}.rb"
  brew trust "${tap_name}" >/dev/null 2>&1 || true
}

fetch_package() {
  need brew
  prepare_tap
  brew_env
  log "Fetching ${formula} ${version} from Harbor via Homebrew"
  brew fetch --force --verbose "${tap_formula}"
}

install_package() {
  need brew
  prepare_tap
  brew_env
  log "Installing ${formula} ${version} from Harbor via Homebrew"
  brew uninstall --force "${formula}" >/dev/null 2>&1 || true
  brew install --force-bottle "${tap_formula}"
  log "Installed binary"
  harbor version
}

inspect_package() {
  need skopeo
  log "Inspecting ${target_image}"
  skopeo inspect --raw --tls-verify=false "docker://${target_image}" \
    | jq '{schemaVersion, annotations, manifests: [.manifests[] | {digest, platform, annotations}]}'
}

proxy_matrix() {
  need brew
  need curl

  local proxy_base="${harbor_url%/}/homebrew/${project}"
  export HOMEBREW_NO_AUTO_UPDATE=1
  export HOMEBREW_NO_ANALYTICS=1
  export HOMEBREW_NO_ENV_HINTS=1
  export HOMEBREW_API_DOMAIN="${proxy_base}/api"
  export HOMEBREW_ARTIFACT_DOMAIN="${proxy_base}"
  export HOMEBREW_ARTIFACT_DOMAIN_NO_FALLBACK=1

  log "Checking Homebrew API through ${HOMEBREW_API_DOMAIN}"
  curl --fail --silent --show-error \
    "${HOMEBREW_API_DOMAIN}/formula/${proxy_formula}.json" >/dev/null
  curl --fail --silent --show-error \
    "${HOMEBREW_API_DOMAIN}/cask/${proxy_cask}.json" >/dev/null

  log "Running read-only developer commands"
  brew search "/^${proxy_formula}$/"
  brew info "${proxy_formula}"
  brew info --cask "${proxy_cask}"
  brew deps --tree "${proxy_formula}"
  if brew info harbor-e2e-definitely-missing; then
    echo "missing formula unexpectedly resolved" >&2
    exit 1
  fi

  log "Fetching, installing, and validating ${proxy_formula}"
  brew fetch --force --force-bottle --verbose "${proxy_formula}"
  brew install "${proxy_formula}"
  brew list --versions "${proxy_formula}"
  brew linkage --test "${proxy_formula}"
  brew pin "${proxy_formula}"
  brew list --pinned | grep -Fx "${proxy_formula}"
  brew unpin "${proxy_formula}"
  if brew list --pinned | grep -Fx "${proxy_formula}"; then
    echo "formula remains pinned after brew unpin" >&2
    exit 1
  fi
  brew reinstall "${proxy_formula}"
  brew upgrade "${proxy_formula}"

  log "Checking Brewfile workflow"
  brew uninstall --force "${proxy_formula}"
  printf 'brew "%s"\n' "${proxy_formula}" > /tmp/HarborBrewfile
  brew bundle install --file=/tmp/HarborBrewfile --no-upgrade
  brew bundle check --file=/tmp/HarborBrewfile

  log "Cleaning installed packages"
  brew uninstall --force "${proxy_formula}"
  brew autoremove
  brew cleanup
  if brew list --versions "${proxy_formula}" | grep -q .; then
    echo "formula remains installed after cleanup" >&2
    exit 1
  fi
  echo "Homebrew proxy matrix passed"
}

case "${suite}" in
  push)
    push_package
    ;;
  fetch)
    fetch_package
    ;;
  install)
    install_package
    ;;
  inspect)
    inspect_package
    ;;
  proxy)
    proxy_matrix
    ;;
  all)
    push_package
    inspect_package
    fetch_package
    install_package
    ;;
  *)
    cat >&2 <<EOF
unknown suite: ${suite}

Usage:
  run-homebrew-compat all
  run-homebrew-compat push
  run-homebrew-compat inspect
  run-homebrew-compat fetch
  run-homebrew-compat install
  run-homebrew-compat proxy
  run-homebrew-compat shell
EOF
    exit 2
    ;;
esac
