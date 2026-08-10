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

	"github.com/stretchr/testify/assert"
)

// TestAfterCommit_NoTransaction covers the non-tx path: without an
// enclosing WithTransaction scope, AfterCommit must run the callback
// immediately on the caller's goroutine so no cleanup is ever lost.
func TestAfterCommit_NoTransaction(t *testing.T) {
	ran := false
	AfterCommit(context.Background(), func() { ran = true })
	assert.True(t, ran, "AfterCommit must run fn immediately when no tx hooks sink is on the ctx")
}

// TestAfterCommit_NilFn is a no-op and must not panic.
func TestAfterCommit_NilFn(t *testing.T) {
	assert.NotPanics(t, func() {
		AfterCommit(context.Background(), nil)
	})
}

// TestAfterCommit_RecoversPanic verifies hook panics are contained so
// one broken hook cannot take out an entire commit path.
func TestAfterCommit_RecoversPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		AfterCommit(context.Background(), func() { panic("boom") })
	})
}

// TestAfterCommit_QueuesWhenHooksPresent asserts that when a hooks sink
// is attached to the context, AfterCommit queues the callback rather
// than running it inline. This is the in-tx path; WithTransaction tests
// live in lib/orm/test for the real-DB commit/rollback semantics.
func TestAfterCommit_QueuesWhenHooksPresent(t *testing.T) {
	h := &txHooks{}
	ctx := context.WithValue(context.Background(), hooksKey{}, h)

	ran := false
	AfterCommit(ctx, func() { ran = true })

	assert.False(t, ran, "hooks must not fire before commit")

	cbs := h.drain()
	assert.Len(t, cbs, 1)

	cbs[0]()
	assert.True(t, ran)
}

// TestTxHooks_TruncateToCheckpoint covers the savepoint-scoping primitive
// used by WithTransaction: a scope records a checkpoint on entry and, if it
// rolls back, drops exactly the callbacks queued after that point.
func TestTxHooks_TruncateToCheckpoint(t *testing.T) {
	h := &txHooks{}

	var fired []string
	queue := func(name string) { h.add(func() { fired = append(fired, name) }) }

	queue("outer-before")
	checkpoint := h.mark()
	assert.Equal(t, 1, checkpoint)

	queue("inner-1")
	queue("inner-2")
	h.truncate(checkpoint)

	queue("outer-after")

	for _, fn := range h.drain() {
		fn()
	}
	assert.Equal(t, []string{"outer-before", "outer-after"}, fired)
}

// TestTxHooks_TruncateNoop asserts truncate is a no-op for a scope that
// registered nothing, and that an out-of-range checkpoint cannot panic or
// silently drop callbacks belonging to an enclosing scope.
func TestTxHooks_TruncateNoop(t *testing.T) {
	h := &txHooks{}
	h.add(func() {})
	h.add(func() {})

	h.truncate(h.mark()) // scope registered nothing
	assert.Len(t, h.afterCommit, 2)

	assert.NotPanics(t, func() { h.truncate(99) })
	assert.Len(t, h.afterCommit, 2)

	assert.NotPanics(t, func() { h.truncate(-1) })
	assert.Len(t, h.afterCommit, 2)
}
