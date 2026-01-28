# ADR-0002: PostgreSQL Connection Pooling with pgxpool

**Status**: Proposed
**Date**: 2026-01-15
**Decision Makers**: Harbor Core Team
**Technical Area**: Database, Performance

## Context

Harbor currently uses `jackc/pgx/v4` through Go's `database/sql` interface via Beego ORM. The connection pooling is managed by `database/sql`, which provides basic pooling but lacks:

1. **Advanced pool management**: No health checking, no warm connection maintenance
2. **Performance tuning**: No prepared statement caching configuration, no connect timeout

### Current State

- **Driver**: `github.com/jackc/pgx/v4` via `database/sql` stdlib adapter
- **ORM**: Beego v2 with 307 `orm.FromContext()` calls across 33 DAO packages
- **Pool config**: `max_idle_conns`, `max_open_conns`, `conn_max_lifetime`, `conn_max_idle_time`
- **Location**: `src/common/dao/pgsql.go`

### Requirements

1. **Backward Compatibility**: Existing Beego ORM code must continue working unchanged
2. **Better Pool Management**: Health checking, connection warmup, configurable timeouts
3. **Performance**: Prepared statement caching, binary protocol support
4. **Observability Ready**: Enable future observability enhancements (see ADR-0003)

## Decision Drivers

- Leverage pgx v5's native pooling for better connection management
- Minimize code changes to existing DAO layer
- Enable future observability integration via pgxpool's tracer interface

## Options Considered

### Option 1: pgxpool with stdlib Bridge (Selected)

Use pgx v5's native `pgxpool` and bridge to `database/sql` via `stdlib.OpenDBFromPool()`.

**Pros**:
- Native pgxpool benefits (health check, warmup, better idle management)
- Full Beego ORM compatibility via stdlib bridge
- Tracer interface enables future observability (see ADR-0003)
- Zero changes to existing DAO code

**Cons**:
- Two layers (pgxpool → stdlib → Beego ORM)
- Some pgxpool features not exposed through database/sql

### Option 2: Replace Beego ORM with pgx Directly

Remove Beego ORM, use pgx/pgxpool directly throughout codebase.

**Pros**:
- Full access to pgx features
- Better performance (no abstraction layers)
- Cleaner architecture

**Cons**:
- Massive refactoring (307+ call sites)
- High risk of regressions
- Long implementation timeline

### Option 3: Keep Current Setup, Add External Pooler

Deploy PgBouncer between Harbor and PostgreSQL.

**Pros**:
- No code changes
- Proven solution

**Cons**:
- Additional infrastructure component
- Operational complexity
- No query-level observability

## Decision

**Selected: Option 1 - pgxpool with stdlib Bridge**

Use pgx v5's `pgxpool` with `stdlib.OpenDBFromPool()` to maintain Beego ORM compatibility while gaining native pool benefits and observability hooks.

### Rationale

1. **Zero DAO Changes**: The 307 `orm.FromContext()` call sites remain unchanged
2. **Extensibility**: pgxpool's tracer interface enables future observability (ADR-0003)
3. **Proven Pattern**: `stdlib.OpenDBFromPool()` is the recommended approach by pgx maintainers

## Implementation

### Phase 1: Core Migration

- [ ] Upgrade `github.com/jackc/pgx/v4` to `github.com/jackc/pgx/v5`
- [ ] Update `src/lib/orm/error.go` - change pgconn import to v5
- [ ] Update `src/common/dao/pgsql.go`:
  - Use `pgxpool.NewWithConfig()` to create pool
  - Bridge via `stdlib.OpenDBFromPool()`
  - Register with Beego ORM via `orm.AddAliasWthDB()`
- [ ] Update `src/cmd/exporter/main.go` - remove legacy stdlib import
- [ ] Update migration driver scheme from `pgx` to `pgx5`
- [ ] Handle backward compatibility: config value `0` uses pgxpool defaults

### Phase 2: Configuration Enhancements

- [ ] Add `health_check_period` config option (default: 1 minute)
- [ ] Add `connect_timeout` config option (default: 10 seconds)
- [ ] Expose pool reference for future observability (ADR-0003)
- [ ] Document config mapping: `max_idle_conns` → `MinConns`, `max_open_conns` → `MaxConns`

## Files to Modify

| File | Change |
|------|--------|
| `src/go.mod` | Upgrade pgx/v4 → pgx/v5 |
| `src/common/dao/pgsql.go` | Use pgxpool with stdlib bridge |
| `src/lib/orm/error.go` | Update pgconn import |
| `src/cmd/exporter/main.go` | Remove legacy stdlib import |
| `src/common/models/database.go` | Add new config fields |
| `src/lib/config/systemconfig.go` | Load new config options |
| `make/harbor.yml` | Document new config options |

## Configuration

### Current → New Mapping

| Harbor Config | database/sql | pgxpool |
|---------------|--------------|---------|
| `max_idle_conns` | MaxIdleConns | **MinConns** |
| `max_open_conns` | MaxOpenConns | **MaxConns** |
| `conn_max_lifetime` | ConnMaxLifetime | MaxConnLifetime |
| `conn_max_idle_time` | ConnMaxIdleTime | MaxConnIdleTime |

### New Options

```yaml
database:
  # Existing (semantic mapping changes)
  max_idle_conns: 50        # Now: minimum warm connections
  max_open_conns: 1000      # Now: maximum pool size
  conn_max_lifetime: 5m
  conn_max_idle_time: 0

  # New options
  health_check_period: 1m   # How often to check idle connections
  connect_timeout: 10s      # Max time to establish new connection
```

## Consequences

### Positive

- Better connection pool management with health checking
- Prepared statement caching for performance
- Modern, actively maintained driver (pgx v5)
- Enables future observability enhancements (ADR-0003)

### Negative

- Slight semantic shift in config options (max_idle_conns → MinConns)

### Mitigations

- Document config mapping clearly in harbor.yml
- Default to pgxpool sensible defaults when config is 0

## Verification

```bash
# Unit tests
task test:unit

# Integration test
task dev:up
curl http://localhost:8080/api/v2.0/ping

# Verify pool initialization in logs
# Look for: "pgxpool initialized: MaxConns=X, MinConns=Y..."
```

## Dependencies

| Library | Purpose |
|---------|---------|
| `github.com/jackc/pgx/v5` | PostgreSQL driver with native pool |

## Related ADRs

- **ADR-0003**: Database Observability (tracing, metrics) - builds on pgxpool foundation

## References

- [pgx v5 Documentation](https://pkg.go.dev/github.com/jackc/pgx/v5)
- [pgxpool Documentation](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)
- [stdlib.OpenDBFromPool](https://pkg.go.dev/github.com/jackc/pgx/v5/stdlib#OpenDBFromPool)

## Changelog

| Date | Change | Author |
|------|--------|--------|
| 2026-01-15 | Initial proposal | Harbor Team |
