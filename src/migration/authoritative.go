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

package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goharbor/harbor/src/lib/log"
)

const (
	defaultMigrationDir     = "migrations/postgresql"
	authoritativeSchemaFile = "harbor_next.sql"

	// authoritativeSchemaLockID is the ASCII encoding of "HNEXTSCH". It
	// serializes reconciliation when multiple core replicas start together.
	authoritativeSchemaLockID int64 = 0x484e455854534348
)

type schemaDB interface {
	Begin(context.Context) (schemaTx, error)
}

type schemaTx interface {
	Exec(context.Context, string, ...any) error
	Commit() error
	Rollback() error
}

type sqlSchemaDB struct {
	db *sql.DB
}

func (d sqlSchemaDB) Begin(ctx context.Context) (schemaTx, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlSchemaTx{tx: tx}, nil
}

type sqlSchemaTx struct {
	tx *sql.Tx
}

func (t sqlSchemaTx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := t.tx.ExecContext(ctx, query, args...)
	return err
}

func (t sqlSchemaTx) Commit() error {
	return t.tx.Commit()
}

func (t sqlSchemaTx) Rollback() error {
	return t.tx.Rollback()
}

func authoritativeSchemaPath() string {
	dir := os.Getenv("POSTGRES_MIGRATION_SCRIPTS_PATH")
	if dir == "" {
		dir = defaultMigrationDir
	}
	return filepath.Join(dir, authoritativeSchemaFile)
}

func applyAuthoritativeSchema(ctx context.Context, db schemaDB, path string) error {
	schema, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read authoritative Harbor Next schema %q: %w", path, err)
	}
	if strings.TrimSpace(string(schema)) == "" {
		return fmt.Errorf("authoritative Harbor Next schema %q is empty", path)
	}

	log.Infof("Applying authoritative Harbor Next schema from %s ...", path)
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin authoritative Harbor Next schema transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", authoritativeSchemaLockID); err != nil {
		return fmt.Errorf("lock authoritative Harbor Next schema: %w", err)
	}
	if err := tx.Exec(ctx, string(schema)); err != nil {
		return fmt.Errorf("apply authoritative Harbor Next schema: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit authoritative Harbor Next schema: %w", err)
	}
	committed = true
	log.Info("Authoritative Harbor Next schema applied successfully")
	return nil
}
