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
	"os"
	"path/filepath"
	"testing"
)

func TestNumberedMigrationExists(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"0180_2.15.0_schema.up.sql",
		"0181_2.15.3_schema.up.sql",
		"harbor_next.sql",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("POSTGRES_MIGRATION_SCRIPTS_PATH", dir)

	if numberedMigrationExists(legacyPatchVersion) {
		t.Errorf("numberedMigrationExists(%d) = true; the legacy reset would be skipped", legacyPatchVersion)
	}
	if !numberedMigrationExists(181) {
		t.Error("numberedMigrationExists(181) = false; want true for a present migration")
	}

	// A future backport occupying the legacy version must disable the reset.
	if err := os.WriteFile(filepath.Join(dir, "0182_2.15.9_schema.up.sql"), []byte("-- test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !numberedMigrationExists(legacyPatchVersion) {
		t.Errorf("numberedMigrationExists(%d) = false with 0182 file present; the reset would rewind a genuine database", legacyPatchVersion)
	}
}

// TestShouldResetLegacyVersion pins the escape hatch to exactly 182: fresh
// databases, upstream-migrated databases (181), and every other version must
// never be rewound.
func TestShouldResetLegacyVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"0180_2.15.0_schema.up.sql",
		"0181_2.15.3_schema.up.sql",
		"harbor_next.sql",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("POSTGRES_MIGRATION_SCRIPTS_PATH", dir)

	cases := []struct {
		name    string
		version uint
		want    bool
	}{
		{"fresh database (0)", 0, false},
		{"old upstream (100)", 100, false},
		{"pre-2.15.3 (180)", 180, false},
		{"upstream-migrated OSS install (181)", 181, false},
		{"retired commercial migrations (182)", 182, true},
		{"beyond legacy (183)", 183, false},
		{"future minor (190)", 190, false},
	}
	for _, tc := range cases {
		if got := shouldResetLegacyVersion(tc.version); got != tc.want {
			t.Errorf("%s: shouldResetLegacyVersion(%d) = %t, want %t", tc.name, tc.version, got, tc.want)
		}
	}

	// Regression: once a real 0182 migration ships, 182 becomes a genuine
	// version and the hatch must stay closed.
	if err := os.WriteFile(filepath.Join(dir, "0182_2.15.9_schema.up.sql"), []byte("-- test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if shouldResetLegacyVersion(182) {
		t.Error("shouldResetLegacyVersion(182) = true with a real 0182 migration present; must not rewind")
	}
}
