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

package dbpool

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common/models"
)

func TestBuildDSN_FromFields(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:     "db.example.com",
		Port:     5432,
		Username: "harbor",
		Password: "secret",
		Database: "registry",
		SSLMode:  "require",
	}

	dsn := BuildDSN(cfg)
	assert.Equal(t, "host=db.example.com port=5432 user=harbor password='secret' dbname=registry sslmode=require timezone=UTC", dsn)
}

func TestBuildDSN_FromURL(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:     "ignored",
		Port:     9999,
		Username: "ignored",
		URL:      "postgresql://user:pass@host:5432/mydb?sslmode=verify-full",
	}

	dsn := BuildDSN(cfg)
	assert.Equal(t, "postgresql://user:pass@host:5432/mydb?sslmode=verify-full", dsn)
}

func TestApplyPoolConfig_ExplicitValues(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:              "localhost",
		Port:              5432,
		Username:          "test",
		Password:          "test",
		Database:          "test",
		SSLMode:           "disable",
		MaxOpenConns:      50,
		MinConns:          5,
		ConnMaxLifetime:   30 * time.Minute,
		ConnMaxIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 2 * time.Minute,
		ConnectTimeout:    15 * time.Second,
	}

	dsn := BuildDSN(cfg)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	applyPoolConfig(poolCfg, cfg)

	assert.Equal(t, int32(50), poolCfg.MaxConns)
	assert.Equal(t, int32(5), poolCfg.MinConns)
	assert.Equal(t, 30*time.Minute, poolCfg.MaxConnLifetime)
	assert.Equal(t, 5*time.Minute, poolCfg.MaxConnIdleTime)
	assert.Equal(t, 2*time.Minute, poolCfg.HealthCheckPeriod)
	assert.Equal(t, 15*time.Second, poolCfg.ConnConfig.ConnectTimeout)
	assert.Equal(t, pgx.QueryExecModeSimpleProtocol, poolCfg.ConnConfig.DefaultQueryExecMode)
}

func TestApplyPoolConfig_ZeroFallbacks(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:     "localhost",
		Port:     5432,
		Username: "test",
		Password: "test",
		Database: "test",
		SSLMode:  "disable",
		// All pool fields are zero values.
	}

	dsn := BuildDSN(cfg)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	pgxDefault := poolCfg.MaxConns // pgxpool sets max(4, NumCPU) during ParseConfig

	applyPoolConfig(poolCfg, cfg)

	assert.Equal(t, pgxDefault, poolCfg.MaxConns, "zero MaxOpenConns should keep pgxpool default")
	assert.Equal(t, int32(DefaultMinConns), poolCfg.MinConns)
	assert.Equal(t, DefaultMaxConnIdleTime, poolCfg.MaxConnIdleTime)
	assert.Equal(t, DefaultHealthCheckPeriod, poolCfg.HealthCheckPeriod)
	assert.Equal(t, DefaultConnectTimeout, poolCfg.ConnConfig.ConnectTimeout)
}

func TestApplyPoolConfig_ZeroMaxConnsKeepsPgxDefault(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:         "localhost",
		Port:         5432,
		Username:     "test",
		Password:     "test",
		Database:     "test",
		SSLMode:      "disable",
		MaxOpenConns: 0,
	}

	dsn := BuildDSN(cfg)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	pgxDefault := poolCfg.MaxConns // max(4, NumCPU)

	applyPoolConfig(poolCfg, cfg)

	assert.Equal(t, pgxDefault, poolCfg.MaxConns, "zero MaxOpenConns must not override pgxpool default")
}

func TestBuildDSN_FieldOrdering(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host: "h", Port: 5432, Username: "u", Password: "p", Database: "d", SSLMode: "disable",
	}
	dsn := BuildDSN(cfg)
	// Pin the exact format. If the order changes, libpq-compatible clients may break.
	assert.Equal(t, "host=h port=5432 user=u password='p' dbname=d sslmode=disable timezone=UTC", dsn)
}

func TestBuildDSN_URLTakesPrecedence(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:     "should-be-ignored",
		Port:     9999,
		Username: "ignored",
		Password: "ignored",
		Database: "ignored",
		SSLMode:  "ignored",
		URL:      "host=real port=5432 user=real dbname=real sslmode=disable",
	}
	dsn := BuildDSN(cfg)
	assert.Equal(t, cfg.URL, dsn, "URL field must override all other fields")
	assert.NotContains(t, dsn, "ignored")
}

func TestApplyPoolConfig_SimpleProtocolAlwaysSet(t *testing.T) {
	// If someone removes the SimpleProtocol line, Beego ORM breaks silently
	// with prepared statement cache errors under concurrent use.
	cfg := &models.PostGreSQL{
		Host: "h", Port: 5432, Username: "u", Password: "p", Database: "d", SSLMode: "disable",
	}
	poolCfg, err := pgxpool.ParseConfig(BuildDSN(cfg))
	require.NoError(t, err)

	applyPoolConfig(poolCfg, cfg)

	assert.Equal(t, pgx.QueryExecModeSimpleProtocol, poolCfg.ConnConfig.DefaultQueryExecMode,
		"SimpleProtocol must always be set — Beego ORM relies on it")
}

func TestApplyPoolConfig_NegativeValuesIgnored(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:              "h",
		Port:              5432,
		Username:          "u",
		Password:          "p",
		Database:          "d",
		SSLMode:           "disable",
		MaxOpenConns:      -1,
		MinConns:          -5,
		ConnMaxLifetime:   -1 * time.Minute,
		ConnMaxIdleTime:   -1 * time.Minute,
		HealthCheckPeriod: -1 * time.Minute,
		ConnectTimeout:    -1 * time.Second,
	}
	poolCfg, err := pgxpool.ParseConfig(BuildDSN(cfg))
	require.NoError(t, err)

	pgxDefaultMax := poolCfg.MaxConns

	applyPoolConfig(poolCfg, cfg)

	// Negative MaxOpenConns (-1 < 0) should not override pgxpool default
	assert.Equal(t, pgxDefaultMax, poolCfg.MaxConns, "negative MaxOpenConns must not set MaxConns")
	// Negative MinConns should fall back to default
	assert.Equal(t, int32(DefaultMinConns), poolCfg.MinConns, "negative MinConns must fall back to default")
	// Negative durations should fall back to defaults
	assert.Equal(t, DefaultMaxConnIdleTime, poolCfg.MaxConnIdleTime)
	assert.Equal(t, DefaultHealthCheckPeriod, poolCfg.HealthCheckPeriod)
	assert.Equal(t, DefaultConnectTimeout, poolCfg.ConnConfig.ConnectTimeout)
}

func TestBuildDSN_PasswordWithSpecialChars(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     string
	}{
		{"single quote", "it's", `password='it\'s'`},
		{"backslash", `pass\word`, `password='pass\\word'`},
		{"both", `it\'s`, `password='it\\\'s'`},
		{"spaces", "p a s s", `password='p a s s'`},
		{"empty", "", `password=''`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &models.PostGreSQL{
				Host: "h", Port: 5432, Username: "u",
				Password: tt.password, Database: "d", SSLMode: "disable",
			}
			dsn := BuildDSN(cfg)
			assert.Contains(t, dsn, tt.want)
		})
	}
}
