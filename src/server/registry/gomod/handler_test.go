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

package gomod

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"

	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/gosum"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

func TestGoClientProxyWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Go client integration test in short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command is not available")
	}

	const (
		modulePath = "example.com/Acme/Hello"
		version    = "v1.2.3"
	)
	fixture := newModuleFixture(t, modulePath, version)
	var upstreamDisabled atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, contentType, ok := fixture.response(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if upstreamDisabled.Load() && strings.Contains(r.URL.Path, "/@v/") && !strings.HasSuffix(r.URL.Path, "/@v/list") {
			http.Error(w, "upstream disabled", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	store := newMemoryStore()
	proxy := pkgproxy.New(
		&proModels.Project{Name: "proxy"},
		&regmodel.Registry{URL: upstream.URL, Type: regmodel.RegistryTypeGo, Status: regmodel.Healthy},
	)
	harbor := httptest.NewServer(newHandlerWithDeps(
		store,
		func(http.ResponseWriter, *http.Request, string) (int64, bool) { return 1, true },
		func(context.Context, string, string) (*pkgproxy.Proxy, error) { return proxy, nil },
	))
	defer harbor.Close()

	clientEnv := newGoClientEnv(t, harbor.URL+"/go/proxy")
	download := runGo(t, t.TempDir(), clientEnv, "mod", "download", "-json", modulePath+"@"+version)
	require.Contains(t, download, `"Version": "`+version+`"`)

	consumer := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(consumer, "go.mod"), []byte("module example.com/consumer\n\ngo 1.22\n"), 0o600))
	runGo(t, consumer, clientEnv, "get", modulePath+"@latest")

	store.waitCached(t)
	upstreamDisabled.Store(true)

	cachedEnv := newGoClientEnv(t, harbor.URL+"/go/proxy")
	runGo(t, t.TempDir(), cachedEnv, "install", modulePath+"/cmd/hello@"+version)
	_, err := os.Stat(filepath.Join(cachedEnv.bin, "hello"))
	require.NoError(t, err, "go install did not produce the expected binary")
}

func TestChecksumDatabaseProxy(t *testing.T) {
	const lookup = "example.com/module v1.0.0 h1:fixture\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/lookup/github.com/!azure/module@v1.0.0", r.URL.Path)
		require.NotContains(t, r.URL.EscapedPath(), "%2521", "sumdb path was double escaped")
		_, _ = io.WriteString(w, lookup)
	}))
	defer upstream.Close()

	proxy := pkgproxy.New(
		&proModels.Project{Name: "proxy"},
		&regmodel.Registry{URL: upstream.URL, Type: regmodel.RegistryTypeGo, Status: regmodel.Healthy},
	)
	h := newHandlerWithDeps(
		newMemoryStore(),
		func(http.ResponseWriter, *http.Request, string) (int64, bool) { return 1, true },
		func(context.Context, string, string) (*pkgproxy.Proxy, error) { return proxy, nil },
	)
	h.(*handler).checksumURL = func(name string) (string, bool) {
		return upstream.URL, name == "sum.golang.org"
	}
	h.(*handler).checksumMirror = gosum.NewMirror(newMemoryChecksumStore())

	supported := httptest.NewRecorder()
	h.ServeHTTP(supported, httptest.NewRequest(http.MethodGet, "/go/proxy/sumdb/sum.golang.org/supported", nil))
	require.Equal(t, http.StatusOK, supported.Code)

	result := httptest.NewRecorder()
	h.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/go/proxy/sumdb/sum.golang.org/lookup/github.com/%21azure/module@v1.0.0", nil))
	require.Equal(t, http.StatusOK, result.Code)
	require.Equal(t, lookup, result.Body.String())
}

func TestRevisionInfoDoesNotFetchNonCanonicalModuleFiles(t *testing.T) {
	const info = `{"Version":"v1.2.3","Time":"2024-01-02T03:04:05Z"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/example.com/module/@v/master.info", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, info)
	}))
	defer upstream.Close()

	store := newMemoryStore()
	proxy := pkgproxy.New(
		&proModels.Project{Name: "proxy"},
		&regmodel.Registry{URL: upstream.URL, Type: regmodel.RegistryTypeGo, Status: regmodel.Healthy},
	)
	h := newHandlerWithDeps(
		store,
		func(http.ResponseWriter, *http.Request, string) (int64, bool) { return 1, true },
		func(context.Context, string, string) (*pkgproxy.Proxy, error) { return proxy, nil },
	)

	result := httptest.NewRecorder()
	h.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/go/proxy/example.com/module/@v/master.info", nil))
	require.Equal(t, http.StatusOK, result.Code)
	require.JSONEq(t, info, result.Body.String())
}

func TestHeadModuleObjectReturnsHeadersWithoutBody(t *testing.T) {
	const moduleFile = "module example.com/module\n\ngo 1.22\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/example.com/module/@v/v1.2.3.mod", r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, moduleFile)
	}))
	defer upstream.Close()

	proxy := pkgproxy.New(
		&proModels.Project{Name: "proxy"},
		&regmodel.Registry{URL: upstream.URL, Type: regmodel.RegistryTypeGo, Status: regmodel.Healthy},
	)
	h := newHandlerWithDeps(
		newMemoryStore(),
		func(http.ResponseWriter, *http.Request, string) (int64, bool) { return 1, true },
		func(context.Context, string, string) (*pkgproxy.Proxy, error) { return proxy, nil },
	)

	result := httptest.NewRecorder()
	h.ServeHTTP(result, httptest.NewRequest(http.MethodHead, "/go/proxy/example.com/module/@v/v1.2.3.mod", nil))
	require.Equal(t, http.StatusOK, result.Code)
	require.Equal(t, fmt.Sprintf("%d", len(moduleFile)), result.Header().Get("Content-Length"))
	require.Empty(t, result.Body.String())
}

func TestParseRequestAcceptsHead(t *testing.T) {
	req, err := parseRequest(http.MethodHead, "/go/proxy/example.com/module/@v/v1.2.3.mod")
	require.NoError(t, err)
	require.Equal(t, &moduleRequest{
		Project: "proxy",
		Module:  "example.com/module",
		Version: "v1.2.3",
		Type:    requestMod,
	}, req)
}

type moduleFixture struct {
	escapedPath    string
	escapedVersion string
	info           []byte
	mod            []byte
	zip            []byte
}

func newModuleFixture(t *testing.T, modulePath, version string) *moduleFixture {
	t.Helper()
	escapedPath, err := module.EscapePath(modulePath)
	require.NoError(t, err)
	escapedVersion, err := module.EscapeVersion(version)
	require.NoError(t, err)
	info, err := json.Marshal(map[string]string{"Version": version, "Time": "2024-01-02T03:04:05Z"})
	require.NoError(t, err)
	mod := []byte(fmt.Sprintf("module %s\n\ngo 1.22\n", modulePath))
	files := map[string]string{
		"go.mod":            string(mod),
		"hello.go":          "package hello\n\nconst Message = \"hello\"\n",
		"cmd/hello/main.go": "package main\n\nfunc main() {}\n",
	}
	return &moduleFixture{
		escapedPath:    escapedPath,
		escapedVersion: escapedVersion,
		info:           info,
		mod:            mod,
		zip:            moduleZip(t, modulePath, version, files),
	}
}

func (f *moduleFixture) response(escapedPath string) ([]byte, string, bool) {
	base := "/" + f.escapedPath
	switch escapedPath {
	case base + "/@v/list":
		return []byte("v1.2.3\n"), "text/plain", true
	case base + "/@latest":
		return f.info, "application/json", true
	case base + "/@v/" + f.escapedVersion + ".info":
		return f.info, "application/json", true
	case base + "/@v/" + f.escapedVersion + ".mod":
		return f.mod, "text/plain", true
	case base + "/@v/" + f.escapedVersion + ".zip":
		return f.zip, "application/zip", true
	default:
		return nil, "", false
	}
}

func moduleZip(t *testing.T, modulePath, version string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(modulePath + "@" + version + "/" + name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

type memoryStore struct {
	mu      sync.RWMutex
	content map[string][]byte
}

type memoryChecksumStore struct {
	mu        sync.RWMutex
	responses map[string]*gosum.Response
}

func newMemoryChecksumStore() *memoryChecksumStore {
	return &memoryChecksumStore{responses: map[string]*gosum.Response{}}
}

func (s *memoryChecksumStore) Open(_ context.Context, project string, database gosum.Database, requestPath string) (*gosum.Response, error) {
	s.mu.RLock()
	response, ok := s.responses[project+"\x00"+database.Name+"\x00"+database.URL+"\x00"+requestPath]
	s.mu.RUnlock()
	if !ok {
		return nil, harborerrors.NotFoundError(nil).WithMessage("checksum response not found")
	}
	clone := *response
	clone.Body = bytes.Clone(response.Body)
	return &clone, nil
}

func (s *memoryChecksumStore) Put(_ context.Context, _ int64, project string, database gosum.Database, requestPath string, response *gosum.Response) error {
	clone := *response
	clone.Body = bytes.Clone(response.Body)
	s.mu.Lock()
	s.responses[project+"\x00"+database.Name+"\x00"+database.URL+"\x00"+requestPath] = &clone
	s.mu.Unlock()
	return nil
}

func newMemoryStore() *memoryStore {
	return &memoryStore{content: map[string][]byte{}}
}

func (s *memoryStore) Publish(_ context.Context, _ int64, _, _, escapedModule, version string, typ requestType, content []byte) error {
	s.mu.Lock()
	s.content[s.key(escapedModule, version, titleFor(typ))] = bytes.Clone(content)
	s.mu.Unlock()
	return nil
}

func (s *memoryStore) Open(_ context.Context, _, escapedModule, version, title string) (*pkgstore.Content, error) {
	s.mu.RLock()
	payload, ok := s.content[s.key(escapedModule, version, title)]
	s.mu.RUnlock()
	if !ok {
		return nil, harborerrors.NotFoundError(nil).WithMessage("module not cached")
	}
	return &pkgstore.Content{Size: int64(len(payload)), Body: io.NopCloser(bytes.NewReader(payload))}, nil
}

func (*memoryStore) key(escapedModule, version, title string) string {
	return strings.Join([]string{escapedModule, version, title}, "\x00")
}

func (s *memoryStore) waitCached(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		count := len(s.content)
		s.mu.RUnlock()
		if count >= 3 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("module endpoints were not cached after the first upstream requests")
}

type goClientEnv struct {
	values []string
	bin    string
}

func newGoClientEnv(t *testing.T, proxyURL string) goClientEnv {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	return goClientEnv{
		bin: bin,
		values: append(os.Environ(),
			"GO111MODULE=on",
			"GOAUTH=off",
			"GOFLAGS=-modcacherw",
			"GOBIN="+bin,
			"GOCACHE="+filepath.Join(root, "build-cache"),
			"GOMODCACHE="+filepath.Join(root, "module-cache"),
			"GONOPROXY=none",
			"GONOSUMDB=none",
			"GOPATH="+filepath.Join(root, "gopath"),
			"GOPROXY="+proxyURL,
			"GOSUMDB=off",
			"GOTOOLCHAIN=local",
			"GOWORK=off",
			"HOME="+root,
		),
	}
}

func runGo(t *testing.T, dir string, env goClientEnv, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = env.values
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go %s failed:\n%s", strings.Join(args, " "), output)
	return string(output)
}
