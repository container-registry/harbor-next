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
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeSchemaDB struct {
	begin func(context.Context) (schemaTx, error)
}

func (d fakeSchemaDB) Begin(ctx context.Context) (schemaTx, error) {
	return d.begin(ctx)
}

type fakeSchemaTx struct {
	queries    []string
	args       [][]any
	execErrors map[int]error
	commitErr  error
	committed  bool
	rolledBack bool
}

func (tx *fakeSchemaTx) Exec(_ context.Context, query string, args ...any) error {
	tx.queries = append(tx.queries, query)
	tx.args = append(tx.args, args)
	return tx.execErrors[len(tx.queries)]
}

func (tx *fakeSchemaTx) Commit() error {
	if tx.commitErr != nil {
		return tx.commitErr
	}
	tx.committed = true
	return nil
}

func (tx *fakeSchemaTx) Rollback() error {
	tx.rolledBack = true
	return nil
}

func TestAuthoritativeSchemaPath(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("POSTGRES_MIGRATION_SCRIPTS_PATH", "")
		want := filepath.Join(defaultMigrationDir, authoritativeSchemaFile)
		if got := authoritativeSchemaPath(); got != want {
			t.Errorf("authoritativeSchemaPath() = %q, want %q", got, want)
		}
	})

	t.Run("configured migration directory", func(t *testing.T) {
		t.Setenv("POSTGRES_MIGRATION_SCRIPTS_PATH", "/test/migrations")
		want := filepath.Join("/test/migrations", authoritativeSchemaFile)
		if got := authoritativeSchemaPath(); got != want {
			t.Errorf("authoritativeSchemaPath() = %q, want %q", got, want)
		}
	})
}

func TestApplyAuthoritativeSchemaIsRepeatable(t *testing.T) {
	const schema = "CREATE TABLE IF NOT EXISTS example (id BIGINT PRIMARY KEY);\n"
	path := writeSchema(t, schema)

	var transactions []*fakeSchemaTx
	db := fakeSchemaDB{begin: func(context.Context) (schemaTx, error) {
		tx := &fakeSchemaTx{}
		transactions = append(transactions, tx)
		return tx, nil
	}}

	for i := 0; i < 2; i++ {
		if err := applyAuthoritativeSchema(context.Background(), db, path); err != nil {
			t.Fatalf("applyAuthoritativeSchema() run %d returned error: %v", i+1, err)
		}
	}

	if got, want := len(transactions), 2; got != want {
		t.Fatalf("transaction count = %d, want %d", got, want)
	}
	for i, tx := range transactions {
		if !tx.committed {
			t.Errorf("transaction %d was not committed", i+1)
		}
		if tx.rolledBack {
			t.Errorf("transaction %d was rolled back", i+1)
		}
		if got, want := len(tx.queries), 3; got != want {
			t.Fatalf("transaction %d query count = %d, want %d", i+1, got, want)
		}
		if got, want := tx.queries[0], "SELECT pg_advisory_xact_lock($1)"; got != want {
			t.Errorf("transaction %d lock query = %q, want %q", i+1, got, want)
		}
		if got, want := tx.args[0][0], any(authoritativeSchemaLockID); got != want {
			t.Errorf("transaction %d lock ID = %v, want %v", i+1, got, want)
		}
		if got, want := tx.queries[1], "SET LOCAL statement_timeout = 0"; got != want {
			t.Errorf("transaction %d timeout query = %q, want %q", i+1, got, want)
		}
		if got := tx.queries[2]; got != schema {
			t.Errorf("transaction %d schema query = %q, want %q", i+1, got, schema)
		}
	}
}

func TestApplyAuthoritativeSchemaErrors(t *testing.T) {
	errBegin := errors.New("begin")
	errLock := errors.New("lock")
	errTimeout := errors.New("timeout")
	errApply := errors.New("apply")
	errCommit := errors.New("commit")

	tests := []struct {
		name         string
		path         func(*testing.T) string
		beginErr     error
		execErrors   map[int]error
		commitErr    error
		wantErr      error
		wantRollback bool
	}{
		{
			name: "missing schema file",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.sql")
			},
		},
		{
			name: "empty schema file",
			path: func(t *testing.T) string {
				return writeSchema(t, " \n\t")
			},
		},
		{
			name:     "begin transaction",
			path:     func(t *testing.T) string { return writeSchema(t, "SELECT 1;") },
			beginErr: errBegin,
			wantErr:  errBegin,
		},
		{
			name:         "acquire advisory lock",
			path:         func(t *testing.T) string { return writeSchema(t, "SELECT 1;") },
			execErrors:   map[int]error{1: errLock},
			wantErr:      errLock,
			wantRollback: true,
		},
		{
			name:         "disable statement timeout",
			path:         func(t *testing.T) string { return writeSchema(t, "SELECT 1;") },
			execErrors:   map[int]error{2: errTimeout},
			wantErr:      errTimeout,
			wantRollback: true,
		},
		{
			name:         "execute schema",
			path:         func(t *testing.T) string { return writeSchema(t, "SELECT 1;") },
			execErrors:   map[int]error{3: errApply},
			wantErr:      errApply,
			wantRollback: true,
		},
		{
			name:         "commit transaction",
			path:         func(t *testing.T) string { return writeSchema(t, "SELECT 1;") },
			commitErr:    errCommit,
			wantErr:      errCommit,
			wantRollback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeSchemaTx{execErrors: tt.execErrors, commitErr: tt.commitErr}
			beginCalled := false
			db := fakeSchemaDB{begin: func(context.Context) (schemaTx, error) {
				beginCalled = true
				if tt.beginErr != nil {
					return nil, tt.beginErr
				}
				return tx, nil
			}}

			err := applyAuthoritativeSchema(context.Background(), db, tt.path(t))
			if err == nil {
				t.Fatal("applyAuthoritativeSchema() returned nil error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("applyAuthoritativeSchema() error = %v, want error wrapping %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && beginCalled {
				t.Error("database transaction began before schema validation completed")
			}
			if tx.rolledBack != tt.wantRollback {
				t.Errorf("transaction rolledBack = %t, want %t", tx.rolledBack, tt.wantRollback)
			}
		})
	}
}

func writeSchema(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), authoritativeSchemaFile)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write schema fixture: %v", err)
	}
	return path
}
