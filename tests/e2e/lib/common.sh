#!/usr/bin/env bash
# Logging, timing, HTTP helpers and the result table.
# shellcheck shell=bash

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_DIM=$'\033[2m'; C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BOLD=$'\033[1m'
else
  C_RESET=''; C_DIM=''; C_RED=''; C_GREEN=''; C_YELLOW=''; C_BOLD=''
fi

log()  { printf '%s==>%s %s\n' "$C_BOLD" "$C_RESET" "$*" >&2; }
info() { printf '    %s%s%s\n' "$C_DIM" "$*" "$C_RESET" >&2; }
warn() { printf '%s!!%s  %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()  { printf '%serror:%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; exit 1; }

now() { date +%s; }

# --- result table -----------------------------------------------------------
# Records accumulate as "name<TAB>status<TAB>seconds<TAB>detail".
RESULTS=()
FAILED=0
SKIPPED=0

record() { # record NAME STATUS SECONDS [DETAIL]
  RESULTS+=("$1	$2	$3	${4:-}")
  case "$2" in
    FAIL) FAILED=$((FAILED + 1)) ;;
    SKIP) SKIPPED=$((SKIPPED + 1)) ;;
  esac
}

# run_check NAME FUNCTION — times it, records PASS/FAIL, never aborts the run.
# The detail line comes from whatever the check writes to $CHECK_DETAIL.
run_check() {
  local name=$1 fn=$2 start rc
  CHECK_DETAIL=""
  log "check: $name"
  start=$(now)
  set +e
  "$fn"
  rc=$?
  set -e
  local elapsed=$(( $(now) - start ))
  if [ "$rc" -eq 0 ]; then
    record "$name" PASS "$elapsed" "$CHECK_DETAIL"
    printf '    %sPASS%s %s (%ss) %s\n' "$C_GREEN" "$C_RESET" "$name" "$elapsed" "$CHECK_DETAIL" >&2
  else
    record "$name" FAIL "$elapsed" "$CHECK_DETAIL"
    printf '    %sFAIL%s %s (%ss) %s\n' "$C_RED" "$C_RESET" "$name" "$elapsed" "$CHECK_DETAIL" >&2
  fi
  return 0
}

print_table() { # print_table TOTAL_SECONDS
  local total=$1 rec name status secs detail colour
  printf '\n'
  printf '%s%-14s %-6s %6s  %s%s\n' "$C_BOLD" "CHECK" "RESULT" "TIME" "DETAIL" "$C_RESET"
  printf -- '---------------------------------------------------------------------------\n'
  for rec in "${RESULTS[@]}"; do
    IFS=$'\t' read -r name status secs detail <<<"$rec"
    case "$status" in
      PASS) colour=$C_GREEN ;;
      FAIL) colour=$C_RED ;;
      *)    colour=$C_YELLOW ;;
    esac
    printf '%-14s %s%-6s%s %5ss  %s\n' "$name" "$colour" "$status" "$C_RESET" "$secs" "$detail"
  done
  printf -- '---------------------------------------------------------------------------\n'
  printf '%-14s %-6s %5ss  %d passed, %d failed, %d skipped\n' \
    "TOTAL" "" "$total" \
    "$(( ${#RESULTS[@]} - FAILED - SKIPPED ))" "$FAILED" "$SKIPPED"
  printf '\n'
}

# --- http -------------------------------------------------------------------

api() { # api METHOD PATH [JSON_BODY]
  local method=$1 path=$2 body=${3:-}
  if [ -n "$body" ]; then
    curl -sS -u "admin:${E2E_ADMIN_PASSWORD}" -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "${E2E_API}${path}"
  else
    curl -sS -u "admin:${E2E_ADMIN_PASSWORD}" -X "$method" "${E2E_API}${path}"
  fi
}

api_code() { # api_code METHOD PATH [JSON_BODY] — prints the HTTP status only
  local method=$1 path=$2 body=${3:-}
  if [ -n "$body" ]; then
    curl -sS -o /dev/null -w '%{http_code}' -u "admin:${E2E_ADMIN_PASSWORD}" -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "${E2E_API}${path}"
  else
    curl -sS -o /dev/null -w '%{http_code}' -u "admin:${E2E_ADMIN_PASSWORD}" -X "$method" "${E2E_API}${path}"
  fi
}

# poll_until TIMEOUT INTERVAL DESCRIPTION COMMAND...
# Returns 0 as soon as COMMAND succeeds, 1 on timeout.
poll_until() {
  local timeout=$1 interval=$2 desc=$3; shift 3
  local start; start=$(now)
  while :; do
    if "$@"; then return 0; fi
    if [ $(( $(now) - start )) -ge "$timeout" ]; then
      warn "timed out after ${timeout}s waiting for ${desc}"
      return 1
    fi
    sleep "$interval"
  done
}
