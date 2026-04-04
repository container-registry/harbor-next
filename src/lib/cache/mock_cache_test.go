package cache

import (
	"context"
	"sync/atomic"
	"time"
)

// mockCache is a function-field mock for the Cache interface.
type mockCache struct {
	ContainsFunc func(ctx context.Context, key string) bool
	DeleteFunc   func(ctx context.Context, key string) error
	FetchFunc    func(ctx context.Context, key string, value any) error
	PingFunc     func(ctx context.Context) error
	SaveFunc     func(ctx context.Context, key string, value any, expiration ...time.Duration) error
	ScanFunc     func(ctx context.Context, match string) (Iterator, error)

	saveCalls atomic.Int64
}

func (m *mockCache) Contains(ctx context.Context, key string) bool {
	if m.ContainsFunc != nil {
		return m.ContainsFunc(ctx, key)
	}
	return false
}

func (m *mockCache) Delete(ctx context.Context, key string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, key)
	}
	return nil
}

func (m *mockCache) Fetch(ctx context.Context, key string, value any) error {
	if m.FetchFunc != nil {
		return m.FetchFunc(ctx, key, value)
	}
	return nil
}

func (m *mockCache) Ping(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

func (m *mockCache) Save(ctx context.Context, key string, value any, expiration ...time.Duration) error {
	m.saveCalls.Add(1)
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, key, value, expiration...)
	}
	return nil
}

func (m *mockCache) Scan(ctx context.Context, match string) (Iterator, error) {
	if m.ScanFunc != nil {
		return m.ScanFunc(ctx, match)
	}
	return nil, nil
}
