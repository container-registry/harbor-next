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

	applyPoolConfig(poolCfg, cfg)

	// MaxOpenConns=0 must fall back to 25, not pgxpool's default of 4.
	assert.Equal(t, int32(DefaultMaxConns), poolCfg.MaxConns, "zero MaxOpenConns should default to 25")
	assert.Equal(t, int32(DefaultMinConns), poolCfg.MinConns)
	assert.Equal(t, DefaultMaxConnIdleTime, poolCfg.MaxConnIdleTime)
	assert.Equal(t, DefaultHealthCheckPeriod, poolCfg.HealthCheckPeriod)
	assert.Equal(t, DefaultConnectTimeout, poolCfg.ConnConfig.ConnectTimeout)
}

func TestApplyPoolConfig_MaxConns25NotPgxDefault4(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:         "localhost",
		Port:         5432,
		Username:     "test",
		Password:     "test",
		Database:     "test",
		SSLMode:      "disable",
		MaxOpenConns: 0, // explicitly zero
	}

	dsn := BuildDSN(cfg)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	applyPoolConfig(poolCfg, cfg)

	// This is the key assertion: pgxpool defaults to 4, but we must use 25.
	assert.Equal(t, int32(25), poolCfg.MaxConns, "must be 25 per user decision, not pgxpool default of 4")
}
