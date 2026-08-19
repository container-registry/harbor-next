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

package cargo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/pkg/permission/types"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

func TestHandlerPublishIndexAndDownload(t *testing.T) {
	store := &memoryStore{entries: map[string][]indexEntry{}, crates: map[string][]byte{}}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	crate := cargoCrate(t, map[string]string{"demo-1.0.0/Cargo.toml": `[package]
name = "demo"
version = "1.0.0"
`})
	body := cargoPublishBody(t, publishedCrateMetadata{
		Name:    "Demo",
		Version: "1.0.0+local",
		Deps: []publishedDependency{{
			Name:            "serde",
			VersionReq:      "^1",
			DefaultFeatures: true,
			Kind:            "normal",
		}},
		Features: map[string][]string{},
	}, crate)

	publish := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/cargo/library/api/v1/crates/new", bytes.NewReader(body))
	handler.ServeHTTP(publish, withSecurity(req))
	if publish.Code != http.StatusOK {
		t.Fatalf("publish status = %d, want %d: %s", publish.Code, http.StatusOK, publish.Body.String())
	}

	config := httptest.NewRecorder()
	handler.ServeHTTP(config, withSecurity(httptest.NewRequest(http.MethodGet, "/cargo/library/config.json", nil)))
	if config.Code != http.StatusOK || !strings.Contains(config.Body.String(), "/cargo/library/api/v1/crates/{crate}/{version}/download") {
		t.Fatalf("config = %d %s", config.Code, config.Body.String())
	}

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, withSecurity(httptest.NewRequest(http.MethodGet, "/cargo/library/de/mo/demo", nil)))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), `"vers":"1.0.0+local"`) {
		t.Fatalf("index = %d %s", index.Code, index.Body.String())
	}

	download := httptest.NewRecorder()
	handler.ServeHTTP(download, withSecurity(httptest.NewRequest(http.MethodGet, "/cargo/library/api/v1/crates/demo/1.0.0+local/download", nil)))
	if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), crate) {
		t.Fatalf("download = %d len %d, want crate len %d", download.Code, download.Body.Len(), len(crate))
	}
}

func TestHandlerConvertsPublishedDependencyToIndexFormat(t *testing.T) {
	store := &memoryStore{entries: map[string][]indexEntry{}, crates: map[string][]byte{}}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	metadata := map[string]any{
		"name": "demo",
		"vers": "1.0.0",
		"deps": []map[string]any{{
			"name":             "itoa",
			"version_req":      "^1",
			"features":         []string{},
			"optional":         false,
			"default_features": true,
			"target":           nil,
			"kind":             "normal",
			"registry":         nil,
		}},
		"features": map[string][]string{},
	}
	body := cargoPublishBody(t, metadata, cargoCrate(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	}))

	publish := httptest.NewRecorder()
	handler.ServeHTTP(publish, withSecurity(httptest.NewRequest(http.MethodPut, "/cargo/library/api/v1/crates/new", bytes.NewReader(body))))
	if publish.Code != http.StatusOK {
		t.Fatalf("publish status = %d, want %d: %s", publish.Code, http.StatusOK, publish.Body.String())
	}

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, withSecurity(httptest.NewRequest(http.MethodGet, "/cargo/library/de/mo/demo", nil)))
	if !strings.Contains(index.Body.String(), `"req":"^1"`) {
		t.Fatalf("index dependency requirement missing: %s", index.Body.String())
	}
}

func TestHandlerYanksAndUnyanksPublishedVersion(t *testing.T) {
	store := &memoryStore{entries: map[string][]indexEntry{}, crates: map[string][]byte{}}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	body := cargoPublishBody(t, publishedCrateMetadata{
		Name:     "demo",
		Version:  "1.0.0",
		Features: map[string][]string{},
	}, cargoCrate(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	}))
	publish := httptest.NewRecorder()
	handler.ServeHTTP(publish, withSecurity(httptest.NewRequest(http.MethodPut, "/cargo/library/api/v1/crates/new", bytes.NewReader(body))))
	if publish.Code != http.StatusOK {
		t.Fatalf("publish status = %d, want %d: %s", publish.Code, http.StatusOK, publish.Body.String())
	}

	yank := httptest.NewRecorder()
	handler.ServeHTTP(yank, withSecurity(httptest.NewRequest(http.MethodDelete, "/cargo/library/api/v1/crates/demo/1.0.0/yank", nil)))
	if yank.Code != http.StatusOK {
		t.Fatalf("yank status = %d, want %d: %s", yank.Code, http.StatusOK, yank.Body.String())
	}
	assertCargoIndexYanked(t, handler, true)

	unyank := httptest.NewRecorder()
	handler.ServeHTTP(unyank, withSecurity(httptest.NewRequest(http.MethodPut, "/cargo/library/api/v1/crates/demo/1.0.0/unyank", nil)))
	if unyank.Code != http.StatusOK {
		t.Fatalf("unyank status = %d, want %d: %s", unyank.Code, http.StatusOK, unyank.Body.String())
	}
	assertCargoIndexYanked(t, handler, false)
}

func assertCargoIndexYanked(t *testing.T, handler http.Handler, want bool) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, withSecurity(httptest.NewRequest(http.MethodGet, "/cargo/library/de/mo/demo", nil)))
	if response.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), fmt.Sprintf(`"yanked":%t`, want)) {
		t.Fatalf("index yanked state = %s, want %t", response.Body.String(), want)
	}
}

func TestIndexPath(t *testing.T) {
	tests := map[string]string{
		"a":     "1/a",
		"ab":    "2/ab",
		"abc":   "3/a/abc",
		"demo":  "de/mo/demo",
		"Serde": "se/rd/serde",
	}
	for name, want := range tests {
		if got := indexPath(name); got != want {
			t.Fatalf("indexPath(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCargoDownloadURL(t *testing.T) {
	tests := []struct {
		name     string
		template string
		crate    string
		version  string
		checksum string
		want     string
		ok       bool
	}{
		{
			name:     "crates.io markerless base",
			template: "https://static.crates.io/crates",
			crate:    "serde",
			version:  "1.0.0",
			want:     "https://static.crates.io/crates/serde/1.0.0/download",
			ok:       true,
		},
		{
			name:     "all standard markers",
			template: "https://example.test/{prefix}/{lowerprefix}/{crate}/{version}/{sha256-checksum}",
			crate:    "Serde",
			version:  "1.0.0+meta",
			checksum: "abc123",
			want:     "https://example.test/Se/rd/se/rd/Serde/1.0.0+meta/abc123",
			ok:       true,
		},
		{
			name:     "unknown marker",
			template: "https://example.test/{unknown}/{crate}",
			crate:    "serde",
			version:  "1.0.0",
			ok:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cargoDownloadURL(tt.template, tt.crate, tt.version, tt.checksum)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("cargoDownloadURL() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPublicProjectAllowsAnonymousPull(t *testing.T) {
	store := &memoryStore{entries: map[string][]indexEntry{}, crates: map[string][]byte{}}
	handler := newHandlerWithDeps(store, func(context.Context, string) (int64, error) { return 7, nil })
	req := httptest.NewRequest(http.MethodGet, "/cargo/public/config.json", nil)
	req = req.WithContext(security.NewContext(req.Context(), anonymousSecurityContext{}))
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"auth-required":false`) {
		t.Fatalf("anonymous Cargo config = %d %s", result.Code, result.Body.String())
	}
}

func TestHandlerRejectsPublishWhenProxyPushIsDisabled(t *testing.T) {
	handler := newHandlerWithDeps(&memoryStore{}, func(context.Context, string) (int64, error) { return 7, nil })
	handler.authorizePush = func(context.Context, string, string) error { return errors.New("push disabled") }

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, withSecurity(httptest.NewRequest(http.MethodPut, "/cargo/proxy/api/v1/crates/new", nil)))

	if response.Code != http.StatusForbidden {
		t.Fatalf("publish status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func cargoPublishBody(t *testing.T, metadata any, crate []byte) []byte {
	t.Helper()
	meta, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4+len(meta)+4+len(crate))
	binary.LittleEndian.PutUint32(body[:4], uint32(len(meta)))
	copy(body[4:], meta)
	offset := 4 + len(meta)
	binary.LittleEndian.PutUint32(body[offset:offset+4], uint32(len(crate)))
	copy(body[offset+4:], crate)
	return body
}

func cargoCrate(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o644}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type memoryStore struct {
	entries map[string][]indexEntry
	crates  map[string][]byte
}

func (s *memoryStore) Publish(_ context.Context, _ int64, project string, publish publishRequest) error {
	name := normalizeCrate(publish.Metadata.Name)
	sum := sha256.Sum256(publish.Content)
	key := project + "/" + name
	s.entries[key] = append(s.entries[key], indexEntry{
		Name:     name,
		Version:  publish.Metadata.Version,
		Deps:     publish.Metadata.Deps,
		Checksum: hex.EncodeToString(sum[:]),
		Features: publish.Metadata.Features,
		Yanked:   false,
	})
	s.crates[key+"/"+publish.Metadata.Version] = append([]byte(nil), publish.Content...)
	return nil
}

func (s *memoryStore) Index(_ context.Context, project, name string) ([]indexEntry, error) {
	entries := s.entries[project+"/"+name]
	if len(entries) == 0 {
		return nil, errors.New("not found")
	}
	return entries, nil
}

func (s *memoryStore) OpenCrate(_ context.Context, project, name, version string) (*pkgstore.Content, error) {
	content, ok := s.crates[project+"/"+name+"/"+version]
	if !ok {
		return nil, errors.New("not found")
	}
	return &pkgstore.Content{Size: int64(len(content)), Body: io.NopCloser(bytes.NewReader(content))}, nil
}

func (s *memoryStore) SetYanked(_ context.Context, _ int64, project, name, version string, yanked bool) error {
	key := project + "/" + name
	for i := range s.entries[key] {
		if s.entries[key][i].Version == version {
			s.entries[key][i].Yanked = yanked
			return nil
		}
	}
	return errors.New("not found")
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
