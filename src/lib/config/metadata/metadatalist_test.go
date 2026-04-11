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

package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
)

// TestDatabaseConfigDefaults pins the default values for all PostgreSQL config
// knobs. If someone changes a default, this test breaks — forcing an explicit
// decision rather than a silent behavior change.
func TestDatabaseConfigDefaults(t *testing.T) {
	expected := map[string]string{
		common.PostGreSQLMaxOpenConns:      "100",
		common.PostGreSQLConnMaxLifetime:   "5m",
		common.PostGreSQLConnMaxIdleTime:   "0",
		common.PostGreSQLHealthCheckPeriod: "1m",
		common.PostGreSQLConnectTimeout:    "10s",
		common.PostGreSQLMinConns:          "2",
		common.PostGreSQLURL:               "",
		common.PostGreSQLHOST:              "postgresql",
		common.PostGreSQLPort:              "5432",
		common.PostGreSQLUsername:           "postgres",
		common.PostGreSQLSSLMode:           "disable",
	}

	meta := Instance()
	for name, want := range expected {
		item, ok := meta.GetByName(name)
		require.True(t, ok, "config key %q must exist in metadata", name)
		assert.Equal(t, want, item.DefaultValue, "default for %q", name)
	}
}

// TestDatabaseConfigRemovedKeys verifies that deprecated config keys are not
// accidentally re-added to the metadata.
func TestDatabaseConfigRemovedKeys(t *testing.T) {
	meta := Instance()

	// POSTGRESQL_MAX_IDLE_CONNS was removed in the pgx/v5 migration.
	// pgxpool manages idle connections via MinConns; there is no MaxIdleConns knob.
	_, exists := meta.GetByName("postgresql_max_idle_conns")
	assert.False(t, exists, "POSTGRESQL_MAX_IDLE_CONNS must not exist — it was removed in the pgx/v5 migration")
}
