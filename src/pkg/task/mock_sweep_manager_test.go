package task

import (
	"context"
)

type mockSweepManager struct {
	CleanFunc                      func(ctx context.Context, execID []int64) error
	FixDanglingStateExecutionFunc  func(ctx context.Context) error
	ListCandidatesFunc             func(ctx context.Context, vendorType string, retainCnt int64) ([]int64, error)
}

func (m *mockSweepManager) Clean(ctx context.Context, execID []int64) error {
	if m.CleanFunc != nil {
		return m.CleanFunc(ctx, execID)
	}
	return nil
}

func (m *mockSweepManager) FixDanglingStateExecution(ctx context.Context) error {
	if m.FixDanglingStateExecutionFunc != nil {
		return m.FixDanglingStateExecutionFunc(ctx)
	}
	return nil
}

func (m *mockSweepManager) ListCandidates(ctx context.Context, vendorType string, retainCnt int64) ([]int64, error) {
	if m.ListCandidatesFunc != nil {
		return m.ListCandidatesFunc(ctx, vendorType, retainCnt)
	}
	return nil, nil
}
