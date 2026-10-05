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

	// Legacy tables created by the numbered migrations that harbor_next.sql
	// declares foreign keys against or reconciles in place.
	legacyDependencies := []string{
		"CREATE TABLE robot (id BIGSERIAL PRIMARY KEY)",
		"CREATE TABLE project (project_id SERIAL PRIMARY KEY)",
		"CREATE TABLE execution (id SERIAL PRIMARY KEY, revision INTEGER)",
	}
	for _, statement := range legacyDependencies {
		if _, err := schemaPool.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("create legacy dependency: %v", err)
		}
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
		"trusted_issuers",
		"robot_trusted_issuers",
		"claim_rules",
		"idx_claim_rules_lookup",
		"idx_trusted_issuers_jwks_cache",
		"multi_format_package",
		"multi_format_version",
		"multi_project_reference",
		"idx_multi_format_package_proj_fmt",
		"idx_multi_format_version_pkg",
		"idx_multi_project_reference_multi",
		"idx_multi_project_reference_sub",
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
		"trusted_issuers": {
			"id", "name", "description", "issuer", "openid_config_url",
			"offline_validation", "supported_algorithms", "claims_supported",
			"jwks_uri", "jwks_keys", "jwks_cached_at", "jwks_expires_at",
			"jwks_last_fetch_attempt", "project_id", "creation_time", "update_time",
		},
		"robot_trusted_issuers": {
			"id", "trusted_issuer_id", "robot_id", "creation_time",
		},
		"claim_rules": {
			"id", "trusted_issuer_id", "robot_id", "claim_path", "value", "creation_time",
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

	// reconciled in place on a table the numbered migrations own
	var revisionType string
	err = schemaPool.DB().QueryRowContext(ctx, `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'execution'
		  AND column_name = 'revision'`).Scan(&revisionType)
	if err != nil {
		t.Errorf("look up execution.revision type: %v", err)
	} else if revisionType != "bigint" {
		t.Errorf("execution.revision is %q, want bigint", revisionType)
	}
}

// The revision guard resolves the execution table through search_path rather
// than current_schema(). This puts execution in a schema that is NOT first in
// search_path, which is where the original guard read false and skipped the
// widening in silence.
func TestExecutionRevisionGuardResolvesThroughSearchPath(t *testing.T) {
	ctx := context.Background()
	cfg := authoritativeTestDatabaseConfig()
	adminPool, err := dbpool.New(ctx, cfg)
	if err != nil {
		t.Fatalf("create admin database pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	suffix := time.Now().UnixNano()
	first := fmt.Sprintf("harbor_next_first_%d", suffix)
	later := fmt.Sprintf("harbor_next_later_%d", suffix)
	for _, name := range []string{first, later} {
		if _, err := adminPool.DB().ExecContext(ctx, "CREATE SCHEMA "+name); err != nil {
			t.Fatalf("create schema %s: %v", name, err)
		}
		t.Cleanup(func() {
			if _, err := adminPool.DB().ExecContext(ctx, "DROP SCHEMA "+name+" CASCADE"); err != nil {
				t.Errorf("drop schema %s: %v", name, err)
			}
		})
	}

	cfg.MaxOpenConns = 4
	schemaPool, err := dbpool.New(ctx, cfg, func(poolCfg *pgxpool.Config) {
		poolCfg.ConnConfig.RuntimeParams["search_path"] = first + ", " + later
	})
	if err != nil {
		t.Fatalf("create schema database pool: %v", err)
	}
	t.Cleanup(schemaPool.Close)

	// execution lives only in the later schema, which is the case the old
	// current_schema() guard could not see
	setup := []string{
		fmt.Sprintf("CREATE TABLE %s.execution (id SERIAL PRIMARY KEY, revision INTEGER)", later),
		fmt.Sprintf("CREATE TABLE %s.robot (id BIGSERIAL PRIMARY KEY)", first),
		fmt.Sprintf("CREATE TABLE %s.project (project_id SERIAL PRIMARY KEY)", first),
	}
	for _, statement := range setup {
		if _, err := schemaPool.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}

	if _, err := schemaPool.DB().ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s.execution (revision) VALUES (7)", later)); err != nil {
		t.Fatalf("seed execution row: %v", err)
	}

	path := authoritativeTestSchemaPath()
	// twice over: the widening must happen, and the repeat must not undo it
	for pass := 1; pass <= 2; pass++ {
		if err := applyAuthoritativeSchema(ctx, sqlSchemaDB{db: schemaPool.DB()}, path); err != nil {
			t.Fatalf("applyAuthoritativeSchema() pass %d: %v", pass, err)
		}

		var revisionType string
		if err := schemaPool.DB().QueryRowContext(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_schema = $1
			  AND table_name = 'execution'
			  AND column_name = 'revision'`, later).Scan(&revisionType); err != nil {
			t.Fatalf("look up execution.revision type on pass %d: %v", pass, err)
		}
		if revisionType != "bigint" {
			t.Fatalf("pass %d: execution.revision is %q, want bigint", pass, revisionType)
		}

		var revision int64
		if err := schemaPool.DB().QueryRowContext(ctx,
			fmt.Sprintf("SELECT revision FROM %s.execution", later)).Scan(&revision); err != nil {
			t.Fatalf("read seeded revision on pass %d: %v", pass, err)
		}
		if revision != 7 {
			t.Errorf("pass %d: seeded revision is %d, want 7 preserved across the widening", pass, revision)
		}
	}
}

// A relation named execution that is not a table must not take the schema
// apply down with it. to_regclass resolves any relation, so an index of that
// name earlier in search_path shadows the table; the guard declines rather
// than sending ALTER TABLE at an index, which would abort the whole file.
func TestExecutionRevisionGuardIgnoresNonTableRelations(t *testing.T) {
	ctx := context.Background()
	cfg := authoritativeTestDatabaseConfig()
	adminPool, err := dbpool.New(ctx, cfg)
	if err != nil {
		t.Fatalf("create admin database pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("harbor_next_shadow_%d", time.Now().UnixNano())
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

	for _, statement := range []string{
		"CREATE TABLE decoy (id BIGSERIAL PRIMARY KEY, revision INTEGER)",
		"CREATE INDEX execution ON decoy (revision)",
		"CREATE TABLE robot (id BIGSERIAL PRIMARY KEY)",
		"CREATE TABLE project (project_id SERIAL PRIMARY KEY)",
	} {
		if _, err := schemaPool.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}

	if err := applyAuthoritativeSchema(ctx, sqlSchemaDB{db: schemaPool.DB()}, authoritativeTestSchemaPath()); err != nil {
		t.Fatalf("applyAuthoritativeSchema() with a shadowing index returned error: %v", err)
	}
}

// Databases created before the trusted issuer rename hold their rows in
// identity_providers and robot_identity_providers. The schema apply renames
// them in place, keeps the rows and their links, and a repeat run changes
// nothing.
func TestTrustedIssuerRenamePreservesData(t *testing.T) {
	ctx := context.Background()
	cfg := authoritativeTestDatabaseConfig()
	adminPool, err := dbpool.New(ctx, cfg)
	if err != nil {
		t.Fatalf("create admin database pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("harbor_next_rename_%d", time.Now().UnixNano())
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

	db := schemaPool.DB()
	for _, statement := range []string{
		"CREATE TABLE robot (id BIGSERIAL PRIMARY KEY)",
		"CREATE TABLE project (project_id SERIAL PRIMARY KEY)",
		"CREATE TABLE properties (id SERIAL PRIMARY KEY, k varchar(64) NOT NULL UNIQUE, v text NOT NULL)",
		`CREATE TABLE permission_policy (id SERIAL PRIMARY KEY, scope varchar(255) NOT NULL,
			resource varchar(255), action varchar(255), effect varchar(255),
			CONSTRAINT unique_rbac_policy UNIQUE (scope, resource, action, effect))`,
		`CREATE TABLE identity_providers (
			id SERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, issuer TEXT NOT NULL,
			openid_config_url TEXT, offline_validation BOOLEAN NOT NULL DEFAULT FALSE,
			supported_algorithms TEXT, claims_supported TEXT, jwks_uri TEXT, jwks_keys JSONB,
			jwks_cached_at TIMESTAMP, jwks_expires_at TIMESTAMP, jwks_last_fetch_attempt TIMESTAMP,
			project_id INT NOT NULL DEFAULT 0, creation_time TIMESTAMP DEFAULT NOW(),
			update_time TIMESTAMP DEFAULT NOW(), UNIQUE (issuer, project_id))`,
		`CREATE TABLE robot_identity_providers (
			id SERIAL PRIMARY KEY,
			identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
			robot_id BIGINT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
			creation_time TIMESTAMP DEFAULT NOW(), UNIQUE (identity_provider_id, robot_id))`,
		`CREATE TABLE claim_rules (
			id SERIAL PRIMARY KEY,
			identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
			robot_id BIGINT NOT NULL DEFAULT 0, claim_path TEXT NOT NULL, value TEXT,
			creation_time TIMESTAMP DEFAULT NOW())`,
		"CREATE INDEX idx_claim_rules_lookup ON claim_rules (identity_provider_id, claim_path, value, robot_id)",
		"CREATE INDEX idx_identity_providers_jwks_cache ON identity_providers (id, jwks_expires_at, jwks_last_fetch_attempt)",
		"INSERT INTO robot (id) VALUES (7)",
		"INSERT INTO identity_providers (name, issuer) VALUES ('kube', 'https://kubernetes.default.svc')",
		"INSERT INTO robot_identity_providers (identity_provider_id, robot_id) VALUES (1, 7)",
		"INSERT INTO claim_rules (identity_provider_id, robot_id, claim_path, value) VALUES (1, 7, 'sub', 'system:serviceaccount:ci:builder')",
		"INSERT INTO properties (k, v) VALUES ('enable_project_federated_idp', 'true')",
		"INSERT INTO permission_policy (scope, resource, action, effect) VALUES ('/project/1', 'federated-idp', 'list', 'allow')",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}

	for pass := 1; pass <= 2; pass++ {
		if err := applyAuthoritativeSchema(ctx, sqlSchemaDB{db: db}, authoritativeTestSchemaPath()); err != nil {
			t.Fatalf("applyAuthoritativeSchema() pass %d: %v", pass, err)
		}

		for _, gone := range []string{
			"identity_providers", "robot_identity_providers", "idx_identity_providers_jwks_cache",
			"identity_providers_id_seq", "robot_identity_providers_id_seq",
		} {
			var exists bool
			if err := db.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", gone).Scan(&exists); err != nil {
				t.Fatalf("look up %s: %v", gone, err)
			}
			if exists {
				t.Errorf("pass %d: %s still exists after the rename", pass, gone)
			}
		}

		var leftover []string
		rows, err := db.QueryContext(ctx, `
			SELECT conname FROM pg_constraint
			WHERE connamespace = to_regnamespace($1) AND conname LIKE '%identity_provider%'`, schemaName)
		if err != nil {
			t.Fatalf("list constraints: %v", err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan constraint: %v", err)
			}
			leftover = append(leftover, name)
		}
		rows.Close()
		if len(leftover) > 0 {
			t.Errorf("pass %d: constraints still named after identity providers: %v", pass, leftover)
		}

		var name, claimValue string
		var robotID int64
		if err := db.QueryRowContext(ctx, `
			SELECT t.name, r.robot_id, c.value
			FROM trusted_issuers t
			JOIN robot_trusted_issuers r ON r.trusted_issuer_id = t.id
			JOIN claim_rules c ON c.trusted_issuer_id = t.id
			WHERE t.issuer = 'https://kubernetes.default.svc'`).Scan(&name, &robotID, &claimValue); err != nil {
			t.Fatalf("pass %d: read renamed rows: %v", pass, err)
		}
		if name != "kube" || robotID != 7 || claimValue != "system:serviceaccount:ci:builder" {
			t.Errorf("pass %d: renamed rows = (%q, %d, %q), want (kube, 7, system:serviceaccount:ci:builder)", pass, name, robotID, claimValue)
		}

		var configKeys, policyResources int
		if err := db.QueryRowContext(ctx,
			"SELECT count(*) FROM properties WHERE k = 'enable_project_federated_robot_accounts' AND v = 'true'").Scan(&configKeys); err != nil {
			t.Fatalf("count config keys: %v", err)
		}
		if err := db.QueryRowContext(ctx,
			"SELECT count(*) FROM permission_policy WHERE resource = 'trusted-issuer'").Scan(&policyResources); err != nil {
			t.Fatalf("count policies: %v", err)
		}
		if configKeys != 1 || policyResources != 1 {
			t.Errorf("pass %d: renamed config keys = %d, policies = %d, want 1 and 1", pass, configKeys, policyResources)
		}
	}

	// the serial sequence followed the table, so new rows still get ids
	if _, err := db.ExecContext(ctx,
		"INSERT INTO trusted_issuers (name, issuer) VALUES ('gitlab', 'https://gitlab.com')"); err != nil {
		t.Fatalf("insert after rename: %v", err)
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
