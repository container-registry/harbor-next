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

package oidc

import (
	"context"
	"testing"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/orm"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

const deadlockTimeout = 10 * time.Second

// Every OIDC-authenticated write, docker push included, onboards the token's
// groups inside the request transaction. Doing it on ORMs of its own takes extra
// pool connections and wedges core under concurrent writes (#850).
func TestPopulateGroupsDBRunsOnTheRequestConnection(t *testing.T) {
	config.InitWithSettings(map[string]any{})
	ormtesting.RegisterLimitedPool(t, 1)

	ctx := orm.NewContext(context.Background(), beegoorm.NewOrm())
	var txErr error
	completed := ormtesting.RunsWithin(deadlockTimeout, func() {
		txErr = orm.WithTransaction(func(txCtx context.Context) error {
			_, err := populateGroupsDB(txCtx, []string{"group-b", "group-a"})
			return err
		})(ctx)
	})

	if !completed {
		t.Fatalf("populateGroupsDB did not return within %s: it asked the pool for a "+
			"second connection while the request transaction still held the first", deadlockTimeout)
	}
	if txErr != nil {
		t.Fatalf("populateGroupsDB failed on the request connection: %v", txErr)
	}
}
