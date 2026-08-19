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

package dao

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common/models"
)

func setStaticAWSTestCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
}

func clearAWSTestCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
}

func requireRDSToken(t *testing.T, token, endpoint, region, username string) {
	t.Helper()
	require.Truef(t, strings.HasPrefix(token, endpoint+"?"), "token endpoint prefix = %q, want %q", token, endpoint+"?")

	values, err := url.ParseQuery(strings.TrimPrefix(token, endpoint+"?"))
	require.NoError(t, err)
	assert.Equal(t, "connect", values.Get("Action"))
	assert.Equal(t, username, values.Get("DBUser"))
	assert.Equal(t, "AWS4-HMAC-SHA256", values.Get("X-Amz-Algorithm"))
	assert.Contains(t, values.Get("X-Amz-Credential"), "/"+region+"/rds-db/aws4_request")
	assert.NotEmpty(t, values.Get("X-Amz-Signature"))
}

func iamTestConfig() *models.PostGreSQL {
	return &models.PostGreSQL{
		Host:       "mydb.rds.amazonaws.com",
		Port:       5432,
		Username:   "harbor_iam_user",
		AWSRegion:  "us-east-1",
		UseIAMAuth: true,
	}
}

func TestResolveAWSRegion(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		envRegion  string
		envDefault string
		want       string
	}{
		{
			name:       "explicit config takes precedence",
			configured: "eu-west-1",
			envRegion:  "us-east-1",
			envDefault: "ap-south-1",
			want:       "eu-west-1",
		},
		{
			name:       "falls back to AWS_REGION",
			configured: "",
			envRegion:  "us-east-1",
			envDefault: "ap-south-1",
			want:       "us-east-1",
		},
		{
			name:       "falls back to AWS_DEFAULT_REGION",
			configured: "",
			envRegion:  "",
			envDefault: "ap-south-1",
			want:       "ap-south-1",
		},
		{
			name:       "returns empty when nothing set",
			configured: "",
			envRegion:  "",
			envDefault: "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", tt.envRegion)
			t.Setenv("AWS_DEFAULT_REGION", tt.envDefault)

			got := resolveAWSRegion(tt.configured)
			if got != tt.want {
				t.Errorf("resolveAWSRegion(%q) = %q, want %q", tt.configured, got, tt.want)
			}
		})
	}
}

func TestIamAuthOption_MissingRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	clearAWSTestCredentials(t)

	cfg := &models.PostGreSQL{
		Host:       "localhost",
		Port:       5432,
		Username:   "test",
		AWSRegion:  "",
		UseIAMAuth: true,
	}

	_, err := iamAuthOption(t.Context(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IAM auth: AWS region is required")
}

func TestIamAuthOption_CapLifetime(t *testing.T) {
	setStaticAWSTestCredentials(t)
	cfg := iamTestConfig()

	opt, err := iamAuthOption(t.Context(), cfg)
	require.NoError(t, err)

	poolCfg := &pgxpool.Config{}
	poolCfg.MaxConnLifetime = 30 * time.Minute
	opt(poolCfg)

	assert.Equal(t, iamAuthMaxConnLifetime, poolCfg.MaxConnLifetime)
}

func TestIamAuthOption_PreserveShorterLifetime(t *testing.T) {
	setStaticAWSTestCredentials(t)
	cfg := iamTestConfig()

	opt, err := iamAuthOption(t.Context(), cfg)
	require.NoError(t, err)

	shorter := 5 * time.Minute
	poolCfg := &pgxpool.Config{}
	poolCfg.MaxConnLifetime = shorter
	opt(poolCfg)

	assert.Equal(t, shorter, poolCfg.MaxConnLifetime)
}

func TestIamAuthOption_InstallsBeforeConnect(t *testing.T) {
	setStaticAWSTestCredentials(t)
	cfg := iamTestConfig()

	opt, err := iamAuthOption(t.Context(), cfg)
	require.NoError(t, err)

	poolCfg := &pgxpool.Config{}
	opt(poolCfg)

	assert.NotNil(t, poolCfg.BeforeConnect)
	assert.Equal(t, iamAuthMaxConnLifetime, poolCfg.MaxConnLifetime)
}

func TestIamAuthOption_CapZeroLifetime(t *testing.T) {
	setStaticAWSTestCredentials(t)
	cfg := iamTestConfig()

	opt, err := iamAuthOption(t.Context(), cfg)
	require.NoError(t, err)

	poolCfg := &pgxpool.Config{}
	opt(poolCfg)

	assert.Equal(t, iamAuthMaxConnLifetime, poolCfg.MaxConnLifetime)
}

func TestIamAuthOption_BeforeConnectGeneratesToken(t *testing.T) {
	setStaticAWSTestCredentials(t)
	cfg := iamTestConfig()

	opt, err := iamAuthOption(t.Context(), cfg)
	require.NoError(t, err)

	poolCfg := &pgxpool.Config{}
	opt(poolCfg)
	require.NotNil(t, poolCfg.BeforeConnect)

	connCfg, err := pgx.ParseConfig("postgres://harbor_iam_user:static-password@mydb.rds.amazonaws.com:5432/registry?sslmode=require")
	require.NoError(t, err)

	err = poolCfg.BeforeConnect(t.Context(), connCfg)
	require.NoError(t, err)
	assert.NotEqual(t, "static-password", connCfg.Password)
	assert.NotEmpty(t, connCfg.Password)
	requireRDSToken(t, connCfg.Password, "mydb.rds.amazonaws.com:5432", "us-east-1", "harbor_iam_user")
}

func TestIamAuthOption_BeforeConnectMissingCredentials(t *testing.T) {
	clearAWSTestCredentials(t)
	cfg := iamTestConfig()

	opt, err := iamAuthOption(t.Context(), cfg)
	require.NoError(t, err)

	poolCfg := &pgxpool.Config{}
	opt(poolCfg)
	require.NotNil(t, poolCfg.BeforeConnect)

	connCfg, err := pgx.ParseConfig("postgres://harbor_iam_user:static-password@mydb.rds.amazonaws.com:5432/registry?sslmode=require")
	require.NoError(t, err)

	err = poolCfg.BeforeConnect(t.Context(), connCfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IAM auth: build token")
}

func TestNewPGSQL_IAMForceSSL(t *testing.T) {
	t.Setenv("HARBOR_ENABLE_COMMERCIAL_AWS_RDS_IAM_AUTH", "true")
	cfg := &models.PostGreSQL{
		Host:       "localhost",
		Port:       5432,
		Username:   "test",
		SSLMode:    "disable",
		UseIAMAuth: true,
	}

	db := NewPGSQL(cfg)
	p := db.(*pgsql)
	if p.cfg.SSLMode != "require" {
		t.Errorf("SSLMode = %q, want %q when IAM auth is enabled", p.cfg.SSLMode, "require")
	}
}

func TestNewPGSQL_IAMForceEmptySSL(t *testing.T) {
	t.Setenv("HARBOR_ENABLE_COMMERCIAL_AWS_RDS_IAM_AUTH", "true")
	cfg := &models.PostGreSQL{
		Host:       "localhost",
		Port:       5432,
		Username:   "test",
		SSLMode:    "",
		UseIAMAuth: true,
	}

	db := NewPGSQL(cfg)
	p := db.(*pgsql)
	assert.Equal(t, "require", p.cfg.SSLMode)
}

func TestNewPGSQL_IAMPreserveSSL(t *testing.T) {
	t.Setenv("HARBOR_ENABLE_COMMERCIAL_AWS_RDS_IAM_AUTH", "true")
	cfg := &models.PostGreSQL{
		Host:       "localhost",
		Port:       5432,
		Username:   "test",
		SSLMode:    "verify-full",
		UseIAMAuth: true,
	}

	db := NewPGSQL(cfg)
	p := db.(*pgsql)
	if p.cfg.SSLMode != "verify-full" {
		t.Errorf("SSLMode = %q, want %q (should not downgrade)", p.cfg.SSLMode, "verify-full")
	}
}

func TestNewPGSQL_NoIAMNoSSLForce(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:       "localhost",
		Port:       5432,
		Username:   "test",
		SSLMode:    "disable",
		UseIAMAuth: false,
	}

	db := NewPGSQL(cfg)
	p := db.(*pgsql)
	if p.cfg.SSLMode != "disable" {
		t.Errorf("SSLMode = %q, want %q when IAM auth is disabled", p.cfg.SSLMode, "disable")
	}
}

func TestIamMigrationToken_GeneratesToken(t *testing.T) {
	setStaticAWSTestCredentials(t)
	cfg := iamTestConfig()

	token, err := iamMigrationToken(t.Context(), cfg)
	require.NoError(t, err)
	requireRDSToken(t, token, "mydb.rds.amazonaws.com:5432", "us-east-1", "harbor_iam_user")
}

func TestIamMigrationToken_MissingRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	clearAWSTestCredentials(t)
	cfg := iamTestConfig()
	cfg.AWSRegion = ""

	_, err := iamMigrationToken(t.Context(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IAM auth: AWS region is required for migration")
}
