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

package replication

import (
	"context"

	repctlmodel "github.com/goharbor/harbor/src/controller/replication/model"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/pkg/task"
)

func (r *replicationTestSuite) TestCreatePolicyRejectsDisabledRegistryType() {
	r.T().Setenv("HARBOR_ENABLE_MODEL_SYNC", "false")
	err := r.createPullPolicy(model.RegistryTypeHuggingFace)
	r.Require().Error(err)
	r.True(errors.IsErr(err, errors.ForbiddenCode))
	r.repMgr.AssertNotCalled(r.T(), "Create")
}

func (r *replicationTestSuite) TestStartRejectsDisabledRegistryType() {
	r.T().Setenv("HARBOR_ENABLE_MODEL_SYNC", "false")
	_, err := r.ctl.Start(context.Background(), &repctlmodel.Policy{
		Enabled:      true,
		SrcRegistry:  &model.Registry{ID: 5, Type: model.RegistryTypeHuggingFace},
		DestRegistry: &model.Registry{ID: 0, Type: model.RegistryTypeHarbor},
	}, nil, task.ExecutionTriggerManual)
	r.Require().Error(err)
	r.True(errors.IsErr(err, errors.ForbiddenCode))
	r.execMgr.AssertNotCalled(r.T(), "Create")
}
