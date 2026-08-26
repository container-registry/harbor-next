//  Copyright Project Harbor Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package inmemory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
)

func TestNewInMemoryManager(t *testing.T) {
	ctx := context.Background()
	inMemoryManager := NewInMemoryManager()
	inMemoryManager.UpdateConfig(ctx, map[string]any{
		"ldap_url":         "ldaps://ldap.vmware.com",
		"ldap_timeout":     5,
		"ldap_verify_cert": true,
	})
	assert.Equal(t, "ldaps://ldap.vmware.com", inMemoryManager.Get(ctx, "ldap_url").GetString())
	assert.Equal(t, 5, inMemoryManager.Get(ctx, "ldap_timeout").GetInt())
	assert.Equal(t, true, inMemoryManager.Get(ctx, "ldap_verify_cert").GetBool())
}

// Walks the whole core path — config store -> GetDatabaseCfg -> models.PostGreSQL —
// to prove POSTGRESQL_MIN_CONNS=0 survives as a configured 0 rather than being
// re-read as "unset" and coerced back to the default. See harbor-next#564.
func TestGetDatabaseCfg_MinConns(t *testing.T) {
	ctx := context.Background()

	mgr := NewInMemoryManager()
	require.NotNil(t, mgr.GetDatabaseCfg().PostGreSQL.MinConns)
	assert.Equal(t, int32(2), *mgr.GetDatabaseCfg().PostGreSQL.MinConns, "metadata default")

	require.NoError(t, mgr.UpdateConfig(ctx, map[string]any{common.PostGreSQLMinConns: 0}))
	minConns := mgr.GetDatabaseCfg().PostGreSQL.MinConns
	require.NotNil(t, minConns, "an explicit 0 must not read back as unset")
	assert.Equal(t, int32(0), *minConns)
}
