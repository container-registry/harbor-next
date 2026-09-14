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

package user

import (
	"context"
	"net/http"
	"testing"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"

	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/lib/orm"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

const deadlockTimeout = 10 * time.Second

// TestPreCheckDeleteRunsOnTheRequestConnection reproduces harbor-next #850 /
// goharbor/harbor#23879: log.Middleware resolves the user name for
// DELETE /api/v2.0/users/{id} inside transaction.Middleware's transaction, and
// resolving it on an ORM of its own needs a second pool connection. In
// production that wedges core once POSTGRESQL_MAX_OPEN_CONNS deletes are in
// flight; with the single-connection pool below one request is enough.
func TestPreCheckDeleteRunsOnTheRequestConnection(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 1)

	resolver, ok := commonevent.Resolvers()[urlPattern]
	if !ok {
		t.Fatalf("no resolver registered for %s", urlPattern)
	}

	ctx := orm.NewContext(context.Background(), beegoorm.NewOrm())
	completed := ormtesting.RunsWithin(deadlockTimeout, func() {
		_ = orm.WithTransaction(func(txCtx context.Context) error {
			resolver.PreCheck(txCtx, "/api/v2.0/users/1", http.MethodDelete)
			return nil
		})(ctx)
	})

	if !completed {
		t.Fatalf("PreCheck did not return within %s: it asked the pool for a second "+
			"connection while the request transaction still held the first", deadlockTimeout)
	}
}
