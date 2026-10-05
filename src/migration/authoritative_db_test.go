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

	if _, err := schemaPool.DB().ExecContext(ctx, "CREATE TABLE robot (id BIGSERIAL PRIMARY KEY)"); err != nil {
		t.Fatalf("create robot dependency: %v", err)
	}

	if _, err := schemaPool.DB().ExecContext(ctx, "CREATE TABLE execution (id SERIAL PRIMARY KEY, revision INTEGER)"); err != nil {
		t.Fatalf("create execution dependency: %v", err)
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

// Databases created before the rename still carry the identity_providers
// layout and the old config and RBAC names. The apply renames them in place
// and keeps the rows.
func TestTrustedIssuersRenameKeepsRows(t *testing.T) {
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

	for _, statement := range []string{
		"CREATE TABLE robot (id BIGSERIAL PRIMARY KEY)",
		"CREATE TABLE execution (id SERIAL PRIMARY KEY, revision INTEGER)",
		"CREATE TABLE properties (id SERIAL PRIMARY KEY, k VARCHAR(64) NOT NULL UNIQUE, v VARCHAR(1024) NOT NULL)",
		`CREATE TABLE permission_policy (id SERIAL PRIMARY KEY, scope VARCHAR(255) NOT NULL, resource VARCHAR(255),
			action VARCHAR(255), effect VARCHAR(255), CONSTRAINT unique_rbac_policy UNIQUE (scope, resource, action, effect))`,
		`CREATE TABLE role_permission (id SERIAL PRIMARY KEY, role_type VARCHAR(255) NOT NULL, role_id INT NOT NULL,
			permission_policy_id INT NOT NULL, CONSTRAINT unique_role_permission UNIQUE (role_type, role_id, permission_policy_id))`,
		`CREATE TABLE identity_providers (id SERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT, issuer TEXT NOT NULL,
			openid_config_url TEXT, offline_validation BOOLEAN NOT NULL DEFAULT FALSE, supported_algorithms TEXT,
			claims_supported TEXT, jwks_uri TEXT, jwks_keys JSONB, jwks_cached_at TIMESTAMP, jwks_expires_at TIMESTAMP,
			jwks_last_fetch_attempt TIMESTAMP, project_id INT NOT NULL DEFAULT 0, creation_time TIMESTAMP DEFAULT NOW(),
			update_time TIMESTAMP DEFAULT NOW(), UNIQUE (issuer, project_id))`,
		`CREATE TABLE robot_identity_providers (id SERIAL PRIMARY KEY,
			identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
			robot_id BIGINT NOT NULL REFERENCES robot(id) ON DELETE CASCADE, creation_time TIMESTAMP DEFAULT NOW(),
			UNIQUE (identity_provider_id, robot_id))`,
		`CREATE TABLE claim_rules (id SERIAL PRIMARY KEY,
			identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
			robot_id BIGINT NOT NULL DEFAULT 0, claim_path TEXT NOT NULL, value TEXT, creation_time TIMESTAMP DEFAULT NOW())`,
		"CREATE INDEX idx_identity_providers_jwks_cache ON identity_providers (id, jwks_expires_at, jwks_last_fetch_attempt)",
		"INSERT INTO robot (id) VALUES (1)",
		"INSERT INTO identity_providers (name, issuer) VALUES ('ci', 'https://issuer.example.com')",
		"INSERT INTO robot_identity_providers (identity_provider_id, robot_id) VALUES (1, 1)",
		"INSERT INTO claim_rules (identity_provider_id, claim_path, value) VALUES (1, 'sub', 'repo:x')",
		"INSERT INTO properties (k, v) VALUES ('enable_commercial_identity_providers', 'true'), ('enable_project_federated_idp', 'true')",
		"INSERT INTO permission_policy (scope, resource, action, effect) VALUES ('/system', 'federated-idp', 'list', 'allow')",
		"INSERT INTO role_permission (role_type, role_id, permission_policy_id) VALUES ('robot', 1, 1)",
	} {
		if _, err := schemaPool.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}

	path := authoritativeTestSchemaPath()
	for pass := 1; pass <= 2; pass++ {
		if err := applyAuthoritativeSchema(ctx, sqlSchemaDB{db: schemaPool.DB()}, path); err != nil {
			t.Fatalf("applyAuthoritativeSchema() pass %d: %v", pass, err)
		}
	}

	for _, gone := range []string{"identity_providers", "robot_identity_providers", "idx_identity_providers_jwks_cache"} {
		var exists bool
		if err := schemaPool.DB().QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", gone).Scan(&exists); err != nil {
			t.Fatalf("look up %s: %v", gone, err)
		}
		if exists {
			t.Errorf("%s still exists after the rename", gone)
		}
	}

	checks := map[string]string{
		"SELECT issuer FROM trusted_issuers WHERE id = 1":                                                  "https://issuer.example.com",
		"SELECT robot_id::text FROM robot_trusted_issuers WHERE trusted_issuer_id = 1":                     "1",
		"SELECT value FROM claim_rules WHERE trusted_issuer_id = 1":                                        "repo:x",
		"SELECT v FROM properties WHERE k = 'enable_commercial_federated_robot_accounts'":                  "true",
		"SELECT v FROM properties WHERE k = 'enable_project_federated_robot_accounts'":                     "true",
		"SELECT resource FROM permission_policy p JOIN role_permission r ON r.permission_policy_id = p.id": "trusted-issuer",
	}
	for query, want := range checks {
		var got string
		if err := schemaPool.DB().QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Errorf("%s: %v", query, err)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", query, got, want)
		}
	}
}

// A relation named execution that is not a table must not take the schema
// apply down with it. to_regclass resolves any relation, so an index of that
// name earlier in search_path shadows the table; the guard declines rather
// than sending ALTER TABLE at an index, which would abort the whole file.

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
	} {
		if _, err := schemaPool.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}

	if err := applyAuthoritativeSchema(ctx, sqlSchemaDB{db: schemaPool.DB()}, authoritativeTestSchemaPath()); err != nil {
		t.Fatalf("applyAuthoritativeSchema() with a shadowing index returned error: %v", err)
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
