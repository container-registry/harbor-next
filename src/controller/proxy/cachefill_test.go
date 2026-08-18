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

package proxy

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoCacheFillBoundsConcurrency verifies that background proxy-cache tasks
// are capped: once the gate is full, further tasks are skipped (not queued and
// not spawned), and a slot frees up when a running task completes.
func TestGoCacheFillBoundsConcurrency(t *testing.T) {
	saved := cacheFillSem
	t.Cleanup(func() { cacheFillSem = saved })
	cacheFillSem = make(chan struct{}, 1) // single slot for a deterministic test

	started := make(chan struct{})
	release := make(chan struct{})
	require.True(t, GoCacheFill("first", func() {
		close(started)
		<-release // hold the only slot until released
	}))
	<-started // ensure the first task is running and holding the slot

	// Gate is full: the second task must be skipped, not run.
	var ran int32
	require.False(t, GoCacheFill("second", func() { atomic.AddInt32(&ran, 1) }))
	assert.Equal(t, int32(0), atomic.LoadInt32(&ran), "task must not run while the gate is full")

	// Free the slot; a new task is accepted again.
	close(release)
	done := make(chan struct{})
	require.Eventually(t, func() bool {
		return GoCacheFill("third", func() { close(done) })
	}, 2*time.Second, 5*time.Millisecond, "a slot should free up after the running task completes")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled task did not run")
	}
}
