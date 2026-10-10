// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	  http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package token

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/distribution/registry/auth/token"
	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestPullOnly(t *testing.T) {
	assert.True(t, pullOnly([]string{"pull"}))
	assert.False(t, pullOnly([]string{"pull", "push"}))
	assert.False(t, pullOnly([]string{"push"}))
	assert.False(t, pullOnly([]string{"delete"}))
	assert.False(t, pullOnly(nil))
}

func TestMirrorAccess(t *testing.T) {
	config.Init()
	config.DefaultMgr().Set(context.TODO(), common.RegistryMirrorEnabled, true)
	config.DefaultMgr().Set(context.TODO(), common.RegistryMirrorNamespaces, "docker.io=dockerhub")

	req := httptest.NewRequest(http.MethodGet, "/service/token?scope=repository:library/nginx:pull&ns=docker.io", nil)

	t.Run("a pull scope gains the routed repository", func(t *testing.T) {
		extra := mirrorAccess(req, []*token.ResourceActions{
			{Type: "repository", Name: "library/nginx", Actions: []string{"pull"}},
		})
		assert.Len(t, extra, 1)
		assert.Equal(t, "dockerhub/library/nginx", extra[0].Name)
		assert.Equal(t, []string{"pull"}, extra[0].Actions)
	})

	t.Run("a push scope is left to the project the client named", func(t *testing.T) {
		assert.Empty(t, mirrorAccess(req, []*token.ResourceActions{
			{Type: "repository", Name: "team/app", Actions: []string{"pull", "push"}},
		}))
	})

	t.Run("a non repository scope is untouched", func(t *testing.T) {
		assert.Empty(t, mirrorAccess(req, []*token.ResourceActions{
			{Type: "registry", Name: "catalog", Actions: []string{"*"}},
		}))
	})

	t.Run("nothing is added while the mirror is off", func(t *testing.T) {
		config.DefaultMgr().Set(context.TODO(), common.RegistryMirrorEnabled, false)
		defer config.DefaultMgr().Set(context.TODO(), common.RegistryMirrorEnabled, true)
		assert.Empty(t, mirrorAccess(req, []*token.ResourceActions{
			{Type: "repository", Name: "library/nginx", Actions: []string{"pull"}},
		}))
	})
}
