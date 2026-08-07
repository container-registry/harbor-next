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

package main

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestBuildDatabaseCfg_MinConnsUnset(t *testing.T) {
	resetViper()

	cfg := buildDatabaseCfg()
	assert.Equal(t, int32(0), cfg.PostGreSQL.MinConns)
	assert.False(t, cfg.PostGreSQL.MinConnsSet)
}

func TestBuildDatabaseCfg_MinConnsExplicitZero(t *testing.T) {
	t.Setenv("HARBOR_DATABASE_MIN_CONNS", "0")
	resetViper()

	cfg := buildDatabaseCfg()
	assert.Equal(t, int32(0), cfg.PostGreSQL.MinConns)
	assert.True(t, cfg.PostGreSQL.MinConnsSet)
}

func resetViper() {
	viper.Reset()
	viper.SetEnvPrefix("harbor")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
}
