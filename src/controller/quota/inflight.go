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

// A reserve that committed on its own short transaction (see Request) is
// visible in quota_usage before the artifact/blob rows of the request it
// belongs to are committed. A concurrent Refresh recalculating usage from
// those tables would not see the in-flight rows and would CAS-overwrite the
// reservation. The in-flight ledger closes that gap: every detached reserve
// is recorded here first, and Refresh adds the ledger sum on top of the
// recalculated usage. Entries are removed once the owning transaction has
// committed (the rows are then counted by the recalculation itself) or after
// the reservation has been rolled back; the per-entry deadline reclaims
// entries orphaned by a crash, erring on the side of over-counting until it
// expires.

const (
	// inflightEntryTTL must outlive the longest request that can sit between
	// a committed reserve and its transaction commit (manifest PUT incl.
	// abstractor fetches). Key-level TTL doubles it as idle-project GC.
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

// trackInflight records resources in the ledger and returns the token to
// remove them with. A failure only degrades Refresh accuracy back to the
// pre-ledger behavior, so it is logged and the reservation proceeds.
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
	// best effort: bounds the key when a busy project never drains to zero entries
	client.Expire(ctx, key, 2*inflightEntryTTL)

	return token, nil
}

// untrackInflight removes one ledger entry. Runs after the owning transaction
// committed or after the reservation was rolled back, so it must not be tied
// to the request's cancellation.
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

// inflightSum returns the total of live ledger entries for the reference.
// Expired entries are skipped and opportunistically deleted. Any error means
// "no adjustment" so Refresh still converges the way it did before the ledger.
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
