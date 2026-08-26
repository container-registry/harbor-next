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

// Package dbenv exposes the PostgreSQL test-environment configuration shared by db-tagged tests.
package dbenv

import (
	"os"
	"strconv"

	"github.com/goharbor/harbor/src/common/models"
)

// PostgreSQLConfig returns a PostGreSQL config from environment.
// Accepts both the existing test convention (POSTGRESQL_USR / POSTGRESQL_PWD)
// and the Harbor config convention (POSTGRESQL_USERNAME / POSTGRESQL_PASSWORD),
// with devenv defaults as fallback.
func PostgreSQLConfig() *models.PostGreSQL {
	port := 5432
	if p := os.Getenv("POSTGRESQL_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	return &models.PostGreSQL{
		Host:     envOr("POSTGRESQL_HOST", "localhost"),
		Port:     port,
		Username: envOr("POSTGRESQL_USR", envOr("POSTGRESQL_USERNAME", "postgres")),
		Password: envOr("POSTGRESQL_PWD", envOr("POSTGRESQL_PASSWORD", "root123")),
		Database: envOr("POSTGRESQL_DATABASE", "registry"),
		SSLMode:  "disable",
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
