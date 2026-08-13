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
	"testing"

	"github.com/amirsalarsafaei/sqlc-pgx-monitoring/dbtracer"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracer_ReturnsFullTracer(t *testing.T) {
	tracer, err := NewTracer("harbor_test")
	require.NoError(t, err)
	require.NotNil(t, tracer)

	// NewTracer's declared return type is dbtracer.Tracer, which implies
	// every pgx/pgxpool tracing interface. If NewTracer ever narrows its
	// return type, these assertions fail at compile time instead of
	// silently losing pool-level spans at runtime.
	var _ pgx.QueryTracer = tracer
	var _ pgx.BatchTracer = tracer
	var _ pgx.ConnectTracer = tracer
	var _ pgx.PrepareTracer = tracer
	var _ pgx.CopyFromTracer = tracer
	var _ pgxpool.AcquireTracer = tracer
	var _ pgxpool.ReleaseTracer = tracer
}

func TestNewTracer_EmptyDBNameErrors(t *testing.T) {
	tracer, err := NewTracer("")
	assert.Nil(t, tracer)
	assert.ErrorIs(t, err, dbtracer.ErrDatabaseNameEmpty)
}

func TestWithTracer_AttachesToPoolConfig(t *testing.T) {
	tracer, err := NewTracer("harbor_test")
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/db")
	require.NoError(t, err)

	WithTracer(tracer)(cfg)
	assert.Same(t, tracer, cfg.ConnConfig.Tracer, "WithTracer must set ConnConfig.Tracer on the pool config")
}
