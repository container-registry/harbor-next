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
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/goharbor/harbor/src/controller/project"
	registryCtl "github.com/goharbor/harbor/src/controller/registry"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	"github.com/goharbor/harbor/src/pkg/reg"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/huggingface"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	registrytesting "github.com/goharbor/harbor/src/testing/controller/registry"
	testingmock "github.com/goharbor/harbor/src/testing/mock"
	regtesting "github.com/goharbor/harbor/src/testing/pkg/reg"
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
		config.InitWithSettings(map[string]any{huggingface.ConfigKey: enabled})
		if got := canProxy(context.TODO(), p); got != enabled {
			t.Errorf("enabled=%v: canProxy = %v, want %v", enabled, got, enabled)
		}
	}
}

// The referrer API of a disabled registry type is served locally.
func TestProxyReferrerDisabledRegistryType(t *testing.T) {
	originalReg, originalProject := registryCtl.Ctl, project.Ctl
	regCtl, projectCtl := &registrytesting.Controller{}, &projecttesting.Controller{}
	registryCtl.Ctl, project.Ctl = regCtl, projectCtl
	defer func() { registryCtl.Ctl, project.Ctl = originalReg, originalProject }()
	// the remote helper reads the registry through reg.Mgr; it must not be reached
	originalMgr := reg.Mgr
	reg.Mgr = &regtesting.Manager{}
	defer func() { reg.Mgr = originalMgr }()
	testingmock.OnAnything(regCtl, "Get").Return(&model.Registry{ID: 1, Type: model.RegistryTypeHuggingFace, Status: model.Healthy}, nil)
	testingmock.OnAnything(projectCtl, "GetByName").Return(&proModels.Project{
		Name: "hf", RegistryID: 1,
		Metadata: map[string]string{proModels.ProMetaProxyReferrerAPI: "true"},
	}, nil)

	t.Setenv(huggingface.EnvKey, "false")
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/v2/hf/model/referrers/sha256:aaaa", nil)
	req = req.WithContext(lib.WithArtifactInfo(req.Context(), lib.ArtifactInfo{
		ProjectName: "hf", Repository: "hf/model", Reference: "sha256:aaaa", Digest: "sha256:aaaa",
	}))
	ProxyReferrerMiddleware()(next).ServeHTTP(httptest.NewRecorder(), req)

	if !nextCalled {
		t.Fatal("the referrer request of a disabled registry type must be served locally")
	}
}
