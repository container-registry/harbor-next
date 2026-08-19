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

package npm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/goharbor/harbor/src/pkg/multiformat/model"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	server "github.com/goharbor/harbor/src/server/registry/multiformat"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

func TestPublishDeniedByProjectPolicy(t *testing.T) {
	store := newFakeStore()
	handler := newTestHandler(store, "http://harbor.example")
	handler.authorizePush = func(_ context.Context, project, registryType string) error {
		if project != "proxy" {
			t.Errorf("project = %q, want proxy", project)
		}
		if registryType != "npm" {
			t.Errorf("registry type = %q, want npm", registryType)
		}
		return errors.New("push disabled")
	}

	body := scopedPublishBody(t, "example", "1.0.0", []byte("package"))
	req := httptest.NewRequest(http.MethodPut, "/npm/proxy/example", strings.NewReader(string(body)))
	response := serve(handler, req)
	if response.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if len(store.published) != 0 {
		t.Errorf("published %d packages, want 0", len(store.published))
	}
}

func TestPeelProject(t *testing.T) {
	cases := []struct {
		in          string
		wantProject string
		wantRest    string
		wantOK      bool
	}{
		{"/proj/lodash", "proj", "/lodash", true},
		{"/proj/@scope/name", "proj", "/@scope/name", true},
		{"/proj/@scope/name/-/name-1.0.0.tgz", "proj", "/@scope/name/-/name-1.0.0.tgz", true},
		{"/proj/-/package/@scope/name/dist-tags", "proj", "/-/package/@scope/name/dist-tags", true},
		{"/proj", "proj", "/", true},
		{"/", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		p, rest, ok := peelProject(c.in)
		if p != c.wantProject || rest != c.wantRest || ok != c.wantOK {
			t.Errorf("peelProject(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, p, rest, ok, c.wantProject, c.wantRest, c.wantOK)
		}
	}
}

// fakeStore is an in-memory StateSource + Publisher + Payload + DistTags
// implementing the multiformat server Deps interfaces for adapter-level tests.
type fakeStore struct {
	published []server.PublishEvent
	state     map[string]model.PackageState // key: project|name
	payloads  map[string][]byte             // key: digest
}

func newFakeStore() *fakeStore {
	return &fakeStore{state: map[string]model.PackageState{}, payloads: map[string][]byte{}}
}

func key(project, name string) string { return project + "|" + name }

func (f *fakeStore) Publish(_ context.Context, project string, ev server.PublishEvent) (int64, error) {
	f.published = append(f.published, ev)
	dig := "sha256:" + ev.Version
	f.payloads[dig] = ev.Payload
	st := f.state[key(project, ev.Name)]
	st.Format = ev.Format
	st.Name = ev.Name
	st.ProjVersion++
	st.Versions = append(st.Versions, model.Version{
		Version:       ev.Version,
		PayloadDigest: dig,
		PayloadSize:   int64(len(ev.Payload)),
		Meta:          ev.Meta,
	})
	if st.DistTags == nil {
		st.DistTags = map[string]string{}
	}
	for k, v := range ev.DistTags {
		st.DistTags[k] = v
	}
	f.state[key(project, ev.Name)] = st
	return st.ProjVersion, nil
}

func (f *fakeStore) PublishFile(_ context.Context, _ string, _ server.FilePublishEvent) (int64, error) {
	return 0, nil
}

func (f *fakeStore) LoadState(_ context.Context, project string, _ int64, _ string, name string) (model.PackageState, bool, error) {
	st, ok := f.state[key(project, name)]
	return st, ok, nil
}

func (f *fakeStore) PayloadBlob(_ context.Context, _, _, _, digest string, _ int64) ([]byte, error) {
	return f.payloads[digest], nil
}

func (f *fakeStore) MavenFileBlob(_ context.Context, _, _, _, _ string, _ int64) ([]byte, error) {
	return nil, nil
}

func (f *fakeStore) SetDistTag(_ context.Context, project string, _ int64, _, name, tag, version string) (int64, error) {
	st := f.state[key(project, name)]
	if st.DistTags == nil {
		st.DistTags = map[string]string{}
	}
	if version == "" {
		delete(st.DistTags, tag)
	} else {
		st.DistTags[tag] = version
	}
	st.ProjVersion++
	f.state[key(project, name)] = st
	return st.ProjVersion, nil
}

func (f *fakeStore) UpdateVersionMetadata(_ context.Context, project string, _ int64, _, name, version string, meta []byte) (int64, error) {
	st := f.state[key(project, name)]
	for i := range st.Versions {
		if st.Versions[i].Version == version {
			st.Versions[i].Meta = append([]byte(nil), meta...)
			st.ProjVersion++
			f.state[key(project, name)] = st
			return st.ProjVersion, nil
		}
	}
	return 0, errors.New("version not found")
}

func (f *fakeStore) DeleteVersion(_ context.Context, project string, _ int64, _, name, version string) (int64, error) {
	st := f.state[key(project, name)]
	versions := st.Versions[:0]
	for _, v := range st.Versions {
		if v.Version != version {
			versions = append(versions, v)
		}
	}
	st.Versions = versions
	for tag, taggedVersion := range st.DistTags {
		if taggedVersion == version {
			delete(st.DistTags, tag)
		}
	}
	st.ProjVersion++
	f.state[key(project, name)] = st
	return st.ProjVersion, nil
}

func newTestHandler(f *fakeStore, baseURL string) *handler {
	return &handler{deps: server.Deps{
		Publisher:     f,
		FilePublisher: f,
		State:         f,
		Payload:       f,
		MavenFile:     f,
		DistTags:      f,
		Versions:      f,
		BaseURL:       baseURL,
	}}
}

// serve runs a request through the StripPrefix wrapper exactly as Register
// mounts it, so path handling matches production.
func serve(h *handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	http.StripPrefix(Prefix, h).ServeHTTP(rec, req)
	return rec
}

func scopedPublishBody(t *testing.T, name, version string, tarball []byte) []byte {
	t.Helper()
	short := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		short = name[i+1:]
	}
	att := short + "-" + version + ".tgz"
	body := map[string]any{
		"_id":       name,
		"name":      name,
		"dist-tags": map[string]string{"latest": version},
		"versions": map[string]any{
			version: map[string]any{
				"name":    name,
				"version": version,
				"dist":    map[string]any{"tarball": "http://client-guessed/wrong"},
			},
		},
		"_attachments": map[string]any{
			att: map[string]any{
				"content_type": "application/octet-stream",
				"data":         base64.StdEncoding.EncodeToString(tarball),
				"length":       len(tarball),
			},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestScopedPublishAndPackument(t *testing.T) {
	f := newFakeStore()
	h := newTestHandler(f, "http://harbor.example")

	const project = "myproj"
	const name = "@acme/widget"
	const version = "1.2.3"
	tarball := []byte("fake-tarball-bytes")

	// Publish: PUT /npm/<project>/@acme/widget (scoped name spans segments).
	pubBody := scopedPublishBody(t, name, version, tarball)
	req := httptest.NewRequest(http.MethodPut, "/npm/"+project+"/@acme/widget", strings.NewReader(string(pubBody)))
	rec := serve(h, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish: status %d body %s", rec.Code, rec.Body.String())
	}
	if len(f.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(f.published))
	}
	ev := f.published[0]
	if ev.Name != name {
		t.Errorf("publish name = %q, want %q", ev.Name, name)
	}
	if ev.Version != version {
		t.Errorf("publish version = %q, want %q", ev.Version, version)
	}
	if !ev.Immutable {
		t.Errorf("npm publish must be immutable")
	}
	if string(ev.Payload) != string(tarball) {
		t.Errorf("payload bytes mismatch: stored %q", string(ev.Payload))
	}

	// Packument: GET /npm/<project>/@acme/widget. The tarball URL is minted from
	// the request host (a registry serves URLs the client can reach), so set Host
	// to the expected external host.
	req = httptest.NewRequest(http.MethodGet, "/npm/"+project+"/@acme/widget", nil)
	req.Host = "harbor.example"
	rec = serve(h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("packument: status %d body %s", rec.Code, rec.Body.String())
	}
	var pkg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("packument not valid JSON: %v", err)
	}
	if pkg["name"] != name {
		t.Errorf("packument name = %v, want %q", pkg["name"], name)
	}
	versions, ok := pkg["versions"].(map[string]any)
	if !ok || versions[version] == nil {
		t.Fatalf("packument missing version %q: %v", version, pkg["versions"])
	}
	vobj := versions[version].(map[string]any)
	dist, ok := vobj["dist"].(map[string]any)
	if !ok {
		t.Fatalf("version missing dist: %v", vobj)
	}
	// Server-minted tarball URL: scope '/' is %2f-encoded in the path; project
	// segment is present; filename drops the scope.
	wantTarball := "http://harbor.example/npm/" + project + "/@acme%2fwidget/-/widget-" + version + ".tgz"
	if dist["tarball"] != wantTarball {
		t.Errorf("tarball URL = %v, want %q", dist["tarball"], wantTarball)
	}
	// Client-guessed dist must be replaced by minted integrity.
	if dist["integrity"] == nil || !strings.HasPrefix(dist["integrity"].(string), "sha512-") {
		t.Errorf("missing/invalid integrity: %v", dist["integrity"])
	}
	if dist["shasum"] == nil {
		t.Errorf("missing shasum")
	}
	// Internal reserved keys must not leak.
	if _, leaked := vobj["_multi-format-ociShasum"]; leaked {
		t.Errorf("_multi-format-ociShasum leaked into packument")
	}

	// ETag must be stable across renders of the same state.
	et := rec.Header().Get("ETag")
	if et == "" {
		t.Errorf("missing ETag")
	}
	req = httptest.NewRequest(http.MethodGet, "/npm/"+project+"/@acme/widget", nil)
	req.Host = "harbor.example"
	req.Header.Set("If-None-Match", et)
	rec = serve(h, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("conditional GET: status %d, want 304", rec.Code)
	}
}

func TestScopedTarballDownload(t *testing.T) {
	f := newFakeStore()
	h := newTestHandler(f, "http://harbor.example")

	const project = "myproj"
	const name = "@acme/widget"
	const version = "2.0.0"
	tarball := []byte("tarball-payload")

	pubBody := scopedPublishBody(t, name, version, tarball)
	req := httptest.NewRequest(http.MethodPut, "/npm/"+project+"/@acme/widget", strings.NewReader(string(pubBody)))
	if rec := serve(h, req); rec.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body.String())
	}

	// Scoped tarball path: scope kept in path, dropped from filename.
	req = httptest.NewRequest(http.MethodGet, "/npm/"+project+"/@acme/widget/-/widget-"+version+".tgz", nil)
	rec := serve(h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tarball: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(tarball) {
		t.Errorf("tarball bytes mismatch: got %q", rec.Body.String())
	}
}

func TestPing(t *testing.T) {
	rec := serve(newTestHandler(newFakeStore(), "http://harbor.example"),
		httptest.NewRequest(http.MethodGet, "/npm/proj/-/ping?write=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestDeprecateUpdatesMetadataWithoutRepublishingPayload(t *testing.T) {
	f := newFakeStore()
	h := newTestHandler(f, "http://harbor.example")
	body := scopedPublishBody(t, "example", "1.0.0", []byte("package"))
	if rec := serve(h, httptest.NewRequest(http.MethodPut, "/npm/proj/example", strings.NewReader(string(body)))); rec.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body.String())
	}

	packument := `{"name":"example","versions":{"1.0.0":{"name":"example","version":"1.0.0","deprecated":"use v2","dist":{"tarball":"ignored"}}},"dist-tags":{"latest":"1.0.0"}}`
	rec := serve(h, httptest.NewRequest(http.MethodPut, "/npm/proj/example", strings.NewReader(packument)))
	if rec.Code != http.StatusOK {
		t.Fatalf("deprecate: status %d body %s", rec.Code, rec.Body.String())
	}
	st := f.state[key("proj", "example")]
	if len(st.Versions) != 1 || !strings.Contains(string(st.Versions[0].Meta), `"deprecated":"use v2"`) {
		t.Fatalf("updated metadata = %s", st.Versions[0].Meta)
	}
	if len(f.published) != 1 {
		t.Fatalf("publish calls = %d, want 1", len(f.published))
	}
}

func TestUnpublishOnlyVersion(t *testing.T) {
	f := newFakeStore()
	h := newTestHandler(f, "http://harbor.example")
	body := scopedPublishBody(t, "example", "1.0.0", []byte("package"))
	if rec := serve(h, httptest.NewRequest(http.MethodPut, "/npm/proj/example", strings.NewReader(string(body)))); rec.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body.String())
	}

	rec := serve(h, httptest.NewRequest(http.MethodDelete, "/npm/proj/example/-rev/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unpublish: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := len(f.state[key("proj", "example")].Versions); got != 0 {
		t.Fatalf("versions = %d, want 0", got)
	}
}

func TestScopedDistTags(t *testing.T) {
	f := newFakeStore()
	h := newTestHandler(f, "http://harbor.example")

	const project = "myproj"
	const name = "@acme/widget"

	pubBody := scopedPublishBody(t, name, "1.0.0", []byte("x"))
	if rec := serve(h, httptest.NewRequest(http.MethodPut, "/npm/"+project+"/@acme/widget", strings.NewReader(string(pubBody)))); rec.Code != http.StatusCreated {
		t.Fatalf("publish: %d", rec.Code)
	}

	// PUT a dist-tag on a scoped name.
	req := httptest.NewRequest(http.MethodPut, "/npm/"+project+"/-/package/@acme/widget/dist-tags/beta", strings.NewReader(`"1.0.0"`))
	rec := serve(h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put dist-tag: %d %s", rec.Code, rec.Body.String())
	}

	// GET dist-tags for the scoped name.
	req = httptest.NewRequest(http.MethodGet, "/npm/"+project+"/-/package/@acme/widget/dist-tags", nil)
	rec = serve(h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dist-tags: %d %s", rec.Code, rec.Body.String())
	}
	var tags map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatal(err)
	}
	if tags["beta"] != "1.0.0" {
		t.Errorf("dist-tags = %v, want beta=1.0.0", tags)
	}
}

func TestProxyPackumentMergesCompleteUpstreamWithLocalState(t *testing.T) {
	var tarballRequests atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			tarballRequests.Add(1)
			_, _ = w.Write([]byte("unexpected eager tarball fetch"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"name": "demo",
			"dist-tags": map[string]string{
				"latest": "2.0.0",
				"next":   "2.0.0",
			},
			"versions": map[string]any{
				"1.0.0": map[string]any{
					"name": "demo", "version": "1.0.0", "source": "upstream",
					"dist": map[string]string{"tarball": upstream.URL + "/demo/-/demo-1.0.0.tgz"},
				},
				"2.0.0": map[string]any{
					"name": "demo", "version": "2.0.0", "source": "upstream",
					"dist": map[string]string{"tarball": upstream.URL + "/demo/-/demo-2.0.0.tgz"},
				},
			},
		})
	}))
	t.Cleanup(upstream.Close)

	store := newFakeStore()
	seedLocalVersion(t, store, "proxy", "demo", "1.0.0", map[string]string{
		"latest": "1.0.0",
		"stable": "1.0.0",
	})
	h := newTestHandler(store, "http://harbor.example")
	h.resolveProxy = testProxyResolver(upstream.URL)

	req := httptest.NewRequest(http.MethodGet, "/npm/proxy/demo", nil)
	req.Host = "harbor.example"
	rec := serve(h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("packument: status %d body %s", rec.Code, rec.Body.String())
	}
	var packument map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &packument); err != nil {
		t.Fatal(err)
	}
	versions := packument["versions"].(map[string]any)
	if len(versions) != 2 || versions["2.0.0"] == nil {
		t.Fatalf("versions = %v, want complete upstream set", versions)
	}
	localVersion := versions["1.0.0"].(map[string]any)
	if localVersion["source"] != "local" {
		t.Errorf("colliding version source = %v, want local", localVersion["source"])
	}
	tags := packument["dist-tags"].(map[string]any)
	if tags["latest"] != "1.0.0" || tags["stable"] != "1.0.0" || tags["next"] != "2.0.0" {
		t.Errorf("dist-tags = %v, want local collisions and upstream additions", tags)
	}
	if got := tarballRequests.Load(); got != 0 {
		t.Errorf("packument fetched %d tarballs, want lazy fetching", got)
	}
}

func TestProxyPackumentFallsBackToCachedLocalState(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)

	store := newFakeStore()
	seedLocalVersion(t, store, "proxy", "demo", "1.0.0", map[string]string{"latest": "1.0.0"})
	h := newTestHandler(store, "http://harbor.example")
	h.resolveProxy = testProxyResolver(upstream.URL)

	rec := serve(h, httptest.NewRequest(http.MethodGet, "/npm/proxy/demo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("packument: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"1.0.0"`) {
		t.Errorf("fallback packument = %s, want cached version", rec.Body.String())
	}
}

func TestProxyTarballIsFetchedAndCachedLazily(t *testing.T) {
	const payload = "proxied-tarball"
	var tarballRequests atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			tarballRequests.Add(1)
			_, _ = w.Write([]byte(payload))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"name":      "demo",
			"dist-tags": map[string]string{"latest": "2.0.0"},
			"versions": map[string]any{
				"1.0.0": map[string]any{
					"name": "demo", "version": "1.0.0",
					"dist": map[string]string{"tarball": upstream.URL + "/demo/-/demo-1.0.0.tgz"},
				},
				"2.0.0": map[string]any{
					"name": "demo", "version": "2.0.0",
					"dist": map[string]string{"tarball": upstream.URL + "/demo/-/demo-2.0.0.tgz"},
				},
			},
		})
	}))
	t.Cleanup(upstream.Close)

	store := newFakeStore()
	h := newTestHandler(store, "http://harbor.example")
	h.resolveProxy = testProxyResolver(upstream.URL)

	packument := serve(h, httptest.NewRequest(http.MethodGet, "/npm/proxy/demo", nil))
	if packument.Code != http.StatusOK {
		t.Fatalf("packument: status %d body %s", packument.Code, packument.Body.String())
	}
	if got := tarballRequests.Load(); got != 0 {
		t.Fatalf("packument fetched %d tarballs, want 0", got)
	}

	tarball := serve(h, httptest.NewRequest(http.MethodGet, "/npm/proxy/demo/-/demo-1.0.0.tgz", nil))
	if tarball.Code != http.StatusOK || tarball.Body.String() != payload {
		t.Fatalf("tarball: status %d body %q", tarball.Code, tarball.Body.String())
	}
	if got := tarballRequests.Load(); got != 1 {
		t.Errorf("tarball requests = %d, want 1", got)
	}
	state, ok, err := store.LoadState(context.Background(), "proxy", 0, formatNPM, "demo")
	if err != nil || !ok || len(state.Versions) != 1 || state.Versions[0].Version != "1.0.0" {
		t.Errorf("cached state = %+v, %v, %v; want version 1.0.0", state, ok, err)
	}

	// The locally synthesized fallback tag must not override current upstream
	// metadata while the upstream remains available.
	merged := serve(h, httptest.NewRequest(http.MethodGet, "/npm/proxy/demo", nil))
	var mergedPackument struct {
		DistTags map[string]string `json:"dist-tags"`
	}
	if err := json.Unmarshal(merged.Body.Bytes(), &mergedPackument); err != nil {
		t.Fatal(err)
	}
	if got := mergedPackument.DistTags["latest"]; got != "2.0.0" {
		t.Errorf("merged latest = %q, want upstream 2.0.0", got)
	}
}

func seedLocalVersion(t *testing.T, store *fakeStore, project, name, version string, distTags map[string]string) {
	t.Helper()
	meta, err := json.Marshal(map[string]string{"name": name, "version": version, "source": "local"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Publish(context.Background(), project, server.PublishEvent{
		Format: formatNPM, Name: name, Version: version, Payload: []byte("local-tarball"), Meta: meta, DistTags: distTags,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testProxyResolver(upstreamURL string) proxyResolver {
	proxy := pkgproxy.New(
		&proModels.Project{Name: "proxy"},
		&regmodel.Registry{URL: upstreamURL, Type: regmodel.RegistryTypeNPM, Status: regmodel.Healthy},
	)
	return func(_ context.Context, project, registryType string) (*pkgproxy.Proxy, error) {
		if project != "proxy" {
			return nil, errors.New("unexpected project")
		}
		if registryType != regmodel.RegistryTypeNPM {
			return nil, errors.New("unexpected registry type")
		}
		return proxy, nil
	}
}
