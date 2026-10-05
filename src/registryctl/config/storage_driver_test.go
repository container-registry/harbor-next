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

package config

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/s3-aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadWithRegistryConfig loads registryctl config pointing at the given
// registry config, the same way registryctl starts in production.
func loadWithRegistryConfig(t *testing.T, registryYAML string) *Configuration {
	t.Helper()
	dir := t.TempDir()
	reg := filepath.Join(dir, "registry.yml")
	require.NoError(t, os.WriteFile(reg, []byte(registryYAML), 0o600))
	ctl := filepath.Join(dir, "registryctl.yml")
	require.NoError(t, os.WriteFile(ctl, []byte("protocol: http\nport: 8080\nregistry_config: "+reg+"\n"), 0o600))
	cfg := &Configuration{}
	require.NoError(t, cfg.Load(ctl, false))
	return cfg
}

// The filesystem driver must resolve registry paths under rootdirectory, or GC
// would delete from the wrong place.
func TestFilesystemDriverHonorsRootDirectory(t *testing.T) {
	root := t.TempDir()
	cfg := loadWithRegistryConfig(t, fmt.Sprintf("version: 0.1\nstorage:\n  filesystem:\n    rootdirectory: %s\n  delete:\n    enabled: true\n", root))
	d := cfg.StorageDriver
	assert.Equal(t, "filesystem", d.Name())

	ctx := context.Background()
	keep := "/docker/registry/v2/repositories/lib/app2/_manifests/revisions/sha256/aa/link"
	gone := "/docker/registry/v2/repositories/lib/app/_manifests/revisions/sha256/aa/link"
	require.NoError(t, d.PutContent(ctx, keep, []byte("k")))
	require.NoError(t, d.PutContent(ctx, gone, []byte("g")))
	_, err := os.Stat(filepath.Join(root, gone))
	require.NoError(t, err, "files land under rootdirectory")

	require.NoError(t, d.Delete(ctx, "/docker/registry/v2/repositories/lib/app"))
	_, err = os.Stat(filepath.Join(root, gone))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(root, keep))
	assert.NoError(t, err, "sibling with a shared name prefix survives")
}

// fakeS3 is a path-style S3 endpoint holding one bucket in memory. It serves
// ListObjects v1 and v2 so either driver generation can talk to it.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	lists   int
	deletes int
}

type listEntry struct {
	Key  string `xml:"Key"`
	Size int    `xml:"Size"`
}

type listResult struct {
	XMLName               xml.Name    `xml:"ListBucketResult"`
	Name                  string      `xml:"Name"`
	Prefix                string      `xml:"Prefix"`
	KeyCount              int         `xml:"KeyCount"`
	MaxKeys               int         `xml:"MaxKeys"`
	IsTruncated           bool        `xml:"IsTruncated"`
	NextContinuationToken string      `xml:"NextContinuationToken,omitempty"`
	NextMarker            string      `xml:"NextMarker,omitempty"`
	Contents              []listEntry `xml:"Contents"`
	CommonPrefixes        []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodPost && q.Has("delete"):
		f.deletes++
		var req struct {
			Objects []struct {
				Key string `xml:"Key"`
			} `xml:"Object"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = xml.Unmarshal(body, &req)
		var sb strings.Builder
		sb.WriteString(`<DeleteResult>`)
		for _, o := range req.Objects {
			delete(f.objects, o.Key)
			sb.WriteString("<Deleted><Key>" + o.Key + "</Key></Deleted>")
		}
		sb.WriteString(`</DeleteResult>`)
		_, _ = io.WriteString(w, sb.String())
	case r.Method == http.MethodGet && key == "":
		f.lists++
		f.list(w, q)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
		w.Header().Set("ETag", `"x"`)
	case r.Method == http.MethodHead || r.Method == http.MethodGet:
		b, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		if r.Method == http.MethodGet {
			_, _ = w.Write(b)
		}
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *fakeS3) list(w http.ResponseWriter, q map[string][]string) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	prefix, delim := get("prefix"), get("delimiter")
	after := get("start-after")
	if tok := get("continuation-token"); tok != "" {
		after = tok
	}
	if m := get("marker"); m != "" {
		after = m
	}
	max := 1000
	if v, err := strconv.Atoi(get("max-keys")); err == nil && v > 0 && v < max {
		max = v
	}
	keys := make([]string, 0, len(f.objects))
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	res := listResult{Name: "bucket", Prefix: prefix, MaxKeys: max}
	seen := map[string]bool{}
	for _, k := range keys {
		if len(res.Contents)+len(res.CommonPrefixes) == max {
			res.IsTruncated = true
			break
		}
		if delim != "" {
			if i := strings.Index(k[len(prefix):], delim); i >= 0 {
				p := k[:len(prefix)+i+1]
				if !seen[p] {
					seen[p] = true
					res.CommonPrefixes = append(res.CommonPrefixes, struct {
						Prefix string `xml:"Prefix"`
					}{p})
				}
				continue
			}
		}
		res.Contents = append(res.Contents, listEntry{Key: k, Size: len(f.objects[k])})
	}
	res.KeyCount = len(res.Contents) + len(res.CommonPrefixes)
	if res.IsTruncated && len(res.Contents) > 0 {
		last := res.Contents[len(res.Contents)-1].Key
		res.NextContinuationToken = last
		res.NextMarker = last
	}
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(res)
}

func (f *fakeS3) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.objects))
	for k := range f.objects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func newS3Driver(t *testing.T, rootdirectory string) (*fakeS3, *Configuration) {
	t.Helper()
	f := &fakeS3{objects: map[string][]byte{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	cfg := loadWithRegistryConfig(t, fmt.Sprintf(`version: 0.1
storage:
  s3:
    region: us-east-1
    bucket: bucket
    accesskey: AKIAEXAMPLE
    secretkey: secret
    regionendpoint: %s
    forcepathstyle: true
    secure: false
    v4auth: true
    rootdirectory: %s
  delete:
    enabled: true
`, srv.URL, rootdirectory))
	return f, cfg
}

// GC deletes by prefix on S3. Pin the prefix guard, the root directory mapping
// and paging past the 1000-key list limit.
func TestS3DriverDeleteScope(t *testing.T) {
	const reg = "/docker/registry/v2"
	for _, root := range []string{"", "/harbor/data"} {
		t.Run("root="+root, func(t *testing.T) {
			f, cfg := newS3Driver(t, root)
			d := cfg.StorageDriver
			assert.Equal(t, "s3aws", d.Name())
			ctx := context.Background()

			base := strings.TrimPrefix(root+reg, "/")
			seed := func(p string) { f.objects[base+p] = []byte("x") }
			seed("/repositories/lib/app/_manifests/revisions/sha256/aa/link")
			seed("/repositories/lib/app/_layers/sha256/bb/link")
			seed("/repositories/lib/app2/_manifests/revisions/sha256/aa/link")
			seed("/repositories/lib/app-x/_layers/sha256/bb/link")
			seed("/blobs/sha256/aa/aa11/data")
			for i := range 2500 {
				seed(fmt.Sprintf("/repositories/big/repo/_layers/sha256/%04d/link", i))
			}
			f.objects["outside/root/object"] = []byte("x")

			require.NoError(t, d.Delete(ctx, reg+"/repositories/lib/app"))
			require.NoError(t, d.Delete(ctx, reg+"/repositories/big/repo"))

			assert.Equal(t, []string{
				base + "/blobs/sha256/aa/aa11/data",
				base + "/repositories/lib/app-x/_layers/sha256/bb/link",
				base + "/repositories/lib/app2/_manifests/revisions/sha256/aa/link",
				"outside/root/object",
			}, f.keys())

			err := d.Delete(ctx, reg+"/repositories/lib/missing")
			assert.Error(t, err)
			assert.Contains(t, fmt.Sprintf("%T", err), "PathNotFoundError")
		})
	}
}

func TestS3DriverWritesUnderRoot(t *testing.T) {
	f, cfg := newS3Driver(t, "/harbor/data")
	require.NoError(t, cfg.StorageDriver.PutContent(context.Background(), "/docker/registry/v2/blobs/sha256/aa/aa11/data", []byte("payload")))
	assert.Equal(t, []string{"harbor/data/docker/registry/v2/blobs/sha256/aa/aa11/data"}, f.keys())
}

// registryctl parses the registry's own config file. Keys the chart ships by
// default must not stop it from starting, including the schema1 block.
func TestLoadsChartDefaultRegistryConfig(t *testing.T) {
	root := t.TempDir()
	cfg := loadWithRegistryConfig(t, fmt.Sprintf(`version: 0.1
log:
  fields:
    service: registry
storage:
  filesystem:
    rootdirectory: %s
  cache:
    layerinfo: redis
  maintenance:
    uploadpurging:
      enabled: true
      age: 168h
      interval: 24h
      dryrun: false
  delete:
    enabled: true
  redirect:
    disable: false
auth:
  htpasswd:
    realm: harbor-registry-basic-realm
    path: /etc/registry/passwd
validation:
  disabled: true
compatibility:
  schema1:
    enabled: true
http:
  addr: :5000
  secret: placeholder
  debug:
    addr: :5001
    prometheus:
      enabled: true
      path: /metrics
redis:
  addr: redis:6379
  db: 2
`, root))
	assert.Equal(t, "filesystem", cfg.StorageDriver.Name())
}
