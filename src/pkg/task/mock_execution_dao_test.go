package task

import (
	"context"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/task/dao"
)

type mockExecutionDAO struct {
	AsyncRefreshStatusFunc func(ctx context.Context, id int64, vendor string) error
	CountFunc              func(ctx context.Context, query *q.Query) (int64, error)
	CreateFunc             func(ctx context.Context, execution *dao.Execution) (int64, error)
	DeleteFunc             func(ctx context.Context, id int64) error
	GetFunc                func(ctx context.Context, id int64) (*dao.Execution, error)
	GetMetricsFunc         func(ctx context.Context, id int64) (*dao.Metrics, error)
	ListFunc               func(ctx context.Context, query *q.Query) ([]*dao.Execution, error)
	RefreshStatusFunc      func(ctx context.Context, id int64) (bool, string, error)
	UpdateFunc             func(ctx context.Context, execution *dao.Execution, props ...string) error
}

func (m *mockExecutionDAO) AsyncRefreshStatus(ctx context.Context, id int64, vendor string) error {
	if m.AsyncRefreshStatusFunc != nil {
		return m.AsyncRefreshStatusFunc(ctx, id, vendor)
	}
	return nil
}

func (m *mockExecutionDAO) Count(ctx context.Context, query *q.Query) (int64, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx, query)
	}
	return 0, nil
}

func (m *mockExecutionDAO) Create(ctx context.Context, execution *dao.Execution) (int64, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, execution)
	}
	return 0, nil
}

func (m *mockExecutionDAO) Delete(ctx context.Context, id int64) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *mockExecutionDAO) Get(ctx context.Context, id int64) (*dao.Execution, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockExecutionDAO) GetMetrics(ctx context.Context, id int64) (*dao.Metrics, error) {
	if m.GetMetricsFunc != nil {
		return m.GetMetricsFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockExecutionDAO) List(ctx context.Context, query *q.Query) ([]*dao.Execution, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, query)
	}
	return nil, nil
}

func (m *mockExecutionDAO) RefreshStatus(ctx context.Context, id int64) (bool, string, error) {
	if m.RefreshStatusFunc != nil {
		return m.RefreshStatusFunc(ctx, id)
	}
	return false, "", nil
}

func (m *mockExecutionDAO) Update(ctx context.Context, execution *dao.Execution, props ...string) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, execution, props...)
	}
	return nil
}
