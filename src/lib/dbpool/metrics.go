// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dbpool

import (
	"fmt"

	"github.com/amirsalarsafaei/sqlc-pgx-monitoring/dbtracer"
	"github.com/amirsalarsafaei/sqlc-pgx-monitoring/poolstatus"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewTracer builds an OTel pgx tracer for the given database. The returned
// value implements query, batch, connect, prepare, copy-from, acquire, and
// release tracing (dbtracer.Tracer), so pgxpool picks up pool-level spans
// in addition to per-query spans. SQL text and args are excluded from spans
// and logs to avoid leaking sensitive data.
func NewTracer(dbName string) (dbtracer.Tracer, error) {
	t, err := dbtracer.NewDBTracer(
		dbName,
		dbtracer.WithIncludeSQLText(false),
		dbtracer.WithLogArgs(false),
		dbtracer.WithShouldLog(func(err error) bool { return err != nil }),
	)
	if err != nil {
		return nil, fmt.Errorf("dbpool: new tracer: %w", err)
	}
	return t, nil
}

// WithTracer returns an Option that attaches the given pgx query tracer to
// the pool. The parameter is typed as pgx.QueryTracer because that is the
// static type of pgxpool.Config.ConnConfig.Tracer; pgxpool recovers the
// pool-level tracer interfaces (AcquireTracer, ReleaseTracer, BatchTracer,
// …) via runtime type assertion, so any tracer that implements them (for
// example the one returned by NewTracer) will have them exercised.
func WithTracer(tracer pgx.QueryTracer) Option {
	return func(cfg *pgxpool.Config) {
		cfg.ConnConfig.Tracer = tracer
	}
}

// RegisterPoolMetrics publishes pgxpool statistics (idle, total, acquired
// connections) as OTel observable gauges.
func RegisterPoolMetrics(pool *pgxpool.Pool) error {
	if err := poolstatus.Register(pool); err != nil {
		return fmt.Errorf("dbpool: register pool metrics: %w", err)
	}
	return nil
}
