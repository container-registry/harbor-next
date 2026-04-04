package systemartifact

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/pkg/task"
	"github.com/goharbor/harbor/src/testing/moq/controller/systemartifact"
)

type CallbackTestSuite struct {
	suite.Suite
	cleanupController *systemartifact.Controller
}

func (suite *CallbackTestSuite) SetupSuite() {
	suite.cleanupController = &systemartifact.Controller{}
	cleanupController = suite.cleanupController
}

func (suite *CallbackTestSuite) TestCleanupCallbackSuccess() {
	{
		ctx := context.TODO()
		var startCalled bool
		suite.cleanupController.StartFunc = func(_ context.Context, async bool, trigger string) error {
			startCalled = true
			suite.True(async)
			suite.Equal(task.ExecutionTriggerSchedule, trigger)
			return nil
		}
		err := cleanupCallBack(ctx, "")
		suite.NoErrorf(err, "Unexpected error : %v", err)
		suite.True(startCalled)
	}
	{
		suite.cleanupController = &systemartifact.Controller{}
		cleanupController = suite.cleanupController
	}

	{
		ctx := context.TODO()
		var startCalled bool
		suite.cleanupController.StartFunc = func(_ context.Context, async bool, trigger string) error {
			startCalled = true
			suite.True(async)
			suite.Equal(task.ExecutionTriggerSchedule, trigger)
			return errors.New("test error")
		}
		err := cleanupCallBack(ctx, "")
		suite.Error(err)
		suite.True(startCalled)
	}

}

func TestCallbackTestSuite(t *testing.T) {
	suite.Run(t, &CallbackTestSuite{})
}
