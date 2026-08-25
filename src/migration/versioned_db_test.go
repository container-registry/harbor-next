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

//go:build db

package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/dbpool"
)

func TestVersionedSchemaAgainstPostgreSQL(t *testing.T) {
	ctx := context.Background()
	cfg := versionedTestDatabaseConfig()
	adminPool, err := dbpool.New(ctx, cfg)
	if err != nil {
		t.Fatalf("create admin database pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("harbor_next_versioned_schema_test_%d", time.Now().UnixNano())
	if _, err := adminPool.DB().ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminPool.DB().ExecContext(ctx, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	cfg.MaxOpenConns = 4
	schemaPool, err := dbpool.New(ctx, cfg, func(poolCfg *pgxpool.Config) {
		poolCfg.ConnConfig.RuntimeParams["search_path"] = schemaName
	})
	if err != nil {
		t.Fatalf("create schema database pool: %v", err)
	}
	t.Cleanup(schemaPool.Close)

	if _, err := schemaPool.DB().ExecContext(ctx, "CREATE TABLE robot (id SERIAL PRIMARY KEY)"); err != nil {
		t.Fatalf("create robot dependency: %v", err)
	}

	dir := versionedTestSchemaDir()
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			errCh <- applyVersionedSchema(ctx, sqlVersionedSchemaDB{db: schemaPool.DB()}, dir)
		}()
	}
	for range 2 {
		if err := <-errCh; err != nil {
			t.Errorf("applyVersionedSchema() concurrent run returned error: %v", err)
		}
	}

	if err := applyVersionedSchema(ctx, sqlVersionedSchemaDB{db: schemaPool.DB()}, dir); err != nil {
		t.Fatalf("applyVersionedSchema() repeat run returned error: %v", err)
	}

	var recorded int
	if err := schemaPool.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM harbor_next_schema_migrations").Scan(&recorded); err != nil {
		t.Fatalf("count harbor_next_schema_migrations: %v", err)
	}
	if recorded != 1 {
		t.Errorf("harbor_next_schema_migrations has %d row(s), want 1 (idempotent repeat run should not re-insert)", recorded)
	}

	objects := []string{
		"branding",
		"identity_providers",
		"robot_identity_providers",
		"claim_rules",
		"idx_claim_rules_lookup",
		"idx_identity_providers_jwks_cache",
	}
	for _, object := range objects {
		var exists bool
		if err := schemaPool.DB().QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", object).Scan(&exists); err != nil {
			t.Errorf("look up %s: %v", object, err)
			continue
		}
		if !exists {
			t.Errorf("versioned schema object %q does not exist", object)
		}
	}
}

func versionedTestDatabaseConfig() *models.PostGreSQL {
	port := 5432
	if value := os.Getenv("POSTGRESQL_PORT"); value != "" {
		if configuredPort, err := strconv.Atoi(value); err == nil {
			port = configuredPort
		}
	}
	return &models.PostGreSQL{
		Host:     environmentOr("POSTGRESQL_HOST", "localhost"),
		Port:     port,
		Username: environmentOr("POSTGRESQL_USR", environmentOr("POSTGRESQL_USERNAME", "postgres")),
		Password: environmentOr("POSTGRESQL_PWD", environmentOr("POSTGRESQL_PASSWORD", "root123")),
		Database: environmentOr("POSTGRESQL_DATABASE", "registry"),
		SSLMode:  "disable",
	}
}

func versionedTestSchemaDir() string {
	if dir := os.Getenv("POSTGRES_MIGRATION_SCRIPTS_PATH"); dir != "" {
		return dir
	}
	return filepath.Join("..", "..", "make", "migrations", "postgresql")
}

func environmentOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
