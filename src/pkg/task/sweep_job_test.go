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

package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/jobservice/job"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
)

type sweepJobTestSuite struct {
	suite.Suite
	jobCtx   *mockjobservice.MockJobContext
	sweepMgr *mockSweepManager
}

func (suite *sweepJobTestSuite) SetupSuite() {
	suite.jobCtx = &mockjobservice.MockJobContext{}
	suite.sweepMgr = &mockSweepManager{}
}

func TestSweepJob(t *testing.T) {
	suite.Run(t, &sweepJobTestSuite{})
}

func (suite *sweepJobTestSuite) TestRun() {
	params := map[string]any{
		"execution_retain_counts": map[string]int{
			"WEBHOOK":     10,
			"REPLICATION": 20,
		},
	}
	// test stop case
	j := &SweepJob{mgr: suite.sweepMgr}
	suite.jobCtx.On("OPCommand").Return(job.StopCommand, true).Once()
	suite.sweepMgr.FixDanglingStateExecutionFunc = func(_ context.Context) error {
		return nil
	}
	err := j.Run(suite.jobCtx, params)
	suite.NoError(err, "stop job should not return error")

	// test sweep error case
	j = &SweepJob{}
	suite.jobCtx.On("OPCommand").Return(job.NilCommand, true)
	err = j.Run(suite.jobCtx, params)
	suite.Error(err, "should got error if sweep failed")

	// test normal case
	j = &SweepJob{mgr: suite.sweepMgr}
	ctx := context.TODO()
	suite.jobCtx.On("OPCommand").Return(job.NilCommand, true)
	suite.jobCtx.On("SystemContext").Return(ctx, nil)
	suite.sweepMgr.ListCandidatesFunc = func(_ context.Context, vendorType string, retainCnt int64) ([]int64, error) {
		if vendorType == "WEBHOOK" && retainCnt == 10 {
			return []int64{1}, nil
		}
		if vendorType == "REPLICATION" && retainCnt == 20 {
			return []int64{2}, nil
		}
		return nil, nil
	}
	suite.sweepMgr.CleanFunc = func(_ context.Context, _ []int64) error {
		return nil
	}
	err = j.Run(suite.jobCtx, params)
	suite.NoError(err)
}
