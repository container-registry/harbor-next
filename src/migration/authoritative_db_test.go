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

func TestAuthoritativeSchemaAgainstPostgreSQL(t *testing.T) {
	ctx := context.Background()
	cfg := authoritativeTestDatabaseConfig()
	adminPool, err := dbpool.New(ctx, cfg)
	if err != nil {
		t.Fatalf("create admin database pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("harbor_next_schema_test_%d", time.Now().UnixNano())
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

	// Stubs use the pre-amendment 0190 shapes so the reconciliation blocks run.
	if _, err := schemaPool.DB().ExecContext(ctx, "CREATE TABLE robot (id BIGSERIAL PRIMARY KEY, creator_ref integer NOT NULL DEFAULT 0)"); err != nil {
		t.Fatalf("create robot dependency: %v", err)
	}
	if _, err := schemaPool.DB().ExecContext(ctx, "CREATE TABLE role_permission (id SERIAL PRIMARY KEY, role_id integer NOT NULL)"); err != nil {
		t.Fatalf("create role_permission dependency: %v", err)
	}
	if _, err := schemaPool.DB().ExecContext(ctx, "CREATE TABLE audit_log_ext (id BIGSERIAL PRIMARY KEY)"); err != nil {
		t.Fatalf("create audit_log_ext dependency: %v", err)
	}

	path := authoritativeTestSchemaPath()
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			errCh <- applyAuthoritativeSchema(ctx, sqlSchemaDB{db: schemaPool.DB()}, path)
		}()
	}
	for range 2 {
		if err := <-errCh; err != nil {
			t.Errorf("applyAuthoritativeSchema() concurrent run returned error: %v", err)
		}
	}

	if err := applyAuthoritativeSchema(ctx, sqlSchemaDB{db: schemaPool.DB()}, path); err != nil {
		t.Fatalf("applyAuthoritativeSchema() repeat run returned error: %v", err)
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
			t.Errorf("authoritative schema object %q does not exist", object)
		}
	}

	columns := map[string][]string{
		"branding": {
			"id", "config", "update_time",
		},
		"identity_providers": {
			"id", "name", "description", "issuer", "openid_config_url",
			"offline_validation", "supported_algorithms", "claims_supported",
			"jwks_uri", "jwks_keys", "jwks_cached_at", "jwks_expires_at",
			"jwks_last_fetch_attempt", "project_id", "creation_time", "update_time",
		},
		"robot_identity_providers": {
			"id", "identity_provider_id", "robot_id", "creation_time",
		},
		"claim_rules": {
			"id", "identity_provider_id", "robot_id", "claim_path", "value", "creation_time",
		},
		"audit_log_ext": {
			"client_address", "user_agent",
		},
	}
	for table, tableColumns := range columns {
		for _, column := range tableColumns {
			var exists bool
			err := schemaPool.DB().QueryRowContext(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = $1
					  AND column_name = $2
				)`, table, column).Scan(&exists)
			if err != nil {
				t.Errorf("look up column %s.%s: %v", table, column, err)
				continue
			}
			if !exists {
				t.Errorf("authoritative schema column %q does not exist", table+"."+column)
			}
		}
	}

	bigintColumns := [][2]string{
		{"robot", "id"},
		{"robot", "creator_ref"},
		{"role_permission", "role_id"},
	}
	for _, tableColumn := range bigintColumns {
		var dataType string
		err := schemaPool.DB().QueryRowContext(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1
			  AND column_name = $2`, tableColumn[0], tableColumn[1]).Scan(&dataType)
		if err != nil {
			t.Errorf("look up column type %s.%s: %v", tableColumn[0], tableColumn[1], err)
			continue
		}
		if dataType != "bigint" {
			t.Errorf("column %s.%s is %q, want bigint", tableColumn[0], tableColumn[1], dataType)
		}
	}
}

func authoritativeTestDatabaseConfig() *models.PostGreSQL {
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

func authoritativeTestSchemaPath() string {
	if dir := os.Getenv("POSTGRES_MIGRATION_SCRIPTS_PATH"); dir != "" {
		return filepath.Join(dir, authoritativeSchemaFile)
	}
	return filepath.Join("..", "..", "make", "migrations", "postgresql", authoritativeSchemaFile)
}

func environmentOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
