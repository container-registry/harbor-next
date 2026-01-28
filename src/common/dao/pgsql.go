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

package dao

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/beego/beego/v2/client/orm"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // import pgx v5 driver for migrator
	_ "github.com/golang-migrate/migrate/v4/source/file"     // import local file driver for migrator
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/lib/log"
)

const (
	defaultMigrationPath = "migrations/postgresql/"
	// iamAuthMaxConnLifetime is the maximum connection lifetime when using IAM auth.
	// IAM tokens expire after 15 minutes, so we set connections to recycle at 14 minutes
	// to ensure tokens are refreshed before expiration.
	iamAuthMaxConnLifetime = 14 * time.Minute
)

type pgsql struct {
	host            string
	port            string
	usr             string
	pwd             string
	database        string
	sslmode         string
	maxIdleConns    int
	maxOpenConns    int
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration
	useIAMAuth      bool
	awsRegion       string
	pool            *pgxpool.Pool
}

// Name returns the name of PostgreSQL
func (p *pgsql) Name() string {
	return "PostgreSQL"
}

// String ...
func (p *pgsql) String() string {
	return fmt.Sprintf("type-%s host-%s port-%s database-%s sslmode-%q",
		p.Name(), p.host, p.port, p.database, p.sslmode)
}

// NewPGSQL returns an instance of postgres
func NewPGSQL(host string, port string, usr string, pwd string, database string, sslmode string, maxIdleConns int, maxOpenConns int, connMaxLifetime time.Duration, connMaxIdleTime time.Duration, useIAMAuth bool, awsRegion string) Database {
	if len(sslmode) == 0 {
		sslmode = "disable"
	}

	// IAM authentication requires SSL
	if useIAMAuth && sslmode == "disable" {
		sslmode = "require"
		log.Info("Forcing sslmode=require for AWS RDS IAM authentication")
	}

	// Cap connection lifetime for IAM auth to ensure token refresh
	if useIAMAuth && (connMaxLifetime == 0 || connMaxLifetime > iamAuthMaxConnLifetime) {
		log.Infof("Capping conn_max_lifetime to %v for AWS RDS IAM authentication", iamAuthMaxConnLifetime)
		connMaxLifetime = iamAuthMaxConnLifetime
	}

	return &pgsql{
		host:            host,
		port:            port,
		usr:             usr,
		pwd:             pwd,
		database:        database,
		sslmode:         sslmode,
		maxIdleConns:    maxIdleConns,
		maxOpenConns:    maxOpenConns,
		connMaxLifetime: connMaxLifetime,
		connMaxIdleTime: connMaxIdleTime,
		useIAMAuth:      useIAMAuth,
		awsRegion:       awsRegion,
	}
}

// Register registers pgSQL to orm with the info wrapped by the instance.
func (p *pgsql) Register(alias ...string) error {
	if err := utils.TestTCPConn(net.JoinHostPort(p.host, p.port), 60, 2); err != nil {
		return err
	}

	// Build pgxpool connection string
	// When using IAM auth, we don't include the password - it will be set in BeforeConnect
	var connString string
	if p.useIAMAuth {
		connString = fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s timezone=UTC",
			p.host, p.port, p.usr, p.database, p.sslmode)
	} else {
		connString = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s timezone=UTC",
			p.host, p.port, p.usr, p.pwd, p.database, p.sslmode)
	}

	// Create pgxpool with configuration
	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("failed to parse pgxpool config: %w", err)
	}

	// Map configuration - only override pgxpool defaults if explicitly configured
	// pgxpool defaults: MaxConns = max(4, runtime.NumCPU()), MinConns = 0
	// database/sql used 0 to mean "unlimited", so we preserve pgxpool defaults for 0
	if p.maxOpenConns > 0 {
		poolConfig.MaxConns = int32(p.maxOpenConns)
	}
	if p.maxIdleConns > 0 {
		poolConfig.MinConns = int32(p.maxIdleConns)
	}
	if p.connMaxLifetime > 0 {
		poolConfig.MaxConnLifetime = p.connMaxLifetime
	}
	if p.connMaxIdleTime > 0 {
		poolConfig.MaxConnIdleTime = p.connMaxIdleTime
	}

	// Configure AWS IAM authentication if enabled
	if p.useIAMAuth {
		if p.awsRegion == "" {
			return fmt.Errorf("aws_region is required when use_iam_auth is enabled")
		}

		// Load AWS configuration
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.awsRegion))
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}

		// Create the endpoint string for RDS IAM auth
		dbEndpoint := net.JoinHostPort(p.host, p.port)

		// Set BeforeConnect hook to generate IAM token for each new connection
		poolConfig.BeforeConnect = func(ctx context.Context, cfg *pgx.ConnConfig) error {
			token, err := auth.BuildAuthToken(ctx, dbEndpoint, p.awsRegion, p.usr, awsCfg.Credentials)
			if err != nil {
				return fmt.Errorf("failed to generate RDS IAM auth token: %w", err)
			}
			cfg.Password = token
			return nil
		}

		log.Infof("AWS RDS IAM authentication enabled for region %s", p.awsRegion)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create pgxpool: %w", err)
	}
	p.pool = pool

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Bridge pgxpool to database/sql for Beego ORM compatibility
	sqlDB := stdlib.OpenDBFromPool(pool)

	// Register driver with Beego ORM
	if err := orm.RegisterDriver("pgx", orm.DRPostgres); err != nil {
		return err
	}

	an := "default"
	if len(alias) != 0 {
		an = alias[0]
	}

	if err := orm.RegisterDataBase(an, "pgx", connString, orm.MaxIdleConnections(p.maxIdleConns),
		orm.MaxOpenConnections(p.maxOpenConns), orm.ConnMaxLifetime(p.connMaxLifetime)); err != nil {
		// If ORM registration fails, we still have the pool via sqlDB
		_ = sqlDB
		return err
	}

	log.Infof("pgxpool initialized: MaxConns=%d, MinConns=%d, MaxLifetime=%v, MaxIdleTime=%v, IAMAuth=%v",
		poolConfig.MaxConns, poolConfig.MinConns, poolConfig.MaxConnLifetime, poolConfig.MaxConnIdleTime, p.useIAMAuth)

	return nil
}

// UpgradeSchema calls migrate tool to upgrade schema to the latest based on the SQL scripts.
func (p *pgsql) UpgradeSchema() error {
	port, err := strconv.Atoi(p.port)
	if err != nil {
		return err
	}
	m, err := NewMigrator(&models.PostGreSQL{
		Host:       p.host,
		Port:       port,
		Username:   p.usr,
		Password:   p.pwd,
		Database:   p.database,
		SSLMode:    p.sslmode,
		UseIAMAuth: p.useIAMAuth,
		AWSRegion:  p.awsRegion,
	})
	if err != nil {
		return err
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil || dbErr != nil {
			log.Warningf("Failed to close migrator, source error: %v, db error: %v", srcErr, dbErr)
		}
	}()
	log.Infof("Upgrading schema for pgsql ...")
	err = m.Up()
	if err == migrate.ErrNoChange {
		log.Infof("No change in schema, skip.")
	} else if err != nil { // migrate.ErrLockTimeout will be thrown when another process is doing migration and timeout.
		log.Errorf("Failed to upgrade schema, error: %q", err)
		return err
	}
	return nil
}

// NewMigrator creates a migrator base on the information
func NewMigrator(database *models.PostGreSQL) (*migrate.Migrate, error) {
	password := database.Password
	sslMode := database.SSLMode

	// Generate IAM token if IAM authentication is enabled
	if database.UseIAMAuth {
		if database.AWSRegion == "" {
			return nil, fmt.Errorf("aws_region is required when use_iam_auth is enabled")
		}

		// IAM authentication requires SSL
		if sslMode == "" || sslMode == "disable" {
			sslMode = "require"
		}

		ctx := context.Background()
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(database.AWSRegion))
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config for migration: %w", err)
		}

		dbEndpoint := net.JoinHostPort(database.Host, strconv.Itoa(database.Port))
		token, err := auth.BuildAuthToken(ctx, dbEndpoint, database.AWSRegion, database.Username, awsCfg.Credentials)
		if err != nil {
			return nil, fmt.Errorf("failed to generate RDS IAM auth token for migration: %w", err)
		}
		password = token
		log.Info("Using AWS RDS IAM authentication for database migration")
	}

	dbURL := url.URL{
		Scheme:   "pgx5",
		User:     url.UserPassword(database.Username, password),
		Host:     net.JoinHostPort(database.Host, strconv.Itoa(database.Port)),
		Path:     database.Database,
		RawQuery: fmt.Sprintf("sslmode=%s", sslMode),
	}

	// For UT
	path := os.Getenv("POSTGRES_MIGRATION_SCRIPTS_PATH")
	if len(path) == 0 {
		path = defaultMigrationPath
	}
	srcURL := fmt.Sprintf("file://%s", path)
	m, err := migrate.New(srcURL, dbURL.String())
	if err != nil {
		return nil, err
	}
	m.Log = newMigrateLogger()
	return m, nil
}
