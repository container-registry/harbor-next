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
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	cjob "github.com/goharbor/harbor/src/common/job"
	cjobmodels "github.com/goharbor/harbor/src/common/job/models"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/task/dao"
)

type taskManagerTestSuite struct {
	suite.Suite
	mgr      *manager
	dao      *mockTaskDAO
	execDAO  *mockExecutionDAO
	jsClient *mockJobserviceClient
}

func (t *taskManagerTestSuite) SetupTest() {
	t.dao = &mockTaskDAO{}
	t.execDAO = &mockExecutionDAO{}
	t.jsClient = &mockJobserviceClient{}
	t.mgr = &manager{
		dao:      t.dao,
		execDAO:  t.execDAO,
		jsClient: t.jsClient,
	}
}

func (t *taskManagerTestSuite) TestCount() {
	t.dao.CountFunc = func(_ context.Context, _ *q.Query) (int64, error) {
		return int64(10), nil
	}
	total, err := t.mgr.Count(nil, nil)
	t.Require().Nil(err)
	t.Equal(int64(10), total)
}

func (t *taskManagerTestSuite) TestCreate() {
	// success to submit job to jobservice
	t.execDAO.GetFunc = func(_ context.Context, _ int64) (*dao.Execution, error) {
		return &dao.Execution{}, nil
	}
	t.dao.CreateFunc = func(_ context.Context, _ *dao.Task) (int64, error) {
		return int64(1), nil
	}
	t.jsClient.SubmitJobFunc = func(_ *cjobmodels.JobData) (string, error) {
		return "1", nil
	}
	t.dao.UpdateFunc = func(_ context.Context, _ *dao.Task, _ ...string) error {
		return nil
	}

	id, err := t.mgr.Create(nil, 1, &Job{}, map[string]any{"a": "b"})
	t.Require().Nil(err)
	t.Equal(int64(1), id)

	// reset mock
	t.SetupTest()

	// failed to submit job to jobservice
	t.execDAO.GetFunc = func(_ context.Context, _ int64) (*dao.Execution, error) {
		return &dao.Execution{}, nil
	}
	t.dao.CreateFunc = func(_ context.Context, _ *dao.Task) (int64, error) {
		return int64(1), nil
	}
	t.jsClient.SubmitJobFunc = func(_ *cjobmodels.JobData) (string, error) {
		return "", errors.New("error")
	}
	t.dao.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}

	id, err = t.mgr.Create(nil, 1, &Job{}, map[string]any{"a": "b"})
	t.Require().NotNil(err)
}

func (t *taskManagerTestSuite) TestStop() {
	// job not found
	t.dao.GetFunc = func(_ context.Context, _ int64) (*dao.Task, error) {
		return &dao.Task{
			ID:          1,
			ExecutionID: 1,
			Status:      job.RunningStatus.String(),
		}, nil
	}
	t.jsClient.PostActionFunc = func(_ string, _ string) error {
		return cjob.ErrJobNotFound
	}
	t.dao.UpdateFunc = func(_ context.Context, _ *dao.Task, _ ...string) error {
		return nil
	}
	t.execDAO.RefreshStatusFunc = func(_ context.Context, _ int64) (bool, string, error) {
		return true, "", nil
	}
	err := t.mgr.Stop(nil, 1)
	t.Require().Nil(err)

	// reset mock
	t.SetupTest()

	// pass
	t.dao.GetFunc = func(_ context.Context, _ int64) (*dao.Task, error) {
		return &dao.Task{
			ID:          1,
			ExecutionID: 1,
			Status:      job.RunningStatus.String(),
		}, nil
	}
	t.jsClient.PostActionFunc = func(_ string, _ string) error {
		return nil
	}
	err = t.mgr.Stop(nil, 1)
	t.Require().Nil(err)
}

func (t *taskManagerTestSuite) TestGet() {
	t.dao.GetFunc = func(_ context.Context, _ int64) (*dao.Task, error) {
		return &dao.Task{
			ID: 1,
		}, nil
	}
	task, err := t.mgr.Get(nil, 1)
	t.Require().Nil(err)
	t.Equal(int64(1), task.ID)
}

func (t *taskManagerTestSuite) TestUpdateExtraAttrs() {
	t.dao.UpdateFunc = func(_ context.Context, _ *dao.Task, _ ...string) error {
		return nil
	}
	err := t.mgr.UpdateExtraAttrs(nil, 1, map[string]any{})
	t.Require().Nil(err)
}

func (t *taskManagerTestSuite) TestList() {
	t.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*dao.Task, error) {
		return []*dao.Task{
			{
				ID: 1,
			},
		}, nil
	}
	tasks, err := t.mgr.List(nil, nil)
	t.Require().Nil(err)
	t.Require().Len(tasks, 1)
	t.Equal(int64(1), tasks[0].ID)
}

func (t *taskManagerTestSuite) TestListScanTasksByReportUUID() {
	t.dao.ListScanTasksByReportUUIDFunc = func(_ context.Context, _ string) ([]*dao.Task, error) {
		return []*dao.Task{
			{
				ID: 1,
			},
		}, nil
	}
	tasks, err := t.mgr.ListScanTasksByReportUUID(nil, "uuid")
	t.Require().Nil(err)
	t.Require().Len(tasks, 1)
	t.Equal(int64(1), tasks[0].ID)
}

func (t *taskManagerTestSuite) TestRetrieveStatusFromTask() {
	t.dao.ListScanTasksByReportUUIDFunc = func(_ context.Context, _ string) ([]*dao.Task, error) {
		return []*dao.Task{
			{
				ID:     1,
				Status: "Success",
			},
		}, nil
	}
	status := t.mgr.RetrieveStatusFromTask(nil, "uuid")
	t.Equal("Success", status)
}

func TestTaskManagerTestSuite(t *testing.T) {
	suite.Run(t, &taskManagerTestSuite{})
}
