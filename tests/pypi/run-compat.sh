#!/usr/bin/env bash
set -euo pipefail

suite="${1:-${SUITE:-all}}"

if [[ "${suite}" == "shell" ]]; then
  exec bash -l
fi

: "${HARBOR_PYPI_URL:?set HARBOR_PYPI_URL, for example http://host.docker.internal:8080/pypi/library/}"
: "${HARBOR_USERNAME:?set HARBOR_USERNAME}"
: "${HARBOR_PASSWORD:?set HARBOR_PASSWORD}"

repo_url="${HARBOR_PYPI_URL%/}/"
simple_url="${repo_url}simple/"
trusted_host="$(python - "$repo_url" <<'PY'
import sys
from urllib.parse import urlsplit

print(urlsplit(sys.argv[1]).hostname or "")
PY
)"
workdir="${WORKDIR:-/work/compat}"
run_id="${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
package_name="${PYPI_FIXTURE_NAME:-harbor-pypi-compat}"
version="${PYPI_FIXTURE_VERSION:-1.0.${run_id}}"

rm -rf "${workdir}"
mkdir -p "${workdir}"
cd "${workdir}"

log() {
  printf '\n==> %s\n' "$*"
}

url_with_basic_auth() {
  python - "$1" "$HARBOR_USERNAME" "$HARBOR_PASSWORD" <<'PY'
import sys
from urllib.parse import quote, urlsplit, urlunsplit

url, username, password = sys.argv[1:4]
parts = urlsplit(url)
netloc = f"{quote(username, safe='')}:{quote(password, safe='')}@{parts.netloc}"
print(urlunsplit((parts.scheme, netloc, parts.path, parts.query, parts.fragment)))
PY
}

create_package() {
  log "Creating Python fixture ${package_name} ${version}"
  mkdir -p "${package_name}/src/harbor_pypi_compat"
  cat > "${package_name}/pyproject.toml" <<TOML
[build-system]
requires = ["setuptools>=68", "wheel"]
build-backend = "setuptools.build_meta"

[project]
name = "${package_name}"
version = "${version}"
description = "Harbor PyPI compatibility fixture"
readme = "README.md"
requires-python = ">=3.9"
license = { text = "Apache-2.0" }
authors = [{ name = "Harbor Compatibility Fixture" }]
classifiers = [
  "Programming Language :: Python :: 3",
  "Programming Language :: Python :: 3 :: Only",
]
TOML
  cat > "${package_name}/README.md" <<MD
# Harbor PyPI Compatibility Fixture

Generated fixture for Harbor PyPI hosted registry testing.
MD
  cat > "${package_name}/src/harbor_pypi_compat/__init__.py" <<PY
def message():
    return "harbor-pypi-compat-${version}"
PY
}

publish_with_twine() {
  create_package
  log "Building wheel and sdist"
  (cd "${package_name}" && python -m build)
  log "Publishing with twine to ${repo_url}"
  twine upload \
    --non-interactive \
    --repository-url "${repo_url}" \
    -u "${HARBOR_USERNAME}" \
    -p "${HARBOR_PASSWORD}" \
    "${package_name}"/dist/*
}

install_with_pip() {
  local target="${workdir}/pip-target"
  local auth_simple
  auth_simple="$(url_with_basic_auth "${simple_url}")"
  log "Installing with pip from ${simple_url}"
  python -m pip install \
    --no-cache-dir \
    --index-url "${auth_simple}" \
    --trusted-host "${trusted_host}" \
    --target "${target}" \
    "${package_name}==${version}"
  PYTHONPATH="${target}" python - <<PY
import harbor_pypi_compat
assert harbor_pypi_compat.message() == "harbor-pypi-compat-${version}"
print(harbor_pypi_compat.message())
PY
}

install_with_uv() {
  local target="${workdir}/uv-target"
  local auth_simple
  auth_simple="$(url_with_basic_auth "${simple_url}")"
  log "Installing with uv from ${simple_url}"
  uv pip install \
    --python "$(command -v python)" \
    --no-cache \
    --index-url "${auth_simple}" \
    --trusted-host "${trusted_host}" \
    --target "${target}" \
    "${package_name}==${version}"
  PYTHONPATH="${target}" python - <<PY
import harbor_pypi_compat
assert harbor_pypi_compat.message() == "harbor-pypi-compat-${version}"
print(harbor_pypi_compat.message())
PY
}

case "${suite}" in
  publish)
    publish_with_twine
    ;;
  pip)
    install_with_pip
    ;;
  uv)
    install_with_uv
    ;;
  all)
    publish_with_twine
    install_with_pip
    install_with_uv
    ;;
  *)
    printf 'unknown suite: %s\n' "${suite}" >&2
    exit 2
    ;;
esac
