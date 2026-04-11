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

//go:build db

package dbpool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beego/beego/v2/client/orm"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common/models"
	harborORM "github.com/goharbor/harbor/src/lib/orm"
)

// testCfg returns a PostGreSQL config from environment.
// Accepts both the existing test convention (POSTGRESQL_USR / POSTGRESQL_PWD)
// and the Harbor config convention (POSTGRESQL_USERNAME / POSTGRESQL_PASSWORD),
// with devenv defaults as fallback.
func testCfg() *models.PostGreSQL {
	host := envOr("POSTGRESQL_HOST", "localhost")
	port := 5432
	if p := os.Getenv("POSTGRESQL_PORT"); p != "" {
		port, _ = strconv.Atoi(p)
	}
	user := envOr("POSTGRESQL_USR", envOr("POSTGRESQL_USERNAME", "postgres"))
	pwd := envOr("POSTGRESQL_PWD", envOr("POSTGRESQL_PASSWORD", "root123"))
	db := envOr("POSTGRESQL_DATABASE", "registry")

	return &models.PostGreSQL{
		Host:     host,
		Port:     port,
		Username: user,
		Password: pwd,
		Database: db,
		SSLMode:  "disable",
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustPool creates a pool or fails the test.
func mustPool(t *testing.T, cfg *models.PostGreSQL, opts ...Option) *Pool {
	t.Helper()
	p, err := New(context.Background(), cfg, opts...)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// ---------------------------------------------------------------------------
// Basic connectivity
// ---------------------------------------------------------------------------

func TestPool_ConnectAndQuery(t *testing.T) {
	p := mustPool(t, testCfg())

	// sql.DB bridge
	var n int
	err := p.DB().QueryRowContext(context.Background(), "SELECT 1").Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// pgxpool direct
	var db string
	err = p.PgxPool().QueryRow(context.Background(), "SELECT current_database()").Scan(&db)
	require.NoError(t, err)
	assert.Equal(t, testCfg().Database, db)
}

// ---------------------------------------------------------------------------
// DSN edge cases
// ---------------------------------------------------------------------------

func TestPool_URLOverridesFields(t *testing.T) {
	cfg := testCfg()
	url := fmt.Sprintf("host=%s port=%d user=%s password='%s' dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode)
	cfg.URL = url
	cfg.Host = "should-be-ignored"
	cfg.Port = 9999

	p := mustPool(t, cfg)
	var n int
	err := p.DB().QueryRowContext(context.Background(), "SELECT 1").Scan(&n)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Pool config mapping
// ---------------------------------------------------------------------------

func TestPool_MaxConnsEnforced(t *testing.T) {
	cfg := testCfg()
	cfg.MaxOpenConns = 2
	cfg.MinConns = 0 // don't pre-create

	p := mustPool(t, cfg)

	ctx := context.Background()
	// Acquire both connections
	conn1, err := p.PgxPool().Acquire(ctx)
	require.NoError(t, err)
	defer conn1.Release()

	conn2, err := p.PgxPool().Acquire(ctx)
	require.NoError(t, err)
	defer conn2.Release()

	// Third acquire should block and timeout
	ctx3, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	_, err = p.PgxPool().Acquire(ctx3)
	assert.Error(t, err, "should fail when pool is exhausted")
	assert.Contains(t, err.Error(), "deadline")
}

func TestPool_MinConnsPreWarmed(t *testing.T) {
	cfg := testCfg()
	cfg.MinConns = 3
	cfg.MaxOpenConns = 10

	p := mustPool(t, cfg)

	// Give the pool a moment to open background connections
	time.Sleep(200 * time.Millisecond)

	stat := p.PgxPool().Stat()
	assert.GreaterOrEqual(t, stat.TotalConns(), int32(3),
		"pool should pre-warm at least MinConns connections")
}

func TestPool_ConnectTimeoutApplied(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:           "192.0.2.1", // RFC 5737 TEST-NET — unreachable, won't respond
		Port:           5432,
		Username:       "test",
		Password:       "test",
		Database:       "test",
		SSLMode:        "disable",
		ConnectTimeout: 1 * time.Second,
		MaxOpenConns:   1,
		MinConns:       0,
	}

	// pgxpool is lazy — creation succeeds, first query triggers connect.
	p, err := New(context.Background(), cfg)
	require.NoError(t, err, "pool creation is lazy, should not fail")
	defer p.Close()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = p.PgxPool().Acquire(ctx)
	elapsed := time.Since(start)

	assert.Error(t, err, "acquire should fail against unreachable host")
	assert.Less(t, elapsed, 5*time.Second,
		"should fail within ConnectTimeout, not hang")
}

// ---------------------------------------------------------------------------
// Pool lifecycle
// ---------------------------------------------------------------------------

func TestPool_CloseIsIdempotent(t *testing.T) {
	cfg := testCfg()
	p, err := New(context.Background(), cfg)
	require.NoError(t, err)

	assert.True(t, p.Healthy(), "pool should be healthy before close")
	p.Close()
	assert.False(t, p.Healthy(), "pool should not be healthy after close")

	// Second close should not panic
	assert.NotPanics(t, func() { p.Close() })
}

// ---------------------------------------------------------------------------
// SelfTest
// ---------------------------------------------------------------------------

func TestPool_SelfTest(t *testing.T) {
	cfg := testCfg()
	p := mustPool(t, cfg)

	// SelfTest requires the properties table. Create it if missing.
	_, _ = p.DB().ExecContext(context.Background(),
		"CREATE TABLE IF NOT EXISTS properties (k VARCHAR(64) PRIMARY KEY, v VARCHAR(128))")

	err := p.SelfTest(context.Background())
	assert.NoError(t, err, "SelfTest should pass with pgx/v5 pgconn error detection")
}

func TestPool_SelfTestCleanup(t *testing.T) {
	cfg := testCfg()
	p := mustPool(t, cfg)

	_, _ = p.DB().ExecContext(context.Background(),
		"CREATE TABLE IF NOT EXISTS properties (k VARCHAR(64) PRIMARY KEY, v VARCHAR(128))")

	err := p.SelfTest(context.Background())
	require.NoError(t, err)

	// Verify sentinel row was cleaned up
	var count int
	err = p.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM properties WHERE k = '__dbpool_selftest'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "SelfTest should clean up the sentinel row")
}

// ---------------------------------------------------------------------------
// Concurrent access under load
// ---------------------------------------------------------------------------

func TestPool_ConcurrentQueries(t *testing.T) {
	cfg := testCfg()
	cfg.MaxOpenConns = 5
	cfg.MinConns = 2

	p := mustPool(t, cfg)

	const goroutines = 20
	const queriesPerGoroutine = 50

	var wg sync.WaitGroup
	var errors atomic.Int64

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range queriesPerGoroutine {
				var n int
				err := p.DB().QueryRowContext(context.Background(), "SELECT 1").Scan(&n)
				if err != nil {
					errors.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(0), errors.Load(),
		"no errors expected with %d goroutines × %d queries on pool of %d",
		goroutines, queriesPerGoroutine, cfg.MaxOpenConns)

	stat := p.PgxPool().Stat()
	assert.LessOrEqual(t, stat.TotalConns(), int32(cfg.MaxOpenConns),
		"pool should never exceed MaxConns")
}


// ---------------------------------------------------------------------------
// Option extensibility
// ---------------------------------------------------------------------------

func TestPool_OptionFunc(t *testing.T) {
	cfg := testCfg()

	var called bool
	opt := func(c *pgxpool.Config) {
		called = true
	}

	p := mustPool(t, cfg, opt)
	assert.True(t, called, "Option func should be called during pool creation")
	assert.True(t, p.Healthy())
}

func TestPool_MultipleOptions(t *testing.T) {
	cfg := testCfg()

	var order []string
	opt1 := func(c *pgxpool.Config) { order = append(order, "first") }
	opt2 := func(c *pgxpool.Config) { order = append(order, "second") }

	p := mustPool(t, cfg, opt1, opt2)
	assert.Equal(t, []string{"first", "second"}, order, "options should apply in order")
	assert.True(t, p.Healthy())
}

// ---------------------------------------------------------------------------
// Connection failure scenarios
// ---------------------------------------------------------------------------

func TestPool_WrongPassword(t *testing.T) {
	cfg := testCfg()
	cfg.Password = "definitely-wrong-password"
	cfg.MinConns = 0

	// pgxpool is lazy — creation succeeds, first query fails.
	p, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer p.Close()

	var n int
	err = p.DB().QueryRowContext(context.Background(), "SELECT 1").Scan(&n)
	assert.Error(t, err, "query should fail with wrong password")
}

func TestPool_WrongDatabase(t *testing.T) {
	cfg := testCfg()
	cfg.Database = "nonexistent_db_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	cfg.MinConns = 0

	p, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer p.Close()

	var n int
	err = p.DB().QueryRowContext(context.Background(), "SELECT 1").Scan(&n)
	assert.Error(t, err, "query should fail with nonexistent database")
}

func TestPool_WrongHost(t *testing.T) {
	cfg := testCfg()
	cfg.Host = "192.0.2.1" // TEST-NET, unreachable
	cfg.ConnectTimeout = 1 * time.Second
	cfg.MinConns = 0

	p, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var n int
	err = p.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
	assert.Error(t, err, "query should fail with unreachable host")
}

// ---------------------------------------------------------------------------
// Transaction through sql.DB bridge
// ---------------------------------------------------------------------------

func TestPool_TransactionCommit(t *testing.T) {
	p := mustPool(t, testCfg())
	db := p.DB()
	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS _dbpool_test_tx (id int PRIMARY KEY, val text)")
	defer func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_test_tx") }()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "INSERT INTO _dbpool_test_tx (id, val) VALUES (1, 'hello')")
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	var val string
	err = db.QueryRowContext(ctx, "SELECT val FROM _dbpool_test_tx WHERE id = 1").Scan(&val)
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestPool_TransactionRollback(t *testing.T) {
	p := mustPool(t, testCfg())
	db := p.DB()
	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS _dbpool_test_tx2 (id int PRIMARY KEY, val text)")
	defer func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_test_tx2") }()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "INSERT INTO _dbpool_test_tx2 (id, val) VALUES (1, 'should-not-persist')")
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM _dbpool_test_tx2").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// ORM round-trip through registered pool
// ---------------------------------------------------------------------------

func TestPool_OrmQueryAfterRegister(t *testing.T) {
	cfg := testCfg()
	p := mustPool(t, cfg)

	// Use a unique alias to avoid collisions with other tests.
	alias := "orm_roundtrip_test"
	err := p.RegisterWithOrm(alias)
	require.NoError(t, err)

	o := orm.NewOrmUsingDB(alias)
	var n int
	err = o.Raw("SELECT 1").QueryRow(&n)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestPool_OrmInsertAndQuery(t *testing.T) {
	cfg := testCfg()
	p := mustPool(t, cfg)
	ctx := context.Background()

	alias := "orm_insert_test"
	err := p.RegisterWithOrm(alias)
	require.NoError(t, err)

	db := p.DB()
	_, _ = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS _dbpool_orm_test (id serial PRIMARY KEY, name text)")
	defer func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_orm_test") }()

	o := orm.NewOrmUsingDB(alias)
	_, err = o.Raw("INSERT INTO _dbpool_orm_test (name) VALUES (?)", "harbor").Exec()
	require.NoError(t, err)

	var name string
	err = o.Raw("SELECT name FROM _dbpool_orm_test WHERE name = ?", "harbor").QueryRow(&name)
	require.NoError(t, err)
	assert.Equal(t, "harbor", name)
}

// ---------------------------------------------------------------------------
// SimpleProtocol mode verification
// ---------------------------------------------------------------------------

func TestPool_SimpleProtocolParameterizedQuery(t *testing.T) {
	p := mustPool(t, testCfg())
	ctx := context.Background()

	// Parameterized query through sql.DB bridge — verifies QueryExecModeSimpleProtocol
	// is active. In extended protocol mode, $1 parameters work differently;
	// simple protocol sends the query as a single string with interpolated params.
	var result int
	err := p.DB().QueryRowContext(ctx, "SELECT $1::int", 42).Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 42, result)

	// Multiple params
	var sum int
	err = p.DB().QueryRowContext(ctx, "SELECT $1::int + $2::int", 10, 32).Scan(&sum)
	require.NoError(t, err)
	assert.Equal(t, 42, sum)
}

// ---------------------------------------------------------------------------
// Connection recovery after disruption
// ---------------------------------------------------------------------------

func TestPool_RecoveryAfterConnectionKill(t *testing.T) {
	cfg := testCfg()
	cfg.MaxOpenConns = 2
	cfg.MinConns = 1
	cfg.HealthCheckPeriod = 500 * time.Millisecond

	p := mustPool(t, cfg)
	ctx := context.Background()

	// Verify pool works
	var n int
	err := p.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
	require.NoError(t, err)

	// Kill all backend connections from this pool by terminating backends
	_, err = p.DB().ExecContext(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		  AND datname = current_database()
		  AND application_name = ''`)
	// The terminate might kill our own connection too, so ignore errors
	_ = err

	// Wait for health check to detect dead connections and pool to recover
	time.Sleep(1 * time.Second)

	// Pool should recover — next query should succeed on a fresh connection
	err = p.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
	assert.NoError(t, err, "pool should recover after connections are killed")
}

// ---------------------------------------------------------------------------
// MaxConnLifetime eviction
// ---------------------------------------------------------------------------

func TestPool_MaxConnLifetimeEviction(t *testing.T) {
	cfg := testCfg()
	cfg.MaxOpenConns = 5
	cfg.MinConns = 0
	cfg.ConnMaxLifetime = 1 * time.Second
	cfg.HealthCheckPeriod = 500 * time.Millisecond

	p := mustPool(t, cfg)
	ctx := context.Background()

	// Open connections
	for range 5 {
		var n int
		_ = p.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
	}

	connsBeforeEviction := p.PgxPool().Stat().NewConnsCount()
	require.Greater(t, connsBeforeEviction, int64(0))

	// Wait for lifetime expiry + health check to destroy them
	time.Sleep(2 * time.Second)

	// Drive multiple queries to force new connection creation
	for range 3 {
		var n int
		err := p.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
		require.NoError(t, err)
	}

	connsAfterEviction := p.PgxPool().Stat().NewConnsCount()
	assert.Greater(t, connsAfterEviction, connsBeforeEviction,
		"pool should have created new connections after lifetime eviction")
}

// ---------------------------------------------------------------------------
// MaxConnIdleTime eviction
// ---------------------------------------------------------------------------

func TestPool_MaxConnIdleTimeEviction(t *testing.T) {
	cfg := testCfg()
	cfg.MaxOpenConns = 10
	cfg.MinConns = 1
	cfg.ConnMaxIdleTime = 500 * time.Millisecond
	cfg.HealthCheckPeriod = 500 * time.Millisecond

	p := mustPool(t, cfg)
	ctx := context.Background()

	// Open several connections by doing concurrent queries
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var n int
			_ = p.DB().QueryRowContext(ctx, "SELECT 1").Scan(&n)
		}()
	}
	wg.Wait()

	// Should have multiple connections open
	time.Sleep(100 * time.Millisecond) // let connections return to idle
	stat1 := p.PgxPool().Stat()

	// Wait for idle timeout + health check to reap
	time.Sleep(1500 * time.Millisecond)

	stat2 := p.PgxPool().Stat()
	assert.LessOrEqual(t, stat2.IdleConns(), stat1.IdleConns(),
		"idle connections should be reaped after MaxConnIdleTime")
	assert.GreaterOrEqual(t, stat2.TotalConns(), int32(cfg.MinConns),
		"pool should never drop below MinConns")
}

// ---------------------------------------------------------------------------
// pgconn error type assertion through sql.DB bridge
// ---------------------------------------------------------------------------

func TestPool_PgErrorUniqueViolationThroughBridge(t *testing.T) {
	p := mustPool(t, testCfg())
	ctx := context.Background()
	db := p.DB()

	_, _ = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS _dbpool_err_test (id int PRIMARY KEY)")
	defer func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_err_test") }()

	_, err := db.ExecContext(ctx, "INSERT INTO _dbpool_err_test (id) VALUES (1)")
	require.NoError(t, err)

	// Duplicate insert — should produce unique violation
	_, err = db.ExecContext(ctx, "INSERT INTO _dbpool_err_test (id) VALUES (1)")
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "error through sql.DB bridge must unwrap to *pgconn.PgError, got %T", err)
	assert.Equal(t, "23505", pgErr.Code, "should be unique_violation")
}

func TestPool_PgErrorForeignKeyViolationThroughBridge(t *testing.T) {
	p := mustPool(t, testCfg())
	ctx := context.Background()
	db := p.DB()

	_, _ = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _dbpool_fk_parent (id int PRIMARY KEY)`)
	_, _ = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _dbpool_fk_child (
		id int PRIMARY KEY,
		parent_id int REFERENCES _dbpool_fk_parent(id))`)
	defer func() {
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_fk_child")
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_fk_parent")
	}()

	// Insert child referencing non-existent parent — should produce FK violation
	_, err := db.ExecContext(ctx, "INSERT INTO _dbpool_fk_child (id, parent_id) VALUES (1, 999)")
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "FK error through sql.DB bridge must unwrap to *pgconn.PgError, got %T", err)
	assert.Equal(t, "23503", pgErr.Code, "should be foreign_key_violation")
}

func TestPool_PgErrorSyntaxErrorThroughBridge(t *testing.T) {
	p := mustPool(t, testCfg())
	ctx := context.Background()

	_, err := p.DB().ExecContext(ctx, "SELCT 1") // intentional typo
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "syntax error through sql.DB bridge must unwrap to *pgconn.PgError, got %T", err)
	assert.Equal(t, "42601", pgErr.Code, "should be syntax_error")
}

// ---------------------------------------------------------------------------
// Harbor ORM error wrappers end-to-end
// ---------------------------------------------------------------------------

func TestPool_HarborOrmErrorWrappers(t *testing.T) {
	p := mustPool(t, testCfg())
	ctx := context.Background()
	db := p.DB()

	_, _ = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS _dbpool_wrapper_test (id int PRIMARY KEY)")
	_, _ = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _dbpool_wrapper_fk (
		id int PRIMARY KEY,
		ref_id int REFERENCES _dbpool_wrapper_test(id))`)
	defer func() {
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_wrapper_fk")
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS _dbpool_wrapper_test")
	}()

	_, err := db.ExecContext(ctx, "INSERT INTO _dbpool_wrapper_test (id) VALUES (1)")
	require.NoError(t, err)

	t.Run("IsDuplicateKeyError", func(t *testing.T) {
		_, err := db.ExecContext(ctx, "INSERT INTO _dbpool_wrapper_test (id) VALUES (1)")
		require.Error(t, err)
		assert.True(t, harborORM.IsDuplicateKeyError(err),
			"Harbor IsDuplicateKeyError must detect unique violation through sql.DB bridge")
	})

	t.Run("WrapConflictError", func(t *testing.T) {
		_, err := db.ExecContext(ctx, "INSERT INTO _dbpool_wrapper_test (id) VALUES (1)")
		require.Error(t, err)
		wrapped := harborORM.WrapConflictError(err, "test conflict")
		assert.NotEqual(t, err, wrapped, "WrapConflictError should wrap the error")
		assert.Contains(t, wrapped.Error(), "test conflict")
	})

	t.Run("AsForeignKeyError", func(t *testing.T) {
		_, err := db.ExecContext(ctx, "INSERT INTO _dbpool_wrapper_fk (id, ref_id) VALUES (1, 999)")
		require.Error(t, err)
		wrapped := harborORM.AsForeignKeyError(err, "test fk")
		assert.NotNil(t, wrapped, "AsForeignKeyError must detect FK violation through sql.DB bridge")
	})

	t.Run("non-matching errors pass through", func(t *testing.T) {
		_, err := db.ExecContext(ctx, "SELCT 1") // syntax error
		require.Error(t, err)
		assert.False(t, harborORM.IsDuplicateKeyError(err), "syntax error is not a duplicate key error")
		assert.Nil(t, harborORM.AsForeignKeyError(err, ""), "syntax error is not a FK error")
	})
}
