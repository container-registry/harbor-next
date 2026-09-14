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

	beegoorm "github.com/beego/beego/v2/client/orm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

func TestReuseContextAttachesORMWhenThereIsNoneToReuse(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)

	got := ReuseContext(context.Background())
	o, err := FromContext(got)
	require.NoError(t, err, "ReuseContext must attach an ORM when the context carries none")
	assert.NotNil(t, o)
}

func TestReuseContextAcceptsNilContext(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)

	_, err := FromContext(ReuseContext(nil))
	assert.NoError(t, err)
}

func TestReuseContextKeepsAnExistingORM(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)

	ctx := NewContext(context.Background(), &ormtesting.FakeOrmer{})
	assert.Equal(t, ctx, ReuseContext(ctx), "a context that already has an ORM must be returned unchanged")
}

// TestReuseContextKeepsALiveTransaction is the property that keeps a request to
// one pool connection (harbor-next #850).
func TestReuseContextKeepsALiveTransaction(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)

	ctx := NewContext(context.Background(), beegoorm.NewOrm())
	err := WithTransaction(func(txCtx context.Context) error {
		assert.Equal(t, txCtx, ReuseContext(txCtx), "an open transaction must be reused, not replaced")
		return nil
	})(ctx)
	require.NoError(t, err)
}

// TestReuseContextReplacesACompletedTransaction covers the audit-event case: the
// request context is captured inside the transaction and used after it commits.
func TestReuseContextReplacesACompletedTransaction(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)

	var captured context.Context
	ctx := NewContext(context.Background(), beegoorm.NewOrm())
	require.NoError(t, WithTransaction(func(txCtx context.Context) error {
		captured = txCtx
		return nil
	})(ctx))

	reused := ReuseContext(captured)
	assert.NotEqual(t, captured, reused, "a completed transaction scope must get a fresh ORM")

	o, err := FromContext(reused)
	require.NoError(t, err)
	_, isTx := o.(beegoorm.TxOrmer)
	assert.False(t, isTx, "the replacement ORM must not be the spent transaction ORM")
}

// TestReuseContextDoesNotTakeASecondConnection runs a query on the reused
// context against a pool of one, where a second ORM would never get a connection.
func TestReuseContextDoesNotTakeASecondConnection(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 1)

	ctx := NewContext(context.Background(), beegoorm.NewOrm())
	completed := ormtesting.RunsWithin(10*time.Second, func() {
		_ = WithTransaction(func(txCtx context.Context) error {
			o, err := FromContext(ReuseContext(txCtx))
			if err != nil {
				return err
			}
			_, err = o.Raw("SELECT 1").Exec()
			return err
		})(ctx)
	})

	assert.True(t, completed, "a query on the reused context must not wait for a second pool connection")
}
