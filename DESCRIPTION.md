# perf: Detach quota reserve/rollback from the request-wide transaction

## Idea

`quota.RequestMiddleware` reserves quota before proxying a push to the registry.
Under `transaction.Middleware` the reserve's version-CAS `UPDATE quota_usage`
executed on the request-wide `TxOrmer`, so the row lock taken by that UPDATE was
held until the request transaction committed — across the proxied registry
round-trip, the abstractor HTTP fetches in `artifact.Ctl.Ensure`, and all
after-response blob bookkeeping. Every concurrent push to the same project
serialized on that one row for the full request duration.

This patch runs the reserve (and the compensating rollback) on a fresh
autocommit ormer when the incoming context carries a transactional ormer. The
row lock now lasts exactly one UPDATE statement. Semantics are unchanged:
reserve-before-proxy, compensate-on-failure via the pre-existing
`rollbackResources` path in `Request`.

## Mechanism

- `src/lib/orm/orm.go`: new `InTransaction(ctx)` helper — true when the ctx
  ormer is a beego `TxOrmer` (i.e. inside `WithTransaction`, e.g. the request-
  wide tx started by the transaction middleware).
- `src/controller/quota/controller.go` `Request`: when the update provider is
  the DB and `orm.InTransaction(ctx)`, the reserve runs on `orm.Clone(ctx)`
  (fresh autocommit ormer, request cancellation kept) and the rollback on
  `orm.Copy(ctx)` (fresh autocommit ormer, cancellation dropped so the
  compensation still lands after the client goes away). No explicit side
  transaction is needed: the reserve is a `SELECT quota` + `SELECT quota_usage`
  + one CAS `UPDATE`, and the version CAS already provides the atomicity, so
  autocommit is strictly better than a BEGIN/COMMIT pair (fewer statements,
  shortest possible lock hold).
- The redis update provider path is untouched (its request-path usage lives in
  redis; the DB is only read on a cache miss via `calcQuota`, and usage is
  flushed to the DB asynchronously).
- Unit tests: `InTransaction` table test; `Request` tests asserting the reserve
  is persisted before `f()` runs and that the rollback compensates (usage back
  to the pre-reserve value) when `f()` fails; `TestInflightLedger` covering the
  ledger round-trip and expiry reaping (skips when redis is unreachable).

## Refresh reconcile (in-flight reservation ledger)

Detaching the reserve opens a window the request-wide transaction used to
close implicitly: the committed reservation is visible in `quota_usage` while
the push's artifact/blob rows are still uncommitted. A concurrent `Refresh`
(artifact delete, GC, retention, manual) recalculates usage from those tables,
cannot see the in-flight rows, and would CAS-overwrite the reservation
(baseline avoided this only because Refresh's UPDATE blocked on the row lock
until the push committed, then CAS-failed and recalculated against committed
rows).

`src/controller/quota/inflight.go` closes the window with a redis ledger:

- `Request` (DB provider only) records the reserved delta under a unique token
  **before** the reserve commits, so no ordering lets Refresh observe the
  reservation without the ledger entry.
- `Refresh`'s `calculateUsage` adds the sum of live ledger entries on top of
  `driver.CalculateUsage`, so in-flight reservations survive a concurrent
  recalculation.
- The entry is removed after the enclosing request transaction commits
  (`orm.AfterCommit`; runs immediately when there is no tx, e.g. the blob PUT
  route) — at that point the rows are countable by the recalculation itself —
  or right after a failed `f()` has been compensated by `rollbackResources`.
- Crash safety: each entry carries a 10-minute deadline; readers skip and reap
  expired entries, and the key has a 2× TTL as idle GC. An orphaned entry
  therefore over-counts refreshes for at most 10 minutes, after which the next
  Refresh converges — deliberately erring toward blocking pushes rather than
  breaching quota.
- Redis unavailability degrades gracefully: track/read failures are logged and
  behavior falls back to the pre-ledger semantics (reserve still happens; the
  race window returns until redis is back). This also means the ledger adds
  zero SQL statements; it is 1–2 redis round-trips per reserving request.

Known residual (documented, judged acceptable): between the request-tx commit
and the `AfterCommit` HDEL, a Refresh counts the rows *and* the ledger entry —
a transient over-count that the same Refresh path corrects on its next run.
If the request tx rolls back after a successful `f()` (commit failure), the
`AfterCommit` hook never fires and the entry lives until its deadline; usage
is over-counted for that window, consistent with the crash exposure below.

## Files changed

- `src/lib/orm/orm.go` (`InTransaction` helper)
- `src/lib/orm/in_transaction_test.go` (new)
- `src/controller/quota/controller.go` (`Request` detach + ledger wiring,
  `Refresh` in-flight adjustment)
- `src/controller/quota/inflight.go` (new: in-flight reservation ledger)
- `src/controller/quota/controller_test.go`, `inflight_test.go` (tests)

## Measurements

Harness (single-client push+pull of alpine:latest, Postgres `log_statement=all`,
same machine/method as the baseline):

| metric      | main@177e51e99 | patched | delta |
|-------------|----------------|---------|-------|
| push stmts  | 286            | 286     | 0     |
| push tx     | 9              | 9       | 0     |
| pull stmts  | 55             | 55      | 0     |

No statement-count change — expected and honest: this patch moves the lock
window, it does not remove queries (autocommit avoids even the +2 BEGIN/COMMIT
the idea budgeted for).

Concurrency lock probe (8 parallel `crane copy` pushes of 8 distinct alpine
images into ONE project, `log_lock_waits=on`, `deadlock_timeout=50ms`,
pg_locks sampled every ~150 ms for waiters whose query touches quota_usage;
identical script run against baseline via `git stash` in this worktree):

| metric                                  | baseline | patched |
|-----------------------------------------|----------|---------|
| quota_usage UPDATEs inside request tx   | 46       | 0       |
| quota_usage UPDATEs autocommit          | 0        | 30      |
| logged lock waits >50ms (quota_usage row) | 2      | 0       |
| pg_locks samples with waiters           | 3 of 16  | 0 of 16 |
| max concurrent backends blocked on row  | 5        | 0       |
| wall time, 8 concurrent pushes          | 2.59 s   | 2.53 s  |

Interpretation: baseline had up to 5 of 8 pushers simultaneously blocked on the
quota_usage tuple; patched shows zero cross-request quota_usage waits in every
sample, and fewer total UPDATE attempts (30 vs 46) because writers no longer
queue behind a long-held row lock and then CAS-fail in a burst. Local wall time
is within noise because the local registry round-trip is sub-ms; the lock hold
it eliminates scales with the registry round-trip (100 ms – seconds against
S3/R2-backed registries in production, where quota_usage contention is the
observed fleet bottleneck).

## Risks

- The reservation is durable independently of the request tx. A core crash (or
  request-tx commit failure) between the committed reserve and the rollback
  leaves the quota over-counted until the next `Refresh` (any subsequent
  successful push/delete of the project, or a manual refresh); the matching
  ledger entry additionally makes refreshes over-count until its 10-minute
  deadline. This matches the direction of the exposure already accepted by the
  redis update provider (asynchronous flush): quota errs toward over-counting,
  never toward breaching the limit.
- Concurrent `Refresh` vs in-flight reserve is reconciled by the ledger (see
  above); with redis down the pre-ledger race window returns, bounded by redis
  availability.
- Each in-flight push briefly takes one extra pooled connection for the
  reserve (2 SELECTs + 1 UPDATE). In the theoretical case of ≥ max_open_conns
  concurrent in-tx requests all reserving at once, requests could wait on the
  pool while holding their tx connection; the window is sub-ms and
  POSTGRESQL_MAX_OPEN_CONNS=100 per service makes this unlikely, but it is a
  new (bounded) pool interaction.
- The rollback deliberately drops request cancellation (`orm.Copy`) so the
  compensation runs even when the client disconnected mid-push; previously the
  request-tx rollback undid the reserve implicitly in that case.

## Rollback

Revert the branch (single commit) or just the `Request` hunk in
`src/controller/quota/controller.go`; `orm.InTransaction` is a pure additive
helper. No config, schema, or API surface involved.
