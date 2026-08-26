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
	"fmt"
	"path/filepath"
	"time"

	"github.com/golang-migrate/migrate/v4"

	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/metric"
)

const (
	// legacyPatchVersion is the schema_migrations value written by the retired
	// 8gears numbered migrations (0181/0182); their DDL now lives in the
	// authoritative harbor_next.sql, so the numbered chain ends below it and
	// golang-migrate cannot orient itself on such databases.
	legacyPatchVersion = 182
	// legacyResetVersion is the last upstream migration those databases
	// actually applied; resetting to it lets the upstream chain continue.
	legacyResetVersion = 180
)

// Migrate upgrades DB schema and do necessary transformation of the data in DB
func Migrate(database *models.Database) error {
	// check the database schema version
	migrator, err := dao.NewMigrator(database.PostGreSQL)
	if err != nil {
		return err
	}
	defer migrator.Close()
	schemaVersion, _, err := migrator.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return err
	}
	log.Debugf("current database schema version: %v", schemaVersion)
	if shouldResetLegacyVersion(schemaVersion) {
		log.Infof("schema version %d stems from the retired commercial numbered migrations; resetting to %d so the upstream chain continues (no tables are touched, their DDL is reconciled via harbor_next.sql)", legacyPatchVersion, legacyResetVersion)
		if err := resetLegacyVersion(context.Background()); err != nil {
			return fmt.Errorf("reset legacy schema version %d to %d: %w", legacyPatchVersion, legacyResetVersion, err)
		}
	}
	start := time.Now()
	if err := dao.UpgradeSchema(database); err != nil {
		return err
	}
	observeMigrationPhase("numbered", start)

	pool := dao.GetPool()
	if pool == nil {
		return fmt.Errorf("apply authoritative Harbor Next schema: database pool is not initialized")
	}
	start = time.Now()
	if err := applyAuthoritativeSchema(context.Background(), sqlSchemaDB{db: pool.DB()}, authoritativeSchemaPath()); err != nil {
		return err
	}
	observeMigrationPhase("authoritative", start)
	return nil
}

// resetLegacyVersion rewinds schema_migrations with a compare-and-set: the
// WHERE clause makes it a no-op when a concurrent core instance already reset
// the version or advanced past it, so a stale 182 read can never rewind a
// database that has moved on (multi-replica startup safety).
func resetLegacyVersion(ctx context.Context) error {
	pool := dao.GetPool()
	if pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	result, err := pool.DB().ExecContext(ctx,
		"UPDATE schema_migrations SET version = $1, dirty = false WHERE version = $2",
		legacyResetVersion, legacyPatchVersion)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		log.Infof("schema version already reset or advanced by another core instance, skipping")
	}
	return nil
}

// shouldResetLegacyVersion is true only for the exact version the retired
// commercial numbered migrations wrote; anything else (including 181, where
// upstream-migrated databases legitimately sit) is never rewound.
func shouldResetLegacyVersion(version uint) bool {
	return version == legacyPatchVersion && !numberedMigrationExists(legacyPatchVersion)
}

// numberedMigrationExists guards the legacy reset: a future release-2.15
// backport may legitimately occupy the version, in which case the database
// state is genuine and must not be rewound.
func numberedMigrationExists(version uint) bool {
	matches, err := filepath.Glob(filepath.Join(migrationSourceDir(), fmt.Sprintf("%04d_*.up.sql", version)))
	if err != nil {
		return false
	}
	return len(matches) > 0
}

// observeMigrationPhase logs and records how long a migration phase took
func observeMigrationPhase(phase string, start time.Time) {
	elapsed := time.Since(start)
	log.Infof("migration phase %q took %s", phase, elapsed)
	metric.MigrationPhaseDuration.WithLabelValues(phase).Observe(elapsed.Seconds())
}
