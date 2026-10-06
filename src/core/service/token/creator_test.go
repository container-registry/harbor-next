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
	"testing"

	"github.com/docker/distribution/registry/auth/token"
	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestRewriteBareRepositoryScopes(t *testing.T) {
	ctx := context.Background()
	prev := config.DefaultCfgManager
	config.InitWithSettings(map[string]any{common.DefaultProjectName: "library"})
	defer func() {
		// the inmemory manager is a shared singleton — restore its value too
		_ = config.DefaultMgr().UpdateConfig(ctx, map[string]any{common.DefaultProjectName: "library"})
		config.DefaultCfgManager = prev
	}()
	access := []*token.ResourceActions{
		{Type: "repository", Name: "alpine", Actions: []string{"pull", "push"}},
		{Type: "repository", Name: "myproj/alpine", Actions: []string{"pull"}},
		{Type: "registry", Name: "catalog", Actions: []string{"*"}},
		{Type: "repository", Name: "", Actions: []string{}},
	}
	rewriteBareRepositoryScopes(ctx, access)
	assert.Equal(t, "library/alpine", access[0].Name)
	assert.Equal(t, "myproj/alpine", access[1].Name)
	assert.Equal(t, "catalog", access[2].Name)
	assert.Equal(t, "", access[3].Name)

	assert.NoError(t, config.DefaultMgr().UpdateConfig(ctx, map[string]any{common.DefaultProjectName: ""}))
	access = []*token.ResourceActions{
		{Type: "repository", Name: "alpine", Actions: []string{"pull", "push"}},
	}
	rewriteBareRepositoryScopes(ctx, access)
	assert.Equal(t, "alpine", access[0].Name)
}
