#!/usr/bin/env bash
#
# One command: bring a real Harbor up, seed it, run the smoke suite, print a
# pass/fail table, tear it down. See tests/e2e/README.md.
#
set -euo pipefail

E2E_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
E2E_REPO_ROOT=$(CDPATH='' cd -- "${E2E_DIR}/../.." && pwd)

# shellcheck source=lib/common.sh
. "${E2E_DIR}/lib/common.sh"

# --- configuration ----------------------------------------------------------
version_default() { sed -n "s/^$1=//p" "${E2E_REPO_ROOT}/versions.env" | tail -1; }

E2E_TAG=${E2E_TAG:-$(version_default HARBOR_E2E_TAG)}
E2E_UPGRADE_FROM=${E2E_UPGRADE_FROM:-}
if [ "${E2E_UPGRADE:-0}" = "1" ] && [ -z "$E2E_UPGRADE_FROM" ]; then
  E2E_UPGRADE_FROM=$(version_default HARBOR_E2E_UPGRADE_FROM)
fi
E2E_IMAGE_REPO=${E2E_IMAGE_REPO:-8gears.container-registry.com/8gcr/}
E2E_BASE_IMAGE=${E2E_BASE_IMAGE:-$(version_default HARBOR_E2E_BASE_IMAGE)}
E2E_CRANE_IMAGE=${E2E_CRANE_IMAGE:-$(version_default HARBOR_E2E_CRANE_IMAGE)}
E2E_DESTREG_IMAGE=${E2E_DESTREG_IMAGE:-$(version_default HARBOR_E2E_DESTREG_IMAGE)}
E2E_PLATFORM=${E2E_PLATFORM:-linux/amd64}

E2E_PROJECT=${E2E_PROJECT:-harbor-e2e}
E2E_PORT=${E2E_PORT:-18080}
E2E_PORT_HTTPS=${E2E_PORT_HTTPS:-$((E2E_PORT + 363))}
E2E_NETWORK=${E2E_NETWORK:-${E2E_PROJECT}-net}
E2E_CACHE_VOLUME=${E2E_CACHE_VOLUME:-harbor-e2e-cache}
E2E_KEEP=${E2E_KEEP:-0}
E2E_LOG_LEVEL=${E2E_LOG_LEVEL:-warning}
SEED_ARTIFACTS=${SEED_ARTIFACTS:-1}

# The scan-scale lane (#478): the jobservice memory limit and the size of one
# vulnerability report are the two variables that decide whether a Scan All
# survives. 0 means unlimited, which is the harness default and is why a
# 100-artifact Scan All passes here. SEED_HEAVY=1 seeds from an image chosen for
# its CVE count so one report is large enough to matter.
E2E_JOBSERVICE_MEMORY=${E2E_JOBSERVICE_MEMORY:-0}
E2E_JOBSERVICE_GOMEMLIMIT=${E2E_JOBSERVICE_GOMEMLIMIT:-off}
SEED_HEAVY=${SEED_HEAVY:-0}
E2E_HEAVY_IMAGE=${E2E_HEAVY_IMAGE:-$(version_default HARBOR_E2E_HEAVY_IMAGE)}
if [ "$SEED_HEAVY" = "1" ]; then E2E_BASE_IMAGE=$E2E_HEAVY_IMAGE; fi

# Fixed, local-only credentials. Nothing here reaches a real deployment.
E2E_ADMIN_PASSWORD=${E2E_ADMIN_PASSWORD:-Harbor12345}
E2E_CORE_SECRET=not-a-secret-only-for-local-e2e
E2E_JOBSERVICE_SECRET=not-a-secret-only-for-local-e2e
E2E_DB_PASSWORD=harbor-e2e

E2E_PRIVATE_PROJECT=e2e-private
E2E_PUBLIC_PROJECT=e2e-public
E2E_REPO=alpine
E2E_TAG_NAME=e2e
E2E_DEST_NAME=e2e-destreg
E2E_POLICY_NAME=e2e-private-push
E2E_REG=portal:8080

E2E_READY_TIMEOUT=${E2E_READY_TIMEOUT:-300}
E2E_SCAN_TIMEOUT=${E2E_SCAN_TIMEOUT:-180}
E2E_SCAN_ALL_TIMEOUT=${E2E_SCAN_ALL_TIMEOUT:-1200}
E2E_REPLICATION_TIMEOUT=${E2E_REPLICATION_TIMEOUT:-180}
E2E_GC_TIMEOUT=${E2E_GC_TIMEOUT:-300}

E2E_URL="http://127.0.0.1:${E2E_PORT}"
E2E_API="${E2E_URL}/api/v2.0"

E2E_WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/harbor-e2e.XXXXXX")
E2E_ENV_FILE="${E2E_WORKDIR}/compose.env"
E2E_LOG_DIR=${E2E_LOG_DIR:-${E2E_DIR}/.logs}

[ -n "$E2E_TAG" ] || die "no image tag: set E2E_TAG or HARBOR_E2E_TAG in versions.env"

# --- engine -----------------------------------------------------------------
# `docker` is often a podman shim that prints a banner on every invocation;
# calling podman directly keeps the output readable. Identify the shim from a
# captured string, not a `| grep -q` pipeline — grep exits early and pipefail
# then reports the whole pipeline as failed.
E2E_ENGINE=""
if docker version >/dev/null 2>&1; then
  case "$(docker --version 2>&1 || true)" in
    *odman*) ;;
    *) E2E_ENGINE=docker ;;
  esac
fi
if [ -z "$E2E_ENGINE" ] && podman version >/dev/null 2>&1; then
  E2E_ENGINE=podman
fi
if [ -z "$E2E_ENGINE" ] && docker version >/dev/null 2>&1; then
  E2E_ENGINE=docker
fi
[ -n "$E2E_ENGINE" ] || die "neither docker nor podman is usable"

if $E2E_ENGINE compose version >/dev/null 2>&1; then
  E2E_COMPOSE="$E2E_ENGINE compose"
elif command -v docker-compose >/dev/null 2>&1; then
  E2E_COMPOSE="docker-compose"
elif command -v podman-compose >/dev/null 2>&1; then
  E2E_COMPOSE="podman-compose"
else
  die "no compose implementation found (tried '$E2E_ENGINE compose', docker-compose, podman-compose)"
fi

# shellcheck source=lib/stack.sh
. "${E2E_DIR}/lib/stack.sh"
# shellcheck source=lib/seed.sh
. "${E2E_DIR}/lib/seed.sh"
# shellcheck source=lib/checks.sh
. "${E2E_DIR}/lib/checks.sh"

# --- lifecycle --------------------------------------------------------------
RUN_START=$(now)
cleanup() {
  local rc=$? down_start
  down_start=$(now)
  stack_down
  [ "$E2E_KEEP" = "1" ] || rm -rf "$E2E_WORKDIR"
  printf '%swall-clock %ss including a %ss teardown%s\n' \
    "$C_DIM" "$(( $(now) - RUN_START ))" "$(( $(now) - down_start ))" "$C_RESET" >&2
  exit "$rc"
}
trap cleanup EXIT INT TERM

usage() {
  sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'
  cat <<'EOF'

Environment:
  E2E_TAG            Harbor image tag                (versions.env HARBOR_E2E_TAG)
  E2E_UPGRADE_FROM   start here, then upgrade to E2E_TAG and check the schema
  E2E_IMAGE_REPO     image repository prefix         (8gears.container-registry.com/8gcr/)
  E2E_PORT           host port for the portal        (18080)
  SEED_ARTIFACTS     artifacts to seed; >1 also runs Scan All   (1)
  SEED_HEAVY=1       seed from HARBOR_E2E_HEAVY_IMAGE instead, for big reports
  E2E_JOBSERVICE_MEMORY   jobservice memory limit, e.g. 512m  (0 = unlimited)
  E2E_JOBSERVICE_GOMEMLIMIT   GOMEMLIMIT inside jobservice    (off)
  E2E_KEEP=1         leave the stack up and print its URL
  E2E_LOG_DIR        where component logs go on failure
EOF
}

case "${1:-}" in
  -h|--help|help) usage; trap - EXIT; exit 0 ;;
  --down)
    write_env_file "$E2E_TAG"
    log "removing the ${E2E_PROJECT} stack"
    compose down -v --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$E2E_WORKDIR"
    trap - EXIT; exit 0 ;;
esac

banner() {
  printf '%s\n' "$C_BOLD"
  printf 'Harbor E2E\n'
  printf '%s' "$C_RESET"
  printf '  images    %s*:%s\n' "$E2E_IMAGE_REPO" "$E2E_TAG"
  if [ -n "$E2E_UPGRADE_FROM" ]; then printf '  upgrade   %s -> %s\n' "$E2E_UPGRADE_FROM" "$E2E_TAG"; fi
  printf '  stack     compose via %s (project %s)\n' "$E2E_COMPOSE" "$E2E_PROJECT"
  printf '  url       %s\n' "$E2E_URL"
  printf '  seed      %s artifact(s) per project from %s\n' "$SEED_ARTIFACTS" "$E2E_BASE_IMAGE"
  printf '  jobsvc    memory %s, GOMEMLIMIT %s\n' \
    "$([ "$E2E_JOBSERVICE_MEMORY" = 0 ] && echo unlimited || echo "$E2E_JOBSERVICE_MEMORY")" \
    "$E2E_JOBSERVICE_GOMEMLIMIT"
  printf '\n'
}

FIRST_TAG=${E2E_UPGRADE_FROM:-$E2E_TAG}

banner
ensure_token_key
write_env_file "$FIRST_TAG"

# A previous run that was killed mid-flight leaves containers behind; they would
# hold the port and the project name.
compose down -v --remove-orphans >/dev/null 2>&1 || true

preflight

# --- run --------------------------------------------------------------------
E2E_EXPECT_VERSION=$FIRST_TAG
run_check up check_up
if [ "$FAILED" -gt 0 ]; then
  dump_logs
  print_table "$(( $(now) - RUN_START ))"
  exit 1
fi

create_project "$E2E_PRIVATE_PROJECT" false
create_project "$E2E_PUBLIC_PROJECT" true
seed_artifacts

if [ -n "$E2E_UPGRADE_FROM" ]; then
  run_check upgrade check_upgrade
  E2E_EXPECT_VERSION=$E2E_TAG
  run_check migration check_migration
fi

run_check push-pull   check_push_pull
run_check scan        check_scan
run_check sbom        check_sbom
run_check replication check_replication
run_check gc          check_gc

if [ "$FAILED" -gt 0 ]; then dump_logs; fi
print_table "$(( $(now) - RUN_START ))"
[ "$FAILED" -eq 0 ]
