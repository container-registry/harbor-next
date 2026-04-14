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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beego/beego/v2/client/orm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/log"
)

// Defaults are chosen for backward compatibility with the pre-pgxpool Harbor
// configuration. PostgreSQL server's own max_connections defaults to 100;
// the metadata default for MaxOpenConns is 100 to match.
const (
	DefaultMinConns        = 2
	DefaultMaxConnIdleTime = 10 * time.Minute
	DefaultHealthCheckPeriod = 1 * time.Minute
	DefaultConnectTimeout    = 10 * time.Second
	healthyTimeout           = 5 * time.Second
)

// Pool wraps a pgxpool.Pool and the bridged *sql.DB for Beego ORM compatibility.
type Pool struct {
	pool *pgxpool.Pool
	db   *sql.DB
}

// Option configures a pgxpool.Config before pool creation.
// Use this extension point for tracers, metrics, or other pgxpool customizations.
type Option func(*pgxpool.Config)

func New(ctx context.Context, cfg *models.PostGreSQL, opts ...Option) (*Pool, error) {
	connStr := BuildDSN(cfg)

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("dbpool: parse config: %w", err)
	}

	applyPoolConfig(poolCfg, cfg)

	for _, opt := range opts {
		opt(poolCfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("dbpool: create pool: %w", err)
	}

	// Do NOT set any pool params on db — pgxpool owns the connection lifecycle,
	// not database/sql. stdlib.OpenDBFromPool intentionally sets MaxIdleConns=0
	// on the sql.DB wrapper so that sql.DB never tries to manage connections itself.
	db := stdlib.OpenDBFromPool(pool)

	return &Pool{pool: pool, db: db}, nil
}

// BuildDSN returns cfg.URL if set (for cloud-managed databases like RDS IAM auth
// where the DSN contains tokens or parameters that don't map to individual fields),
// otherwise constructs a libpq key-value connection string from config fields.
func BuildDSN(cfg *models.PostGreSQL) string {
	if cfg.URL != "" {
		return cfg.URL
	}
	// Escape password for libpq key-value format: single-quote it and
	// escape any embedded single-quotes and backslashes.
	escaped := strings.ReplaceAll(cfg.Password, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return fmt.Sprintf("host=%s port=%d user=%s password='%s' dbname=%s sslmode=%s timezone=UTC",
		cfg.Host, cfg.Port, cfg.Username, escaped, cfg.Database, cfg.SSLMode)
}

func applyPoolConfig(poolCfg *pgxpool.Config, cfg *models.PostGreSQL) {
	// 0 means "not set" — leave pgxpool's default: max(4, runtime.NumCPU()).
	if cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	}

	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	} else {
		poolCfg.MinConns = DefaultMinConns
	}

	if cfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}

	if cfg.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.ConnMaxIdleTime
	} else {
		poolCfg.MaxConnIdleTime = DefaultMaxConnIdleTime
	}

	if cfg.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	} else {
		poolCfg.HealthCheckPeriod = DefaultHealthCheckPeriod
	}

	if cfg.ConnectTimeout > 0 {
		poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	} else {
		poolCfg.ConnConfig.ConnectTimeout = DefaultConnectTimeout
	}

	// Beego ORM uses string interpolation, not prepared statements.
	// Simple protocol avoids statement cache issues.
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
}

// RegisterWithOrm registers the bridged *sql.DB with Beego ORM.
// Do NOT use orm.RegisterDataBase — it opens its own sql.DB and fights pgxpool.
func (p *Pool) RegisterWithOrm(alias ...string) error {
	if err := orm.RegisterDriver("pgx", orm.DRPostgres); err != nil {
		if !strings.Contains(err.Error(), "already registered") {
			return fmt.Errorf("dbpool: RegisterDriver(%q): %w", "pgx", err)
		}
	}

	aliasName := "default"
	if len(alias) > 0 {
		aliasName = alias[0]
	}

	if err := orm.AddAliasWthDB(aliasName, "pgx", p.db); err != nil {
		return fmt.Errorf("dbpool: AddAliasWthDB(%q): %w", aliasName, err)
	}

	got, err := orm.GetDB(aliasName)
	if err != nil {
		return fmt.Errorf("dbpool: verify GetDB(%q): %w", aliasName, err)
	}
	if got != p.db {
		return fmt.Errorf("dbpool: GetDB(%q) returned unexpected *sql.DB", aliasName)
	}

	return nil
}

// SelfTest verifies that pgconn error codes are correctly detected through the
// sql.DB bridge. This catches the case where someone imports pgx/v4's pgconn
// instead of v5, which would silently break error classification (IsDuplicateKeyError
// and friends in lib/orm/error.go would never match).
func (p *Pool) SelfTest(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx,
		"INSERT INTO properties (k, v) VALUES ('__dbpool_selftest', '') ON CONFLICT (k) DO NOTHING")
	if err != nil {
		return fmt.Errorf("dbpool: self-test setup: %w", err)
	}
	defer func() {
		_, _ = p.db.ExecContext(ctx, "DELETE FROM properties WHERE k = '__dbpool_selftest'")
	}()

	_, err = p.db.ExecContext(ctx,
		"INSERT INTO properties (k, v) VALUES ('__dbpool_selftest', 'x')")
	if err == nil {
		return fmt.Errorf("dbpool: self-test expected unique violation, got nil error")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("dbpool: pgconn error detection broken: expected *pgconn.PgError, got %T / %v", err, err)
	}
	if pgErr.Code != "23505" {
		return fmt.Errorf("dbpool: pgconn error detection broken: expected code 23505, got %s", pgErr.Code)
	}

	return nil
}

func (p *Pool) DB() *sql.DB       { return p.db }
func (p *Pool) PgxPool() *pgxpool.Pool { return p.pool }

func (p *Pool) Close() {
	// Close sql.DB first — it borrows connections from the pool via Acquire(),
	// so release its references before draining the underlying pool.
	if err := p.db.Close(); err != nil {
		log.Warningf("dbpool: close sql.DB: %v", err)
	}
	p.pool.Close()
}

func (p *Pool) Healthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), healthyTimeout)
	defer cancel()
	return p.pool.Ping(ctx) == nil
}
