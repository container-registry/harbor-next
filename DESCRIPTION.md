# perf: Lazy CVE allowlist fetch in vulnerable middleware

## Idea

`src/server/middleware/vulnerable/vulnerable.go` runs on every manifest GET/HEAD and
fetched the project with `project.WithEffectCVEAllowlist()` before checking whether the
project has vulnerability prevention (`prevent_vul`) enabled at all. That option pulls
the `cve_allowlist` row (or, when the project reuses the system allowlist, the system
`cve_allowlist` row) on every manifest request, even though the value is only consumed
when prevention is enforced. `prevent_vul` off is the overwhelming default, and the
allowlist query path is not covered by the redis object cache, so this is a real DB
round-trip per pull fleet-wide (including trivy pull-backs during auto-scan).

## Change

`src/server/middleware/vulnerable/vulnerable.go`:

1. First fetch the project with default options (project + metadata only, no allowlist).
2. Return early when `VulPrevented()` is false — no allowlist query at all.
3. The scanner/cosign bypass (`util.SkipPolicyChecking`) also returns before any
   allowlist query now, so scanner-driven pulls skip it even with `prevent_vul` on.
4. Only on the enforced path, re-fetch the project with
   `project.WithEffectCVEAllowlist()`. This goes through the controller's
   `loadEffectCVEAllowlists`, so `ReuseSysCVEAllowlist` semantics and
   `CVEAllowlist.IsExpired()` behavior are byte-identical to before.

Cost on the enforced path: one extra project + project_metadata read (both covered by
the redis object cache in production config). Enforcement outcomes are unchanged.

`src/server/middleware/vulnerable/vulnerable_test.go`: added `AssertNumberOfCalls`
assertions — 1 project Get when prevention is disabled and for scanner pulls (allowlist
never fetched), 2 on the enforced path.

## Measurements

Same harness as baseline (devenv, Postgres log_statement=all, crane push+pull
alpine:latest, redis object cache off).

| metric | baseline main@177e51e99 | patched (2 runs) |
|---|---|---|
| push stmts | 286 | 288 / 283 |
| push tx | 9 | 9 / 9 |
| pull stmts | 55 | 54 / 54 |
| cve_allowlist stmts in pull window | 1 | **0** (counted in run 2) |

Honest read: the pull saving is exactly 1 statement (54 vs 55), stable across runs, and
run 2 counted zero `cve_allowlist` statements in the pull window, confirming the
mechanism. The test project does not trigger the 2-statement case (project allowlist +
system merge). Push numbers vary by ±3 between runs (periodic health checks and async
handlers land inside the 8s capture window), so the push-side saving (crane's manifest
existence check) is within noise. This is a small win, but it applies to every manifest
GET/HEAD in the fleet, including the 4 trivy pull-back pipelines per auto-scanned push,
and it survives production config because the allowlist path is not redis-cached.

## Risks

Low. `prevent_vul` off: strictly fewer queries, same outcome. `prevent_vul` on: one
extra project/metadata read (redis-cached in prod), allowlist loaded via the same
controller path as before. A project whose `prevent_vul` flips between the two Get
calls within one request could see the second read's metadata — same class of
non-transactional read as before, not a new race.

## Rollback

Revert the single commit on `perf/vulnerable-lazy-allowlist`; the change is confined to
`src/server/middleware/vulnerable/vulnerable.go` and its test file.
