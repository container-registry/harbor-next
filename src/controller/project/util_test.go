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

package project

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/project/models"
)

type fakeProjectController struct {
	Controller
	listFunc func(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error)
}

func (f *fakeProjectController) List(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error) {
	if f.listFunc != nil {
		return f.listFunc(ctx, query, options...)
	}
	return nil, nil
}

func TestListAll_NormalPagination(t *testing.T) {
	origCtl := Ctl
	defer func() { Ctl = origCtl }()

	allProjects := []*models.Project{
		{ProjectID: 1, Name: "proj-1"},
		{ProjectID: 2, Name: "proj-2"},
		{ProjectID: 3, Name: "proj-3"},
		{ProjectID: 4, Name: "proj-4"},
		{ProjectID: 5, Name: "proj-5"},
	}

	Ctl = &fakeProjectController{
		listFunc: func(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error) {
			start := (query.PageNumber - 1) * query.PageSize
			if start >= int64(len(allProjects)) {
				return nil, nil
			}
			end := start + query.PageSize
			if end > int64(len(allProjects)) {
				end = int64(len(allProjects))
			}
			return allProjects[start:end], nil
		},
	}

	ctx := context.Background()
	chunkSize := 2
	ch := ListAll(ctx, chunkSize, nil)

	var received []*models.Project
	for res := range ch {
		require.NoError(t, res.Error)
		received = append(received, res.Data)
	}

	assert.Equal(t, len(allProjects), len(received))
	for i, p := range received {
		assert.Equal(t, allProjects[i].ProjectID, p.ProjectID)
		assert.Equal(t, allProjects[i].Name, p.Name)
	}
}

func TestListAll_ContextCancelled_PreventsGoroutineLeak(t *testing.T) {
	origCtl := Ctl
	defer func() { Ctl = origCtl }()

	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	projects := make([]*models.Project, 20)
	for i := 0; i < 20; i++ {
		projects[i] = &models.Project{
			ProjectID: int64(i + 1),
			Name:      fmt.Sprintf("proj-%d", i+1),
		}
	}

	Ctl = &fakeProjectController{
		listFunc: func(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error) {
			return projects, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	chunkSize := 2
	ch := ListAll(ctx, chunkSize, nil)

	res, ok := <-ch
	require.True(t, ok)
	require.NoError(t, res.Error)
	assert.Equal(t, int64(1), res.Data.ProjectID)
	cancel()
	_ = ch
}

func TestListAll_ContextAlreadyCancelled(t *testing.T) {
	origCtl := Ctl
	defer func() { Ctl = origCtl }()

	listCalled := false
	Ctl = &fakeProjectController{
		listFunc: func(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error) {
			listCalled = true
			return nil, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := ListAll(ctx, 5, nil)

	var count int
	for range ch {
		count++
	}

	assert.Equal(t, 0, count)
	assert.False(t, listCalled, "Ctl.List should not be called if context is already cancelled")
}

func TestListAll_ErrorFromList(t *testing.T) {
	origCtl := Ctl
	defer func() { Ctl = origCtl }()

	expectedErr := errors.New("database connection failed")
	Ctl = &fakeProjectController{
		listFunc: func(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error) {
			return nil, expectedErr
		},
	}

	ctx := context.Background()
	ch := ListAll(ctx, 5, nil)

	res, ok := <-ch
	require.True(t, ok)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "database connection failed")
	assert.Nil(t, res.Data)

	_, ok = <-ch
	assert.False(t, ok)
}

func TestListAll_EmptyResult(t *testing.T) {
	origCtl := Ctl
	defer func() { Ctl = origCtl }()

	Ctl = &fakeProjectController{
		listFunc: func(ctx context.Context, query *q.Query, options ...Option) ([]*models.Project, error) {
			return []*models.Project{}, nil
		},
	}

	ctx := context.Background()
	ch := ListAll(ctx, 5, nil)

	var count int
	for range ch {
		count++
	}

	assert.Equal(t, 0, count)
}
