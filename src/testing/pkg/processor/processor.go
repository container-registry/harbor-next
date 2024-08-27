// Code generated by mockery v2.43.2. DO NOT EDIT.

package processor

import (
	context "context"

	artifact "github.com/goharbor/harbor/src/pkg/artifact"

	mock "github.com/stretchr/testify/mock"

	processor "github.com/goharbor/harbor/src/controller/artifact/processor"
)

// Processor is an autogenerated mock type for the Processor type
type Processor struct {
	mock.Mock
}

// AbstractAddition provides a mock function with given fields: ctx, _a1, additionType
func (_m *Processor) AbstractAddition(ctx context.Context, _a1 *artifact.Artifact, additionType string) (*processor.Addition, error) {
	ret := _m.Called(ctx, _a1, additionType)

	if len(ret) == 0 {
		panic("no return value specified for AbstractAddition")
	}

	var r0 *processor.Addition
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *artifact.Artifact, string) (*processor.Addition, error)); ok {
		return rf(ctx, _a1, additionType)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *artifact.Artifact, string) *processor.Addition); ok {
		r0 = rf(ctx, _a1, additionType)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*processor.Addition)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *artifact.Artifact, string) error); ok {
		r1 = rf(ctx, _a1, additionType)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// AbstractMetadata provides a mock function with given fields: ctx, _a1, manifest
func (_m *Processor) AbstractMetadata(ctx context.Context, _a1 *artifact.Artifact, manifest []byte) error {
	ret := _m.Called(ctx, _a1, manifest)

	if len(ret) == 0 {
		panic("no return value specified for AbstractMetadata")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *artifact.Artifact, []byte) error); ok {
		r0 = rf(ctx, _a1, manifest)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetArtifactType provides a mock function with given fields: ctx, _a1
func (_m *Processor) GetArtifactType(ctx context.Context, _a1 *artifact.Artifact) string {
	ret := _m.Called(ctx, _a1)

	if len(ret) == 0 {
		panic("no return value specified for GetArtifactType")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context, *artifact.Artifact) string); ok {
		r0 = rf(ctx, _a1)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// ListAdditionTypes provides a mock function with given fields: ctx, _a1
func (_m *Processor) ListAdditionTypes(ctx context.Context, _a1 *artifact.Artifact) []string {
	ret := _m.Called(ctx, _a1)

	if len(ret) == 0 {
		panic("no return value specified for ListAdditionTypes")
	}

	var r0 []string
	if rf, ok := ret.Get(0).(func(context.Context, *artifact.Artifact) []string); ok {
		r0 = rf(ctx, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// NewProcessor creates a new instance of Processor. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewProcessor(t interface {
	mock.TestingT
	Cleanup(func())
}) *Processor {
	mock := &Processor{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
