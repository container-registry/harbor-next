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

package registry

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/errors"
)

type storedManifest struct {
	contentType string
	payload     string
}

// fakeRegistry is a minimal in-memory v2 registry that records every call.
type fakeRegistry struct {
	mu        sync.Mutex
	manifests map[string]map[string]storedManifest // repo -> ref -> manifest
	blobs     map[string]map[string]bool           // repo -> digest
	calls     []string
	accepts   [][]string
}

var fakePath = regexp.MustCompile(`^/v2/(.+)/(manifests|blobs)/(.+)$`)

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{manifests: map[string]map[string]storedManifest{}, blobs: map[string]map[string]bool{}}
}

func (f *fakeRegistry) putManifest(repo, ct, payload string, tags ...string) string {
	if f.manifests[repo] == nil {
		f.manifests[repo] = map[string]storedManifest{}
	}
	d := digest.FromString(payload).String()
	for _, ref := range append(tags, d) {
		f.manifests[repo][ref] = storedManifest{ct, payload}
	}
	return d
}

func (f *fakeRegistry) putBlob(repo, d string) {
	if f.blobs[repo] == nil {
		f.blobs[repo] = map[string]bool{}
	}
	f.blobs[repo][d] = true
}

func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/v2/" {
		return
	}
	f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI())
	if strings.HasSuffix(r.URL.Path, "/blobs/uploads/") && r.Method == http.MethodPost {
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/"), "/blobs/uploads/")
		from, d := r.URL.Query().Get("from"), r.URL.Query().Get("mount")
		if !f.blobs[from][d] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.putBlob(repo, d)
		w.WriteHeader(http.StatusCreated)
		return
	}
	m := fakePath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	repo, kind, ref := m[1], m[2], m[3]
	if kind == "blobs" {
		if f.blobs[repo][ref] {
			w.Header().Set("Content-Length", "1")
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if r.Method == http.MethodGet {
			f.accepts = append(f.accepts, r.Header.Values("Accept"))
		}
		mf, ok := f.manifests[repo][ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", mf.contentType)
		w.Header().Set("Docker-Content-Digest", digest.FromString(mf.payload).String())
		w.Header().Set("Content-Length", strconv.Itoa(len(mf.payload)))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, mf.payload)
		}
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		d := f.putManifest(repo, r.Header.Get("Content-Type"), string(body), ref)
		w.Header().Set("Docker-Content-Digest", d)
		w.WriteHeader(http.StatusCreated)
	}
}

func (f *fakeRegistry) manifestPuts() []string {
	var puts []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, http.MethodPut+" ") {
			puts = append(puts, strings.TrimPrefix(c, http.MethodPut+" "))
		}
	}
	return puts
}

func (f *fakeRegistry) mounts() []string {
	var out []string
	for _, c := range f.calls {
		if strings.Contains(c, "?mount=") {
			out = append(out, c)
		}
	}
	return out
}

func newFakeClient(t *testing.T) (*fakeRegistry, Client) {
	t.Helper()
	f := newFakeRegistry()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, NewClient(srv.URL, "", "", true)
}

const (
	cfgD   = "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7"
	layerD = "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
	frgnD  = "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"
)

func imageManifest(mediaType, cfgType, layerType string) string {
	return fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,
  "config":{"mediaType":%q,"digest":%q,"size":2},
  "layers":[{"mediaType":%q,"digest":%q,"size":3}]}`, mediaType, cfgType, cfgD, layerType, layerD)
}

func indexOf(mediaType, childType string, children ...string) string {
	var entries []string
	for _, c := range children {
		entries = append(entries, fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":%d}`, childType, digest.FromString(c), len(c)))
	}
	return fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[%s]}`, mediaType, strings.Join(entries, ","))
}

var (
	ociMf    = imageManifest("application/vnd.oci.image.manifest.v1+json", "application/vnd.oci.image.config.v1+json", "application/vnd.oci.image.layer.v1.tar+gzip")
	dockerMf = imageManifest("application/vnd.docker.distribution.manifest.v2+json", "application/vnd.docker.container.image.v1+json", "application/vnd.docker.image.rootfs.diff.tar.gzip")
)

func TestCopyImageManifests(t *testing.T) {
	for _, tc := range []struct{ ct, payload string }{
		{"application/vnd.oci.image.manifest.v1+json", ociMf},
		{"application/vnd.docker.distribution.manifest.v2+json", dockerMf},
	} {
		t.Run(tc.ct, func(t *testing.T) {
			f, c := newFakeClient(t)
			d := f.putManifest("src/app", tc.ct, tc.payload, "v1")
			f.putBlob("src/app", cfgD)
			f.putBlob("src/app", layerD)

			require.NoError(t, c.Copy("src/app", "v1", "dst/app", "v1", false))

			got := f.manifests["dst/app"]["v1"]
			assert.Equal(t, tc.payload, got.payload, "pushed bytes must be identical")
			assert.Equal(t, tc.ct, got.contentType)
			assert.Equal(t, d, digest.FromString(got.payload).String())
			assert.True(t, f.blobs["dst/app"][cfgD])
			assert.True(t, f.blobs["dst/app"][layerD])
			assert.Len(t, f.mounts(), 2)
		})
	}
}

func TestCopyIndexCopiesChildrenFirst(t *testing.T) {
	for _, tc := range []struct{ ct, childType, child string }{
		{"application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json", ociMf},
		{"application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.docker.distribution.manifest.v2+json", dockerMf},
	} {
		t.Run(tc.ct, func(t *testing.T) {
			f, c := newFakeClient(t)
			childD := f.putManifest("src/app", tc.childType, tc.child)
			idx := indexOf(tc.ct, tc.childType, tc.child)
			idxD := f.putManifest("src/app", tc.ct, idx, "multi")
			f.putBlob("src/app", cfgD)
			f.putBlob("src/app", layerD)

			require.NoError(t, c.Copy("src/app", "multi", "dst/app", "multi", false))

			assert.Equal(t, []string{
				"/v2/dst/app/manifests/" + childD,
				"/v2/dst/app/manifests/multi",
			}, f.manifestPuts())
			assert.Equal(t, idx, f.manifests["dst/app"]["multi"].payload)
			assert.Equal(t, idxD, digest.FromString(f.manifests["dst/app"]["multi"].payload).String())
		})
	}
}

func TestCopyNestedIndex(t *testing.T) {
	f, c := newFakeClient(t)
	f.putBlob("src/app", cfgD)
	f.putBlob("src/app", layerD)
	childD := f.putManifest("src/app", "application/vnd.oci.image.manifest.v1+json", ociMf)
	inner := indexOf("application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json", ociMf)
	innerD := f.putManifest("src/app", "application/vnd.oci.image.index.v1+json", inner)
	outer := indexOf("application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.index.v1+json", inner)
	f.putManifest("src/app", "application/vnd.oci.image.index.v1+json", outer, "nested")

	require.NoError(t, c.Copy("src/app", "nested", "dst/app", "nested", false))
	assert.Equal(t, []string{
		"/v2/dst/app/manifests/" + childD,
		"/v2/dst/app/manifests/" + innerD,
		"/v2/dst/app/manifests/nested",
	}, f.manifestPuts())
}

func TestCopySkipsForeignAndExistingLayers(t *testing.T) {
	f, c := newFakeClient(t)
	mf := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json",
  "config":{"mediaType":"application/vnd.docker.container.image.v1+json","digest":%q,"size":2},
  "layers":[{"mediaType":"application/vnd.docker.image.rootfs.foreign.diff.tar.gzip","digest":%q,"size":9,"urls":["https://example.com/x"]},
            {"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","digest":%q,"size":3}]}`, cfgD, frgnD, layerD)
	f.putManifest("src/app", "application/vnd.docker.distribution.manifest.v2+json", mf, "v1")
	f.putBlob("src/app", cfgD)
	f.putBlob("src/app", layerD)
	f.putBlob("dst/app", layerD)

	require.NoError(t, c.Copy("src/app", "v1", "dst/app", "v1", false))
	assert.Equal(t, []string{"POST /v2/dst/app/blobs/uploads/?mount=" + cfgD + "&from=src/app"}, f.mounts())
	assert.False(t, f.blobs["dst/app"][frgnD])
}

// Copy must never replace a different artifact unless override is set.
func TestCopyOverrideRules(t *testing.T) {
	t.Run("same digest is a no-op", func(t *testing.T) {
		f, c := newFakeClient(t)
		f.putManifest("src/app", "application/vnd.oci.image.manifest.v1+json", ociMf, "v1")
		f.putManifest("dst/app", "application/vnd.oci.image.manifest.v1+json", ociMf, "v1")
		require.NoError(t, c.Copy("src/app", "v1", "dst/app", "v1", false))
		assert.Empty(t, f.manifestPuts())
		assert.Empty(t, f.mounts())
	})
	t.Run("different digest without override is refused", func(t *testing.T) {
		f, c := newFakeClient(t)
		f.putManifest("src/app", "application/vnd.oci.image.manifest.v1+json", ociMf, "v1")
		f.putManifest("dst/app", "application/vnd.docker.distribution.manifest.v2+json", dockerMf, "v1")
		err := c.Copy("src/app", "v1", "dst/app", "v1", false)
		assert.True(t, errors.IsErr(err, errors.PreconditionCode), "got %v", err)
		assert.Empty(t, f.manifestPuts())
		assert.Equal(t, dockerMf, f.manifests["dst/app"]["v1"].payload)
	})
	t.Run("different digest with override replaces", func(t *testing.T) {
		f, c := newFakeClient(t)
		f.putManifest("src/app", "application/vnd.oci.image.manifest.v1+json", ociMf, "v1")
		f.putManifest("dst/app", "application/vnd.docker.distribution.manifest.v2+json", dockerMf, "v1")
		f.putBlob("src/app", cfgD)
		f.putBlob("src/app", layerD)
		require.NoError(t, c.Copy("src/app", "v1", "dst/app", "v1", true))
		assert.Equal(t, ociMf, f.manifests["dst/app"]["v1"].payload)
	})
	t.Run("missing source fails and writes nothing", func(t *testing.T) {
		f, c := newFakeClient(t)
		err := c.Copy("src/app", "nope", "dst/app", "v1", true)
		assert.Error(t, err)
		assert.Empty(t, f.manifestPuts())
	})
}

func TestManifestExistDescriptor(t *testing.T) {
	f, c := newFakeClient(t)
	d := f.putManifest("p/a", "application/vnd.oci.image.manifest.v1+json", ociMf, "v1")

	ok, desc, err := c.ManifestExist("p/a", "v1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, d, desc.Digest.String())
	assert.Equal(t, "application/vnd.oci.image.manifest.v1+json", desc.MediaType)
	assert.Equal(t, int64(len(ociMf)), desc.Size)

	ok, desc, err = c.ManifestExist("p/a", "missing")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, desc)
}

func TestPullManifestAcceptHeaders(t *testing.T) {
	f, c := newFakeClient(t)
	f.putManifest("p/a", "application/vnd.oci.image.manifest.v1+json", ociMf, "v1")

	_, _, err := c.PullManifest("p/a", "v1")
	require.NoError(t, err)
	_, _, err = c.PullManifest("p/a", "v1", "application/vnd.oci.image.manifest.v1+json")
	require.NoError(t, err)

	require.Len(t, f.accepts, 2)
	assert.Equal(t, []string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, f.accepts[0])
	assert.Equal(t, []string{"application/vnd.oci.image.manifest.v1+json"}, f.accepts[1])
}

func TestPullManifestReturnsDigestAndBytes(t *testing.T) {
	for _, tc := range []struct{ ct, payload string }{
		{"application/vnd.oci.image.manifest.v1+json", ociMf},
		{"application/vnd.docker.distribution.manifest.v2+json", dockerMf},
		{"application/vnd.oci.image.index.v1+json", indexOf("application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json", ociMf)},
		{"application/vnd.docker.distribution.manifest.list.v2+json", indexOf("application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.docker.distribution.manifest.v2+json", dockerMf)},
	} {
		t.Run(tc.ct, func(t *testing.T) {
			f, c := newFakeClient(t)
			d := f.putManifest("p/a", tc.ct, tc.payload, "v1")
			m, got, err := c.PullManifest("p/a", "v1")
			require.NoError(t, err)
			assert.Equal(t, d, got)
			mt, payload, err := m.Payload()
			require.NoError(t, err)
			assert.Equal(t, tc.ct, mt)
			assert.Equal(t, tc.payload, string(payload))
		})
	}
}

func TestCopyRefusesSchema1Children(t *testing.T) {
	for _, child := range []string{
		"application/vnd.docker.distribution.manifest.v1+prettyjws",
		"application/vnd.docker.distribution.manifest.v1+json",
	} {
		t.Run(child, func(t *testing.T) {
			f, c := newFakeClient(t)
			list := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.list.v2+json","manifests":[{"mediaType":%q,"digest":%q,"size":10}]}`, child, layerD)
			f.putManifest("src/app", "application/vnd.docker.distribution.manifest.list.v2+json", list, "old")
			f.putBlob("src/app", layerD)

			err := c.Copy("src/app", "old", "dst/app", "old", true)
			assert.True(t, errors.IsErr(err, errors.UNSUPPORTED), "got %v", err)
			assert.Empty(t, f.manifestPuts(), "nothing pushed")
			assert.Empty(t, f.mounts(), "nothing mounted")
		})
	}
}

func TestPullManifestRefusesSchema1(t *testing.T) {
	f, c := newFakeClient(t)
	f.putManifest("p/a", "application/vnd.docker.distribution.manifest.v1+prettyjws",
		`{"schemaVersion":1,"name":"p/a","tag":"v1","fsLayers":[],"history":[],"signatures":[]}`, "v1")
	assert.NotPanics(t, func() {
		_, _, err := c.PullManifest("p/a", "v1")
		assert.Error(t, err)
	})
}
