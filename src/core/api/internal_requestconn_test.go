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

//go:build !db

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"
	beecontext "github.com/beego/beego/v2/server/web/context"

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/orm"
	_ "github.com/goharbor/harbor/src/pkg/config/db"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

// POST /api/internal/syncquota reads and saves configuration from the handler,
// inside the request transaction. Doing that on an ORM of its own needs a second
// pool connection (#850).
func TestSyncQuotaRunsOnTheRequestConnection(t *testing.T) {
	config.Init()
	ormtesting.RegisterLimitedPool(t, 1)

	ctx := orm.NewContext(context.Background(), beegoorm.NewOrm())
	completed := ormtesting.RunsWithin(10*time.Second, func() {
		_ = orm.WithTransaction(func(txCtx context.Context) error {
			ia := &InternalAPI{}
			ia.Ctx = beecontext.NewContext()
			ia.Ctx.Reset(httptest.NewRecorder(),
				httptest.NewRequest(http.MethodPost, "/api/internal/syncquota", nil).WithContext(txCtx))
			ia.SyncQuota()
			return nil
		})(ctx)
	})

	if !completed {
		t.Fatalf("SyncQuota did not return within 10s: it asked the pool for a second " +
			"connection while the request transaction still held the first")
	}
}
