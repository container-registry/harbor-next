package task

import (
	"context"

	"github.com/goharbor/harbor/src/lib/q"
)

type mockTaskManager struct {
	CountFunc                      func(ctx context.Context, query *q.Query) (int64, error)
	CreateFunc                     func(ctx context.Context, executionID int64, job *Job, extraAttrs ...map[string]any) (int64, error)
	ExecutionIDsByVendorAndStatusFunc func(ctx context.Context, vendorType string, status string) ([]int64, error)
	GetFunc                        func(ctx context.Context, id int64) (*Task, error)
	GetLogFunc                     func(ctx context.Context, id int64) ([]byte, error)
	GetLogByJobIDFunc              func(ctx context.Context, jobID string) ([]byte, error)
	IsTaskFinishedFunc             func(ctx context.Context, reportID string) bool
	ListFunc                       func(ctx context.Context, query *q.Query) ([]*Task, error)
	ListScanTasksByReportUUIDFunc  func(ctx context.Context, uuid string) ([]*Task, error)
	RetrieveStatusFromTaskFunc     func(ctx context.Context, reportID string) string
	StopFunc                       func(ctx context.Context, id int64) error
	UpdateFunc                     func(ctx context.Context, task *Task, props ...string) error
	UpdateExtraAttrsFunc           func(ctx context.Context, id int64, extraAttrs map[string]any) error
	UpdateStatusInBatchFunc        func(ctx context.Context, jobIDs []string, status string, batchSize int) error
}

func (m *mockTaskManager) Count(ctx context.Context, query *q.Query) (int64, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx, query)
	}
	return 0, nil
}

func (m *mockTaskManager) Create(ctx context.Context, executionID int64, job *Job, extraAttrs ...map[string]any) (int64, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, executionID, job, extraAttrs...)
	}
	return 0, nil
}

func (m *mockTaskManager) ExecutionIDsByVendorAndStatus(ctx context.Context, vendorType string, status string) ([]int64, error) {
	if m.ExecutionIDsByVendorAndStatusFunc != nil {
		return m.ExecutionIDsByVendorAndStatusFunc(ctx, vendorType, status)
	}
	return nil, nil
}

func (m *mockTaskManager) Get(ctx context.Context, id int64) (*Task, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockTaskManager) GetLog(ctx context.Context, id int64) ([]byte, error) {
	if m.GetLogFunc != nil {
		return m.GetLogFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockTaskManager) GetLogByJobID(ctx context.Context, jobID string) ([]byte, error) {
	if m.GetLogByJobIDFunc != nil {
		return m.GetLogByJobIDFunc(ctx, jobID)
	}
	return nil, nil
}

func (m *mockTaskManager) IsTaskFinished(ctx context.Context, reportID string) bool {
	if m.IsTaskFinishedFunc != nil {
		return m.IsTaskFinishedFunc(ctx, reportID)
	}
	return false
}

func (m *mockTaskManager) List(ctx context.Context, query *q.Query) ([]*Task, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, query)
	}
	return nil, nil
}

func (m *mockTaskManager) ListScanTasksByReportUUID(ctx context.Context, uuid string) ([]*Task, error) {
	if m.ListScanTasksByReportUUIDFunc != nil {
		return m.ListScanTasksByReportUUIDFunc(ctx, uuid)
	}
	return nil, nil
}

func (m *mockTaskManager) RetrieveStatusFromTask(ctx context.Context, reportID string) string {
	if m.RetrieveStatusFromTaskFunc != nil {
		return m.RetrieveStatusFromTaskFunc(ctx, reportID)
	}
	return ""
}

func (m *mockTaskManager) Stop(ctx context.Context, id int64) error {
	if m.StopFunc != nil {
		return m.StopFunc(ctx, id)
	}
	return nil
}

func (m *mockTaskManager) Update(ctx context.Context, task *Task, props ...string) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, task, props...)
	}
	return nil
}

func (m *mockTaskManager) UpdateExtraAttrs(ctx context.Context, id int64, extraAttrs map[string]any) error {
	if m.UpdateExtraAttrsFunc != nil {
		return m.UpdateExtraAttrsFunc(ctx, id, extraAttrs)
	}
	return nil
}

func (m *mockTaskManager) UpdateStatusInBatch(ctx context.Context, jobIDs []string, status string, batchSize int) error {
	if m.UpdateStatusInBatchFunc != nil {
		return m.UpdateStatusInBatchFunc(ctx, jobIDs, status, batchSize)
	}
	return nil
}
