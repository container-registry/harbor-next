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

package hook

import (
	"context"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/jobservice/common/list"
	"github.com/goharbor/harbor/src/jobservice/common/utils"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/jobservice/tests"
	"github.com/goharbor/harbor/src/lib/errors"
)

// HookAgentTestSuite tests functions of hook agent
type HookAgentTestSuite struct {
	suite.Suite

	namespace string
	pool      *redis.Pool
	agent     *basicAgent

	event *Event
	jid   string
}

// TestHookAgentTestSuite is entry of go test
func TestHookAgentTestSuite(t *testing.T) {
	suite.Run(t, new(HookAgentTestSuite))
}

// SetupSuite prepares test suites
func (suite *HookAgentTestSuite) SetupSuite() {
	suite.pool = tests.GiveMeRedisPool()
	suite.namespace = tests.GiveMeTestNamespace()

	suite.agent = &basicAgent{
		context:   context.TODO(),
		namespace: suite.namespace,
		redisPool: suite.pool,
	}

	suite.prepareData()
}

// TearDownSuite prepares test suites
func (suite *HookAgentTestSuite) TearDownSuite() {
	conn := suite.pool.Get()
	defer func() {
		_ = conn.Close()
	}()

	_ = tests.ClearAll(suite.namespace, conn)
}

func (suite *HookAgentTestSuite) prepareData() {
	suite.jid = utils.MakeIdentifier()
	rev := time.Now().Unix()
	stats := &job.Stats{
		Info: &job.StatsInfo{
			JobID:    suite.jid,
			Status:   job.RunningStatus.String(),
			Revision: rev,
			JobKind:  job.KindGeneric,
			JobName:  job.SampleJob,
		},
	}
	t := job.NewBasicTrackerWithStats(context.TODO(), stats, suite.namespace, suite.pool, nil, list.New())
	err := t.Save()
	suite.NoError(err, "mock job stats")

	suite.event = &Event{
		URL:       "http://domain.com",
		Message:   "HookAgentTestSuite",
		Timestamp: time.Now().Unix(),
		Data: &job.StatusChange{
			JobID:  suite.jid,
			Status: job.SuccessStatus.String(),
			Metadata: &job.StatsInfo{
				JobID:    suite.jid,
				Status:   job.SuccessStatus.String(),
				Revision: rev,
				JobKind:  job.KindGeneric,
				JobName:  job.SampleJob,
			},
		},
	}
}

// TestEventSending ...
func (suite *HookAgentTestSuite) TestEventSending() {
	mc := &mockClient{}
	mc.On("SendEvent", suite.event).Return(nil)
	suite.agent.client = mc

	err := suite.agent.Trigger(suite.event)
	require.Nil(suite.T(), err, "agent trigger: nil error expected but got %s", err)

	// check
	suite.checkStatus()
}

// TestEventSending ...
func (suite *HookAgentTestSuite) TestEventSendingError() {
	mc := &mockClient{}
	mc.On("SendEvent", suite.event).Return(errors.New("internal server error: for testing"))
	suite.agent.client = mc

	err := suite.agent.Trigger(suite.event)

	suite.Error(err)
}

// retryEvent is a hook event for a job with no stats in Redis, which keeps
// isOutdated deterministic whatever the other tests in this suite have acked.
func (suite *HookAgentTestSuite) retryEvent() *Event {
	return &Event{
		URL:       "http://127.0.0.1:9999/hook",
		Message:   "retry test",
		Timestamp: time.Now().Unix(),
		Data: &job.StatusChange{
			JobID:  utils.MakeIdentifier(),
			Status: job.SuccessStatus.String(),
			Metadata: &job.StatsInfo{
				JobID:    utils.MakeIdentifier(),
				Status:   job.SuccessStatus.String(),
				Revision: time.Now().Unix(),
				JobKind:  job.KindGeneric,
				JobName:  job.SampleJob,
			},
		},
	}
}

func (suite *HookAgentTestSuite) retryAgent(ctx context.Context, client Client) *basicAgent {
	return &basicAgent{
		context:   ctx,
		namespace: suite.namespace,
		redisPool: suite.pool,
		client:    client,
		tokens:    make(chan struct{}, 1),
	}
}

// TestRetryResendsUntilSuccess covers the background retry path: the first send
// fails, the loop backs off and sends again.
func (suite *HookAgentTestSuite) TestRetryResendsUntilSuccess() {
	evt := suite.retryEvent()

	mc := &mockClient{}
	mc.On("SendEvent", evt).Return(errors.New("internal server error: for testing")).Once()
	mc.On("SendEvent", evt).Return(nil).Once()

	// A deadline well above the first backoff interval, so a regression that
	// stops the second send from succeeding fails the test instead of holding
	// the suite for the full errRetryBackoff.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suite.retryAgent(ctx, mc).retry(evt)

	mc.AssertExpectations(suite.T())
}

// TestResendStopsAtMaxElapsedTime drives the production retry loop, not a
// rebuilt copy of it, so dropping WithMaxElapsedTime at the call site fails here.
func (suite *HookAgentTestSuite) TestResendStopsAtMaxElapsedTime() {
	evt := suite.retryEvent()

	mc := &mockClient{}
	mc.On("SendEvent", evt).Return(errors.New("internal server error: for testing"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := suite.retryAgent(ctx, mc).resend(evt, 100*time.Millisecond)

	suite.Error(err)
	suite.ErrorIs(err, backoff.ErrMaxElapsedTime)
}

// TestResendStopsOnCancelledContext covers the agent context reaching the retry
// loop. On a cancelled context the operation runs once and gives up; with a
// context that never cancels it would instead run out the ceiling below and
// report ErrMaxElapsedTime.
func (suite *HookAgentTestSuite) TestResendStopsOnCancelledContext() {
	evt := suite.retryEvent()

	mc := &mockClient{}
	mc.On("SendEvent", evt).Return(errors.New("internal server error: for testing")).Once()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := suite.retryAgent(ctx, mc).resend(evt, 5*time.Second)

	suite.Error(err)
	suite.ErrorIs(err, context.Canceled)
	mc.AssertExpectations(suite.T())
}

func (suite *HookAgentTestSuite) checkStatus() {
	t := job.NewBasicTrackerWithID(context.TODO(), suite.jid, suite.namespace, suite.pool, nil, list.New())
	err := t.Load()
	require.NoError(suite.T(), err, "load updated job stats")
	require.NotNil(suite.T(), t.Job(), "latest job stats")
	suite.Equal(job.SuccessStatus.String(), t.Job().Info.HookAck.Status, "ack status")
}

type mockClient struct {
	mock.Mock
}

func (mc *mockClient) SendEvent(evt *Event) error {
	args := mc.Called(evt)
	return args.Error(0)
}
