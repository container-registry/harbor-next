package dbtracer

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"
)

var pgxPoolConnOperationReleased = PGXPoolConnOperationKey.String("release")

// TraceRelease implements Tracer.
func (dt *dbTracer) TraceRelease(_ *pgxpool.Pool, _ pgxpool.TraceReleaseData) {
	dt.connReleaseCounter.Add(context.Background(), 1, metric.WithAttributes(
		pgxPoolConnOperationReleased,
	))
}
