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

package pypi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

func TestHandlerUploadSimpleAndDownload(t *testing.T) {
	store := &memoryStore{packages: map[string]*storedPackage{}, content: map[string][]byte{}}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	body, contentType := pypiUploadBody(t, map[string]string{
		":action":         "file_upload",
		"name":            "Demo_Pkg",
		"version":         "1.0.0+local",
		"summary":         "demo",
		"requires_python": ">=3.11",
		"requires_dist":   "requests>=2",
	}, "demo_pkg-1.0.0+local-py3-none-any.whl", []byte("wheel-content"))

	upload := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pypi/library/", body)
	req.Header.Set("Content-Type", contentType)
	handler.ServeHTTP(upload, withSecurity(req))
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d: %s", upload.Code, http.StatusCreated, upload.Body.String())
	}

	simple := httptest.NewRecorder()
	handler.ServeHTTP(simple, withSecurity(httptest.NewRequest(http.MethodGet, "/pypi/library/simple/demo-pkg/", nil)))
	if simple.Code != http.StatusOK {
		t.Fatalf("simple status = %d, want %d: %s", simple.Code, http.StatusOK, simple.Body.String())
	}
	if !strings.Contains(simple.Body.String(), "/pypi/library/packages/demo-pkg/1.0.0+local/demo_pkg-1.0.0+local-py3-none-any.whl#sha256=") {
		t.Fatalf("simple body missing distribution link: %s", simple.Body.String())
	}
	if !strings.Contains(simple.Body.String(), `data-requires-python="&gt;=3.11"`) {
		t.Fatalf("simple body missing requires-python: %s", simple.Body.String())
	}
	if !strings.Contains(simple.Body.String(), `data-core-metadata="sha256=`) {
		t.Fatalf("simple body missing core metadata attribute: %s", simple.Body.String())
	}

	root := httptest.NewRecorder()
	handler.ServeHTTP(root, withSecurity(httptest.NewRequest(http.MethodGet, "/pypi/library/simple/", nil)))
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), `<a href="demo-pkg/">demo-pkg</a>`) {
		t.Fatalf("root = %d %q, want package listing", root.Code, root.Body.String())
	}

	rootJSON := httptest.NewRecorder()
	rootJSONReq := httptest.NewRequest(http.MethodGet, "/pypi/library/simple/", nil)
	rootJSONReq.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	handler.ServeHTTP(rootJSON, withSecurity(rootJSONReq))
	if rootJSON.Code != http.StatusOK {
		t.Fatalf("root json status = %d, want %d: %s", rootJSON.Code, http.StatusOK, rootJSON.Body.String())
	}
	var rootResp simpleRootResponse
	if err := json.Unmarshal(rootJSON.Body.Bytes(), &rootResp); err != nil {
		t.Fatal(err)
	}
	if len(rootResp.Projects) != 1 || rootResp.Projects[0].Name != "demo-pkg" || rootResp.Meta.APIVersion != "1.1" {
		t.Fatalf("root json = %+v", rootResp)
	}

	jsonIndex := httptest.NewRecorder()
	jsonReq := httptest.NewRequest(http.MethodGet, "/pypi/library/simple/demo-pkg/", nil)
	jsonReq.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	handler.ServeHTTP(jsonIndex, withSecurity(jsonReq))
	if jsonIndex.Code != http.StatusOK {
		t.Fatalf("json index status = %d, want %d: %s", jsonIndex.Code, http.StatusOK, jsonIndex.Body.String())
	}
	if got := jsonIndex.Header().Get("Content-Type"); got != simpleAPIJSONContentType {
		t.Fatalf("json content type = %q, want %q", got, simpleAPIJSONContentType)
	}
	var simpleJSON simpleProjectResponse
	if err := json.Unmarshal(jsonIndex.Body.Bytes(), &simpleJSON); err != nil {
		t.Fatal(err)
	}
	if simpleJSON.Meta.APIVersion != "1.1" || simpleJSON.Name != "demo-pkg" {
		t.Fatalf("json index = %+v", simpleJSON)
	}
	if len(simpleJSON.Versions) != 1 || simpleJSON.Versions[0] != "1.0.0+local" {
		t.Fatalf("json versions = %v", simpleJSON.Versions)
	}
	if len(simpleJSON.Files) != 1 || simpleJSON.Files[0].Size != int64(len("wheel-content")) || simpleJSON.Files[0].CoreMetadata["sha256"] == "" || simpleJSON.Files[0].UploadTime == "" {
		t.Fatalf("json files = %+v", simpleJSON.Files)
	}

	download := httptest.NewRecorder()
	handler.ServeHTTP(download, withSecurity(httptest.NewRequest(http.MethodGet, "/pypi/library/packages/demo-pkg/1.0.0+local/demo_pkg-1.0.0+local-py3-none-any.whl", nil)))
	if download.Code != http.StatusOK || download.Body.String() != "wheel-content" {
		t.Fatalf("download = %d %q, want 200 wheel-content", download.Code, download.Body.String())
	}

	metadata := httptest.NewRecorder()
	handler.ServeHTTP(metadata, withSecurity(httptest.NewRequest(http.MethodGet, "/pypi/library/packages/demo-pkg/1.0.0+local/demo_pkg-1.0.0+local-py3-none-any.whl.metadata", nil)))
	if metadata.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, want %d: %s", metadata.Code, http.StatusOK, metadata.Body.String())
	}
	if !strings.Contains(metadata.Body.String(), "Name: Demo_Pkg\n") || !strings.Contains(metadata.Body.String(), "Requires-Dist: requests>=2\n") {
		t.Fatalf("metadata body = %q", metadata.Body.String())
	}
}

func TestCoreMetadataFromWheelPreservesProvidesExtra(t *testing.T) {
	var wheel bytes.Buffer
	writer := zip.NewWriter(&wheel)
	metadataFile, err := writer.Create("demo-1.0.0.dist-info/METADATA")
	if err != nil {
		t.Fatal(err)
	}
	want := "Metadata-Version: 2.4\nName: demo\nVersion: 1.0.0\nProvides-Extra: cli\nRequires-Dist: click; extra == 'cli'\n\n"
	if _, err := metadataFile.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	got := coreMetadataFromUpload(uploadRequest{
		Name:     "demo",
		Version:  "1.0.0",
		Filename: "demo-1.0.0-py3-none-any.whl",
		Content:  wheel.Bytes(),
	})
	if string(got) != want {
		t.Fatalf("core metadata = %q, want wheel METADATA %q", got, want)
	}
}

func TestParseRequestNormalizesSimpleName(t *testing.T) {
	req, err := parseRequest(http.MethodGet, "/pypi/library/simple/Demo_Pkg/")
	if err != nil {
		t.Fatal(err)
	}
	if req.Project != "library" || req.Package != "demo-pkg" || req.Type != requestSimple {
		t.Fatalf("parseRequest() = %+v", req)
	}
}

func TestParseRequestRejectsEncodedSlash(t *testing.T) {
	if _, err := parseRequest(http.MethodGet, "/pypi/library/simple/demo%2Fnested/"); err == nil {
		t.Fatal("parseRequest accepted an encoded slash in a package name")
	}
}

func TestPublicProjectAllowsAnonymousPull(t *testing.T) {
	store := &memoryStore{
		packages: map[string]*storedPackage{"public/demo": {Name: "demo"}},
		content:  map[string][]byte{},
	}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	req := httptest.NewRequest(http.MethodGet, "/pypi/public/simple/demo/", nil)
	req = req.WithContext(security.NewContext(req.Context(), anonymousSecurityContext{}))
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	if result.Code != http.StatusOK {
		t.Fatalf("anonymous public pull status = %d, want %d: %s", result.Code, http.StatusOK, result.Body.String())
	}
}

func TestHandlerRejectsPublishWhenProxyPushIsDisabled(t *testing.T) {
	handler := newHandlerWithDeps(&memoryStore{}, func(context.Context, string) (int64, error) { return 7, nil })
	handler.authorizePush = func(context.Context, string, string) error { return errors.New("push disabled") }

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, withSecurity(httptest.NewRequest(http.MethodPost, "/pypi/proxy/", nil)))

	if response.Code != http.StatusForbidden {
		t.Fatalf("publish status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestProxySimpleKeepsUpstreamVersionsVisibleAfterOneVersionIsCached(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/simple/demo/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", simpleAPIJSONContentType)
		_, _ = w.Write([]byte(`{"meta":{"api-version":"1.1"},"name":"demo","versions":["1.0.0","2.0.0"],"files":[{"filename":"demo-1.0.0-py3-none-any.whl","url":"https://files.example/demo-1.0.0-py3-none-any.whl","hashes":{"sha256":"one"}},{"filename":"demo-2.0.0-py3-none-any.whl","url":"https://files.example/demo-2.0.0-py3-none-any.whl","hashes":{"sha256":"two"}}]}`))
	}))
	defer upstream.Close()

	store := &memoryStore{
		packages: map[string]*storedPackage{"proxy/demo": {
			Name: "demo",
			Versions: []storedVersion{{
				Version: "1.0.0",
				Distributions: []distributionMetadata{{
					Filename: "demo-1.0.0-py3-none-any.whl",
				}},
			}},
		}},
		content: map[string][]byte{},
	}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	handler.proxyProject = func(context.Context, string, string) (*pkgproxy.Proxy, error) {
		return pkgproxy.New(&proModels.Project{}, &regmodel.Registry{URL: upstream.URL}), nil
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pypi/proxy/simple/demo/", nil)
	request.Header.Set("Accept", simpleAPIJSONContentType)
	handler.ServeHTTP(response, withSecurity(request))

	if response.Code != http.StatusOK {
		t.Fatalf("simple status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"2.0.0"`) {
		t.Fatalf("upstream version hidden by partial local cache: %s", response.Body.String())
	}
}

func TestRewriteSimpleHTMLPreservesHash(t *testing.T) {
	payload := []byte(`<a href="../../files/demo-1.0.whl#sha256=abc123" data-core-metadata="sha256=def456">demo</a>`)
	got := string(rewriteSimpleHTML(payload, "https://harbor.test", "proxy", "demo"))
	if !strings.Contains(got, `/pypi/proxy/packages/demo/1.0/demo-1.0.whl#sha256=abc123`) {
		t.Fatalf("rewritten link lost its integrity hash: %s", got)
	}
	if !strings.Contains(got, `data-core-metadata="sha256=def456"`) {
		t.Fatalf("rewritten link lost its core metadata attribute: %s", got)
	}
}

func TestResolveUpstreamReference(t *testing.T) {
	proxy := pkgproxy.New(&proModels.Project{}, &regmodel.Registry{URL: "https://packages.example/repository/"})
	got, ok := resolveUpstreamReference(proxy, "simple/demo/", "../../files/demo-1.0.whl")
	if !ok || got != "https://packages.example/repository/files/demo-1.0.whl" {
		t.Fatalf("resolveUpstreamReference() = %q, %v", got, ok)
	}

	proxy.Registry.URL = "https://packages.example/simple/"
	if got := upstreamSimplePath(proxy, "demo"); got != "demo/" {
		t.Fatalf("upstreamSimplePath() = %q, want %q", got, "demo/")
	}
	parsed, err := url.Parse(got)
	if err != nil || parsed.Scheme != "https" {
		t.Fatalf("resolved URL is invalid: %q", got)
	}
}

func pypiUploadBody(t *testing.T, fields map[string]string, filename string, content []byte) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("content", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

type memoryStore struct {
	packages map[string]*storedPackage
	content  map[string][]byte
}

func (s *memoryStore) Publish(_ context.Context, _ int64, project string, upload uploadRequest) error {
	key := project + "/" + upload.NormalizedName
	sum := sha256.Sum256(upload.Content)
	coreMetadata := coreMetadataFromUpload(upload)
	metadataSum := sha256.Sum256(coreMetadata)
	pkg := s.packages[key]
	if pkg == nil {
		pkg = &storedPackage{Name: upload.NormalizedName}
		s.packages[key] = pkg
	}
	pkg.Versions = append(pkg.Versions, storedVersion{
		Version:        upload.Version,
		RequiresPython: upload.RequiresPython,
		Distributions: []distributionMetadata{{
			Filename:       upload.Filename,
			ContentType:    upload.ContentType,
			Size:           int64(len(upload.Content)),
			SHA256:         hex.EncodeToString(sum[:]),
			MetadataSHA256: hex.EncodeToString(metadataSum[:]),
			UploadedAt:     time.Now().UTC(),
		}},
	})
	s.content[key+"/"+upload.Version+"/"+upload.Filename] = append([]byte(nil), upload.Content...)
	s.content[key+"/"+upload.Version+"/"+upload.Filename+".metadata"] = append([]byte(nil), coreMetadata...)
	return nil
}

func (s *memoryStore) ListPackages(_ context.Context, project string) ([]string, error) {
	var packages []string
	for key, pkg := range s.packages {
		if strings.HasPrefix(key, project+"/") {
			packages = append(packages, pkg.Name)
		}
	}
	sort.Strings(packages)
	return packages, nil
}

func (s *memoryStore) Load(_ context.Context, project, name string) (*storedPackage, error) {
	pkg := s.packages[project+"/"+name]
	if pkg == nil {
		return nil, errors.New("not found")
	}
	return pkg, nil
}

func (s *memoryStore) OpenDistribution(_ context.Context, project, name, version, filename string) (*pkgstore.Content, error) {
	content, ok := s.content[project+"/"+name+"/"+version+"/"+filename]
	if !ok {
		return nil, errors.New("not found")
	}
	return &pkgstore.Content{Size: int64(len(content)), Body: io.NopCloser(bytes.NewReader(content))}, nil
}

func (s *memoryStore) OpenMetadata(_ context.Context, project, name, version, filename string) (*pkgstore.Content, error) {
	content, ok := s.content[project+"/"+name+"/"+version+"/"+filename+".metadata"]
	if !ok {
		return nil, errors.New("not found")
	}
	return &pkgstore.Content{Size: int64(len(content)), Body: io.NopCloser(bytes.NewReader(content))}, nil
}

type testSecurityContext struct{}

func (testSecurityContext) Name() string { return "test" }

func (testSecurityContext) IsAuthenticated() bool { return true }

func (testSecurityContext) GetUsername() string { return "admin" }

func (testSecurityContext) IsSysAdmin() bool { return true }

func (testSecurityContext) IsSolutionUser() bool { return false }

func (testSecurityContext) Can(_ context.Context, action types.Action, resource types.Resource) bool {
	return (action == rbac.ActionPull || action == rbac.ActionPush) && strings.Contains(string(resource), string(rbac.ResourceRepository))
}

func withSecurity(req *http.Request) *http.Request {
	return req.WithContext(security.NewContext(req.Context(), testSecurityContext{}))
}

type anonymousSecurityContext struct{ testSecurityContext }

func (anonymousSecurityContext) IsAuthenticated() bool { return false }

func (anonymousSecurityContext) GetUsername() string { return "" }
