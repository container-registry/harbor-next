# ADR-0003: Database Observability with pgx Monitoring

**Status**: Accepted
**Date**: 2026-01-15
**Decision Makers**: 8gcr Team
**Technical Area**: Observability, Database, Monitoring

## Context

Harbor uses PostgreSQL via the `jackc/pgx` driver. We need comprehensive database observability to:

1. Detect and analyze slow queries
2. Identify query patterns for potential caching opportunities
3. Monitor connection pool health and saturation
4. Support SLA monitoring with percentile-based metrics (p50, p95, p99)

### Requirements

1. **Slow Query Detection**: Histogram-based latency metrics for percentile analysis
2. **Query Counting**: Ability to group and count similar queries
3. **Pool Saturation Monitoring**: Track connection pool wait times and saturation
4. **OpenTelemetry Compliance**: Follow OTel semantic conventions for interoperability
5. **Prometheus Integration**: Export metrics to Prometheus for Grafana dashboards
6. **No sqlc Dependency**: Must work with plain pgx (Harbor doesn't use sqlc)

## Decision Drivers

- Histogram support for percentile-based alerting (p95, p99 SLAs)
- Complete pool metrics including `EmptyAcquireWaitTime`
- Single library solution to reduce maintenance burden
- OpenTelemetry semantic conventions compliance
- Active maintenance and reasonable test coverage

## Options Considered

### Option 1: otelpgx + pgxpoolprometheus

Use two libraries: [exaring/otelpgx](https://github.com/exaring/otelpgx) for tracing and [IBM/pgxpoolprometheus](https://github.com/IBM/pgxpoolprometheus) for pool metrics.

**otelpgx:**
- OpenTelemetry tracing for queries, batches, connections, prepare, copy operations
- Pool metrics via `RecordStats()` (OTel format)
- Test coverage: 8.7%
- 189 stars, 26 contributors
- Apache 2.0 license

**pgxpoolprometheus:**
- 12 native Prometheus metrics for pool statistics
- Test coverage: 90%
- 6 contributors (IBM)
- Apache 2.0 license

| Metric Type | otelpgx | pgxpoolprometheus |
|-------------|---------|-------------------|
| Query duration | Histogram | N/A |
| Pool acquire duration | Counter (via RecordStats) | Counter only |
| EmptyAcquireWaitTime | Yes (via RecordStats) | **Missing** |

**Pros:**
- Native Prometheus metrics from pgxpoolprometheus (no OTel bridge needed)
- Mature, well-tested pool metrics library

**Cons:**
- Two libraries to maintain and configure
- pgxpoolprometheus missing `EmptyAcquireWaitTime` metric (critical for saturation detection)
- Pool acquire duration is counter-only (no percentile analysis)
- Mixed metric formats (OTel tracing + native Prometheus metrics)
- Low test coverage in otelpgx (8.7%)

### Option 2: otelpgx with RecordStats Only

Use only otelpgx, including its `RecordStats()` for pool metrics.

**Pros:**
- Single library
- Has `EmptyAcquireWaitTime` metric
- Consistent OTel format

**Cons:**
- Pool acquire duration still counter-only (no histogram)
- Requires OTel-to-Prometheus bridge
- Low test coverage (8.7%)

### Option 3: sqlc-pgx-monitoring (Selected)

Use [amirsalarsafaei/sqlc-pgx-monitoring](https://github.com/amirsalarsafaei/sqlc-pgx-monitoring) with OpenTelemetry-to-Prometheus bridge.

**Despite the name, this library works without sqlc** - the sqlc query name extraction is optional and gracefully skipped when not present.

**Pros:**
- Single library for both tracing and metrics
- Full histogram support for query AND pool acquire duration
- Includes `EmptyAcquireWaitTime` metric
- Higher test coverage (84% average: 88.9% tracer, 78% pool)
- OpenTelemetry semantic conventions v1.26.0 compliant
- MIT license
- Active maintenance (last update August 2025)
- Works with plain pgx (no sqlc required)

**Cons:**
- Requires OTel-to-Prometheus bridge (5 lines of setup)
- Requires Go 1.23+

## Decision

**Selected: Option 3 - sqlc-pgx-monitoring**

We will use sqlc-pgx-monitoring with the OpenTelemetry-to-Prometheus exporter bridge.

### Rationale

1. **Complete Histogram Support**: Provides histograms for both query duration AND pool acquire duration, enabling percentile-based SLA monitoring (p50, p95, p99)

2. **Pool Saturation Detection**: Includes `pgx.pool.acquire.wait.duration` metric, critical for detecting pool exhaustion before it causes outages

3. **Single Library**: Reduces maintenance burden compared to managing two separate libraries

4. **Better Test Coverage**: 84% average coverage vs. otelpgx's 8.7%

5. **OTel Compliance**: Follows OpenTelemetry semantic conventions v1.26.0, ensuring compatibility with standard observability backends

6. **Works Without sqlc**: Confirmed by code analysis - the library gracefully handles absence of sqlc query name comments

### Metrics Provided

#### Query Operation Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `db.client.operation.duration` | Histogram | Query latency with configurable buckets |

Default histogram buckets (seconds): `[0.001, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10]`

#### Pool Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `db.client.connections.usage` | Gauge | Current connections (idle/used) |
| `db.client.connections.max` | Gauge | Maximum pool size |
| `db.client.connections.pending_requests` | Gauge | Connections being constructed |
| `pgx.pool.acquires` | Counter | Cumulative successful acquires |
| `pgx.pool.canceled_acquires` | Counter | Canceled acquisitions |
| `pgx.pool.waited_for_acquires` | Counter | Acquisitions that waited |
| `pgx.pool.connections.created` | Counter | New connections opened |
| `pgx.pool.connections.destroyed` | Counter | Connections destroyed (by reason) |
| `pgx.pool.acquire.duration` | Counter | Total acquire duration |
| `pgx.pool.acquire.wait.duration` | Counter | Wait time when pool empty |
| `pgx.pool.trace.acquire.duration` | Histogram | Acquire latency distribution |

#### Tracing Spans

| Span Name | Operation |
|-----------|-----------|
| `postgresql.query` | Individual queries |
| `postgresql.batch` | Batch operations |
| `postgresql.batch.query` | Individual queries within batch |
| `postgresql.connect` | Connection establishment |
| `postgresql.copy_from` | COPY operations |
| `postgresql.prepare` | Prepared statements |
| `pgxpool.acquire` | Pool connection acquisition |

#### Span Attributes (OTel Semconv Compliant)

| Attribute | Description |
|-----------|-------------|
| `db.system` | `postgresql` |
| `db.namespace` | Database name |
| `db.operation.name` | Query/operation name |
| `db.query.text` | SQL text (optional, disabled by default) |
| `db.collection.name` | Table name (for COPY) |
| `db.response.status_code` | PostgreSQL error code |
| `pgx.operation.type` | query/batch/connect/prepare/copy_from |
| `pgx.status` | OK/UNKNOWN_ERROR/{pg_severity} |

### Implementation

#### Setup Code

```go
package database

import (
    "context"
    "fmt"

    "github.com/amirsalarsafaei/sqlc-pgx-monitoring/dbtracer"
    "github.com/amirsalarsafaei/sqlc-pgx-monitoring/poolstatus"
    "github.com/jackc/pgx/v5/pgxpool"

    "go.opentelemetry.io/otel"
    promexporter "go.opentelemetry.io/otel/exporters/prometheus"
    "go.opentelemetry.io/otel/sdk/metric"
)

// InitOTelPrometheusExporter sets up the OTel-to-Prometheus bridge.
// Call this once at application startup.
func InitOTelPrometheusExporter() error {
    exporter, err := promexporter.New()
    if err != nil {
        return fmt.Errorf("create prometheus exporter: %w", err)
    }
    provider := metric.NewMeterProvider(metric.WithReader(exporter))
    otel.SetMeterProvider(provider)
    return nil
}

// NewPool creates a pgxpool with observability enabled.
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
    // Create tracer
    tracer, err := dbtracer.NewDBTracer("harbor",
        dbtracer.WithIncludeSQLText(false),      // Don't log SQL in spans (security)
        dbtracer.WithLogArgs(false),             // Don't log query arguments
    )
    if err != nil {
        return nil, fmt.Errorf("create db tracer: %w", err)
    }

    // Parse config and attach tracer
    cfg, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    cfg.ConnConfig.Tracer = tracer

    // Create pool
    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }

    // Register pool metrics
    poolstatus.Register(pool)

    return pool, nil
}
```

#### Prometheus Queries for Alerting

```promql
# Slow query detection (p99 > 500ms)
histogram_quantile(0.99,
  rate(db_client_operation_duration_bucket[5m])
) > 0.5

# Pool saturation (p99 acquire time > 100ms)
histogram_quantile(0.99,
  rate(pgx_pool_trace_acquire_duration_bucket[5m])
) > 0.1

# Pool wait time trending up
rate(pgx_pool_acquire_wait_duration[5m]) > 0

# Connection pool utilization
db_client_connections_usage{state="used"} / db_client_connections_max > 0.8
```

#### Grafana Dashboard Panels

1. **Query Latency Heatmap**: `db_client_operation_duration_bucket`
2. **Query Latency Percentiles**: p50, p95, p99 lines
3. **Pool Connections**: Stacked area (used, idle, max)
4. **Pool Acquire Latency**: Histogram heatmap
5. **Pool Wait Time**: Rate of wait duration

### Query Counting Strategy

Neither this library nor alternatives provide automatic query normalization. For counting similar queries:

**Option A: Use Prepared Statements**
```go
// Queries with consistent names can be grouped
_, err := pool.Exec(ctx, "get_user_by_id", "SELECT * FROM users WHERE id = $1", userID)
```

**Option B: Application-Level Instrumentation**
```go
func (r *Repo) GetUser(ctx context.Context, id int) (*User, error) {
    ctx, span := tracer.Start(ctx, "repo.GetUser")
    defer span.End()
    // Query execution...
}
```

**Option C: Observability Backend Normalization**
Tools like Grafana Tempo and Jaeger can normalize queries for grouping.

## Consequences

### Positive

- Complete histogram support for percentile-based SLA monitoring
- Single library reduces maintenance overhead
- Pool saturation detection via wait time metrics
- OTel semantic conventions ensure backend compatibility
- Higher confidence from better test coverage

### Negative

- Requires OTel-to-Prometheus bridge setup
- Go 1.23+ requirement (may need upgrade)
- Query counting requires additional application-level work

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Library maintenance stops | MIT license allows forking; code is well-structured |
| OTel bridge adds overhead | Minimal overhead; same pattern used across industry |
| Missing features | Library is actively maintained; can contribute PRs |

## Alternatives Rejected

### pgxpoolprometheus (for pool metrics)

Rejected because it lacks `EmptyAcquireWaitTime` metric and only provides counters (not histograms) for acquire duration, making percentile analysis impossible.

### otelpgx alone

Rejected because pool acquire duration is counter-only. While it has `EmptyAcquireWaitTime`, it doesn't provide the histogram distribution needed for p95/p99 alerting on pool acquire latency.

## References

- [sqlc-pgx-monitoring GitHub](https://github.com/amirsalarsafaei/sqlc-pgx-monitoring)
- [OpenTelemetry Database Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/database/)
- [OpenTelemetry Prometheus Exporter](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/prometheus)
- [pgx v5 Documentation](https://pkg.go.dev/github.com/jackc/pgx/v5)

## Changelog

| Date | Change | Author |
|------|--------|--------|
| 2026-01-15 | Initial decision | 8gcr Team |
