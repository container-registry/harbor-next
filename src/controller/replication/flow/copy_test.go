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
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	repctlmodel "github.com/goharbor/harbor/src/controller/replication/model"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/pkg/task"
	taskpkg "github.com/goharbor/harbor/src/pkg/task"
	mocktask "github.com/goharbor/harbor/src/testing/moq/pkg/task"
)

type copyFlowTestSuite struct {
	suite.Suite
}

func (c *copyFlowTestSuite) TestRun() {
	adp := &mockAdapter{}
	factory := &mockFactory{}
	factory.On("AdapterPattern").Return(nil)
	factory.On("Create", mock.Anything).Return(adp, nil)
	adapter.RegisterFactory("TEST_FOR_COPY_FLOW", factory)

	adp.On("Info").Return(&model.RegistryInfo{
		SupportedResourceTypes: []string{
			model.ResourceTypeArtifact,
		},
	}, nil)
	adp.On("FetchArtifacts", mock.Anything).Return([]*model.Resource{
		{
			Type: model.ResourceTypeArtifact,
			Metadata: &model.ResourceMetadata{
				Repository: &model.Repository{
					Name: "proxy/hello-world",
				},
				Vtags: []string{"latest"},
			},
			Override: false,
		},
	}, nil)
	adp.On("PrepareForPush", mock.Anything).Return(nil)

	execMgr := &mocktask.ExecutionManager{}
	execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
		return &task.Execution{
			Status: job.RunningStatus.String(),
		}, nil
	}

	taskMgr := &mocktask.Manager{}
	taskMgr.CreateFunc = func(_ context.Context, _ int64, _ *taskpkg.Job, _ ...map[string]any) (int64, error) {
		return int64(1), nil
	}

	policy := &repctlmodel.Policy{
		SrcRegistry: &model.Registry{
			Type: "TEST_FOR_COPY_FLOW",
		},
		DestRegistry: &model.Registry{
			Type: "TEST_FOR_COPY_FLOW",
		},
	}
	flow := &copyFlow{
		executionID:  1,
		policy:       policy,
		executionMgr: execMgr,
		taskMgr:      taskMgr,
	}
	err := flow.Run(context.Background())
	c.Require().Nil(err)
}

func TestCopyFlowTestSuite(t *testing.T) {
	suite.Run(t, &copyFlowTestSuite{})
}
