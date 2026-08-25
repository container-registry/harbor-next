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
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeVersionedSchemaDB struct {
	begin func(context.Context) (versionedSchemaTx, error)
}

func (d fakeVersionedSchemaDB) Begin(ctx context.Context) (versionedSchemaTx, error) {
	return d.begin(ctx)
}

type fakeVersionedSchemaTx struct {
	current    string
	queries    []string
	args       [][]any
	inserted   []string
	execErrors map[int]error
	commitErr  error
	committed  bool
	rolledBack bool
}

func (tx *fakeVersionedSchemaTx) Exec(_ context.Context, query string, args ...any) error {
	tx.queries = append(tx.queries, query)
	tx.args = append(tx.args, args)
	if query == "INSERT INTO harbor_next_schema_migrations(version) VALUES ($1)" {
		tx.inserted = append(tx.inserted, args[0].(string))
	}
	return tx.execErrors[len(tx.queries)]
}

func (tx *fakeVersionedSchemaTx) QueryRowScan(_ context.Context, dest *string, _ string, _ ...any) error {
	*dest = tx.current
	return nil
}

func (tx *fakeVersionedSchemaTx) Commit() error {
	if tx.commitErr != nil {
		return tx.commitErr
	}
	tx.committed = true
	return nil
}

func (tx *fakeVersionedSchemaTx) Rollback() error {
	tx.rolledBack = true
	return nil
}

func writeVersionedSchema(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write schema fixture: %v", err)
	}
}

func TestApplyVersionedSchemaAppliesOnlyNewer(t *testing.T) {
	dir := t.TempDir()
	writeVersionedSchema(t, dir, "harbor_next_2.15.0.sql", "SELECT 'old';")
	writeVersionedSchema(t, dir, "harbor_next_2.16.0.sql", "SELECT 'new';")
	writeVersionedSchema(t, dir, "0190_2.16.0_schema.up.sql", "SELECT 'ignored, not our prefix';")

	tx := &fakeVersionedSchemaTx{current: "2.15.0"}
	db := fakeVersionedSchemaDB{begin: func(context.Context) (versionedSchemaTx, error) { return tx, nil }}

	if err := applyVersionedSchema(context.Background(), db, dir); err != nil {
		t.Fatalf("applyVersionedSchema() returned error: %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("committed=%t rolledBack=%t", tx.committed, tx.rolledBack)
	}
	if got, want := tx.inserted, []string{"2.16.0"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("inserted versions = %v, want %v", got, want)
	}
}

func TestApplyVersionedSchemaNoOpWhenCurrent(t *testing.T) {
	dir := t.TempDir()
	writeVersionedSchema(t, dir, "harbor_next_2.16.0.sql", "SELECT 'new';")

	tx := &fakeVersionedSchemaTx{current: "2.16.0"}
	db := fakeVersionedSchemaDB{begin: func(context.Context) (versionedSchemaTx, error) { return tx, nil }}

	if err := applyVersionedSchema(context.Background(), db, dir); err != nil {
		t.Fatalf("applyVersionedSchema() returned error: %v", err)
	}
	// only the lock + table-create + version-read exec calls, no schema apply/insert
	if len(tx.inserted) != 0 {
		t.Errorf("inserted versions = %v, want none", tx.inserted)
	}
}

func TestApplyVersionedSchemaAppliesInAscendingOrder(t *testing.T) {
	dir := t.TempDir()
	writeVersionedSchema(t, dir, "harbor_next_2.16.1.sql", "SELECT 'second';")
	writeVersionedSchema(t, dir, "harbor_next_2.16.0.sql", "SELECT 'first';")

	tx := &fakeVersionedSchemaTx{current: "0.0.0"}
	db := fakeVersionedSchemaDB{begin: func(context.Context) (versionedSchemaTx, error) { return tx, nil }}

	if err := applyVersionedSchema(context.Background(), db, dir); err != nil {
		t.Fatalf("applyVersionedSchema() returned error: %v", err)
	}
	if got, want := tx.inserted, []string{"2.16.0", "2.16.1"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("applied order = %v, want %v", got, want)
	}
}

func TestApplyVersionedSchemaErrors(t *testing.T) {
	errBegin := errors.New("begin")
	errLock := errors.New("lock")
	errApply := errors.New("apply")
	errCommit := errors.New("commit")

	tests := []struct {
		name         string
		dir          func(*testing.T) string
		beginErr     error
		execErrors   map[int]error
		commitErr    error
		wantErr      error
		wantRollback bool
	}{
		{
			name: "missing directory",
			dir:  func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
		},
		{
			name: "empty schema file",
			dir: func(t *testing.T) string {
				dir := t.TempDir()
				writeVersionedSchema(t, dir, "harbor_next_2.16.0.sql", " \n\t")
				return dir
			},
			wantRollback: true,
		},
		{
			name:     "begin transaction",
			dir:      func(t *testing.T) string { return t.TempDir() },
			beginErr: errBegin,
			wantErr:  errBegin,
		},
		{
			name:         "acquire advisory lock",
			dir:          func(t *testing.T) string { return t.TempDir() },
			execErrors:   map[int]error{1: errLock},
			wantErr:      errLock,
			wantRollback: true,
		},
		{
			name: "execute schema",
			dir: func(t *testing.T) string {
				dir := t.TempDir()
				writeVersionedSchema(t, dir, "harbor_next_2.16.0.sql", "SELECT 1;")
				return dir
			},
			execErrors:   map[int]error{3: errApply},
			wantErr:      errApply,
			wantRollback: true,
		},
		{
			name:         "commit transaction",
			dir:          func(t *testing.T) string { return t.TempDir() },
			commitErr:    errCommit,
			wantErr:      errCommit,
			wantRollback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeVersionedSchemaTx{current: "0.0.0", execErrors: tt.execErrors, commitErr: tt.commitErr}
			db := fakeVersionedSchemaDB{begin: func(context.Context) (versionedSchemaTx, error) {
				if tt.beginErr != nil {
					return nil, tt.beginErr
				}
				return tx, nil
			}}

			err := applyVersionedSchema(context.Background(), db, tt.dir(t))
			if err == nil {
				t.Fatal("applyVersionedSchema() returned nil error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("applyVersionedSchema() error = %v, want error wrapping %v", err, tt.wantErr)
			}
			if tx.rolledBack != tt.wantRollback {
				t.Errorf("transaction rolledBack = %t, want %t", tx.rolledBack, tt.wantRollback)
			}
		})
	}
}
