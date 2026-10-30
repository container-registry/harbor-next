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

package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common/models"
)

func TestConnStringPrefersURL(t *testing.T) {
	cfg := &models.PostGreSQL{Host: "db", Port: 5432, Username: "u", Password: "p", Database: "registry"}
	got, err := connString(cfg)
	assert.NoError(t, err)
	assert.Equal(t, "postgres://u:p@db:5432/registry?sslmode=disable", got)

	cfg.URL = "postgres://iam@rds.example:5432/registry?sslmode=require&token=x"
	got, err = connString(cfg)
	assert.NoError(t, err)
	assert.Equal(t, cfg.URL, got)
}
