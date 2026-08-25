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
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/goharbor/harbor/src/lib/log"
	libredis "github.com/goharbor/harbor/src/lib/redis"
	"github.com/goharbor/harbor/src/pkg/quota/types"
)

// A detached reserve (see Request) commits before the artifact/blob rows it
// pays for; a concurrent Refresh recalculating from those tables would
// CAS-overwrite it. This ledger closes the gap: reserves are recorded before
// they commit, Refresh adds the ledger sum, entries are removed on tx commit
// or rollback. Deadlines reclaim crash orphans, erring toward over-counting.

const (
	// must outlive the longest request between reserve and tx commit
	// (manifest PUT incl. abstractor fetches)
	inflightEntryTTL = 10 * time.Minute
)

type inflightEntry struct {
	Resources types.ResourceList `json:"r"`
	Deadline  int64              `json:"exp"`
}

func inflightKey(reference, referenceID string) string {
	// same namespace convention as updateUsageByRedis
	return fmt.Sprintf("cache:quota:inflight:%s:%s", reference, referenceID)
}

func (c *controller) trackInflight(ctx context.Context, reference, referenceID string, resources types.ResourceList) (string, error) {
	client, err := libredis.GetHarborClient()
	if err != nil {
		return "", err
	}

	token := uuid.NewString()
	payload, err := json.Marshal(inflightEntry{
		Resources: resources,
		Deadline:  time.Now().Add(inflightEntryTTL).Unix(),
	})
	if err != nil {
		return "", err
	}

	key := inflightKey(reference, referenceID)
	if err := client.HSet(ctx, key, token, payload).Err(); err != nil {
		return "", err
	}
	// key-level TTL alone is not enough: a busy project keeps refreshing it,
	// so expired fields are additionally reaped in inflightSum
	client.Expire(ctx, key, 2*inflightEntryTTL)

	return token, nil
}

func (c *controller) untrackInflight(ctx context.Context, reference, referenceID, token string) {
	client, err := libredis.GetHarborClient()
	if err != nil {
		log.G(ctx).Warningf("quota inflight: failed to get redis client to untrack %s/%s: %v", reference, referenceID, err)
		return
	}
	if err := client.HDel(context.WithoutCancel(ctx), inflightKey(reference, referenceID), token).Err(); err != nil {
		log.G(ctx).Warningf("quota inflight: failed to untrack %s/%s (entry expires at its deadline): %v", reference, referenceID, err)
	}
}

// Errors degrade to "no adjustment": Refresh then behaves as before the ledger.
func (c *controller) inflightSum(ctx context.Context, reference, referenceID string) types.ResourceList {
	client, err := libredis.GetHarborClient()
	if err != nil {
		log.G(ctx).Warningf("quota inflight: failed to get redis client for %s/%s: %v", reference, referenceID, err)
		return nil
	}

	key := inflightKey(reference, referenceID)
	entries, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		log.G(ctx).Warningf("quota inflight: failed to read ledger for %s/%s: %v", reference, referenceID, err)
		return nil
	}

	now := time.Now().Unix()
	var sum types.ResourceList
	var expired []string
	for token, raw := range entries {
		var entry inflightEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil || entry.Deadline <= now {
			expired = append(expired, token)
			continue
		}
		sum = types.Add(sum, entry.Resources)
	}
	if len(expired) > 0 {
		client.HDel(context.WithoutCancel(ctx), key, expired...)
	}

	return sum
}
