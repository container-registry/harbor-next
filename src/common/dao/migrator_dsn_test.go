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
	"testing"

	"github.com/goharbor/harbor/src/common/models"
)

func TestMigratorDSNFromFields(t *testing.T) {
	cfg := &models.PostGreSQL{
		Host:     "db.internal",
		Port:     5432,
		Username: "postgres",
		Password: "root123",
		Database: "registry",
		SSLMode:  "disable",
	}
	got, err := migratorDSN(cfg)
	if err != nil {
		t.Fatalf("migratorDSN() error = %v", err)
	}
	want := "pgx5://postgres:root123@db.internal:5432/registry?sslmode=disable"
	if got != want {
		t.Errorf("migratorDSN() = %q, want %q", got, want)
	}
}

func TestMigratorDSNHonorsURL(t *testing.T) {
	// e.g. an RDS IAM auth token embedded as the password, per 0006-aws-rds-iam-auth
	cfg := &models.PostGreSQL{URL: "postgres://iam-user:rds-token@rds.example.com:5432/registry?sslmode=require"}
	got, err := migratorDSN(cfg)
	if err != nil {
		t.Fatalf("migratorDSN() error = %v", err)
	}
	want := "pgx5://iam-user:rds-token@rds.example.com:5432/registry?sslmode=require"
	if got != want {
		t.Errorf("migratorDSN() = %q, want %q — URL field must take precedence over individual fields", got, want)
	}
}

func TestMigratorDSNInvalidURL(t *testing.T) {
	cfg := &models.PostGreSQL{URL: "://not a url"}
	if _, err := migratorDSN(cfg); err == nil {
		t.Error("migratorDSN() with an invalid URL returned nil error")
	}
}
