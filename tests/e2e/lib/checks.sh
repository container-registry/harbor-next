#!/usr/bin/env bash
# The smoke suite. Each check returns 0/1 and sets CHECK_DETAIL.
# shellcheck shell=bash

# 1 — up and healthy. The bring-up lives inside the check so the table shows
# what the stack actually costs to start.
check_up() {
  stack_up "$FIRST_TAG" || { CHECK_DETAIL="compose up failed"; return 1; }
  _assert_healthy_version
}

_assert_healthy_version() {
  if ! wait_healthy "$E2E_READY_TIMEOUT"; then
    CHECK_DETAIL="unhealthy: $(unhealthy_components) — $(first_fatal)"
    return 1
  fi
  local info version
  info=$(api GET /systeminfo) || { CHECK_DETAIL="/systeminfo did not answer"; return 1; }
  version=$(printf '%s' "$info" | jq -r '.harbor_version // empty')
  [ -n "$version" ] || { CHECK_DETAIL="/systeminfo returned no harbor_version"; return 1; }
  CHECK_DETAIL="harbor ${version}"
  # The tag carries a leading v, the reported version does not.
  case "$version" in
    "${E2E_EXPECT_VERSION#v}"*) ;;
    *) CHECK_DETAIL="$CHECK_DETAIL (expected ${E2E_EXPECT_VERSION#v}*)"; return 1 ;;
  esac
}

# 1b — the upgrade itself, only in the upgrade lane. Postgres keeps its data
# directory, so the new core replays real migrations over a populated database.
check_upgrade() {
  E2E_SCHEMA_BEFORE=$(schema_version)
  E2E_REPOS_BEFORE=$(repo_count)
  stack_up "$E2E_TAG" --force-recreate \
    core jobservice registry registryctl portal exporter trivy-adapter \
    || { CHECK_DETAIL="compose up failed on ${E2E_TAG}"; return 1; }
  local saved=$E2E_EXPECT_VERSION
  E2E_EXPECT_VERSION=$E2E_TAG
  if ! _assert_healthy_version; then
    E2E_EXPECT_VERSION=$saved
    return 1
  fi
  E2E_EXPECT_VERSION=$saved
  E2E_SCHEMA_AFTER=$(schema_version)
  E2E_REPOS_AFTER=$(repo_count)
  CHECK_DETAIL="${E2E_UPGRADE_FROM} -> ${E2E_TAG}, $CHECK_DETAIL"
}

# 2 — push / pull, private and public
check_push_pull() {
  local priv pub
  priv=$(artifact_digest "$E2E_PRIVATE_PROJECT" "$E2E_REPO")
  pub=$(artifact_digest "$E2E_PUBLIC_PROJECT" "$E2E_REPO")
  [ -n "$priv" ] || { CHECK_DETAIL="no artifact in ${E2E_PRIVATE_PROJECT}/${E2E_REPO}"; return 1; }
  [ -n "$pub" ]  || { CHECK_DETAIL="no artifact in ${E2E_PUBLIC_PROJECT}/${E2E_REPO}"; return 1; }

  # Authenticated pull of both, then an anonymous pull that must succeed on the
  # public project and be refused on the private one.
  local out
  out=$(crane_sh <<EOF 2>&1
crane manifest --insecure ${E2E_REG}/${E2E_PRIVATE_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} >/dev/null
crane manifest --insecure ${E2E_REG}/${E2E_PUBLIC_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} >/dev/null
rm -f /cache/.docker/config.json
crane manifest --insecure ${E2E_REG}/${E2E_PUBLIC_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} >/dev/null \
  || { echo "ANON_PUBLIC_DENIED"; exit 1; }
if crane manifest --insecure ${E2E_REG}/${E2E_PRIVATE_PROJECT}/${E2E_REPO}:${E2E_TAG_NAME} >/dev/null 2>&1; then
  echo "ANON_PRIVATE_ALLOWED"; exit 1
fi
echo OK
EOF
  ) || { CHECK_DETAIL="pull failed: $(printf '%s' "$out" | tail -1)"; return 1; }

  CHECK_DETAIL="private ${priv:7:12}, public ${pub:7:12}"
}

_scan_status() { # _scan_status PROJECT REPO DIGEST
  api GET "/projects/$1/repositories/$2/artifacts/$3?with_scan_overview=true" \
    | jq -r '.scan_overview | to_entries[0].value.scan_status // "none"'
}

_scan_settled() {
  local st; st=$(_scan_status "$1" "$2" "$3")
  [ "$st" = "Success" ] || [ "$st" = "Error" ]
}

# 3 — scan one artifact through Trivy to Success
check_scan() {
  local digest; digest=$(artifact_digest "$E2E_PRIVATE_PROJECT" "$E2E_REPO")
  [ -n "$digest" ] || { CHECK_DETAIL="no artifact to scan"; return 1; }

  local code
  code=$(api_code POST "/projects/${E2E_PRIVATE_PROJECT}/repositories/${E2E_REPO}/artifacts/${digest}/scan")
  [ "$code" = "202" ] || { CHECK_DETAIL="scan request returned HTTP $code"; return 1; }

  poll_until "$E2E_SCAN_TIMEOUT" 3 "the scan to settle" \
    _scan_settled "$E2E_PRIVATE_PROJECT" "$E2E_REPO" "$digest" \
    || { CHECK_DETAIL="scan still $(_scan_status "$E2E_PRIVATE_PROJECT" "$E2E_REPO" "$digest") after ${E2E_SCAN_TIMEOUT}s"; return 1; }

  local overview status total severity
  overview=$(api GET "/projects/${E2E_PRIVATE_PROJECT}/repositories/${E2E_REPO}/artifacts/${digest}?with_scan_overview=true" \
    | jq -c '.scan_overview | to_entries[0].value')
  status=$(printf '%s' "$overview" | jq -r '.scan_status')
  total=$(printf '%s' "$overview" | jq -r '.summary.total // 0')
  severity=$(printf '%s' "$overview" | jq -r '.severity // "-"')
  [ "$status" = "Success" ] || { CHECK_DETAIL="scan status $status"; return 1; }
  # A Success with an empty report is the failure mode #863 fixed — assert the
  # report actually carries findings for a base image that is known to have them.
  [ "$total" -gt 0 ] 2>/dev/null || { CHECK_DETAIL="scan Success but the report is empty"; return 1; }
  CHECK_DETAIL="${total} vulns, top severity ${severity}"

  [ "$SEED_ARTIFACTS" -gt 1 ] || return 0
  _scan_all || return 1
}

# /scans/all/metrics reports per-status counts under .metrics, not at the top
# level, and flips .ongoing a few seconds after the request is accepted.
_scan_all_done() {
  local m total completed ongoing pending running
  m=$(api GET /scans/all/metrics) || return 1
  total=$(printf '%s' "$m" | jq -r '.total // 0')
  completed=$(printf '%s' "$m" | jq -r '.completed // 0')
  ongoing=$(printf '%s' "$m" | jq -r '.ongoing // false')
  pending=$(printf '%s' "$m" | jq -r '.metrics.Pending // 0')
  running=$(printf '%s' "$m" | jq -r '.metrics.Running // 0')
  [ "$ongoing" = "false" ] \
    && [ "$total" -gt 0 ] \
    && [ "$completed" -eq "$total" ] \
    && [ "$pending" -eq 0 ] \
    && [ "$running" -eq 0 ]
}

# The scale profile: the reported failure is "scanning 100 artifacts fails", so
# SEED_ARTIFACTS=N turns that into a reproducible check instead of a story.
# Artifacts are counted by digest, so N seeded artifacts means N scan jobs.
_scan_all() {
  local code restarts_before
  restarts_before=$(jobservice_restarts)
  code=$(api_code POST /system/scanAll/schedule '{"schedule":{"type":"Manual"}}')
  case "$code" in 201|202) ;; *) CHECK_DETAIL="$CHECK_DETAIL; Scan All returned HTTP $code"; return 1 ;; esac
  # The request is accepted before the queue fills; polling too early reads the
  # previous run's metrics and declares victory.
  sleep 10
  local finished=0
  poll_until "$E2E_SCAN_ALL_TIMEOUT" 5 "Scan All to finish" _scan_all_done && finished=1

  local m success error stopped total
  m=$(api GET /scans/all/metrics)
  success=$(printf '%s' "$m" | jq -r '.metrics.Success // 0')
  error=$(printf '%s' "$m" | jq -r '.metrics.Error // 0')
  stopped=$(printf '%s' "$m" | jq -r '.metrics.Stopped // 0')
  total=$(printf '%s' "$m" | jq -r '.total // 0')
  CHECK_DETAIL="$CHECK_DETAIL; Scan All ${success}/${total} ok, ${error} error, ${stopped} stopped"

  # An OOM-killed jobservice comes back with its queue intact and the re-delivered
  # jobs usually succeed, so Scan All can report every artifact scanned while the
  # pod is crashlooping underneath. Observed: 20/20 ok, 0 errors, one OOM kill.
  # The metrics cannot see this, so the container state is the assertion (#478).
  _report_jobservice_health "$restarts_before"

  [ "$finished" = 1 ] || { CHECK_DETAIL="$CHECK_DETAIL; did not finish in ${E2E_SCAN_ALL_TIMEOUT}s"; return 1; }
  [ "$_JOBSVC_KILLED" = 0 ] || return 1
  [ "$error" -eq 0 ] && [ "$stopped" -eq 0 ] && [ "$success" -eq "$total" ]
}

# Sets _JOBSVC_KILLED to 1 if jobservice died during the Scan All.
_report_jobservice_health() { # _report_jobservice_health RESTARTS_BEFORE
  local before=$1 after peak oom note=""
  after=$(jobservice_restarts)
  oom=$(jobservice_oom_count)
  peak=$(jobservice_peak_bytes)
  _JOBSVC_KILLED=0
  if [ "$after" -gt "$before" ] || [ "$oom" = "1" ]; then _JOBSVC_KILLED=1; fi
  if [ "$after" -gt "$before" ]; then
    note="; jobservice restarted $(( after - before ))x"
    [ "$oom" = "1" ] && note="${note} (last exit OOMKilled)"
  elif [ "$oom" = "1" ]; then
    note="; jobservice OOMKilled"
  fi
  if [ "${peak:-0}" -gt 0 ] 2>/dev/null; then
    note="${note}; jobservice peak $(( peak / 1024 / 1024 )) MiB"
    [ "$E2E_JOBSERVICE_MEMORY" = "0" ] || note="${note} of ${E2E_JOBSERVICE_MEMORY}"
  fi
  CHECK_DETAIL="${CHECK_DETAIL}${note}"
}

_sbom_settled() {
  local st
  st=$(api GET "/projects/$1/repositories/$2/artifacts/$3?with_sbom_overview=true" \
    | jq -r '.sbom_overview.scan_status // "none"')
  [ "$st" = "Success" ] || [ "$st" = "Error" ]
}

# 4 — generate an SBOM and check the accessory lands
check_sbom() {
  local digest; digest=$(artifact_digest "$E2E_PUBLIC_PROJECT" "$E2E_REPO")
  [ -n "$digest" ] || { CHECK_DETAIL="no artifact for SBOM"; return 1; }

  local code
  code=$(api_code POST "/projects/${E2E_PUBLIC_PROJECT}/repositories/${E2E_REPO}/artifacts/${digest}/scan" \
    '{"scan_type":"sbom"}')
  [ "$code" = "202" ] || { CHECK_DETAIL="SBOM request returned HTTP $code"; return 1; }

  poll_until "$E2E_SCAN_TIMEOUT" 3 "the SBOM to settle" \
    _sbom_settled "$E2E_PUBLIC_PROJECT" "$E2E_REPO" "$digest" \
    || { CHECK_DETAIL="SBOM did not settle in ${E2E_SCAN_TIMEOUT}s"; return 1; }

  local art status accessory size
  art=$(api GET "/projects/${E2E_PUBLIC_PROJECT}/repositories/${E2E_REPO}/artifacts/${digest}?with_sbom_overview=true&with_accessory=true")
  status=$(printf '%s' "$art" | jq -r '.sbom_overview.scan_status // "none"')
  accessory=$(printf '%s' "$art" | jq -r '[.accessories[]? | select(.type == "sbom.harbor")] | length')
  size=$(printf '%s' "$art" | jq -r '[.accessories[]? | select(.type == "sbom.harbor") | .size] | first // 0')
  [ "$status" = "Success" ] || { CHECK_DETAIL="SBOM status $status"; return 1; }
  # Generation succeeding while the push is dropped is exactly the CORE_URL
  # failure mode, so the accessory — not the status — is the assertion.
  [ "$accessory" -ge 1 ] || { CHECK_DETAIL="SBOM Success but no sbom.harbor accessory was pushed"; return 1; }
  CHECK_DETAIL="sbom.harbor accessory, ${size} B"
}

_replication_settled() {
  local st
  st=$(api GET "/replication/executions?policy_id=${E2E_POLICY_ID}&page_size=1" | jq -r '.[0].status // "none"')
  case "$st" in Succeed|Failed|Stopped) return 0 ;; *) return 1 ;; esac
}

# 5 — push replication out of a PRIVATE project. Public projects replicate even
# when the secret authorizer is skipped, so only the private case is diagnostic.
check_replication() {
  register_destination || { CHECK_DETAIL="could not register the destination registry"; return 1; }
  local code
  code=$(api_code POST /replication/policies "$(cat <<JSON
{
  "name": "${E2E_POLICY_NAME}",
  "dest_registry": {"id": ${E2E_DEST_ID}},
  "dest_namespace": "replica",
  "dest_namespace_replace_count": -1,
  "filters": [{"type": "name", "value": "${E2E_PRIVATE_PROJECT}/**"}],
  "trigger": {"type": "manual"},
  "enabled": true,
  "override": true,
  "copy_by_chunk": false
}
JSON
  )")
  case "$code" in 201|409) ;; *) CHECK_DETAIL="policy create returned HTTP $code"; return 1 ;; esac

  E2E_POLICY_ID=$(api GET "/replication/policies?q=name%3D${E2E_POLICY_NAME}" | jq -r '.[0].id')
  [ -n "$E2E_POLICY_ID" ] && [ "$E2E_POLICY_ID" != "null" ] || { CHECK_DETAIL="could not read the policy id"; return 1; }

  code=$(api_code POST /replication/executions "{\"policy_id\":${E2E_POLICY_ID}}")
  [ "$code" = "201" ] || { CHECK_DETAIL="execution returned HTTP $code"; return 1; }

  poll_until "$E2E_REPLICATION_TIMEOUT" 3 "replication to settle" _replication_settled \
    || { CHECK_DETAIL="replication did not settle in ${E2E_REPLICATION_TIMEOUT}s"; return 1; }

  local exec status succeed failed total
  exec=$(api GET "/replication/executions?policy_id=${E2E_POLICY_ID}&page_size=1" | jq -c '.[0]')
  status=$(printf '%s' "$exec" | jq -r '.status')
  succeed=$(printf '%s' "$exec" | jq -r '.succeed // 0')
  failed=$(printf '%s' "$exec" | jq -r '.failed // 0')
  total=$(printf '%s' "$exec" | jq -r '.total // 0')
  CHECK_DETAIL="${status} ${succeed}/${total}, ${failed} failed"
  [ "$status" = "Succeed" ] && [ "$failed" -eq 0 ] || {
    CHECK_DETAIL="$CHECK_DETAIL — $(_replication_failure_hint)"
    return 1
  }

  # The execution can report Succeed without the bytes landing, so read the
  # destination catalogue back.
  local catalog
  catalog=$(crane_sh <<EOF 2>&1
crane catalog --insecure destreg:5000
EOF
  ) || { CHECK_DETAIL="$CHECK_DETAIL; could not read the destination catalogue"; return 1; }
  printf '%s' "$catalog" | grep -q "replica/${E2E_REPO}" \
    || { CHECK_DETAIL="$CHECK_DETAIL; replica/${E2E_REPO} missing at the destination"; return 1; }
}

_replication_failure_hint() {
  printf 'a 401 on the private repo is the CORE_URL split, see PR #873'
}

_gc_settled() {
  local st
  st=$(api GET "/system/gc?page_size=1&sort=-creation_time" | jq -r '.[0].job_status // "none"')
  case "$st" in Success|Error|Stopped) return 0 ;; *) return 1 ;; esac
}

# 6 — GC with delete_untagged
check_gc() {
  local code
  code=$(api_code POST /system/gc/schedule \
    '{"schedule":{"type":"Manual"},"parameters":{"delete_untagged":true,"dry_run":false,"workers":1}}')
  [ "$code" = "201" ] || { CHECK_DETAIL="GC request returned HTTP $code"; return 1; }

  poll_until "$E2E_GC_TIMEOUT" 3 "GC to settle" _gc_settled \
    || { CHECK_DETAIL="GC did not settle in ${E2E_GC_TIMEOUT}s"; return 1; }

  local job status id
  job=$(api GET "/system/gc?page_size=1&sort=-creation_time" | jq -c '.[0]')
  status=$(printf '%s' "$job" | jq -r '.job_status')
  id=$(printf '%s' "$job" | jq -r '.id')
  CHECK_DETAIL="job ${id} ${status}"
  [ "$status" = "Success" ]
}

# 7 — the upgrade path. Only runs when E2E_UPGRADE_FROM is set; the versions and
# artifact count are captured by run.sh before and after the upgrade.
check_migration() {
  local before=${E2E_SCHEMA_BEFORE} after=${E2E_SCHEMA_AFTER}
  local version_after dirty_after
  version_after=${after%%|*}
  dirty_after=${after##*|}
  CHECK_DETAIL="${E2E_UPGRADE_FROM} schema ${before%%|*} -> ${E2E_TAG} schema ${version_after}"

  [ -n "$version_after" ] || { CHECK_DETAIL="could not read schema_migrations after the upgrade"; return 1; }
  [ "$dirty_after" = "f" ] || { CHECK_DETAIL="$CHECK_DETAIL — schema_migrations is dirty"; return 1; }
  [ "${E2E_REPOS_AFTER}" -ge "${E2E_REPOS_BEFORE}" ] || {
    CHECK_DETAIL="$CHECK_DETAIL — repositories dropped from ${E2E_REPOS_BEFORE} to ${E2E_REPOS_AFTER}"
    return 1
  }
  CHECK_DETAIL="$CHECK_DETAIL, clean, ${E2E_REPOS_AFTER} repos intact"
}
