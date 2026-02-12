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

// Package fork provides database schema migration management for fork-specific
// changes that live alongside upstream Harbor migrations without modifying them.
//
// It uses golang-migrate with a separate version table (fork_schema_migrations)
// so fork and upstream migration state are tracked independently.
package fork

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/log"
)

const (
	// ForkMigrationsTable is the PostgreSQL table used to track fork migration versions.
	// This is separate from the upstream "schema_migrations" table.
	ForkMigrationsTable = "fork_schema_migrations"

	// DefaultForkMigrationPath is the default path to fork migration SQL files.
	DefaultForkMigrationPath = "migrations/postgresql/fork/"
)

// Manager handles fork-specific database schema migrations.
type Manager struct {
	migrator *migrate.Migrate
}

// NewManager creates a new fork migration manager.
func NewManager(database *models.PostGreSQL) (*Manager, error) {
	m, err := newMigrator(database)
	if err != nil {
		return nil, fmt.Errorf("failed to create fork migrator: %w", err)
	}
	return &Manager{migrator: m}, nil
}

// Up applies all pending fork migrations.
func (m *Manager) Up() error {
	log.Info("[fork-migrator] Applying fork migrations...")
	err := m.migrator.Up()
	if err == migrate.ErrNoChange {
		log.Info("[fork-migrator] No pending fork migrations.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("[fork-migrator] failed to apply migrations: %w", err)
	}
	log.Info("[fork-migrator] Fork migrations applied successfully.")
	return nil
}

// Down rolls back ALL fork migrations (full rollback to clean upstream state).
func (m *Manager) Down() error {
	log.Info("[fork-migrator] Rolling back all fork migrations...")
	err := m.migrator.Down()
	if err == migrate.ErrNoChange {
		log.Info("[fork-migrator] No fork migrations to roll back.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("[fork-migrator] failed to roll back migrations: %w", err)
	}
	log.Info("[fork-migrator] All fork migrations rolled back successfully.")
	return nil
}

// Steps applies or rolls back N migrations. Positive N = forward, negative N = backward.
func (m *Manager) Steps(n int) error {
	log.Infof("[fork-migrator] Applying %d migration step(s)...", n)
	err := m.migrator.Steps(n)
	if err == migrate.ErrNoChange {
		log.Info("[fork-migrator] No change.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("[fork-migrator] step migration failed: %w", err)
	}
	log.Infof("[fork-migrator] %d step(s) applied successfully.", n)
	return nil
}

// MigrateTo migrates to a specific version. Use this for targeted rollback.
func (m *Manager) MigrateTo(version uint) error {
	log.Infof("[fork-migrator] Migrating to version %d...", version)
	err := m.migrator.Migrate(version)
	if err == migrate.ErrNoChange {
		log.Infof("[fork-migrator] Already at version %d.", version)
		return nil
	}
	if err != nil {
		return fmt.Errorf("[fork-migrator] migration to version %d failed: %w", version, err)
	}
	log.Infof("[fork-migrator] Migrated to version %d successfully.", version)
	return nil
}

// Version returns the current fork migration version and dirty state.
func (m *Manager) Version() (version uint, dirty bool, err error) {
	return m.migrator.Version()
}

// Force sets the migration version without running migrations.
// Use this to recover from a dirty state after a failed migration.
func (m *Manager) Force(version int) error {
	log.Infof("[fork-migrator] Forcing version to %d...", version)
	return m.migrator.Force(version)
}

// Close releases the migrator resources.
func (m *Manager) Close() (source error, database error) {
	return m.migrator.Close()
}

func newMigrator(database *models.PostGreSQL) (*migrate.Migrate, error) {
	dbURL := url.URL{
		Scheme: "pgx",
		User:   url.UserPassword(database.Username, database.Password),
		Host:   net.JoinHostPort(database.Host, strconv.Itoa(database.Port)),
		Path:   database.Database,
		RawQuery: fmt.Sprintf("sslmode=%s&x-migrations-table=%s",
			database.SSLMode, ForkMigrationsTable),
	}

	path := os.Getenv("FORK_MIGRATION_SCRIPTS_PATH")
	if path == "" {
		path = DefaultForkMigrationPath
	}
	srcURL := fmt.Sprintf("file://%s", path)

	m, err := migrate.New(srcURL, dbURL.String())
	if err != nil {
		return nil, err
	}
	m.Log = newForkMigrateLogger()
	return m, nil
}

// forkMigrateLogger implements github.com/golang-migrate/migrate/v4.Logger
type forkMigrateLogger struct {
	logger *log.Logger
}

func newForkMigrateLogger() *forkMigrateLogger {
	return &forkMigrateLogger{
		logger: log.DefaultLogger().WithDepth(5),
	}
}

func (l *forkMigrateLogger) Verbose() bool {
	return l.logger.GetLevel() <= log.DebugLevel
}

func (l *forkMigrateLogger) Printf(format string, v ...any) {
	l.logger.Infof("[fork-migrator] "+format, v...)
}
