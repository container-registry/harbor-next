package scandataexport

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/scan/export"
	"github.com/goharbor/harbor/src/pkg/task"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
	systemartifacttesting "github.com/goharbor/harbor/src/testing/moq/pkg/systemartifact"
	testingTask "github.com/goharbor/harbor/src/testing/moq/pkg/task"
)

type ScanDataExportExecutionTestSuite struct {
	suite.Suite
	execMgr        *testingTask.ExecutionManager
	taskMgr        *testingTask.Manager
	sysArtifactMgr *systemartifacttesting.Manager
	ctl            *controller
}

func (suite *ScanDataExportExecutionTestSuite) SetupSuite() {
}

func (suite *ScanDataExportExecutionTestSuite) TestGetTask() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.sysArtifactMgr = &systemartifacttesting.Manager{}
	suite.ctl = &controller{
		execMgr:        suite.execMgr,
		taskMgr:        suite.taskMgr,
		makeCtx:        func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
		sysArtifactMgr: suite.sysArtifactMgr,
	}
	// valid task execution record exists for an execution id
	{
		t := task.Task{
			ID:             1,
			VendorType:     "SCAN_DATA_EXPORT",
			ExecutionID:    100,
			Status:         "Success",
			StatusMessage:  "",
			RunCount:       1,
			JobID:          "TestJobId",
			ExtraAttrs:     nil,
			CreationTime:   time.Time{},
			StartTime:      time.Time{},
			UpdateTime:     time.Time{},
			EndTime:        time.Time{},
			StatusRevision: 0,
		}

		tasks := []*task.Task{&t}
		suite.taskMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*task.Task, error) {
			return tasks, nil
		}
		returnedTask, err := suite.ctl.GetTask(context.Background(), 100)
		suite.NoError(err)
		suite.Equal(t, *returnedTask)
	}

	// no task records exist for an execution id
	{
		suite.taskMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*task.Task, error) {
			return []*task.Task{}, nil
		}
		_, err := suite.ctl.GetTask(context.Background(), 100)
		suite.Error(err)
	}

	// listing of tasks returns an error
	{
		suite.taskMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*task.Task, error) {
			return nil, errors.New("test error")
		}
		_, err := suite.ctl.GetTask(context.Background(), 100)
		suite.Error(err)
	}

}

func (suite *ScanDataExportExecutionTestSuite) TestGetExecution() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.sysArtifactMgr = &systemartifacttesting.Manager{}
	suite.ctl = &controller{
		execMgr:        suite.execMgr,
		taskMgr:        suite.taskMgr,
		makeCtx:        func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
		sysArtifactMgr: suite.sysArtifactMgr,
	}
	// get execution succeeds
	attrs := make(map[string]any)
	attrs[export.JobNameAttribute] = "test-job"
	attrs[export.UserNameAttribute] = "test-user"
	attrs[export.DigestKey] = "sha256:d04b98f48e8f8bcc15c6ae5ac050801cd6dcfd428fb5f9e65c4e16e7807340fa"
	attrs["status_message"] = "test-message"
	{
		exec := task.Execution{
			ID:            100,
			VendorType:    "SCAN_DATA_EXPORT",
			VendorID:      -1,
			Status:        "Success",
			StatusMessage: "",
			Metrics:       nil,
			Trigger:       "Manual",
			ExtraAttrs:    attrs,
			StartTime:     time.Time{},
			UpdateTime:    time.Time{},
			EndTime:       time.Time{},
		}
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &exec, nil
		}
		suite.sysArtifactMgr.ExistsFunc = func(_ context.Context, _ string, _ string, _ string) (bool, error) {
			return true, nil
		}

		exportExec, err := suite.ctl.GetExecution(context.TODO(), 100)
		suite.NoError(err)
		suite.Equal(exec.ID, exportExec.ID)
		suite.Equal("test-user", exportExec.UserName)
		suite.Equal("test-job", exportExec.JobName)
		suite.Equal("test-message", exportExec.StatusMessage)
		suite.Equal(true, exportExec.FilePresent)
	}

	// get execution fails
	{
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return nil, errors.New("test error")
		}
		exportExec, err := suite.ctl.GetExecution(context.TODO(), 100)
		suite.Error(err)
		suite.Nil(exportExec)
	}

	// get execution returns null
	{
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return nil, nil
		}
		exportExec, err := suite.ctl.GetExecution(context.TODO(), 100)
		suite.NoError(err)
		suite.Nil(exportExec)
	}

}

func (suite *ScanDataExportExecutionTestSuite) TestGetExecutionSysArtifactExistFail() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.sysArtifactMgr = &systemartifacttesting.Manager{}
	suite.ctl = &controller{
		execMgr:        suite.execMgr,
		taskMgr:        suite.taskMgr,
		makeCtx:        func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
		sysArtifactMgr: suite.sysArtifactMgr,
	}
	// get execution succeeds
	attrs := make(map[string]any)
	attrs[export.JobNameAttribute] = "test-job"
	attrs[export.UserNameAttribute] = "test-user"
	{
		exec := task.Execution{
			ID:            100,
			VendorType:    "SCAN_DATA_EXPORT",
			VendorID:      -1,
			Status:        "Success",
			StatusMessage: "",
			Metrics:       nil,
			Trigger:       "Manual",
			ExtraAttrs:    attrs,
			StartTime:     time.Time{},
			UpdateTime:    time.Time{},
			EndTime:       time.Time{},
		}
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &exec, nil
		}
		suite.sysArtifactMgr.ExistsFunc = func(_ context.Context, _ string, _ string, _ string) (bool, error) {
			return false, errors.New("test error")
		}

		exportExec, err := suite.ctl.GetExecution(context.TODO(), 100)
		suite.NoError(err)
		suite.Equal(exec.ID, exportExec.ID)
		suite.Equal("test-user", exportExec.UserName)
		suite.Equal("test-job", exportExec.JobName)
		suite.Equal(false, exportExec.FilePresent)
	}
}

func (suite *ScanDataExportExecutionTestSuite) TestGetExecutionList() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.sysArtifactMgr = &systemartifacttesting.Manager{}
	suite.ctl = &controller{
		execMgr:        suite.execMgr,
		taskMgr:        suite.taskMgr,
		makeCtx:        func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
		sysArtifactMgr: suite.sysArtifactMgr,
	}
	// get execution succeeds
	attrs := make(map[string]any)
	attrs[export.JobNameAttribute] = "test-job"
	attrs[export.UserNameAttribute] = "test-user"
	{
		exec := task.Execution{
			ID:            100,
			VendorType:    "SCAN_DATA_EXPORT",
			VendorID:      -1,
			Status:        "Success",
			StatusMessage: "",
			Metrics:       nil,
			Trigger:       "Manual",
			ExtraAttrs:    attrs,
			StartTime:     time.Time{},
			UpdateTime:    time.Time{},
			EndTime:       time.Time{},
		}
		execs := []*task.Execution{&exec}
		suite.execMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*task.Execution, error) {
			return execs, nil
		}
		suite.sysArtifactMgr.ExistsFunc = func(_ context.Context, _ string, _ string, _ string) (bool, error) {
			return true, nil
		}
		exportExec, err := suite.ctl.ListExecutions(context.TODO(), "test-user")
		suite.NoError(err)

		suite.Equal(1, len(exportExec))
		suite.Equal("test-user", exportExec[0].UserName)
		suite.Equal("test-job", exportExec[0].JobName)
	}

	// get execution fails
	{
		suite.execMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*task.Execution, error) {
			return nil, errors.New("test error")
		}
		exportExec, err := suite.ctl.ListExecutions(context.TODO(), "test-user")
		suite.Error(err)
		suite.Nil(exportExec)
	}
}

func (suite *ScanDataExportExecutionTestSuite) TestStart() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.ctl = &controller{
		execMgr: suite.execMgr,
		taskMgr: suite.taskMgr,
		makeCtx: func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}
	// execution manager and task manager return successfully
	{
		var capturedAttrs map[string]any
		suite.execMgr.CreateFunc = func(_ context.Context, _ string, _ int64, _ string, extraAttrs ...map[string]any) (int64, error) {
			if len(extraAttrs) > 0 {
				capturedAttrs = extraAttrs[0]
			}
			return int64(10), nil
		}
		suite.taskMgr.CreateFunc = func(_ context.Context, _ int64, _ *task.Job, _ ...map[string]any) (int64, error) {
			return int64(20), nil
		}
		ctx := context.Background()
		ctx = context.WithValue(ctx, export.CsvJobVendorIDKey, int(-1))
		criteria := export.Request{}
		criteria.Projects = []int64{1}
		criteria.UserName = "test-user"
		criteria.JobName = "test-job"
		executionId, err := suite.ctl.Start(ctx, criteria)
		suite.NoError(err)
		suite.Equal(int64(10), executionId)
		// validate execution manager was called with correct attrs
		suite.Equal("test-job", capturedAttrs[export.JobNameAttribute])
		suite.Equal("test-user", capturedAttrs[export.UserNameAttribute])
		// Verify Create was called
		assert.NotEmpty(suite.T(), suite.execMgr.CreateCalls())
	}

}

func (suite *ScanDataExportExecutionTestSuite) TestDeleteExecution() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.ctl = &controller{
		execMgr: suite.execMgr,
		taskMgr: suite.taskMgr,
		makeCtx: func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}
	suite.execMgr.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := suite.ctl.DeleteExecution(context.TODO(), int64(1))
	suite.NoError(err)
}

func (suite *ScanDataExportExecutionTestSuite) TestStartWithExecManagerError() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.ctl = &controller{
		execMgr: suite.execMgr,
		taskMgr: suite.taskMgr,
		makeCtx: func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}
	// execution manager returns an error
	{
		ctx := context.Background()
		ctx = context.WithValue(ctx, export.CsvJobVendorIDKey, int(-1))
		suite.execMgr.CreateFunc = func(_ context.Context, _ string, _ int64, _ string, _ ...map[string]any) (int64, error) {
			return int64(-1), errors.New("Test Error")
		}
		_, err := suite.ctl.Start(ctx, export.Request{JobName: "test-job", UserName: "test-user"})
		suite.Error(err)
	}
}

func (suite *ScanDataExportExecutionTestSuite) TestStartWithTaskManagerError() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.ctl = &controller{
		execMgr: suite.execMgr,
		taskMgr: suite.taskMgr,
		makeCtx: func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}
	// execution manager is successful but task manager returns an error
	{
		ctx := context.Background()
		ctx = context.WithValue(ctx, export.CsvJobVendorIDKey, int(-1))
		suite.execMgr.CreateFunc = func(_ context.Context, _ string, _ int64, _ string, _ ...map[string]any) (int64, error) {
			return int64(10), nil
		}
		suite.taskMgr.CreateFunc = func(_ context.Context, _ int64, _ *task.Job, _ ...map[string]any) (int64, error) {
			return int64(-1), errors.New("Test Error")
		}
		suite.execMgr.StopAndWaitFunc = func(_ context.Context, _ int64, _ time.Duration) error {
			return nil
		}
		suite.execMgr.MarkErrorFunc = func(_ context.Context, _ int64, _ string) error {
			return nil
		}
		_, err := suite.ctl.Start(ctx, export.Request{JobName: "test-job", UserName: "test-user", Projects: []int64{1}})
		suite.Error(err)
	}
}

func (suite *ScanDataExportExecutionTestSuite) TearDownSuite() {
	suite.execMgr = nil
	suite.taskMgr = nil
}

func TestScanDataExportExecutionTestSuite(t *testing.T) {
	suite.Run(t, &ScanDataExportExecutionTestSuite{})
}
