# DB connection-exhaustion repro

A deterministic reproduction of the core connection-pool deadlock reported in
harbor-next [#850], [#856] and [#92]. It wedges the real Harbor middleware
chain against a real PostgreSQL, in about a minute, without a Kubernetes
cluster, a scanner or a populated registry.

```bash
./run.sh                      # pool of 4 connections
REPRO_MAX_CONNS=10 ./run.sh   # any pool size; the result is the same shape
REPRO_KEEP_PG=1 ./run.sh      # leave PostgreSQL up after the run
```

Needs Docker Compose v2 — the `docker compose` plugin, with `--wait`. The script
checks for it and says so rather than failing in a way that looks like a repro bug.

It starts its own PostgreSQL (project `dbpool-repro`, bound to
`127.0.0.1:55432`, tmpfs data directory) so it cannot disturb a `task dev:up`
instance or the clusters, and is not reachable from the network.
`max_connections` is derived from `REPRO_MAX_CONNS` with headroom, so the
server's own limit is never what the repro ends up measuring.

To watch the wedge in `pg_stat_activity`, connect from a second terminal *while*
a run is inside one of its observation windows. The stranded sessions belong to
the test process, so PostgreSQL tears them down when it exits; `REPRO_KEEP_PG=1`
keeps the server, not the wedge.

## What it drives

`src/lib/dbpool/exhaustion_repro_test.go`, behind the `dbpool_repro` build tag,
serves `orm.Middleware` → `transaction.Middleware` → a handler over
`httptest`. These are the production middlewares, not stand-ins. The handler's
write path is the shape `src/pkg/auditext/event/user/user.go:61` has today: a
query issued on `orm.Context()` while the request's own transaction is still
open, so the request needs **two** pool connections and holds the first while
it waits for the second.

Requests are released through a barrier. Under production load, N requests are
inside their transactions at the same moment by chance; the barrier makes that
overlap happen every run. It adds no connection use of its own.

## What each phase proves

| Phase | Setup | Expected |
|---|---|---|
| 1 | `MaxConns - 1` concurrent two-connection writes | all complete — one connection is always free, so they serialise |
| 2 | `MaxConns` concurrent two-connection writes | none complete; `pg_stat_activity` shows exactly `MaxConns` sessions `idle in transaction` |
| 3 | one unrelated `GET` during the wedge (no transaction, no second connection) | also hangs — the outage is instance-wide, not endpoint-local |
| 4 | every client hangs up | the transactions stay open — nothing releases them but a restart |

The threshold is exact: `MaxConns - 1` concurrent requests always finish,
`MaxConns` never do. There is no load level in between and no flakiness to
tune, because the deadlock is structural — N holders of one connection each,
all waiting for the N+1st.

Phase 4 is the part that turns a bug into an outage. `ormerTx.Begin` calls
beego's `Ormer.Begin`, which passes `context.Background()` to `BeginTx`
(`beego/v2@v2.3.10/client/orm/orm.go:530`), and `pgxpool` has no acquire
timeout of its own. No request deadline, client disconnect or job timeout can
reach `pgxpool.Acquire`, which is why the three-minute timeout in `startScanAll`
had no effect on the production incident in #856.

## Using it against a fix

The test asserts the broken behaviour, so it fails when the bug is gone — which
is the point. Read the phase it fails on:

- fails phase 2 (requests complete) — the second connection is gone, or the
  acquire is now bounded and requests fail instead of hanging.
- fails phase 3 (the GET completes) — reads no longer queue behind writers.
- fails phase 4 (transactions released) — a client disconnect now unwinds the
  acquire.

[#92]: https://github.com/container-registry/harbor-next/issues/92
[#850]: https://github.com/container-registry/harbor-next/issues/850
[#856]: https://github.com/container-registry/harbor-next/issues/856
