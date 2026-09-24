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

package artifactinfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestRewriteMirrorRepository(t *testing.T) {
	config.Init()
	config.DefaultMgr().Set(context.TODO(), common.RegistryMirrorEnabled, true)
	config.DefaultMgr().Set(context.TODO(), common.RegistryMirrorNamespaces, "docker.io=dockerhub,*=dockerhub")

	cases := []struct {
		name     string
		method   string
		url      string
		repo     string
		wantRepo string
		wantOK   bool
		wantPath string
	}{
		{
			name:     "a manifest pull is routed",
			method:   http.MethodGet,
			url:      "/v2/library/nginx/manifests/latest?ns=docker.io",
			repo:     "library/nginx",
			wantRepo: "dockerhub/library/nginx",
			wantOK:   true,
			wantPath: "/v2/dockerhub/library/nginx/manifests/latest",
		},
		{
			name:     "a manifest HEAD is routed, it is how containerd resolves a tag",
			method:   http.MethodHead,
			url:      "/v2/library/nginx/manifests/latest?ns=docker.io",
			repo:     "library/nginx",
			wantRepo: "dockerhub/library/nginx",
			wantOK:   true,
			wantPath: "/v2/dockerhub/library/nginx/manifests/latest",
		},
		{
			name:     "a blob GET is routed",
			method:   http.MethodGet,
			url:      "/v2/library/nginx/blobs/sha256:cafe?ns=docker.io",
			repo:     "library/nginx",
			wantRepo: "dockerhub/library/nginx",
			wantOK:   true,
			wantPath: "/v2/dockerhub/library/nginx/blobs/sha256:cafe",
		},
		{
			name:     "a tag listing is routed",
			method:   http.MethodGet,
			url:      "/v2/library/nginx/tags/list?ns=docker.io",
			repo:     "library/nginx",
			wantRepo: "dockerhub/library/nginx",
			wantOK:   true,
			wantPath: "/v2/dockerhub/library/nginx/tags/list",
		},
		{
			name:     "a blob HEAD is left alone, a push opens with one",
			method:   http.MethodHead,
			url:      "/v2/team/app/blobs/sha256:cafe",
			repo:     "team/app",
			wantRepo: "team/app",
		},
		{
			name:     "a manifest PUT is left alone",
			method:   http.MethodPut,
			url:      "/v2/team/app/manifests/v1",
			repo:     "team/app",
			wantRepo: "team/app",
		},
		{
			name:     "a blob upload is left alone",
			method:   http.MethodPost,
			url:      "/v2/team/app/blobs/uploads/",
			repo:     "team/app",
			wantRepo: "team/app",
		},
		{
			name:     "a manifest delete is left alone",
			method:   http.MethodDelete,
			url:      "/v2/team/app/manifests/sha256:cafe",
			repo:     "team/app",
			wantRepo: "team/app",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.url, nil)
			before := req.URL.Path

			repo, ok := rewriteMirrorRepository(req, c.repo)

			assert.Equal(t, c.wantRepo, repo)
			assert.Equal(t, c.wantOK, ok)
			if c.wantOK {
				assert.Equal(t, c.wantPath, req.URL.Path)
			} else {
				assert.Equal(t, before, req.URL.Path)
			}
		})
	}
}
