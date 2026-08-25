// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/goharbor/harbor/src/lib/log"
)

const (
	defaultMigrationDir = "migrations/postgresql"

	// versionedSchemaLockID is the ASCII encoding of "HNEXTVER".
	versionedSchemaLockID int64 = 0x484e45585645524b
)

var versionedSchemaFileRe = regexp.MustCompile(`^harbor_next_(\d+\.\d+\.\d+)\.sql$`)

type versionedSchemaDB interface {
	Begin(context.Context) (versionedSchemaTx, error)
}

type versionedSchemaTx interface {
	Exec(context.Context, string, ...any) error
	QueryRowScan(ctx context.Context, dest *string, query string, args ...any) error
	Commit() error
	Rollback() error
}

type sqlVersionedSchemaDB struct {
	db *sql.DB
}

func (d sqlVersionedSchemaDB) Begin(ctx context.Context) (versionedSchemaTx, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlVersionedSchemaTx{tx: tx}, nil
}

type sqlVersionedSchemaTx struct {
	tx *sql.Tx
}

func (t sqlVersionedSchemaTx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := t.tx.ExecContext(ctx, query, args...)
	return err
}

func (t sqlVersionedSchemaTx) QueryRowScan(ctx context.Context, dest *string, query string, args ...any) error {
	return t.tx.QueryRowContext(ctx, query, args...).Scan(dest)
}

func (t sqlVersionedSchemaTx) Commit() error {
	return t.tx.Commit()
}

func (t sqlVersionedSchemaTx) Rollback() error {
	return t.tx.Rollback()
}

func versionedSchemaDir() string {
	dir := os.Getenv("POSTGRES_MIGRATION_SCRIPTS_PATH")
	if dir == "" {
		dir = defaultMigrationDir
	}
	return dir
}

type versionedSchemaFile struct {
	version *semver.Version
	path    string
}

// listVersionedSchemaFiles returns harbor_next_<version>.sql files in dir, sorted ascending by version
func listVersionedSchemaFiles(dir string) ([]versionedSchemaFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list versioned Harbor Next schema dir %q: %w", dir, err)
	}
	var files []versionedSchemaFile
	for _, e := range entries {
		m := versionedSchemaFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, err := semver.NewVersion(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse version from %q: %w", e.Name(), err)
		}
		files = append(files, versionedSchemaFile{version: v, path: filepath.Join(dir, e.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version.LessThan(files[j].version) })
	return files, nil
}

// applyVersionedSchema applies every harbor_next_<version>.sql file newer than the
// last recorded version, tracked in harbor_next_schema_migrations, in ascending order
func applyVersionedSchema(ctx context.Context, db versionedSchemaDB, dir string) error {
	files, err := listVersionedSchemaFiles(dir)
	if err != nil {
		return err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin versioned Harbor Next schema transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", versionedSchemaLockID); err != nil {
		return fmt.Errorf("lock versioned Harbor Next schema: %w", err)
	}
	if err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS harbor_next_schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create harbor_next_schema_migrations: %w", err)
	}

	var current string
	if err := tx.QueryRowScan(ctx, &current, "SELECT COALESCE(MAX(version), '0.0.0') FROM harbor_next_schema_migrations"); err != nil {
		return fmt.Errorf("read current versioned Harbor Next schema version: %w", err)
	}
	currentVersion, err := semver.NewVersion(current)
	if err != nil {
		return fmt.Errorf("parse current versioned Harbor Next schema version %q: %w", current, err)
	}

	applied := 0
	for _, f := range files {
		if !f.version.GreaterThan(currentVersion) {
			continue
		}
		schema, err := os.ReadFile(f.path)
		if err != nil {
			return fmt.Errorf("read versioned Harbor Next schema %q: %w", f.path, err)
		}
		if strings.TrimSpace(string(schema)) == "" {
			return fmt.Errorf("versioned Harbor Next schema %q is empty", f.path)
		}
		log.Infof("Applying versioned Harbor Next schema %s ...", f.path)
		if err := tx.Exec(ctx, string(schema)); err != nil {
			return fmt.Errorf("apply versioned Harbor Next schema %q: %w", f.path, err)
		}
		if err := tx.Exec(ctx, "INSERT INTO harbor_next_schema_migrations(version) VALUES ($1)", f.version.Original()); err != nil {
			return fmt.Errorf("record versioned Harbor Next schema %q: %w", f.path, err)
		}
		applied++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit versioned Harbor Next schema: %w", err)
	}
	committed = true
	log.Infof("Versioned Harbor Next schema reconciliation applied %d file(s)", applied)
	return nil
}
