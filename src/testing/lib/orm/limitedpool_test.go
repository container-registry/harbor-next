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

package orm

import (
	"context"
	"testing"
	"time"
)

// A timed-out RunsWithin must not leave the pool's only connection held, or
// every later test in the binary that touches the pool hangs.
func TestRunsWithinReleasesThePoolOnTimeout(t *testing.T) {
	db := RegisterLimitedPool(t, 1)

	completed := RunsWithin(200*time.Millisecond, func() {
		held, err := db.Conn(context.Background())
		if err != nil {
			return
		}
		defer held.Close()
		second, err := db.Conn(context.Background())
		if err != nil {
			return
		}
		_ = second.Close()
	})
	if completed {
		t.Fatal("a second acquire on a pool of one must not complete")
	}

	if inUse := db.Stats().InUse; inUse != 0 {
		t.Fatalf("%d connection(s) still held after RunsWithin returned", inUse)
	}
	if max := db.Stats().MaxOpenConnections; max != 1 {
		t.Fatalf("pool cap is %d after RunsWithin returned, want 1", max)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("pool unusable after a timed-out RunsWithin: %v", err)
	}
	_ = c.Close()
}
