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
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/task/dao"
)

// hookExecDAO is a function-field mock for dao.ExecutionDAO used in hook tests.
type hookExecDAO struct {
	GetFunc              func(ctx context.Context, id int64) (*dao.Execution, error)
	AsyncRefreshStatusFunc func(ctx context.Context, id int64, vendor string) error
}

func (h *hookExecDAO) Count(context.Context, *q.Query) (int64, error)              { return 0, nil }
func (h *hookExecDAO) List(context.Context, *q.Query) ([]*dao.Execution, error)     { return nil, nil }
func (h *hookExecDAO) Get(ctx context.Context, id int64) (*dao.Execution, error)    { return h.GetFunc(ctx, id) }
func (h *hookExecDAO) Create(context.Context, *dao.Execution) (int64, error)        { return 0, nil }
func (h *hookExecDAO) Update(context.Context, *dao.Execution, ...string) error      { return nil }
func (h *hookExecDAO) Delete(context.Context, int64) error                          { return nil }
func (h *hookExecDAO) GetMetrics(context.Context, int64) (*dao.Metrics, error)      { return nil, nil }
func (h *hookExecDAO) RefreshStatus(context.Context, int64) (bool, string, error)   { return false, "", nil }
func (h *hookExecDAO) AsyncRefreshStatus(ctx context.Context, id int64, vendor string) error {
	return h.AsyncRefreshStatusFunc(ctx, id, vendor)
}

// hookTaskDAO is a function-field mock for dao.TaskDAO used in hook tests.
type hookTaskDAO struct {
	ListFunc         func(ctx context.Context, query *q.Query) ([]*dao.Task, error)
	UpdateStatusFunc func(ctx context.Context, id int64, status string, statusRevision int64) error
}

func (h *hookTaskDAO) Count(context.Context, *q.Query) (int64, error)                          { return 0, nil }
func (h *hookTaskDAO) List(ctx context.Context, query *q.Query) ([]*dao.Task, error)            { return h.ListFunc(ctx, query) }
func (h *hookTaskDAO) Get(context.Context, int64) (*dao.Task, error)                            { return nil, nil }
func (h *hookTaskDAO) Create(context.Context, *dao.Task) (int64, error)                         { return 0, nil }
func (h *hookTaskDAO) Update(context.Context, *dao.Task, ...string) error                       { return nil }
func (h *hookTaskDAO) UpdateStatus(ctx context.Context, id int64, status string, statusRevision int64) error {
	return h.UpdateStatusFunc(ctx, id, status, statusRevision)
}
func (h *hookTaskDAO) Delete(context.Context, int64) error                                      { return nil }
func (h *hookTaskDAO) ListStatusCount(context.Context, int64) ([]*dao.StatusCount, error)       { return nil, nil }
func (h *hookTaskDAO) GetMaxEndTime(context.Context, int64) (time.Time, error)                  { return time.Time{}, nil }
func (h *hookTaskDAO) UpdateStatusInBatch(context.Context, []string, string, int) error         { return nil }
func (h *hookTaskDAO) ExecutionIDsByVendorAndStatus(context.Context, string, string) ([]int64, error) {
	return nil, nil
}
func (h *hookTaskDAO) ListScanTasksByReportUUID(context.Context, string) ([]*dao.Task, error) {
	return nil, nil
}

type hookHandlerTestSuite struct {
	suite.Suite
	handler *HookHandler
	execDAO *hookExecDAO
	taskDAO *hookTaskDAO
}

func (h *hookHandlerTestSuite) SetupTest() {
	h.execDAO = &hookExecDAO{}
	h.taskDAO = &hookTaskDAO{}
	h.handler = &HookHandler{
		taskDAO:      h.taskDAO,
		executionDAO: h.execDAO,
	}
}

func (h *hookHandlerTestSuite) TestHandle() {
	// handle check in data
	checkInProcessorRegistry["test"] = func(ctx context.Context, task *Task, sc *job.StatusChange) (err error) { return nil }
	defer delete(checkInProcessorRegistry, "test")
	h.taskDAO.ListFunc = func(_ context.Context, _ *q.Query) ([]*dao.Task, error) {
		return []*dao.Task{
			{
				ID:          1,
				ExecutionID: 1,
			},
		}, nil
	}
	h.execDAO.GetFunc = func(_ context.Context, _ int64) (*dao.Execution, error) {
		return &dao.Execution{
			ID:         1,
			VendorType: "test",
		}, nil
	}
	sc := &job.StatusChange{
		CheckIn:  "data",
		Metadata: &job.StatsInfo{},
	}
	err := h.handler.Handle(nil, sc)
	h.Require().Nil(err)

	// reset mock
	h.SetupTest()

	// handle status changing
	h.taskDAO.ListFunc = func(_ context.Context, _ *q.Query) ([]*dao.Task, error) {
		return []*dao.Task{
			{
				ID:          1,
				ExecutionID: 1,
			},
		}, nil
	}
	h.taskDAO.UpdateStatusFunc = func(_ context.Context, _ int64, _ string, _ int64) error {
		return nil
	}
	h.execDAO.GetFunc = func(_ context.Context, _ int64) (*dao.Execution, error) {
		return &dao.Execution{
			ID:         1,
			VendorType: "test",
		}, nil
	}

	// test update status non-immediately when receive the hook
	{
		h.execDAO.AsyncRefreshStatusFunc = func(_ context.Context, _ int64, _ string) error {
			return nil
		}
		sc = &job.StatusChange{
			Status: job.SuccessStatus.String(),
			Metadata: &job.StatsInfo{
				Revision: time.Now().Unix(),
			},
		}
		err = h.handler.Handle(nil, sc)
		h.Require().Nil(err)
	}
}

func TestHookHandlerTestSuite(t *testing.T) {
	suite.Run(t, &hookHandlerTestSuite{})
}
