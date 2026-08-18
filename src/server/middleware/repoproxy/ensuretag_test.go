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

package repoproxy

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/lib"
)

type fakeTagEnsurer struct{ calls int32 }

func (f *fakeTagEnsurer) EnsureTag(_ context.Context, _ lib.ArtifactInfo, _ string) error {
	atomic.AddInt32(&f.calls, 1)
	return errors.New("tag not associated yet")
}

// TestEnsureTagWithRetryStopsOnContextCancel verifies the retry loop is
// context-aware: on a cancelled/expired context it returns promptly instead of
// sleeping the full interval, and does not keep calling EnsureTag.
func TestEnsureTagWithRetryStopsOnContextCancel(t *testing.T) {
	f := &fakeTagEnsurer{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	done := make(chan struct{})
	start := time.Now()
	go func() {
		ensureTagWithRetry(ctx, f, lib.ArtifactInfo{ProjectName: "proj", Repository: "proj/busybox", Tag: "latest"}, "sha256:abc")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ensureTagWithRetry did not return promptly on a cancelled context")
	}
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, int32(0), atomic.LoadInt32(&f.calls), "EnsureTag must not be called once the context is done")
}
