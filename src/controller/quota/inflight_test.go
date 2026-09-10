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

package quota

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libredis "github.com/goharbor/harbor/src/lib/redis"
	"github.com/goharbor/harbor/src/pkg/quota/types"
)

func TestInflightLedger(t *testing.T) {
	client, err := libredis.GetHarborClient()
	if err != nil {
		t.Skipf("redis not available: %v", err)
	}
	ctx := context.TODO()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable: %v", err)
	}
	c := &controller{}
	ref, refID := "project", "inflight-test"
	key := inflightKey(ref, refID)
	t.Cleanup(func() { client.Del(ctx, key) })
	client.Del(ctx, key)

	assert.Len(t, c.inflightSum(ctx, ref, refID), 0)

	res := types.ResourceList{types.ResourceStorage: 100}
	token, err := c.trackInflight(ctx, ref, refID, res)
	require.NoError(t, err)

	sum := c.inflightSum(ctx, ref, refID)
	assert.Equal(t, int64(100), sum[types.ResourceStorage])

	token2, err := c.trackInflight(ctx, ref, refID, types.ResourceList{types.ResourceStorage: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(150), c.inflightSum(ctx, ref, refID)[types.ResourceStorage])

	c.untrackInflight(ctx, ref, refID, token)
	assert.Equal(t, int64(50), c.inflightSum(ctx, ref, refID)[types.ResourceStorage])
	c.untrackInflight(ctx, ref, refID, token2)
	assert.Len(t, c.inflightSum(ctx, ref, refID), 0)

	// expired entries are ignored and reaped
	stale, err := json.Marshal(inflightEntry{
		Resources: types.ResourceList{types.ResourceStorage: 999},
		Deadline:  time.Now().Add(-time.Minute).Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, client.HSet(ctx, key, "stale-token", stale).Err())
	assert.Len(t, c.inflightSum(ctx, ref, refID), 0)
	n, err := client.HExists(ctx, key, "stale-token").Result()
	require.NoError(t, err)
	assert.False(t, n, "expired entry should have been reaped")
}
