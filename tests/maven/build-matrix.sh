#!/usr/bin/env bash
set -euo pipefail

context_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

build_one() {
  local java_version="$1"
  local maven_version="$2"
  local tag="harbor-maven-compat:jdk${java_version}-mvn${maven_version}"

  docker build \
    -f "${context_dir}/Containerfile" \
    --build-arg "JAVA_VERSION=${java_version}" \
    --build-arg "MAVEN_VERSION=${maven_version}" \
    -t "${tag}" \
    "${context_dir}"
}

build_one 8 3.6.3
build_one 11 3.8.8
build_one 17 3.9.9
build_one 21 3.9.9
