package task

import (
	"context"
	"time"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/task/dao"
)

type mockTaskDAO struct {
	CountFunc                      func(ctx context.Context, query *q.Query) (int64, error)
	CreateFunc                     func(ctx context.Context, task *dao.Task) (int64, error)
	DeleteFunc                     func(ctx context.Context, id int64) error
	ExecutionIDsByVendorAndStatusFunc func(ctx context.Context, vendorType string, status string) ([]int64, error)
	GetFunc                        func(ctx context.Context, id int64) (*dao.Task, error)
	GetMaxEndTimeFunc              func(ctx context.Context, executionID int64) (time.Time, error)
	ListFunc                       func(ctx context.Context, query *q.Query) ([]*dao.Task, error)
	ListScanTasksByReportUUIDFunc  func(ctx context.Context, uuid string) ([]*dao.Task, error)
	ListStatusCountFunc            func(ctx context.Context, executionID int64) ([]*dao.StatusCount, error)
	UpdateFunc                     func(ctx context.Context, task *dao.Task, props ...string) error
	UpdateStatusFunc               func(ctx context.Context, id int64, status string, statusRevision int64) error
	UpdateStatusInBatchFunc        func(ctx context.Context, jobIDs []string, status string, batchSize int) error
}

func (m *mockTaskDAO) Count(ctx context.Context, query *q.Query) (int64, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx, query)
	}
	return 0, nil
}

func (m *mockTaskDAO) Create(ctx context.Context, task *dao.Task) (int64, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, task)
	}
	return 0, nil
}

func (m *mockTaskDAO) Delete(ctx context.Context, id int64) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *mockTaskDAO) ExecutionIDsByVendorAndStatus(ctx context.Context, vendorType string, status string) ([]int64, error) {
	if m.ExecutionIDsByVendorAndStatusFunc != nil {
		return m.ExecutionIDsByVendorAndStatusFunc(ctx, vendorType, status)
	}
	return nil, nil
}

func (m *mockTaskDAO) Get(ctx context.Context, id int64) (*dao.Task, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockTaskDAO) GetMaxEndTime(ctx context.Context, executionID int64) (time.Time, error) {
	if m.GetMaxEndTimeFunc != nil {
		return m.GetMaxEndTimeFunc(ctx, executionID)
	}
	return time.Time{}, nil
}

func (m *mockTaskDAO) List(ctx context.Context, query *q.Query) ([]*dao.Task, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, query)
	}
	return nil, nil
}

func (m *mockTaskDAO) ListScanTasksByReportUUID(ctx context.Context, uuid string) ([]*dao.Task, error) {
	if m.ListScanTasksByReportUUIDFunc != nil {
		return m.ListScanTasksByReportUUIDFunc(ctx, uuid)
	}
	return nil, nil
}

func (m *mockTaskDAO) ListStatusCount(ctx context.Context, executionID int64) ([]*dao.StatusCount, error) {
	if m.ListStatusCountFunc != nil {
		return m.ListStatusCountFunc(ctx, executionID)
	}
	return nil, nil
}

func (m *mockTaskDAO) Update(ctx context.Context, task *dao.Task, props ...string) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, task, props...)
	}
	return nil
}

func (m *mockTaskDAO) UpdateStatus(ctx context.Context, id int64, status string, statusRevision int64) error {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, status, statusRevision)
	}
	return nil
}

func (m *mockTaskDAO) UpdateStatusInBatch(ctx context.Context, jobIDs []string, status string, batchSize int) error {
	if m.UpdateStatusInBatchFunc != nil {
		return m.UpdateStatusInBatchFunc(ctx, jobIDs, status, batchSize)
	}
	return nil
}
