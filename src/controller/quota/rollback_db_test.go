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

//go:build db

package quota

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/quota"
	"github.com/goharbor/harbor/src/pkg/quota/types"
	htesting "github.com/goharbor/harbor/src/testing"
)

type RollbackTestSuite struct {
	htesting.Suite

	ctl       *controller
	reference string
}

func (suite *RollbackTestSuite) SetupSuite() {
	suite.Suite.SetupSuite()
	suite.Suite.ClearSQLs = []string{
		"DELETE FROM quota WHERE id > 1",
		"DELETE FROM quota_usage WHERE id > 1",
	}
	suite.ctl = &controller{quotaMgr: quota.Mgr}
	suite.reference = "rollback-bound"
}

// lockQuotaRow holds the quota row's write lock until the returned function is
// called, on a connection of its own. Any UPDATE of that row waits for it.
func (suite *RollbackTestSuite) lockQuotaRow(id int64) func() {
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		err := orm.WithTransaction(func(ctx context.Context) error {
			o, err := orm.FromContext(ctx)
			if err != nil {
				return err
			}
			var ids []int64
			if _, err := o.Raw("SELECT id FROM quota WHERE id = ? FOR UPDATE", id).QueryRows(&ids); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})(orm.Context())
		suite.NoError(err)
	}()

	<-locked
	return func() {
		close(release)
		<-done
	}
}

// TestRollbackGivesUpOnALockedRow is the regression test for the deadline that
// was not one. updateUsageByDB runs its CAS through Beego's ORM, which takes
// no context, so before the database was told the deadline too, the UPDATE sat
// on the row lock with a pool connection in hand for as long as the lock
// holder wanted - the whole failure this timeout exists to prevent.
func (suite *RollbackTestSuite) TestRollbackGivesUpOnALockedRow() {
	ctx := orm.Context()
	hard := types.ResourceList{types.ResourceStorage: 100}
	id, err := quota.Mgr.Create(ctx, suite.reference, uuid.New().String(), hard, types.ResourceList{types.ResourceStorage: 10})
	suite.Require().NoError(err)
	q, err := quota.Mgr.Get(ctx, id)
	suite.Require().NoError(err)

	defer suite.lockQuotaRow(id)()

	// short enough that a test waiting out the lock is unmistakable, and the
	// same value reaches both the context and statement_timeout, exactly as
	// the rollback path in Request sets it up
	original := rollbackTimeout
	rollbackTimeout = time.Second
	defer func() { rollbackTimeout = original }()

	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()

	start := time.Now()
	err = suite.ctl.rollbackUsage(rbCtx, q.Reference, q.ReferenceID, types.ResourceList{types.ResourceStorage: 10}, updateQuotaProviderDB)
	elapsed := time.Since(start)

	suite.Error(err, "the rollback cannot succeed while another transaction holds the row")
	suite.Less(elapsed, 10*time.Second, "the rollback waited out the lock instead of its own deadline")
}

func TestRollbackTestSuite(t *testing.T) {
	suite.Run(t, &RollbackTestSuite{})
}
