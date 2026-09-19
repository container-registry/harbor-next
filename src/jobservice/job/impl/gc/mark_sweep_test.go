package gc

import (
	"context"
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/q"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
)

type fakeProjectController struct {
	project.Controller
	listFunc func(ctx context.Context, query *q.Query, options ...project.Option) ([]*proModels.Project, error)
}

func (f *fakeProjectController) List(ctx context.Context, query *q.Query, options ...project.Option) ([]*proModels.Project, error) {
	if f.listFunc != nil {
		return f.listFunc(ctx, query, options...)
	}
	return nil, nil
}

func TestMarkOrSweepUntaggedBlobs_ContextCancelledOnStop(t *testing.T) {
	origCtl := project.Ctl
	defer func() { project.Ctl = origCtl }()

	producerExited := make(chan struct{})
	mockCtl := &fakeProjectController{
		listFunc: func(ctx context.Context, query *q.Query, options ...project.Option) ([]*proModels.Project, error) {
			go func() {
				<-ctx.Done()
				close(producerExited)
			}()
			return []*proModels.Project{
				{ProjectID: 1, Name: "proj-1"},
				{ProjectID: 2, Name: "proj-2"},
			}, nil
		},
	}
	project.Ctl = mockCtl

	mockCtx := &mockjobservice.MockJobContext{}
	mockCtx.On("OPCommand").Return(job.StopCommand, true)

	gc := &GarbageCollector{}
	_, err := gc.markOrSweepUntaggedBlobs(mockCtx)
	assert.ErrorIs(t, err, errGcStop)

	select {
	case <-producerExited:
		// Success: deferred cancel() in markOrSweepUntaggedBlobs cancelled
		// the context passed to project.ListAll when GC stopped early.
	case <-time.After(2 * time.Second):
		t.Fatal("ListAll context was NOT cancelled when GC stopped early")
	}
}
