#!/usr/bin/env bash
# Classify Go test packages into pure (no external services) and db (need PostgreSQL/Redis).
#
# Usage: list-test-packages.sh [pure|db|all]
#   pure  — packages whose tests do NOT need PostgreSQL or Redis (default)
#   db    — packages whose tests need PostgreSQL and/or Redis
#   all   — all testable packages (minus CI-excluded ones)
#
# Packages are excluded from both lists if they need infrastructure that
# neither the CI runner nor the local dev environment provides (LDAP server,
# /etc/core/ certificate files, full Harbor stack, external SMTP, etc.) or
# have known flaky test-ordering issues in parallel execution.
#
# Detection: DB-dependent packages are identified by grep-ing *_test.go files
# for the setup functions PrepareTestForPostgresSQL, InitDatabaseFromEnv, and
# GiveMeRedisPool. This runs in <0.2s on modern hardware.

set -euo pipefail

SRC_DIR="$(git -C "$(dirname "$0")/../.." rev-parse --show-toplevel)/src"
cd "$SRC_DIR"

# Packages excluded from CI — need infra that is not available.
EXCLUDE_RE=$(
  IFS='|'
  cat <<'PATTERNS'
/controller/ldap$
/core/auth/ldap$
/pkg/ldap$
/core/api$
/core/controllers$
/pkg/proxy/connection$
/pkg/token$
/server/middleware/security$
/controller/systeminfo$
/pkg/jobmonitor$
/common/utils/email$
/jobservice/runner$
/jobservice/logger/getter$
/controller/usergroup/test$
/pkg/scan/dao/scanner$
/pkg/scan/export$
/pkg/scan/dao/scan$
/pkg/scan/postprocessors$
/pkg/systemartifact$
PATTERNS
)
EXCLUDE_RE=$(echo "$EXCLUDE_RE" | tr '\n' '|' | sed 's/|$//')

# All packages minus excluded ones.
all_pkgs=$(go list ./... | grep -vE "$EXCLUDE_RE")

# Find directories whose test files reference real DB/Redis setup.
db_pkg_list=$(
  grep -rl 'PrepareTestForPostgresSQL\|InitDatabaseFromEnv\|GiveMeRedisPool' \
    --include='*_test.go' . |
    sed 's|^\./||; s|/[^/]*$||' |
    sort -u |
    sed 's|^|./|' |
    xargs go list 2>/dev/null || true
)

mode="${1:-pure}"
case "$mode" in
  pure)
    if [ -n "$db_pkg_list" ]; then
      echo "$all_pkgs" | grep -vFx "$db_pkg_list"
    else
      echo "$all_pkgs"
    fi
    ;;
  db)
    if [ -n "$db_pkg_list" ]; then
      # Intersect: only DB packages that aren't excluded.
      echo "$all_pkgs" | grep -Fx "$db_pkg_list" || true
    fi
    ;;
  all)
    echo "$all_pkgs"
    ;;
  *)
    echo "Usage: $0 [pure|db|all]" >&2
    exit 1
    ;;
esac
