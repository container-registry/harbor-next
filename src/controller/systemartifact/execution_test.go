package systemartifact

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	scheduler2 "github.com/goharbor/harbor/src/pkg/scheduler"
	"github.com/goharbor/harbor/src/pkg/task"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
	"github.com/goharbor/harbor/src/testing/moq/pkg/scheduler"
	"github.com/goharbor/harbor/src/testing/moq/pkg/systemartifact"
	testingTask "github.com/goharbor/harbor/src/testing/moq/pkg/task"
)

type SystemArtifactCleanupTestSuite struct {
	suite.Suite
	execMgr    *testingTask.ExecutionManager
	taskMgr    *testingTask.Manager
	cleanupMgr *systemartifact.Manager
	ctl        *controller
	sched      *scheduler.Scheduler
}

func (suite *SystemArtifactCleanupTestSuite) SetupSuite() {
}

func (suite *SystemArtifactCleanupTestSuite) TestStartCleanup() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.cleanupMgr = &systemartifact.Manager{}
	suite.ctl = &controller{
		execMgr:           suite.execMgr,
		taskMgr:           suite.taskMgr,
		systemArtifactMgr: suite.cleanupMgr,
		makeCtx:           func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}

	{

		ctx := context.TODO()

		executionID := int64(1)
		taskId := int64(1)

		suite.execMgr.CreateFunc = func(_ context.Context, _ string, _ int64, _ string, _ ...map[string]any) (int64, error) {
			return executionID, nil
		}

		suite.taskMgr.CreateFunc = func(_ context.Context, _ int64, j *task.Job, _ ...map[string]any) (int64, error) {
			assert.Equal(suite.T(), "SYSTEM_ARTIFACT_CLEANUP", j.Name)
			return taskId, nil
		}

		suite.execMgr.MarkDoneFunc = func(_ context.Context, _ int64, _ string) error {
			return nil
		}

		err := suite.ctl.Start(ctx, false, "SCHEDULE")
		suite.NoError(err)
		assert.NotEmpty(suite.T(), suite.taskMgr.CreateCalls())
	}
}

func (suite *SystemArtifactCleanupTestSuite) TestStartCleanupErrorDuringCreate() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.cleanupMgr = &systemartifact.Manager{}
	suite.ctl = &controller{
		execMgr:           suite.execMgr,
		taskMgr:           suite.taskMgr,
		systemArtifactMgr: suite.cleanupMgr,
		makeCtx:           func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}

	{

		ctx := context.TODO()

		suite.execMgr.CreateFunc = func(_ context.Context, _ string, _ int64, _ string, _ ...map[string]any) (int64, error) {
			return int64(0), errors.New("test error")
		}

		suite.execMgr.MarkDoneFunc = func(_ context.Context, _ int64, _ string) error {
			return nil
		}

		err := suite.ctl.Start(ctx, false, "SCHEDULE")
		suite.Error(err)
	}
}

func (suite *SystemArtifactCleanupTestSuite) TestStartCleanupErrorDuringTaskCreate() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.cleanupMgr = &systemartifact.Manager{}
	suite.ctl = &controller{
		execMgr:           suite.execMgr,
		taskMgr:           suite.taskMgr,
		systemArtifactMgr: suite.cleanupMgr,
		makeCtx:           func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}

	{

		ctx := context.TODO()

		executionID := int64(1)

		suite.execMgr.CreateFunc = func(_ context.Context, _ string, _ int64, _ string, _ ...map[string]any) (int64, error) {
			return executionID, nil
		}

		suite.taskMgr.CreateFunc = func(_ context.Context, _ int64, _ *task.Job, _ ...map[string]any) (int64, error) {
			return int64(0), errors.New("test error")
		}

		suite.execMgr.MarkErrorFunc = func(_ context.Context, _ int64, _ string) error {
			return nil
		}
		suite.execMgr.StopAndWaitWithErrorFunc = func(_ context.Context, _ int64, _ time.Duration, _ error) error {
			return nil
		}

		err := suite.ctl.Start(ctx, false, "SCHEDULE")
		suite.Error(err)
	}
}

func (suite *SystemArtifactCleanupTestSuite) TestScheduleCleanupJobNoPreviousSchedule() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.cleanupMgr = &systemartifact.Manager{}
	suite.sched = &scheduler.Scheduler{}

	suite.ctl = &controller{
		execMgr:           suite.execMgr,
		taskMgr:           suite.taskMgr,
		systemArtifactMgr: suite.cleanupMgr,
		makeCtx:           func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}

	scheduleCalled := false
	suite.sched.ScheduleFunc = func(_ context.Context, _ string, _ int64, _ string, _ string, _ string, _ any, _ map[string]any) (int64, error) {
		scheduleCalled = true
		return int64(1), nil
	}
	suite.sched.ListSchedulesFunc = func(_ context.Context, _ *q.Query) ([]*scheduler2.Schedule, error) {
		return make([]*scheduler2.Schedule, 0), nil
	}
	sched = suite.sched
	ctx := context.TODO()

	ScheduleCleanupTask(ctx)

	suite.True(scheduleCalled)
}

func (suite *SystemArtifactCleanupTestSuite) TestScheduleCleanupJobPreviousSchedule() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.cleanupMgr = &systemartifact.Manager{}
	suite.sched = &scheduler.Scheduler{}

	suite.ctl = &controller{
		execMgr:           suite.execMgr,
		taskMgr:           suite.taskMgr,
		systemArtifactMgr: suite.cleanupMgr,
		makeCtx:           func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}

	scheduleCalled := false
	suite.sched.ScheduleFunc = func(_ context.Context, _ string, _ int64, _ string, _ string, _ string, _ any, _ map[string]any) (int64, error) {
		scheduleCalled = true
		return int64(1), nil
	}

	existingSchedule := scheduler2.Schedule{ID: int64(10)}
	suite.sched.ListSchedulesFunc = func(_ context.Context, _ *q.Query) ([]*scheduler2.Schedule, error) {
		return []*scheduler2.Schedule{&existingSchedule}, nil
	}
	sched = suite.sched
	ctx := context.TODO()

	ScheduleCleanupTask(ctx)

	suite.False(scheduleCalled)
}

func (suite *SystemArtifactCleanupTestSuite) TestScheduleCleanupJobPreviousScheduleError() {
	suite.taskMgr = &testingTask.Manager{}
	suite.execMgr = &testingTask.ExecutionManager{}
	suite.cleanupMgr = &systemartifact.Manager{}
	suite.sched = &scheduler.Scheduler{}

	suite.ctl = &controller{
		execMgr:           suite.execMgr,
		taskMgr:           suite.taskMgr,
		systemArtifactMgr: suite.cleanupMgr,
		makeCtx:           func() context.Context { return orm.NewContext(nil, &ormtesting.FakeOrmer{}) },
	}

	scheduleCalled := false
	suite.sched.ScheduleFunc = func(_ context.Context, _ string, _ int64, _ string, _ string, _ string, _ any, _ map[string]any) (int64, error) {
		scheduleCalled = true
		return int64(1), nil
	}

	suite.sched.ListSchedulesFunc = func(_ context.Context, _ *q.Query) ([]*scheduler2.Schedule, error) {
		return nil, errors.New("test error")
	}
	sched = suite.sched
	ctx := context.TODO()

	ScheduleCleanupTask(ctx)

	suite.False(scheduleCalled)
}

func (suite *SystemArtifactCleanupTestSuite) TearDownSuite() {
	suite.execMgr = nil
	suite.taskMgr = nil
	suite.cleanupMgr = nil
	suite.ctl = nil
	suite.sched = nil
}

func TestScanDataExportExecutionTestSuite(t *testing.T) {
	suite.Run(t, &SystemArtifactCleanupTestSuite{})
}
