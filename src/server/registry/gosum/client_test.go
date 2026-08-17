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

package gosum

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/sumdb"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/sumdb/note"

	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

func TestGoClientCustomChecksumDatabaseWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Go client integration test in short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command is not available")
	}

	const (
		modulePath = "example.com/checksum/hello"
		version    = "v1.0.0"
	)
	fixture := newClientModuleFixture(t, modulePath, version)
	moduleProxy := httptest.NewServer(http.HandlerFunc(fixture.serve))
	defer moduleProxy.Close()

	signerKey, verifierKey, err := note.GenerateKey(rand.Reader, "harbor-test-sumdb")
	require.NoError(t, err)
	sumOps := sumdb.NewTestServer(signerKey, func(path, vers string) ([]byte, error) {
		if path != modulePath || vers != version {
			return nil, fmt.Errorf("unexpected module %s@%s", path, vers)
		}
		return []byte(fmt.Sprintf("%s %s %s\n%s %s/go.mod %s\n", path, vers, fixture.zipHash, path, vers, fixture.modHash)), nil
	})
	sumHandler := sumdb.NewServer(sumOps)
	var sumdbDisabled atomic.Bool
	var sumdbRequests atomic.Int64
	sumAuthority := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sumdbRequests.Add(1)
		if sumdbDisabled.Load() {
			http.Error(w, "checksum authority disabled", http.StatusBadGateway)
			return
		}
		sumHandler.ServeHTTP(w, r)
	}))
	defer sumAuthority.Close()

	proxy := pkgproxy.New(
		&proModels.Project{Name: "checksums"},
		&regmodel.Registry{Name: "test-sumdb", URL: sumAuthority.URL, Type: regmodel.RegistryTypeGoSumDB, Status: regmodel.Healthy},
	)
	harbor := httptest.NewServer(newHandlerWithDeps(
		NewMirror(newMemoryStore()),
		func(http.ResponseWriter, *http.Request, string) (int64, bool) { return 11, true },
		func(context.Context, string, string) (*pkgproxy.Proxy, error) { return proxy, nil },
	))
	defer harbor.Close()

	firstEnv := newClientEnv(t, moduleProxy.URL, verifierKey+" "+harbor.URL+"/go-sumdb/checksums")
	runClientGo(t, t.TempDir(), firstEnv, "mod", "download", modulePath+"@"+version)
	consumer := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(consumer, "go.mod"), []byte("module example.com/consumer\n\ngo 1.22\n"), 0o600))
	runClientGo(t, consumer, firstEnv, "get", modulePath+"@latest")
	require.Positive(t, sumdbRequests.Load())

	sumdbDisabled.Store(true)
	requestsBeforeWarmRead := sumdbRequests.Load()
	secondEnv := newClientEnv(t, moduleProxy.URL, verifierKey+" "+harbor.URL+"/go-sumdb/checksums")
	runClientGo(t, t.TempDir(), secondEnv, "install", modulePath+"/cmd/hello@"+version)
	_, err = os.Stat(filepath.Join(secondEnv.bin, "hello"))
	require.NoError(t, err, "go install did not produce the expected binary")
	require.Equal(t, requestsBeforeWarmRead, sumdbRequests.Load(), "warm immutable checksum responses reached the disabled authority")
}

type clientModuleFixture struct {
	basePath string
	version  string
	info     []byte
	mod      []byte
	zip      []byte
	modHash  string
	zipHash  string
}

func newClientModuleFixture(t *testing.T, modulePath, version string) *clientModuleFixture {
	t.Helper()
	info, err := json.Marshal(map[string]string{"Version": version, "Time": "2026-07-10T00:00:00Z"})
	require.NoError(t, err)
	mod := []byte(fmt.Sprintf("module %s\n\ngo 1.22\n", modulePath))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"go.mod":            string(mod),
		"hello.go":          "package hello\n\nconst Message = \"hello\"\n",
		"cmd/hello/main.go": "package main\n\nfunc main() {}\n",
	} {
		writer, err := zw.Create(modulePath + "@" + version + "/" + name)
		require.NoError(t, err)
		_, err = io.WriteString(writer, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	modHash, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(mod)), nil
	})
	require.NoError(t, err)
	zipPath := filepath.Join(t.TempDir(), "module.zip")
	require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o600))
	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	require.NoError(t, err)
	return &clientModuleFixture{
		basePath: "/" + modulePath,
		version:  version,
		info:     info,
		mod:      mod,
		zip:      buf.Bytes(),
		modHash:  modHash,
		zipHash:  zipHash,
	}
}

func (f *clientModuleFixture) serve(w http.ResponseWriter, r *http.Request) {
	base := f.basePath
	switch r.URL.Path {
	case base + "/@v/list":
		_, _ = io.WriteString(w, f.version+"\n")
	case base + "/@latest", base + "/@v/" + f.version + ".info":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.info)
	case base + "/@v/" + f.version + ".mod":
		_, _ = w.Write(f.mod)
	case base + "/@v/" + f.version + ".zip":
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(f.zip)
	default:
		http.NotFound(w, r)
	}
}

type clientEnv struct {
	values []string
	bin    string
}

func newClientEnv(t *testing.T, proxyURL, checksumDatabase string) clientEnv {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	return clientEnv{
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
			"GOSUMDB="+checksumDatabase,
			"GOTOOLCHAIN=local",
			"GOWORK=off",
			"HOME="+root,
		),
	}
}

func runClientGo(t *testing.T, dir string, env clientEnv, args ...string) string {
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
