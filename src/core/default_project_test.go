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

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/project/models"
	quotamodels "github.com/goharbor/harbor/src/pkg/quota"
	"github.com/goharbor/harbor/src/testing/controller/project"
	"github.com/goharbor/harbor/src/testing/controller/quota"
	"github.com/goharbor/harbor/src/testing/mock"
)

type defaultProjectFixture struct {
	init     *defaultProjectInit
	projects *project.Controller
	quotas   *quota.Controller
	stored   []string
	txs      int
}

func newDefaultProjectFixture(quotaEnabled bool) *defaultProjectFixture {
	f := &defaultProjectFixture{projects: &project.Controller{}, quotas: &quota.Controller{}}
	f.init = &defaultProjectInit{
		projectCtl:   f.projects,
		quotaCtl:     f.quotas,
		quotaEnabled: func(context.Context) bool { return quotaEnabled },
		storageLimit: func(context.Context) (int64, error) { return 100, nil },
		setDefault: func(_ context.Context, name string) error {
			f.stored = append(f.stored, name)
			return nil
		},
		existsLocked: func(ctx context.Context, name string) (bool, error) {
			return f.projects.Exists(ctx, name)
		},
		inTx: func(ctx context.Context, fn func(context.Context) error) error {
			f.txs++
			return fn(ctx)
		},
		getDefault: func(context.Context) string {
			if len(f.stored) == 0 {
				return ""
			}
			return f.stored[len(f.stored)-1]
		},
	}
	return f
}

func TestApplyEnvUnset(t *testing.T) {
	f := newDefaultProjectFixture(true)
	require.NoError(t, f.init.applyEnv(context.TODO()))
	assert.Empty(t, f.stored)
}

func TestApplyEnvEmptyPinsFallbackOff(t *testing.T) {
	t.Setenv(common.DefaultProjectNameEnv, "")
	f := newDefaultProjectFixture(true)
	require.NoError(t, f.init.applyEnv(context.TODO()))
	assert.Equal(t, []string{""}, f.stored)
}

func TestApplyEnvInvalidName(t *testing.T) {
	t.Setenv(common.DefaultProjectNameEnv, "Bad/Name")
	f := newDefaultProjectFixture(true)
	assert.Error(t, f.init.applyEnv(context.TODO()))
	assert.Empty(t, f.stored)
}

func TestApplyEnvCreatesMissingProject(t *testing.T) {
	t.Setenv(common.DefaultProjectNameEnv, "catchall")
	f := newDefaultProjectFixture(true)
	f.projects.On("Exists", mock.Anything, "catchall").Return(false, nil).Once()
	f.projects.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Name == "catchall" && p.OwnerID == adminUserID && p.Metadata["public"] == "false"
	})).Return(int64(7), nil).Once()

	require.NoError(t, f.init.applyEnv(context.TODO()))
	assert.Equal(t, []string{"catchall"}, f.stored)
	assert.Equal(t, 1, f.txs, "create and store must share one transaction")
	f.projects.AssertExpectations(t)
}

func TestApplyEnvExistingProject(t *testing.T) {
	t.Setenv(common.DefaultProjectNameEnv, "library")
	f := newDefaultProjectFixture(true)
	f.projects.On("Exists", mock.Anything, "library").Return(true, nil).Once()

	require.NoError(t, f.init.applyEnv(context.TODO()))
	assert.Equal(t, []string{"library"}, f.stored)
	f.projects.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestEnsureToleratesConcurrentCreate(t *testing.T) {
	f := newDefaultProjectFixture(false)
	f.projects.On("Exists", mock.Anything, "catchall").Return(false, nil).Once()
	f.projects.On("Create", mock.Anything, mock.Anything).Return(int64(0), errors.ConflictError(nil)).Once()
	f.projects.On("Exists", mock.Anything, "catchall").Return(true, nil).Once()

	assert.NoError(t, f.init.ensure(context.TODO(), "catchall"))
}

func TestEnsureCreateFailure(t *testing.T) {
	f := newDefaultProjectFixture(false)
	f.projects.On("Exists", mock.Anything, "catchall").Return(false, nil).Twice()
	f.projects.On("Create", mock.Anything, mock.Anything).Return(int64(0), errors.New("db down")).Once()

	assert.Error(t, f.init.ensure(context.TODO(), "catchall"))
}

func TestReconcileCreatesMissingQuota(t *testing.T) {
	f := newDefaultProjectFixture(true)
	f.stored = []string{"catchall"}
	f.projects.On("Exists", mock.Anything, "catchall").Return(true, nil).Once()
	f.projects.On("GetByName", mock.Anything, "catchall").Return(&models.Project{ProjectID: 9, Name: "catchall"}, nil)
	f.quotas.On("GetByRef", mock.Anything, "project", "9").Return(nil, errors.NotFoundError(nil)).Once()
	f.quotas.On("Create", mock.Anything, "project", "9", mock.Anything).Return(int64(1), nil).Once()

	require.NoError(t, f.init.reconcile(context.TODO()))
	f.quotas.AssertExpectations(t)
}

func TestReconcileKeepsExistingQuota(t *testing.T) {
	f := newDefaultProjectFixture(true)
	f.stored = []string{"library"}
	f.projects.On("Exists", mock.Anything, "library").Return(true, nil).Once()
	f.projects.On("GetByName", mock.Anything, "library").Return(&models.Project{ProjectID: 1, Name: "library"}, nil)
	f.quotas.On("GetByRef", mock.Anything, "project", "1").Return(&quotamodels.Quota{}, nil).Once()

	require.NoError(t, f.init.reconcile(context.TODO()))
	f.quotas.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestReconcileToleratesConcurrentQuota(t *testing.T) {
	f := newDefaultProjectFixture(true)
	f.stored = []string{"catchall"}
	f.projects.On("Exists", mock.Anything, "catchall").Return(true, nil).Once()
	f.projects.On("GetByName", mock.Anything, "catchall").Return(&models.Project{ProjectID: 9, Name: "catchall"}, nil)
	f.quotas.On("GetByRef", mock.Anything, "project", "9").Return(nil, errors.NotFoundError(nil)).Once()
	f.quotas.On("Create", mock.Anything, "project", "9", mock.Anything).Return(int64(0), errors.ConflictError(nil)).Once()
	f.quotas.On("GetByRef", mock.Anything, "project", "9").Return(&quotamodels.Quota{}, nil).Once()

	assert.NoError(t, f.init.reconcile(context.TODO()))
}

func TestReconcileDanglingDefaultOnlyWarns(t *testing.T) {
	f := newDefaultProjectFixture(true)
	f.stored = []string{"gone"}
	f.projects.On("Exists", mock.Anything, "gone").Return(false, nil).Once()

	assert.NoError(t, f.init.reconcile(context.TODO()))
	f.projects.AssertNotCalled(t, "GetByName", mock.Anything, mock.Anything)
}

func TestReconcileQuotaDisabled(t *testing.T) {
	f := newDefaultProjectFixture(false)
	f.stored = []string{"library"}
	f.projects.On("Exists", mock.Anything, "library").Return(true, nil).Once()

	assert.NoError(t, f.init.reconcile(context.TODO()))
	f.quotas.AssertNotCalled(t, "GetByRef", mock.Anything, mock.Anything, mock.Anything)
}

func TestEnsureForOverwrite(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		f := newDefaultProjectFixture(false)
		assert.NoError(t, f.init.ensureForOverwrite(context.TODO()))
	})
	t.Run("no default project key", func(t *testing.T) {
		t.Setenv(configOverwriteJSONEnv, `{"read_only": false}`)
		f := newDefaultProjectFixture(false)
		assert.NoError(t, f.init.ensureForOverwrite(context.TODO()))
		f.projects.AssertNotCalled(t, "Exists", mock.Anything, mock.Anything)
	})
	t.Run("creates the named project", func(t *testing.T) {
		t.Setenv(configOverwriteJSONEnv, `{"default_project_name": "catchall"}`)
		f := newDefaultProjectFixture(false)
		f.projects.On("Exists", mock.Anything, "catchall").Return(false, nil).Once()
		f.projects.On("Create", mock.Anything, mock.Anything).Return(int64(3), nil).Once()
		assert.NoError(t, f.init.ensureForOverwrite(context.TODO()))
		f.projects.AssertExpectations(t)
	})
	t.Run("skipped while DEFAULT_PROJECT_NAME is set", func(t *testing.T) {
		t.Setenv(configOverwriteJSONEnv, `{"default_project_name": "catchall"}`)
		t.Setenv(common.DefaultProjectNameEnv, "library")
		f := newDefaultProjectFixture(false)
		assert.NoError(t, f.init.ensureForOverwrite(context.TODO()))
		f.projects.AssertNotCalled(t, "Exists", mock.Anything, mock.Anything)
	})
	t.Run("malformed JSON left to OverwriteConfig", func(t *testing.T) {
		t.Setenv(configOverwriteJSONEnv, `{not json`)
		f := newDefaultProjectFixture(false)
		assert.NoError(t, f.init.ensureForOverwrite(context.TODO()))
	})
	t.Run("invalid name left to validation", func(t *testing.T) {
		t.Setenv(configOverwriteJSONEnv, `{"default_project_name": "Bad Name"}`)
		f := newDefaultProjectFixture(false)
		assert.NoError(t, f.init.ensureForOverwrite(context.TODO()))
		f.projects.AssertNotCalled(t, "Exists", mock.Anything, mock.Anything)
	})
}
