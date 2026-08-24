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
- The redis update provider path is untouched (it does not touch the DB in the
  request path). `Refresh` is untouched: its recalculation reads uncommitted
  rows of the request tx, so detaching it would persist usage derived from data
  that may roll back.
- Unit tests: `InTransaction` table test; `Request` tests asserting the reserve
  is persisted before `f()` runs and that the rollback compensates (usage back
  to the pre-reserve value) when `f()` fails.

## Files changed

- `src/lib/orm/orm.go` (+12)
- `src/lib/orm/in_transaction_test.go` (new)
- `src/controller/quota/controller.go` (+18, -2)
- `src/controller/quota/controller_test.go` (+38)

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
  successful push/delete of the project, or a manual refresh). This matches
  the exposure already accepted by the redis update provider, which flushes
  usage to the DB asynchronously.
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
