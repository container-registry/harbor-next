package task

import (
	"github.com/goharbor/harbor/src/common/job/models"
	"github.com/goharbor/harbor/src/jobservice/job"
)

type mockJobserviceClient struct {
	GetExecutionsFunc     func(uuid string) ([]job.Stats, error)
	GetJobLogFunc         func(uuid string) ([]byte, error)
	GetJobServiceConfigFunc func() (*job.Config, error)
	PostActionFunc        func(uuid string, action string) error
	SubmitJobFunc         func(data *models.JobData) (string, error)
}

func (m *mockJobserviceClient) GetExecutions(uuid string) ([]job.Stats, error) {
	if m.GetExecutionsFunc != nil {
		return m.GetExecutionsFunc(uuid)
	}
	return nil, nil
}

func (m *mockJobserviceClient) GetJobLog(uuid string) ([]byte, error) {
	if m.GetJobLogFunc != nil {
		return m.GetJobLogFunc(uuid)
	}
	return nil, nil
}

func (m *mockJobserviceClient) GetJobServiceConfig() (*job.Config, error) {
	if m.GetJobServiceConfigFunc != nil {
		return m.GetJobServiceConfigFunc()
	}
	return nil, nil
}

func (m *mockJobserviceClient) PostAction(uuid string, action string) error {
	if m.PostActionFunc != nil {
		return m.PostActionFunc(uuid, action)
	}
	return nil
}

func (m *mockJobserviceClient) SubmitJob(data *models.JobData) (string, error) {
	if m.SubmitJobFunc != nil {
		return m.SubmitJobFunc(data)
	}
	return "", nil
}
