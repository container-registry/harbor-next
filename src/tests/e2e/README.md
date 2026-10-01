# End-to-end tests: database hot-row contention fixes

Verifies the four fixes for the 2026-08 push-burst incident (quota CAS
backoff, blob Touch debounce, unlimited-quota reservation skip, coalesced
async refresh) against a RUNNING Harbor - by default the slot-0 dev
environment (`task dev:up SLOT=0`).

Run:

    cd src && go test -tags e2e ./tests/e2e/ -v -count=1

Configuration (env, with slot-0 defaults):

    E2E_CORE_URL        http://localhost:8080
    E2E_ADMIN_USER      admin
    E2E_ADMIN_PASSWORD  Harbor12345
    E2E_DB_DSN          postgres://postgres:root123@localhost:5432/registry?sslmode=disable
    E2E_ASYNC_REFRESH   (unset)

`TestAsyncRefreshConvergence` runs only when `E2E_ASYNC_REFRESH=1` is set,
and expects the target core to have been started with
`QUOTA_ASYNC_REFRESH_DURATION`; otherwise the test is skipped.
