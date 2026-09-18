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
// limitations under the License

package artifactinfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestParseURL(t *testing.T) {
	cases := []struct {
		input  string
		expect map[string]string
		match  bool
		rc     string
	}{
		{
			input:  "/api/projects",
			expect: map[string]string{},
			match:  false,
		},
		{
			input:  "/v2/_catalog",
			expect: map[string]string{},
			match:  false,
		},
		{
			input: "/v2/no-project-repo/tags/list",
			expect: map[string]string{
				lib.RepositorySubexp: "no-project-repo",
			},
			match: true,
		},
		{
			input: "/v2/development/golang/manifests/sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
			expect: map[string]string{
				lib.RepositorySubexp: "development/golang",
				lib.ReferenceSubexp:  "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
				lib.DigestSubexp:     "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
			},
			match: true,
		},
		{
			input: "/v2/development/golang/manifests/shaxxx:**********************************************************************************************************************************",

			expect: map[string]string{
				lib.RepositorySubexp: "development/golang",
				lib.ReferenceSubexp:  "shaxxx:**********************************************************************************************************************************",
				"tag":                "shaxxx:**********************************************************************************************************************************",
			},
			match: true,
		},
		{
			input: "/v2/multi/sector/repository/blobs/sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
			expect: map[string]string{
				lib.RepositorySubexp: "multi/sector/repository",
				lib.DigestSubexp:     "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
			},
			match: true,
		},
		{
			input:  "/v2/blobs/uploads",
			expect: map[string]string{},
			match:  false,
		},
		{
			input: "/v2/library/ubuntu/blobs/uploads",
			expect: map[string]string{
				lib.RepositorySubexp: "library/ubuntu",
			},
			match: true,
		},
		{
			input: "/v2/library/ubuntu/blobs/uploads/?mount=sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f&from=old/ubuntu",
			expect: map[string]string{
				lib.RepositorySubexp: "library/ubuntu",
				blobMountDigest:      "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
				blobMountRepo:        "old/ubuntu",
			},
			match: true,
		},
		{
			input: "/v2/library/centos/blobs/uploads/u-12345",
			expect: map[string]string{
				lib.RepositorySubexp: "library/centos",
			},
			match: true,
		},
		{
			input:  "/v2/library/centos/manifest/.Invalid",
			expect: map[string]string{},
			match:  false,
			rc:     errors.NotFoundCode,
		},
	}

	for _, c := range cases {
		url, err := url.Parse(c.input)
		if err != nil {
			panic(err)
		}
		e, m, err := parse(url)
		if err != nil {
			assert.True(t, errors.IsErr(err, c.rc))
		}
		assert.Equal(t, c.match, m)
		assert.Equal(t, c.expect, e)
	}
}

type handler struct {
	ctx context.Context
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	h.ctx = req.Context()
}
func TestPopulateArtifactInfo(t *testing.T) {

	none := lib.ArtifactInfo{}
	cases := []struct {
		req *http.Request
		sc  int
		art lib.ArtifactInfo
	}{
		{
			req: httptest.NewRequest(http.MethodDelete, "/v2/hello-world/manifests/latest", nil),
			sc:  http.StatusBadRequest,
			art: none,
		},
		{
			req: httptest.NewRequest(http.MethodDelete, "/v2/library/hello-world/manifests/latest", nil),
			sc:  http.StatusOK,
			art: lib.ArtifactInfo{
				Repository:  "library/hello-world",
				Reference:   "latest",
				ProjectName: "library",
				Tag:         "latest",
			},
		},
		{
			req: httptest.NewRequest(http.MethodPost, "/v2/library/ubuntu/blobs/uploads/?mount=sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f&from=no-project", nil),
			sc:  http.StatusBadRequest,
			art: none,
		},
		{
			req: httptest.NewRequest(http.MethodPost, "/v2/library/ubuntu/blobs/uploads/?from=old/ubuntu&mount=sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f", nil),
			sc:  http.StatusOK,
			art: lib.ArtifactInfo{
				Repository:           "library/ubuntu",
				ProjectName:          "library",
				BlobMountRepository:  "old/ubuntu",
				BlobMountDigest:      "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
				BlobMountProjectName: "old",
			},
		},
		{
			req: httptest.NewRequest(http.MethodDelete, "/v2/library/hello-world/manifests/sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f", nil),
			sc:  http.StatusOK,
			art: lib.ArtifactInfo{
				Repository:  "library/hello-world",
				Reference:   "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
				Digest:      "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f",
				ProjectName: "library",
			},
		},
	}
	next := &handler{}

	for _, tt := range cases {
		rec := httptest.NewRecorder()

		Middleware()(next).ServeHTTP(rec, tt.req)
		assert.Equal(t, tt.sc, rec.Code)
		if tt.art != none {
			a := lib.GetArtifactInfo(next.ctx)
			assert.NotEqual(t, none, a)
			assert.Equal(t, tt.art, a)
		}
	}
}

func TestDefaultProjectRewrite(t *testing.T) {
	prev := config.DefaultCfgManager
	config.InitWithSettings(map[string]any{common.DefaultProjectName: "library"})
	defer func() {
		// the inmemory manager is a shared singleton — restore its value too
		_ = config.DefaultMgr().UpdateConfig(context.Background(), map[string]any{common.DefaultProjectName: "library"})
		config.DefaultCfgManager = prev
	}()

	next := &handler{}
	mw := Middleware()(next)

	// bare manifest reference lands in the default project
	req := httptest.NewRequest(http.MethodPut, "/v2/hello-world/manifests/latest", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/v2/library/hello-world/manifests/latest", req.URL.Path)
	art := lib.GetArtifactInfo(next.ctx)
	assert.Equal(t, "library/hello-world", art.Repository)
	assert.Equal(t, "library", art.ProjectName)
	assert.Equal(t, "latest", art.Tag)

	// bare blob upload
	req = httptest.NewRequest(http.MethodPost, "/v2/busybox/blobs/uploads/", nil)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/v2/library/busybox/blobs/uploads/", req.URL.Path)
	assert.Equal(t, "library/busybox", lib.GetArtifactInfo(next.ctx).Repository)

	// bare blob mount source is qualified as well
	req = httptest.NewRequest(http.MethodPost, "/v2/library/ubuntu/blobs/uploads/?mount=sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f&from=no-project", nil)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	art = lib.GetArtifactInfo(next.ctx)
	assert.Equal(t, "library/no-project", art.BlobMountRepository)
	assert.Equal(t, "library", art.BlobMountProjectName)
	assert.Equal(t, "library/no-project", req.URL.Query().Get("from"))

	// qualified repository is untouched
	req = httptest.NewRequest(http.MethodDelete, "/v2/dev/img/manifests/latest", nil)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/v2/dev/img/manifests/latest", req.URL.Path)
	assert.Equal(t, "dev/img", lib.GetArtifactInfo(next.ctx).Repository)

	// empty setting disables the rewrite and restores the legacy rejection
	assert.NoError(t, config.DefaultMgr().UpdateConfig(context.Background(), map[string]any{common.DefaultProjectName: ""}))
	req = httptest.NewRequest(http.MethodPut, "/v2/hello-world/manifests/latest", nil)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "/v2/hello-world/manifests/latest", req.URL.Path)
}
