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

package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	repctlmodel "github.com/goharbor/harbor/src/controller/replication/model"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/pkg/task"
	testingTask "github.com/goharbor/harbor/src/testing/pkg/task"
)

type configurableAdapter struct {
	*mockAdapter
	got *model.SourceOptions
}

func (c *configurableAdapter) ConfigureSource(opts *model.SourceOptions) error {
	c.got = opts
	return nil
}

func sourceOptionsPolicy(srcType, dstType string) *repctlmodel.Policy {
	return &repctlmodel.Policy{
		SrcRegistry:  &model.Registry{Type: srcType},
		DestRegistry: &model.Registry{Type: dstType},
		Filters:      []*model.Filter{{Type: model.FilterTypeFile, Value: "*.json"}},
	}
}

func registerAdapter(t *testing.T, registryType string, a adapter.Adapter) {
	t.Helper()
	factory := &mockFactory{}
	factory.On("AdapterPattern").Return(nil)
	factory.On("Create", mock.Anything).Return(a, nil)
	require.NoError(t, adapter.RegisterFactory(registryType, factory))
}

func TestCopyFlowRejectsSourceOptionsOfUnsupportingAdapter(t *testing.T) {
	registerAdapter(t, "TEST_COPY_FLOW_NO_SOURCE_OPTIONS", &mockAdapter{})

	policy := sourceOptionsPolicy("TEST_COPY_FLOW_NO_SOURCE_OPTIONS", "TEST_COPY_FLOW_NO_SOURCE_OPTIONS")
	err := (&copyFlow{executionID: 1, policy: policy}).Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support source options")
}

func TestCopyFlowCarriesSourceOptions(t *testing.T) {
	srcAdp := &configurableAdapter{mockAdapter: &mockAdapter{}}
	dstAdp := &configurableAdapter{mockAdapter: &mockAdapter{}}
	registerAdapter(t, "TEST_COPY_FLOW_SOURCE_OPTIONS_SRC", srcAdp)
	registerAdapter(t, "TEST_COPY_FLOW_SOURCE_OPTIONS_DST", dstAdp)
	var configuredAtFetch *model.SourceOptions
	srcAdp.On("FetchArtifacts", mock.Anything).Run(func(mock.Arguments) {
		configuredAtFetch = srcAdp.got
	}).Return([]*model.Resource{{
		Type: model.ResourceTypeArtifact,
		Metadata: &model.ResourceMetadata{
			Repository: &model.Repository{Name: "org/model"},
			Artifacts:  []*model.Artifact{{Tags: []string{"main"}}},
		},
	}}, nil)
	dstAdp.On("Info").Return(&model.RegistryInfo{SupportedResourceTypes: []string{model.ResourceTypeArtifact}}, nil)
	dstAdp.On("PrepareForPush", mock.Anything).Return(nil)

	execMgr := &testingTask.ExecutionManager{}
	execMgr.On("Get", mock.Anything, mock.Anything).Return(&task.Execution{Status: job.RunningStatus.String()}, nil)
	taskMgr := &testingTask.Manager{}
	var src, dst model.Resource
	taskMgr.On("Create", mock.Anything, mock.Anything, mock.MatchedBy(func(j *task.Job) bool {
		return json.Unmarshal([]byte(j.Parameters["src_resource"].(string)), &src) == nil &&
			json.Unmarshal([]byte(j.Parameters["dst_resource"].(string)), &dst) == nil
	}), mock.Anything).Return(int64(1), nil).Once()

	policy := sourceOptionsPolicy("TEST_COPY_FLOW_SOURCE_OPTIONS_SRC", "TEST_COPY_FLOW_SOURCE_OPTIONS_DST")
	flow := &copyFlow{executionID: 1, policy: policy, executionMgr: execMgr, taskMgr: taskMgr}
	require.NoError(t, flow.Run(context.Background()))

	want := &model.SourceOptions{FileFilter: &model.FileFilter{Include: []string{"*.json"}}}
	assert.Equal(t, want, configuredAtFetch, "the source adapter is configured before it fetches")
	assert.Nil(t, dstAdp.got, "the destination adapter is not configured")
	srcAdp.AssertExpectations(t)
	dstAdp.AssertExpectations(t)
	assert.Equal(t, want, src.SourceOptions, "the transfer gets the options with the source resource")
	assert.Nil(t, dst.SourceOptions)
}
