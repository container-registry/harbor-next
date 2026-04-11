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

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers pgx5:// scheme for migrator
	_ "github.com/golang-migrate/migrate/v4/source/file"     // import local file driver for migrator

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/lib/dbpool"
	"github.com/goharbor/harbor/src/lib/log"
)

const defaultMigrationPath = "migrations/postgresql/"

type pgsql struct {
	cfg  *models.PostGreSQL
	pool *dbpool.Pool
}

// Name returns the name of PostgreSQL
func (p *pgsql) Name() string {
	return "PostgreSQL"
}

// String ...
func (p *pgsql) String() string {
	return fmt.Sprintf("type-%s host-%s port-%d database-%s sslmode-%q",
		p.Name(), p.cfg.Host, p.cfg.Port, p.cfg.Database, p.cfg.SSLMode)
}

// NewPGSQL returns an instance of postgres
func NewPGSQL(cfg *models.PostGreSQL) Database {
	if len(cfg.SSLMode) == 0 {
		cfg.SSLMode = "disable"
	}
	return &pgsql{cfg: cfg}
}

// Register registers pgSQL to orm with the info wrapped by the instance.
func (p *pgsql) Register(alias ...string) error {
	if err := utils.TestTCPConn(net.JoinHostPort(p.cfg.Host, strconv.Itoa(p.cfg.Port)), 60, 2); err != nil {
		return err
	}

	pool, err := dbpool.New(context.Background(), p.cfg)
	if err != nil {
		return fmt.Errorf("dbpool: %w", err)
	}

	if err := pool.RegisterWithOrm(alias...); err != nil {
		pool.Close()
		return err
	}

	p.pool = pool
	setActivePool(pool)

	return nil
}

// UpgradeSchema calls migrate tool to upgrade schema to the latest based on the SQL scripts.
func (p *pgsql) UpgradeSchema() error {
	m, err := NewMigrator(p.cfg)
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
	dbURL := url.URL{
		Scheme:   "pgx5",
		User:     url.UserPassword(database.Username, database.Password),
		Host:     net.JoinHostPort(database.Host, strconv.Itoa(database.Port)),
		Path:     database.Database,
		RawQuery: fmt.Sprintf("sslmode=%s", database.SSLMode),
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
