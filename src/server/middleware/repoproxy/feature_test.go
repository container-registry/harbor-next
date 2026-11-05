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

package repoproxy

import (
	"context"
	"os"
	"testing"

	registryCtl "github.com/goharbor/harbor/src/controller/registry"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/huggingface"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	registrytesting "github.com/goharbor/harbor/src/testing/controller/registry"
	testingmock "github.com/goharbor/harbor/src/testing/mock"
)

// A proxy cache project of a disabled registry type keeps serving what it has
// cached but never reaches the upstream.
func TestCanProxyDisabledRegistryType(t *testing.T) {
	original := registryCtl.Ctl
	ctl := &registrytesting.Controller{}
	registryCtl.Ctl = ctl
	defer func() { registryCtl.Ctl = original }()
	testingmock.OnAnything(ctl, "Get").Return(&model.Registry{ID: 1, Type: model.RegistryTypeHuggingFace, Status: model.Healthy}, nil)

	t.Setenv(huggingface.EnvKey, "")
	if err := os.Unsetenv(huggingface.EnvKey); err != nil {
		t.Fatal(err)
	}
	p := &proModels.Project{Name: "hf", RegistryID: 1}
	for _, enabled := range []bool{false, true} {
		config.InitWithSettings(map[string]any{metadata.EnableHuggingFaceModelImport: enabled})
		if got := canProxy(context.TODO(), p); got != enabled {
			t.Errorf("enabled=%v: canProxy = %v, want %v", enabled, got, enabled)
		}
	}
}
