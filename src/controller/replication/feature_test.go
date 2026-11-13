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
	"os"

	repctlmodel "github.com/goharbor/harbor/src/controller/replication/model"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/huggingface"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/pkg/task"
	"github.com/goharbor/harbor/src/testing/mock"
)

func (r *replicationTestSuite) disableHuggingFace() {
	r.T().Setenv(huggingface.EnvKey, "")
	r.Require().NoError(os.Unsetenv(huggingface.EnvKey))
	config.InitWithSettings(map[string]any{huggingface.ConfigKey: false})
}

func (r *replicationTestSuite) TestCreatePolicyRejectsDisabledSourceType() {
	r.disableHuggingFace()
	err := r.createPullPolicy(model.RegistryTypeHuggingFace)
	r.True(errors.IsErr(err, errors.BadRequestCode))
	r.Contains(err.Error(), "registry type huggingface is disabled")
	r.repMgr.AssertNotCalled(r.T(), "Create", mock.Anything, mock.Anything)
}

func (r *replicationTestSuite) TestStartRejectsDisabledSourceType() {
	r.disableHuggingFace()
	_, err := r.ctl.Start(context.Background(), &repctlmodel.Policy{
		Enabled:     true,
		SrcRegistry: &model.Registry{ID: 5, Type: model.RegistryTypeHuggingFace},
	}, nil, task.ExecutionTriggerManual)
	r.True(errors.IsErr(err, errors.BadRequestCode))
	r.execMgr.AssertNotCalled(r.T(), "Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
