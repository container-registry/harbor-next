#!/usr/bin/env bash
set -euo pipefail

suite="${1:-${SUITE:-all}}"

if [[ "${suite}" == "shell" ]]; then
  exec bash -l
fi

: "${HARBOR_CARGO_URL:?set HARBOR_CARGO_URL, for example http://host.docker.internal:8080/cargo/library/}"
: "${HARBOR_USERNAME:?set HARBOR_USERNAME}"
: "${HARBOR_PASSWORD:?set HARBOR_PASSWORD}"

repo_url="${HARBOR_CARGO_URL%/}/"
workdir="${WORKDIR:-/work/compat}"
run_id="${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
crate_name="${CARGO_FIXTURE_NAME:-harbor-cargo-compat}"
version="${CARGO_FIXTURE_VERSION:-1.0.${run_id}}"
token="$(printf '%s:%s' "${HARBOR_USERNAME}" "${HARBOR_PASSWORD}" | base64 | tr -d '\n')"

rm -rf "${workdir}"
mkdir -p "${workdir}/cargo-home"
export CARGO_HOME="${workdir}/cargo-home"
cd "${workdir}"

log() {
  printf '\n==> %s\n' "$*"
}

configure_cargo() {
  mkdir -p "${CARGO_HOME}"
  cat > "${CARGO_HOME}/config.toml" <<TOML
[registry]
global-credential-providers = ["cargo:token"]

[registries.harbor]
index = "sparse+${repo_url}"
token = "${token}"
TOML
}

create_crate() {
  log "Creating Cargo fixture ${crate_name} ${version}"
  cargo new --bin "${crate_name}"
  cat > "${crate_name}/Cargo.toml" <<TOML
[package]
name = "${crate_name}"
version = "${version}"
edition = "2021"
description = "Harbor Cargo compatibility fixture"
license = "Apache-2.0"
repository = "https://github.com/container-registry/8gcr"

[dependencies]
TOML
  cat > "${crate_name}/src/lib.rs" <<RS
pub fn message() -> &'static str {
    "harbor-cargo-compat-${version}"
}
RS
  cat > "${crate_name}/src/main.rs" <<RS
fn main() {
    println!("{}", harbor_cargo_compat::message());
}
RS
}

publish_crate() {
  configure_cargo
  create_crate
  log "Publishing with cargo to ${repo_url}"
  (cd "${crate_name}" && cargo publish --registry harbor --allow-dirty)
}

install_crate() {
  configure_cargo
  local install_root="${workdir}/install-root"
  mkdir -p "${install_root}"
  log "Installing with cargo from ${repo_url}"
  CARGO_INSTALL_ROOT="${install_root}" cargo install \
    --registry harbor \
    --version "${version}" \
    "${crate_name}"
  "${install_root}/bin/${crate_name}" | grep "harbor-cargo-compat-${version}"
}

consume_as_dependency() {
  configure_cargo
  log "Resolving as a dependency from ${repo_url}"
  cargo new --bin consumer
  cat >> consumer/Cargo.toml <<TOML
${crate_name//-/_} = { package = "${crate_name}", version = "=${version}", registry = "harbor" }
TOML
  cat > consumer/src/main.rs <<RS
fn main() {
    assert_eq!(harbor_cargo_compat::message(), "harbor-cargo-compat-${version}");
    println!("{}", harbor_cargo_compat::message());
}
RS
  (cd consumer && cargo build)
}

case "${suite}" in
  publish)
    publish_crate
    ;;
  install)
    install_crate
    ;;
  dependency)
    consume_as_dependency
    ;;
  all)
    publish_crate
    install_crate
    consume_as_dependency
    ;;
  *)
    printf 'unknown suite: %s\n' "${suite}" >&2
    exit 2
    ;;
esac
