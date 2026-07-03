# Harbor Security Export — Top 10 Fix Candidates

Date: 2026-07-01

Ranking criteria: impact first, then clear PoC, easy verification, likely present in upstream/main. Prioritize privilege escalation, user/secret data theft, and Harbor crash/DoS.

Status values:

- Working: reproduced locally.
- Not working: PoC/claim did not reproduce as a Harbor security issue.
- Needs testing: not validated yet.

## Top 10

| Rank | ID | Status | Issue | Impact | PoC quality | Fix size | Notes |
|---:|---|---|---|---|---|---|---|
| 1 | #45 | Not working | System robot with `/system/user:update` can grant true sysadmin | Critical privilege escalation | Clear, safe PoC | Small | Treat as not reproducible as a Harbor issue after review. |
| 2 | #56 | Not working | Webhook SSRF with response-body readback via task logs | Internal service/data theft | Full runtime PoC | Medium | Treat as not reproducible as a Harbor issue after review. |
| 3 | #49-F2 | Not working | Project robot with `member:create/update` can grant projectAdmin | Project takeover | Full runtime PoC | Small | Treat as not reproducible as a Harbor issue after review. |
| 4 | #35 | Working | Manifest upload OOM via unbounded `io.ReadAll` | Harbor core crash / cross-tenant DoS | Safe standalone PoC, verified locally | Medium | Push-capable user can exhaust core memory. Confirmed with capped core; uncapped core showed sustained high memory pressure. |
| 5 | #51 | Needs testing | Forged cosign accessory satisfies content-trust policy | Supply-chain trust bypass | Full runtime PoC | Hard | Harbor checks signature accessory existence, not cryptographic validity. |
| 6 | #52-F4 | Needs testing | Repo-scoped registry token pulls sibling repos in same project | Private repo data leak | Full runtime PoC | Medium | Token verifier discards repository segment and checks project only. |
| 7 | #23 | Needs testing | Webhook policy API returns outbound `auth_header` to read-only maintainers | Secret theft | Full runtime PoC | Small | Mirror scanner credential redaction fix. |
| 8 | #54-F2 | Needs testing | P2P preheat provider credential retarget | External credential theft | Full runtime PoC | Small | Update keeps stored credential while allowing endpoint change. |
| 9 | #15 | Needs testing | Proxy-cache SSRF readback | Internal response body leaked to puller | Full runtime PoC | Medium | Lower than #56 because target is system-admin configured. |
| 10 | #20-F3 | Needs testing | Password change does not revoke existing sessions | Stolen session persists | Full runtime PoC | Medium | Real CWE-613. Fix likely session password-version check. |

## Current Working Candidate

Pick **#35**.

Why:

- Reproduced locally as a core OOM with a push-capable user and a 1 GiB core memory cap.
- Same uncapped test against `localhost:8080` did not crash a 16 GiB container, but memory rose to about 6.9 GiB for a 1.5 GiB invalid manifest body and remained high after the request.
- The PoC is deterministic, standalone, and safe for local test environments.
- Verified locally with `security-pocs/poc-35-manifest-upload-oom.sh`.

## Important Skips

| ID | Reason |
|---|---|
| #47 | Already fixed on current `upstream/main` and this branch: blob-mount source project is now checked by `tokenIssuedAfterProjectCreation`. |
| #60 | Claimed SQLi is not reproducible; sink exists but attacker path is not reachable. |
| #21 | Scanner credential disclosure already mitigated upstream. |
| #46 | Root-cause transport SSRF is real, but non-webhook sinks are mostly system-admin-gated. Fix overlaps #56/#15. |

## Local PoC Files

- `security-pocs/poc-45-robot-userupdate-sysadmin.sh`
- `security-pocs/poc-56-webhook-ssrf-readback.sh`
- `security-pocs/poc-49-project-robot-member-projectadmin.sh`
- `security-pocs/poc-35-manifest-upload-oom.sh`

Keep these private until coordinated disclosure completes.
