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
package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/jobservice/common/query"
	"github.com/goharbor/harbor/src/jobservice/common/utils"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/jobservice/job/impl/sample"
	"github.com/goharbor/harbor/src/jobservice/worker"
)

// ControllerTestSuite tests functions of core controller
type ControllerTestSuite struct {
	suite.Suite

	manager *fakeManager
	worker  *fakeWorker
	ctl     Interface

	res    *job.Stats
	jobID  string
	params job.Parameters
}

// SetupSuite prepares test suite
func (suite *ControllerTestSuite) SetupSuite() {
	suite.ctl = NewController(suite, suite)

	suite.params = make(job.Parameters)
	suite.params["name"] = "testing:v1"

	suite.jobID = utils.MakeIdentifier()
	suite.res = &job.Stats{
		Info: &job.StatsInfo{
			JobID: suite.jobID,
		},
	}
}

// Prepare for each test case
func (suite *ControllerTestSuite) SetupTest() {
	suite.worker = &fakeWorker{}

	suite.worker.IsKnownJobFunc = func(name string) (any, bool) {
		if name == job.SampleJob {
			return (*sample.Job)(nil), true
		}
		return nil, false
	}
	suite.worker.ValidateJobParametersFunc = func(_ any, _ job.Parameters) error {
		return nil
	}

	fakeMgr := &fakeManager{}
	fakeMgr.SaveJobFunc = func(_ *job.Stats) error {
		return nil
	}
	fakeMgr.GetJobFunc = func(jobID string) (*job.Stats, error) {
		if jobID == suite.jobID {
			return suite.res, nil
		}
		return nil, nil
	}

	suite.manager = fakeMgr

}

// TestControllerTestSuite is suite entry for 'go test'
func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}

// SetupSuite prepares test suite
func (suite *ControllerTestSuite) TestLaunchGenericJob() {
	req := createJobReq("Generic")

	suite.worker.EnqueueFunc = func(jobName string, params job.Parameters, isUnique bool, webHook string) (*job.Stats, error) {
		return suite.res, nil
	}

	res, err := suite.ctl.LaunchJob(req)
	require.Nil(suite.T(), err, "launch job: nil error expected but got %s", err)
	assert.Equal(suite.T(), suite.jobID, res.Info.JobID, "mismatch job ID")
}

// TestLaunchScheduledJob ...
func (suite *ControllerTestSuite) TestLaunchScheduledJob() {
	req := createJobReq("Scheduled")

	suite.worker.ScheduleFunc = func(jobName string, params job.Parameters, runAfterSeconds uint64, isUnique bool, webHook string) (*job.Stats, error) {
		return suite.res, nil
	}

	res, err := suite.ctl.LaunchJob(req)
	require.Nil(suite.T(), err, "launch scheduled job: nil error expected but got %s", err)
	assert.Equal(suite.T(), suite.jobID, res.Info.JobID, "mismatch job ID")
}

// TestLaunchPeriodicJob ...
func (suite *ControllerTestSuite) TestLaunchPeriodicJob() {
	req := createJobReq("Periodic")

	suite.worker.PeriodicallyEnqueueFunc = func(jobName string, params job.Parameters, cronSetting string, isUnique bool, webHook string) (*job.Stats, error) {
		return suite.res, nil
	}

	res, err := suite.ctl.LaunchJob(req)
	require.Nil(suite.T(), err, "launch periodic job: nil error expected but got %s", err)
	assert.Equal(suite.T(), suite.jobID, res.Info.JobID, "mismatch job ID")
}

// TestGetJobStats ...
func (suite *ControllerTestSuite) TestGetJobStats() {
	res, err := suite.ctl.GetJob(suite.jobID)
	require.Nil(suite.T(), err, "get job stats: nil error expected but got %s", err)
	assert.Equal(suite.T(), suite.jobID, res.Info.JobID, "mismatch job ID")
}

// TestJobActions ...
func (suite *ControllerTestSuite) TestJobActions() {
	suite.worker.StopJobFunc = func(_ string) error {
		return nil
	}
	suite.worker.RetryJobFunc = func(_ string) error {
		return nil
	}

	err := suite.ctl.StopJob(suite.jobID)
	err = suite.ctl.RetryJob(suite.jobID)

	assert.Nil(suite.T(), err, "job action: nil error expected but got %s", err)
}

// TestCheckStatus ...
func (suite *ControllerTestSuite) TestCheckStatus() {
	suite.worker.StatsFunc = func() (*worker.Stats, error) {
		return &worker.Stats{
			Pools: []*worker.StatsData{
				{
					Status: "running",
				},
			},
		}, nil
	}

	st, err := suite.ctl.CheckStatus()
	require.Nil(suite.T(), err, "check worker status: nil error expected but got %s", err)
	assert.Equal(suite.T(), 1, len(st.Pools), "expected 1 pool status but got 0")
	assert.Equal(suite.T(), "running", st.Pools[0].Status, "expected running pool but got %s", st.Pools[0].Status)
}

// TestInvalidChecks ...
func (suite *ControllerTestSuite) TestInvalidChecks() {
	req := createJobReq("kind")

	_, err := suite.ctl.LaunchJob(req)
	assert.NotNil(suite.T(), err, "invalid job kind: error expected but got nil")

	req.Job.Metadata.JobKind = job.KindGeneric
	req.Job.Name = "fake"
	_, err = suite.ctl.LaunchJob(req)
	assert.NotNil(suite.T(), err, "invalid job name: error expected but got nil")

	req.Job.Metadata.JobKind = job.KindScheduled
	req.Job.Name = job.SampleJob
	req.Job.Metadata.ScheduleDelay = 0
	_, err = suite.ctl.LaunchJob(req)
	assert.NotNil(suite.T(), err, "invalid scheduled job: error expected but got nil")

	req.Job.Metadata.JobKind = job.KindPeriodic
	req.Job.Metadata.Cron = "x x x x x x"
	_, err = suite.ctl.LaunchJob(req)
	assert.NotNil(suite.T(), err, "invalid job name: error expected but got nil")
}

// TestGetScheduledJobs ...
func (suite *ControllerTestSuite) TestGetScheduledJobs() {
	extras := make(query.ExtraParameters)
	extras.Set(query.ExtraParamKeyKind, job.KindScheduled)
	q := &query.Parameter{
		PageSize:   20,
		PageNumber: 1,
		Extras:     extras,
	}

	fakeMgr := &fakeManager{}
	fakeMgr.SaveJobFunc = func(_ *job.Stats) error {
		return nil
	}
	fakeMgr.GetJobFunc = func(jobID string) (*job.Stats, error) {
		if jobID == suite.jobID {
			return suite.res, nil
		}
		return nil, nil
	}
	fakeMgr.GetScheduledJobsFunc = func(_ *query.Parameter) ([]*job.Stats, int64, error) {
		return []*job.Stats{suite.res}, int64(1), nil
	}
	suite.manager = fakeMgr

	_, total, err := suite.ctl.GetJobs(q)
	require.Nil(suite.T(), err, "scheduled jobs: nil error expected but got %s", err)
	assert.Equal(suite.T(), int64(1), total, "expected 1 item but got 0")
}

// TestGetPeriodicExecutions tests GetPeriodicExecutions
func (suite *ControllerTestSuite) TestGetPeriodicExecutions() {
	q := &query.Parameter{
		PageSize:   10,
		PageNumber: 1,
		Extras:     make(query.ExtraParameters),
	}

	fakeMgr := &fakeManager{}
	fakeMgr.SaveJobFunc = func(_ *job.Stats) error {
		return nil
	}
	fakeMgr.GetJobFunc = func(jobID string) (*job.Stats, error) {
		if jobID == suite.jobID {
			return suite.res, nil
		}
		return nil, nil
	}
	fakeMgr.GetPeriodicExecutionFunc = func(_ string, _ *query.Parameter) ([]*job.Stats, int64, error) {
		return []*job.Stats{suite.res}, int64(1), nil
	}
	suite.manager = fakeMgr

	_, total, err := suite.ctl.GetPeriodicExecutions("1000", q)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), total)
}

func createJobReq(kind string) *job.Request {
	params := make(job.Parameters)
	params["name"] = "testing:v1"
	return &job.Request{
		Job: &job.RequestBody{
			Name:       job.SampleJob,
			Parameters: params,
			StatusHook: "http://localhost:9090",
			Metadata: &job.Metadata{
				JobKind:       kind,
				IsUnique:      true,
				ScheduleDelay: 100,
				Cron:          "5 * * * * *",
			},
		},
	}
}

// Implement worker interface
func (suite *ControllerTestSuite) Start() error {
	return suite.worker.Start()
}

func (suite *ControllerTestSuite) GetPoolID() string {
	return suite.worker.GetPoolID()
}

func (suite *ControllerTestSuite) RegisterJobs(jobs map[string]any) error {
	return suite.worker.RegisterJobs(jobs)
}

func (suite *ControllerTestSuite) Enqueue(jobName string, params job.Parameters, isUnique bool, webHook string) (*job.Stats, error) {
	return suite.worker.Enqueue(jobName, params, isUnique, webHook)
}

func (suite *ControllerTestSuite) Schedule(jobName string, params job.Parameters, runAfterSeconds uint64, isUnique bool, webHook string) (*job.Stats, error) {
	return suite.worker.Schedule(jobName, params, runAfterSeconds, isUnique, webHook)
}

func (suite *ControllerTestSuite) PeriodicallyEnqueue(jobName string, params job.Parameters, cronSetting string, isUnique bool, webHook string) (*job.Stats, error) {
	return suite.worker.PeriodicallyEnqueue(jobName, params, cronSetting, isUnique, webHook)
}

func (suite *ControllerTestSuite) Stats() (*worker.Stats, error) {
	return suite.worker.Stats()
}

func (suite *ControllerTestSuite) IsKnownJob(name string) (any, bool) {
	return suite.worker.IsKnownJob(name)
}

func (suite *ControllerTestSuite) ValidateJobParameters(jobType any, params job.Parameters) error {
	return suite.worker.ValidateJobParameters(jobType, params)
}

func (suite *ControllerTestSuite) StopJob(jobID string) error {
	return suite.worker.StopJob(jobID)
}

func (suite *ControllerTestSuite) RetryJob(jobID string) error {
	return suite.worker.RetryJob(jobID)
}

// Implement manager interface
func (suite *ControllerTestSuite) GetJobs(q *query.Parameter) ([]*job.Stats, int64, error) {
	return suite.manager.GetJobs(q)
}

func (suite *ControllerTestSuite) GetPeriodicExecution(pID string, q *query.Parameter) ([]*job.Stats, int64, error) {
	return suite.manager.GetPeriodicExecution(pID, q)
}

func (suite *ControllerTestSuite) GetScheduledJobs(q *query.Parameter) ([]*job.Stats, int64, error) {
	return suite.manager.GetScheduledJobs(q)
}

func (suite *ControllerTestSuite) GetJob(jobID string) (*job.Stats, error) {
	return suite.manager.GetJob(jobID)
}

func (suite *ControllerTestSuite) SaveJob(j *job.Stats) error {
	return suite.manager.SaveJob(j)
}

// fake worker
type fakeWorker struct {
	StartFunc                func() error
	GetPoolIDFunc            func() string
	RegisterJobsFunc         func(jobs map[string]any) error
	EnqueueFunc              func(jobName string, params job.Parameters, isUnique bool, webHook string) (*job.Stats, error)
	ScheduleFunc             func(jobName string, params job.Parameters, runAfterSeconds uint64, isUnique bool, webHook string) (*job.Stats, error)
	PeriodicallyEnqueueFunc  func(jobName string, params job.Parameters, cronSetting string, isUnique bool, webHook string) (*job.Stats, error)
	StatsFunc                func() (*worker.Stats, error)
	IsKnownJobFunc           func(name string) (any, bool)
	ValidateJobParametersFunc func(jobType any, params job.Parameters) error
	StopJobFunc              func(jobID string) error
	RetryJobFunc             func(jobID string) error
}

func (f *fakeWorker) Start() error {
	if f.StartFunc != nil {
		return f.StartFunc()
	}
	return nil
}

func (f *fakeWorker) GetPoolID() string {
	if f.GetPoolIDFunc != nil {
		return f.GetPoolIDFunc()
	}
	return ""
}

func (f *fakeWorker) RegisterJobs(jobs map[string]any) error {
	if f.RegisterJobsFunc != nil {
		return f.RegisterJobsFunc(jobs)
	}
	return nil
}

func (f *fakeWorker) Enqueue(jobName string, params job.Parameters, isUnique bool, webHook string) (*job.Stats, error) {
	if f.EnqueueFunc != nil {
		return f.EnqueueFunc(jobName, params, isUnique, webHook)
	}
	return nil, nil
}

func (f *fakeWorker) Schedule(jobName string, params job.Parameters, runAfterSeconds uint64, isUnique bool, webHook string) (*job.Stats, error) {
	if f.ScheduleFunc != nil {
		return f.ScheduleFunc(jobName, params, runAfterSeconds, isUnique, webHook)
	}
	return nil, nil
}

func (f *fakeWorker) PeriodicallyEnqueue(jobName string, params job.Parameters, cronSetting string, isUnique bool, webHook string) (*job.Stats, error) {
	if f.PeriodicallyEnqueueFunc != nil {
		return f.PeriodicallyEnqueueFunc(jobName, params, cronSetting, isUnique, webHook)
	}
	return nil, nil
}

func (f *fakeWorker) Stats() (*worker.Stats, error) {
	if f.StatsFunc != nil {
		return f.StatsFunc()
	}
	return nil, nil
}

func (f *fakeWorker) IsKnownJob(name string) (any, bool) {
	if f.IsKnownJobFunc != nil {
		return f.IsKnownJobFunc(name)
	}
	return nil, false
}

func (f *fakeWorker) ValidateJobParameters(jobType any, params job.Parameters) error {
	if f.ValidateJobParametersFunc != nil {
		return f.ValidateJobParametersFunc(jobType, params)
	}
	return nil
}

func (f *fakeWorker) StopJob(jobID string) error {
	if f.StopJobFunc != nil {
		return f.StopJobFunc(jobID)
	}
	return nil
}

func (f *fakeWorker) RetryJob(jobID string) error {
	if f.RetryJobFunc != nil {
		return f.RetryJobFunc(jobID)
	}
	return nil
}

// fake manager
type fakeManager struct {
	GetJobsFunc              func(q *query.Parameter) ([]*job.Stats, int64, error)
	GetPeriodicExecutionFunc func(pID string, q *query.Parameter) ([]*job.Stats, int64, error)
	GetScheduledJobsFunc     func(q *query.Parameter) ([]*job.Stats, int64, error)
	GetJobFunc               func(jobID string) (*job.Stats, error)
	SaveJobFunc              func(j *job.Stats) error
}

func (fm *fakeManager) GetJobs(q *query.Parameter) ([]*job.Stats, int64, error) {
	if fm.GetJobsFunc != nil {
		return fm.GetJobsFunc(q)
	}
	return nil, 0, nil
}

func (fm *fakeManager) GetPeriodicExecution(pID string, q *query.Parameter) ([]*job.Stats, int64, error) {
	if fm.GetPeriodicExecutionFunc != nil {
		return fm.GetPeriodicExecutionFunc(pID, q)
	}
	return nil, 0, nil
}

func (fm *fakeManager) GetScheduledJobs(q *query.Parameter) ([]*job.Stats, int64, error) {
	if fm.GetScheduledJobsFunc != nil {
		return fm.GetScheduledJobsFunc(q)
	}
	return nil, 0, nil
}

func (fm *fakeManager) GetJob(jobID string) (*job.Stats, error) {
	if fm.GetJobFunc != nil {
		return fm.GetJobFunc(jobID)
	}
	return nil, nil
}

func (fm *fakeManager) SaveJob(j *job.Stats) error {
	if fm.SaveJobFunc != nil {
		return fm.SaveJobFunc(j)
	}
	return nil
}
