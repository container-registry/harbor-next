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

// fork-migrator is a standalone CLI tool for managing fork-specific database
// schema migrations independently from upstream Harbor migrations.
//
// It tracks fork migration state in a separate "fork_schema_migrations" table,
// leaving the upstream "schema_migrations" table completely untouched.
//
// Usage:
//
//	fork-migrator <command> [args]
//
// Commands:
//
//	up            Apply all pending fork migrations
//	down          Roll back ALL fork migrations (return to upstream-only state)
//	steps N       Apply N steps (positive=forward, negative=backward)
//	migrate-to V  Migrate to specific version V
//	version       Print current fork migration version
//	force V       Force-set version to V (for recovering from dirty state)
//	status        Show current state summary
package main

import (
	"fmt"
	"os"
	"strconv"

	migrate "github.com/golang-migrate/migrate/v4"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/log"
	forkmigration "github.com/goharbor/harbor/src/pkg/migration/fork"
)

var defaultAttrs = map[string]string{
	"POSTGRESQL_HOST":     "localhost",
	"POSTGRESQL_PORT":     "5432",
	"POSTGRESQL_USERNAME": "postgres",
	"POSTGRESQL_PASSWORD": "password",
	"POSTGRESQL_DATABASE": "registry",
	"POSTGRESQL_SSLMODE":  "disable",
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	db := buildDBConfig()

	mgr, err := forkmigration.NewManager(db)
	if err != nil {
		log.Fatalf("Failed to initialize fork migrator: %v", err)
	}
	defer mgr.Close()

	switch command {
	case "up":
		if err := mgr.Up(); err != nil {
			log.Fatalf("Up failed: %v", err)
		}

	case "down":
		if err := mgr.Down(); err != nil {
			log.Fatalf("Down failed: %v", err)
		}

	case "steps":
		if len(os.Args) < 3 {
			log.Fatal("Usage: fork-migrator steps <N>")
		}
		n, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalf("Invalid step count %q: %v", os.Args[2], err)
		}
		if err := mgr.Steps(n); err != nil {
			log.Fatalf("Steps failed: %v", err)
		}

	case "migrate-to":
		if len(os.Args) < 3 {
			log.Fatal("Usage: fork-migrator migrate-to <version>")
		}
		v, err := strconv.ParseUint(os.Args[2], 10, 32)
		if err != nil {
			log.Fatalf("Invalid version %q: %v", os.Args[2], err)
		}
		if err := mgr.MigrateTo(uint(v)); err != nil {
			log.Fatalf("Migrate-to failed: %v", err)
		}

	case "version":
		printVersion(mgr)

	case "force":
		if len(os.Args) < 3 {
			log.Fatal("Usage: fork-migrator force <version>")
		}
		v, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalf("Invalid version %q: %v", os.Args[2], err)
		}
		if err := mgr.Force(v); err != nil {
			log.Fatalf("Force failed: %v", err)
		}
		fmt.Printf("Fork migration version forced to %d\n", v)

	case "status":
		printStatus(mgr)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func buildDBConfig() *models.PostGreSQL {
	port, _ := strconv.Atoi(getAttr("POSTGRESQL_PORT"))
	return &models.PostGreSQL{
		Host:         getAttr("POSTGRESQL_HOST"),
		Port:         port,
		Username:     getAttr("POSTGRESQL_USERNAME"),
		Password:     getAttr("POSTGRESQL_PASSWORD"),
		Database:     getAttr("POSTGRESQL_DATABASE"),
		SSLMode:      getAttr("POSTGRESQL_SSLMODE"),
		MaxIdleConns: 5,
		MaxOpenConns: 5,
	}
}

func getAttr(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return defaultAttrs[k]
}

func printVersion(mgr *forkmigration.Manager) {
	version, dirty, err := mgr.Version()
	if err == migrate.ErrNilVersion {
		fmt.Println("Fork migration version: none (no fork migrations applied)")
		return
	}
	if err != nil {
		log.Fatalf("Failed to get version: %v", err)
	}
	dirtyStr := ""
	if dirty {
		dirtyStr = " (DIRTY - run 'force' to recover)"
	}
	fmt.Printf("Fork migration version: %d%s\n", version, dirtyStr)
}

func printStatus(mgr *forkmigration.Manager) {
	fmt.Println("=== Fork Migration Status ===")
	fmt.Printf("Migration table:  %s\n", forkmigration.ForkMigrationsTable)
	fmt.Printf("Migration path:   %s\n", getAttr("FORK_MIGRATION_SCRIPTS_PATH"))
	if p := getAttr("FORK_MIGRATION_SCRIPTS_PATH"); p == "" {
		fmt.Printf("Migration path:   %s (default)\n", forkmigration.DefaultForkMigrationPath)
	}
	fmt.Printf("Database:         %s@%s:%s/%s\n",
		getAttr("POSTGRESQL_USERNAME"),
		getAttr("POSTGRESQL_HOST"),
		getAttr("POSTGRESQL_PORT"),
		getAttr("POSTGRESQL_DATABASE"),
	)
	printVersion(mgr)
	fmt.Println()
	fmt.Println("Upstream migrations are tracked separately in 'schema_migrations'.")
	fmt.Println("Fork migrations do NOT affect upstream state.")
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `fork-migrator — Manage fork-specific database schema migrations

These migrations are tracked independently from upstream Harbor migrations.
Upstream's "schema_migrations" table is never touched.

Usage:
  fork-migrator <command> [args]

Commands:
  up              Apply all pending fork migrations
  down            Roll back ALL fork migrations (return to upstream-only schema)
  steps <N>       Apply N steps (positive=forward, negative=backward)
  migrate-to <V>  Migrate to a specific fork version
  version         Print current fork migration version
  force <V>       Force-set version (recover from dirty/failed migration)
  status          Show migration state summary

Environment variables:
  POSTGRESQL_HOST       Database host     (default: localhost)
  POSTGRESQL_PORT       Database port     (default: 5432)
  POSTGRESQL_USERNAME   Database user     (default: postgres)
  POSTGRESQL_PASSWORD   Database password (default: password)
  POSTGRESQL_DATABASE   Database name     (default: registry)
  POSTGRESQL_SSLMODE    SSL mode          (default: disable)
  FORK_MIGRATION_SCRIPTS_PATH  Path to fork .sql files (default: migrations/postgresql/fork/)

Workflow — upgrading upstream while preserving fork changes:
  1. fork-migrator down          # roll back fork migrations
  2. <merge upstream changes>
  3. standalone-db-migrator      # apply upstream migrations
  4. fork-migrator up            # re-apply fork migrations
`)
}
