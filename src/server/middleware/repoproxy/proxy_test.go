//  Copyright Project Harbor Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package repoproxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/local"
	"github.com/goharbor/harbor/src/common/security/proxycachesecret"
	securitySecret "github.com/goharbor/harbor/src/common/security/secret"
	"github.com/goharbor/harbor/src/controller/project"
	registryCtl "github.com/goharbor/harbor/src/controller/registry"
	"github.com/goharbor/harbor/src/lib"
	liberrors "github.com/goharbor/harbor/src/lib/errors"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	registrytesting "github.com/goharbor/harbor/src/testing/controller/registry"
	testingmock "github.com/goharbor/harbor/src/testing/mock"
)

func TestIsProxySession(t *testing.T) {
	sc1 := securitySecret.NewSecurityContext("123456789", nil)
	otherCtx := security.NewContext(context.Background(), sc1)

	sc2 := proxycachesecret.NewSecurityContext("library/hello-world")
	proxyCtx := security.NewContext(context.Background(), sc2)

	user := &models.User{
		Username: "robot$library+scanner-8ec3b47a-fd29-11ee-9681-0242c0a87009",
	}
	userSc := local.NewSecurityContext(user)
	scannerCtx := security.NewContext(context.Background(), userSc)

	otherRobot := &models.User{
		Username: "robot$library+test-8ec3b47a-fd29-11ee-9681-0242c0a87009",
	}
	userSc2 := local.NewSecurityContext(otherRobot)
	nonScannerCtx := security.NewContext(context.Background(), userSc2)

	cases := []struct {
		name string
		in   context.Context
		want bool
	}{
		{
			name: `normal`,
			in:   otherCtx,
			want: false,
		},
		{
			name: `proxy user`,
			in:   proxyCtx,
			want: true,
		},
		{
			name: `robot account`,
			in:   scannerCtx,
			want: true,
		},
		{
			name: `non scanner robot`,
			in:   nonScannerCtx,
			want: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := isProxySession(tt.in, "library")
			if got != tt.want {
				t.Errorf(`(%v) = %v; want "%v"`, tt.in, got, tt.want)
			}
		})
	}
}

func TestServeBlob(t *testing.T) {
	const dig = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	t.Run("complete blob is sent with Content-Length and is not chunked", func(t *testing.T) {
		// Larger than net/http's 2048-byte pre-chunking buffer, which forces
		// chunked encoding when Content-Length is not set up front.
		body := bytes.Repeat([]byte("a"), 4096)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if err := serveBlob(w, bytes.NewReader(body), int64(len(body)), dig); err != nil {
				t.Errorf("serveBlob returned error: %v", err)
			}
		}))
		defer srv.Close()

		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.ContentLength != int64(len(body)) {
			t.Errorf("Content-Length = %d, want %d", resp.ContentLength, len(body))
		}
		if len(resp.TransferEncoding) != 0 {
			t.Errorf("response is chunked (Transfer-Encoding=%v), want a fixed Content-Length", resp.TransferEncoding)
		}
		if got := resp.Header.Get("Docker-Content-Digest"); got != dig {
			t.Errorf("Docker-Content-Digest = %q, want %q", got, dig)
		}
		if got := resp.Header.Get("Etag"); got != dig {
			t.Errorf("Etag = %q, want %q", got, dig)
		}
		got, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if !bytes.Equal(got, body) {
			t.Errorf("body mismatch: got %d bytes, want %d", len(got), len(body))
		}
	})

	t.Run("truncated upstream read is rejected as a short body", func(t *testing.T) {
		body := bytes.Repeat([]byte("a"), 4096)
		const declared = 8192
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if err := serveBlob(w, bytes.NewReader(body), declared, dig); err != nil {
				t.Errorf("serveBlob returned error %v for a short read; a committed partial response must not error", err)
			}
		}))
		defer srv.Close()

		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.ContentLength != declared {
			t.Errorf("Content-Length = %d, want %d", resp.ContentLength, declared)
		}
		if _, err := io.ReadAll(resp.Body); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("reading truncated body: got err %v, want io.ErrUnexpectedEOF", err)
		}
	})

	t.Run("upstream failure before the first byte clears the blob headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		err := serveBlob(rec, &errReader{err: io.ErrUnexpectedEOF}, 4096, dig)
		if err == nil {
			t.Fatal("serveBlob returned nil error for a failing reader, want an error")
		}
		for _, h := range []string{"Content-Length", "Docker-Content-Digest", "Etag"} {
			if got := rec.Header().Get(h); got != "" {
				t.Errorf("%s = %q after early failure, want it cleared", h, got)
			}
		}
	})

	t.Run("upstream failure after a partial write does not append an error body", func(t *testing.T) {
		const size = 4096
		partial := bytes.Repeat([]byte("a"), 1024)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reader := io.MultiReader(bytes.NewReader(partial), &errReader{err: io.ErrUnexpectedEOF})
			if err := serveBlob(w, reader, size, dig); err != nil {
				t.Errorf("serveBlob returned error %v after a partial write, want nil", err)
			}
		}))
		defer srv.Close()

		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.ContentLength != size {
			t.Errorf("Content-Length = %d, want %d", resp.ContentLength, size)
		}
		body, readErr := io.ReadAll(resp.Body)
		if !errors.Is(readErr, io.ErrUnexpectedEOF) {
			t.Errorf("reading partial body: got err %v, want io.ErrUnexpectedEOF", readErr)
		}
		if !bytes.Equal(body, partial) {
			t.Errorf("body = %d bytes, want the %d streamed bytes with no error payload appended", len(body), len(partial))
		}
	})
}

// errReader fails on the first read without producing any bytes.
type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func TestMatchRepositoryFilter(t *testing.T) {
	cases := []struct {
		name          string
		repository    string
		filterPattern string
		filterKind    string
		want          bool
	}{
		{
			name:          "empty pattern matches all",
			repository:    "library/nginx",
			filterPattern: "",
			filterKind:    "",
			want:          true,
		},
		{
			name:          "regex exact match",
			repository:    "library/nginx",
			filterPattern: "^library/nginx$",
			filterKind:    "regex",
			want:          true,
		},
		{
			name:          "regex partial match is anchored",
			repository:    "library/nginx",
			filterPattern: "nginx",
			filterKind:    "regex",
			want:          false,
		},
		{
			name:          "regex prefix match is anchored",
			repository:    "library/nginx",
			filterPattern: "^library/",
			filterKind:    "regex",
			want:          false,
		},
		{
			name:          "regex wildcard match",
			repository:    "library/nginx",
			filterPattern: "^library/.*",
			filterKind:    "regex",
			want:          true,
		},
		{
			name:          "regex no match",
			repository:    "library/nginx",
			filterPattern: "^other/",
			filterKind:    "regex",
			want:          false,
		},
		{
			name:          "regex invalid pattern treated as no match",
			repository:    "library/nginx",
			filterPattern: "[invalid",
			filterKind:    "regex",
			want:          false,
		},
		{
			name:          "doublestar exact match",
			repository:    "library/nginx",
			filterPattern: "library/nginx",
			filterKind:    "doublestar",
			want:          true,
		},
		{
			name:          "doublestar single star",
			repository:    "library/nginx",
			filterPattern: "library/*",
			filterKind:    "doublestar",
			want:          true,
		},
		{
			name:          "doublestar double star",
			repository:    "org/team/repo",
			filterPattern: "org/**",
			filterKind:    "doublestar",
			want:          true,
		},
		{
			name:          "doublestar match all",
			repository:    "library/nginx",
			filterPattern: "**",
			filterKind:    "doublestar",
			want:          true,
		},
		{
			name:          "doublestar no match",
			repository:    "library/nginx",
			filterPattern: "other/*",
			filterKind:    "doublestar",
			want:          false,
		},
		{
			name:          "doublestar alternation",
			repository:    "library/nginx",
			filterPattern: "library/{nginx,alpine}",
			filterKind:    "doublestar",
			want:          true,
		},
		{
			name:          "empty kind defaults to doublestar",
			repository:    "library/nginx",
			filterPattern: "library/**",
			filterKind:    "",
			want:          true,
		},
		{
			name:          "empty pattern with kind set matches all",
			repository:    "library/nginx",
			filterPattern: "",
			filterKind:    "regex",
			want:          true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRepositoryFilter(tt.repository, tt.filterPattern, tt.filterKind)
			if got != tt.want {
				t.Errorf("matchRepositoryFilter(%q, %q, %q) = %v; want %v",
					tt.repository, tt.filterPattern, tt.filterKind, got, tt.want)
			}
		})
	}
}

// TestCheckRepositoryFilterAppliesToBlobAndManifest ensures the repository filter
// is enforced regardless of whether the request is for a manifest or a blob, so
// a blob digest known out-of-band can't be used to bypass the project's filter.
func TestCheckRepositoryFilterAppliesToBlobAndManifest(t *testing.T) {
	art := lib.ArtifactInfo{ProjectName: "proxy_project", Repository: "proxy_project/other/nginx"}

	cases := []struct {
		name      string
		metadata  map[string]string
		wantBlock bool
	}{
		{
			name:      "no filter configured",
			metadata:  map[string]string{},
			wantBlock: false,
		},
		{
			name:      "matching filter allows",
			metadata:  map[string]string{proModels.ProMetaProxyCacheFilterPattern: "other/**"},
			wantBlock: false,
		},
		{
			name:      "non-matching filter blocks",
			metadata:  map[string]string{proModels.ProMetaProxyCacheFilterPattern: "library/**"},
			wantBlock: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			p := &proModels.Project{Name: art.ProjectName, Metadata: tt.metadata}
			err := checkRepositoryFilter(p, art)
			if tt.wantBlock {
				if err == nil || !liberrors.IsNotFoundErr(err) {
					t.Errorf("checkRepositoryFilter() = %v; want NotFoundError", err)
				}
			} else if err != nil {
				t.Errorf("checkRepositoryFilter() = %v; want nil", err)
			}
		})
	}
}

func TestTagsListMiddlewareRepositoryFilter(t *testing.T) {
	// Mock project.Ctl
	originalProjectController := project.Ctl
	projectController := &projecttesting.Controller{}
	project.Ctl = projectController
	defer func() {
		project.Ctl = originalProjectController
	}()

	art := lib.ArtifactInfo{
		ProjectName: "proxy_project",
		Repository:  "proxy_project/other/nginx",
	}

	p := &proModels.Project{
		Name: art.ProjectName,
		Metadata: map[string]string{
			proModels.ProMetaProxyCacheFilterPattern: "library/**",
		},
	}

	testingmock.OnAnything(projectController, "GetByName").Return(p, nil)

	req := httptest.NewRequest("GET", "/v2/proxy_project/other/nginx/tags/list", nil)
	req = req.WithContext(lib.WithArtifactInfo(req.Context(), art))
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	TagsListMiddleware()(next).ServeHTTP(rec, req)

	if nextCalled {
		t.Error("expected next handler not to be called, but it was")
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestTagsListMiddlewareDefaultLibrary(t *testing.T) {
	// Mock project.Ctl
	originalProjectController := project.Ctl
	projectController := &projecttesting.Controller{}
	project.Ctl = projectController
	defer func() {
		project.Ctl = originalProjectController
	}()

	// Mock registry.Ctl
	originalRegistryController := registryCtl.Ctl
	registryController := &registrytesting.Controller{}
	registryCtl.Ctl = registryController
	defer func() {
		registryCtl.Ctl = originalRegistryController
	}()

	art := lib.ArtifactInfo{
		ProjectName: "proxy_project",
		Repository:  "proxy_project/nginx",
	}

	p := &proModels.Project{
		Name:       art.ProjectName,
		RegistryID: 1,
	}

	reg := &model.Registry{
		ID:   1,
		Type: model.RegistryTypeDockerHub,
	}

	testingmock.OnAnything(projectController, "GetByName").Return(p, nil)
	testingmock.OnAnything(registryController, "Get").Return(reg, nil)

	req := httptest.NewRequest("GET", "/v2/proxy_project/nginx/tags/list", nil)
	req = req.WithContext(lib.WithArtifactInfo(req.Context(), art))
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	TagsListMiddleware()(next).ServeHTTP(rec, req)

	if nextCalled {
		t.Error("expected next handler not to be called, but it was")
	}

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("expected status code %d, got %d", http.StatusMovedPermanently, rec.Code)
	}

	expectedLocation := "/v2/proxy_project/library/nginx/tags/list"
	if location := rec.Header().Get("Location"); location != expectedLocation {
		t.Errorf("expected Location header %q, got %q", expectedLocation, location)
	}
}
