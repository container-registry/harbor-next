#!/usr/bin/env bash
# Seeding: projects, artifacts, replication endpoint.
# shellcheck shell=bash

# crane_sh runs a script (on stdin) inside a throwaway crane container attached
# to the stack network. Pushing to the internal portal:8080 keeps image traffic
# off the host port and makes the token realm match the request host.
#
# /cache is a named volume that survives teardown, so the base image is pulled
# from its upstream registry once per machine rather than once per run.
crane_sh() {
  {
    printf 'set -e\n'
    printf 'PATH=/ko-app:/busybox:$PATH\n'
    printf 'export HOME=/cache\n'
    printf 'crane auth login --insecure %s -u admin -p %s >/dev/null 2>&1\n' \
      "$E2E_REG" "$E2E_ADMIN_PASSWORD"
    cat
  } | $E2E_ENGINE run --rm -i \
        --network "$E2E_NETWORK" \
        -v "${E2E_CACHE_VOLUME}:/cache" \
        --entrypoint '' "$E2E_CRANE_IMAGE" /busybox/sh -s
}

create_project() { # create_project NAME PUBLIC(true|false)
  local code
  code=$(api_code POST /projects \
    "{\"project_name\":\"$1\",\"metadata\":{\"public\":\"$2\"}}")
  case "$code" in
    201|409) return 0 ;;
    *) die "creating project $1 returned HTTP $code" ;;
  esac
}

# The base layout is cached under /cache/base. Seeding then appends a unique
# marker layer per extra artifact, which is near-instant and still produces a
# real OS image the scanner has something to say about.
seed_artifacts() {
  local n=${SEED_ARTIFACTS}
  log "seeding ${n} artifact(s) per project from ${E2E_BASE_IMAGE}"
  if ! seed_script "$n" > "${E2E_WORKDIR}/seed.log" 2>&1; then
    sed -n '1,30p' "${E2E_WORKDIR}/seed.log" >&2
    die "seeding failed"
  fi
}

seed_script() {
  local n=$1
  crane_sh <<EOF
cache_dir=/cache/base-\$(echo "${E2E_BASE_IMAGE}" | /busybox/tr -c 'a-zA-Z0-9' '_')
if [ ! -f "\$cache_dir/oci-layout" ]; then
  rm -rf "\$cache_dir"
  crane pull --platform ${E2E_PLATFORM} --format oci "${E2E_BASE_IMAGE}" "\$cache_dir"
fi
crane push --insecure "\$cache_dir" ${E2E_REG}/${E2E_PRIVATE_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} >/dev/null
crane push --insecure "\$cache_dir" ${E2E_REG}/${E2E_PUBLIC_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} >/dev/null

i=2
while [ "\$i" -le "${n}" ]; do
  mkdir -p "/tmp/l\$i"
  echo "harbor-e2e-seed-\$i" > "/tmp/l\$i/marker.txt"
  ( cd /tmp && /busybox/tar cf "/tmp/layer\$i.tar" "l\$i" )
  crane append --insecure \
    -b ${E2E_REG}/${E2E_PRIVATE_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} \
    -f "/tmp/layer\$i.tar" \
    -t "${E2E_REG}/${E2E_PRIVATE_PROJECT}/${E2E_REPO}-\$i:${E2E_TAG_NAME}" >/dev/null
  i=\$((i + 1))
done
EOF
}

register_destination() {
  local code
  code=$(api_code POST /registries \
    "{\"name\":\"${E2E_DEST_NAME}\",\"type\":\"docker-registry\",\"url\":\"http://destreg:5000\",\"insecure\":true}")
  case "$code" in
    201|409) : ;;
    *) return 1 ;;
  esac
  E2E_DEST_ID=$(api GET "/registries?q=name%3D${E2E_DEST_NAME}" | jq -r '.[0].id')
  [ -n "$E2E_DEST_ID" ] && [ "$E2E_DEST_ID" != "null" ]
}

artifact_digest() { # artifact_digest PROJECT REPO
  api GET "/projects/$1/repositories/$2/artifacts?page_size=1" | jq -r '.[0].digest // empty'
}

repo_count() {
  api GET "/statistics" | jq -r '.total_repo_count // 0'
}
