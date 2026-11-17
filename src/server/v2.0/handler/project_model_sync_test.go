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

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/controller/registry"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	registrytesting "github.com/goharbor/harbor/src/testing/controller/registry"
	"github.com/goharbor/harbor/src/testing/mock"
)

func TestValidateProjectReqGatesProxyCacheRegistryType(t *testing.T) {
	t.Setenv("PERMITTED_REGISTRY_TYPES_FOR_PROXY_CACHE", model.RegistryTypeHuggingFace)
	ctl := &registrytesting.Controller{}
	mock.OnAnything(ctl, "Get").Return(&model.Registry{ID: 9, Type: model.RegistryTypeHuggingFace}, nil)
	old := registry.Ctl
	registry.Ctl = ctl
	defer func() { registry.Ctl = old }()

	id := int64(9)
	req := &models.ProjectReq{ProjectName: "hf", RegistryID: &id, Metadata: &models.ProjectMetadata{}}

	t.Setenv("HARBOR_ENABLE_MODEL_SYNC", "false")
	err := (&projectAPI{}).validateProjectReq(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.IsErr(err, errors.ForbiddenCode))

	t.Setenv("HARBOR_ENABLE_MODEL_SYNC", "true")
	assert.NoError(t, (&projectAPI{}).validateProjectReq(context.Background(), req))
}
