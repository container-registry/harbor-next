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

package image

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	trans "github.com/goharbor/harbor/src/controller/replication/transfer"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/distribution"
)

type mf struct{ ct, payload string }

// recordingRegistry serves manifests by digest or tag and records every write.
type recordingRegistry struct {
	fakeRegistry
	manifests map[string]mf // "repo@ref" -> manifest
	blobs     map[string]bool
	pulledMf  []string
	pulledBl  []string
	pushedMf  []string
	pushedBl  []string
	pushed    map[string]mf
}

func newRecording() *recordingRegistry {
	return &recordingRegistry{manifests: map[string]mf{}, blobs: map[string]bool{}, pushed: map[string]mf{}}
}

func (r *recordingRegistry) add(repo, ct, payload string, tags ...string) string {
	d := digest.FromString(payload).String()
	for _, ref := range append(tags, d) {
		r.manifests[repo+"@"+ref] = mf{ct, payload}
	}
	return d
}

func (r *recordingRegistry) ManifestExist(repo, ref string) (bool, *distribution.Descriptor, error) {
	m, ok := r.manifests[repo+"@"+ref]
	if !ok {
		return false, nil, nil
	}
	return true, &distribution.Descriptor{Digest: digest.FromString(m.payload), MediaType: m.ct, Size: int64(len(m.payload))}, nil
}

func (r *recordingRegistry) PullManifest(repo, ref string, _ ...string) (distribution.Manifest, string, error) {
	r.pulledMf = append(r.pulledMf, ref)
	m, ok := r.manifests[repo+"@"+ref]
	if !ok {
		return nil, "", errors.NotFoundError(nil)
	}
	man, _, err := distribution.UnmarshalManifest(m.ct, []byte(m.payload))
	if err != nil {
		return nil, "", err
	}
	return man, digest.FromString(m.payload).String(), nil
}

func (r *recordingRegistry) PushManifest(repo, ref, ct string, payload []byte) (string, error) {
	r.pushedMf = append(r.pushedMf, ref)
	r.pushed[ref] = mf{ct, string(payload)}
	r.manifests[repo+"@"+ref] = mf{ct, string(payload)}
	return digest.FromBytes(payload).String(), nil
}

func (r *recordingRegistry) BlobExist(_, d string) (bool, error) { return r.blobs[d], nil }

func (r *recordingRegistry) PullBlob(_, d string) (int64, io.ReadCloser, error) {
	r.pulledBl = append(r.pulledBl, d)
	return 1, io.NopCloser(bytes.NewReader([]byte{'a'})), nil
}

func (r *recordingRegistry) PushBlob(_, d string, _ int64, _ io.Reader) error {
	r.pushedBl = append(r.pushedBl, d)
	return nil
}

const (
	tCfg   = "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7"
	tLayer = "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
	tFrgn  = "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"
)

func tImage(mt, cfgType, layerType string) string {
	return fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"digest":%q,"size":2},"layers":[{"mediaType":%q,"digest":%q,"size":3}]}`,
		mt, cfgType, tCfg, layerType, tLayer)
}

func tIndex(mt, childType string, children ...string) string {
	var e []string
	for _, c := range children {
		e = append(e, fmt.Sprintf(`{"mediaType":%q,"digest":%q,"size":%d}`, childType, digest.FromString(c), len(c)))
	}
	return fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[%s]}`, mt, strings.Join(e, ","))
}

var (
	tOCI    = tImage("application/vnd.oci.image.manifest.v1+json", "application/vnd.oci.image.config.v1+json", "application/vnd.oci.image.layer.v1.tar+gzip")
	tDocker = tImage("application/vnd.docker.distribution.manifest.v2+json", "application/vnd.docker.container.image.v1+json", "application/vnd.docker.image.rootfs.diff.tar.gzip")
)

func newTransfer(src, dst *recordingRegistry) *transfer {
	return &transfer{logger: log.DefaultLogger(), isStopped: func() bool { return false }, src: src, dst: dst}
}

// Which descriptor media types are copied as artifacts, as blobs, or skipped.
func TestCopyContentRouting(t *testing.T) {
	const d = "sha256:aa11111111111111111111111111111111111111111111111111111111111111"
	artifactTypes := []string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}
	for _, mt := range artifactTypes {
		t.Run("artifact/"+mt, func(t *testing.T) {
			src, dst := newRecording(), newRecording()
			err := newTransfer(src, dst).copyContent(distribution.Descriptor{MediaType: mt, Digest: digest.Digest(d)}, "s", "d", trans.NewOptions())
			assert.Error(t, err, "missing source manifest surfaces as an error")
			assert.Equal(t, []string{d}, src.pulledMf)
			assert.Empty(t, src.pulledBl)
		})
	}
	blobTypes := []string{
		"application/vnd.oci.image.layer.v1.tar+gzip",
		"application/vnd.oci.image.config.v1+json",
		"application/vnd.docker.image.rootfs.diff.tar.gzip",
		"application/vnd.docker.container.image.v1+json",
		"application/vnd.oci.empty.v1+json",
		"application/octet-stream",
		"",
	}
	for _, mt := range blobTypes {
		t.Run("blob/"+mt, func(t *testing.T) {
			src, dst := newRecording(), newRecording()
			require.NoError(t, newTransfer(src, dst).copyContent(distribution.Descriptor{MediaType: mt, Digest: digest.Digest(d), Size: 1}, "s", "d", trans.NewOptions()))
			assert.Empty(t, src.pulledMf)
			assert.Equal(t, []string{d}, src.pulledBl)
			assert.Equal(t, []string{d}, dst.pushedBl)
		})
	}
	t.Run("foreign layer is skipped", func(t *testing.T) {
		src, dst := newRecording(), newRecording()
		require.NoError(t, newTransfer(src, dst).copyContent(distribution.Descriptor{MediaType: "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip", Digest: digest.Digest(d)}, "s", "d", trans.NewOptions()))
		assert.Empty(t, src.pulledMf)
		assert.Empty(t, src.pulledBl)
		assert.Empty(t, dst.pushedBl)
	})
}

func TestCopyArtifactIndexAndBytes(t *testing.T) {
	for _, tc := range []struct{ ct, childType, child string }{
		{"application/vnd.oci.image.index.v1+json", "application/vnd.oci.image.manifest.v1+json", tOCI},
		{"application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.docker.distribution.manifest.v2+json", tDocker},
	} {
		t.Run(tc.ct, func(t *testing.T) {
			src, dst := newRecording(), newRecording()
			childD := src.add("s", tc.childType, tc.child)
			idx := tIndex(tc.ct, tc.childType, tc.child)
			src.add("s", tc.ct, idx, "v1")

			require.NoError(t, newTransfer(src, dst).copyArtifact("s", "v1", "d", "v1", true, trans.NewOptions()))
			assert.Equal(t, []string{childD, "v1"}, dst.pushedMf, "children before the index")
			assert.Equal(t, mf{tc.ct, idx}, dst.pushed["v1"], "index bytes unchanged")
			assert.Equal(t, mf{tc.childType, tc.child}, dst.pushed[childD], "child bytes unchanged")
			assert.ElementsMatch(t, []string{tCfg, tLayer}, dst.pushedBl)
		})
	}
}

func TestCopyArtifactSkipsForeignLayer(t *testing.T) {
	src, dst := newRecording(), newRecording()
	m := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","digest":%q,"size":2},"layers":[{"mediaType":"application/vnd.docker.image.rootfs.foreign.diff.tar.gzip","digest":%q,"size":9,"urls":["https://example.com/x"]},{"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","digest":%q,"size":3}]}`, tCfg, tFrgn, tLayer)
	src.add("s", "application/vnd.docker.distribution.manifest.v2+json", m, "v1")
	dst.blobs[tLayer] = true

	require.NoError(t, newTransfer(src, dst).copyArtifact("s", "v1", "d", "v1", true, trans.NewOptions()))
	assert.Equal(t, []string{tCfg}, dst.pushedBl)
	assert.Equal(t, m, dst.pushed["v1"].payload)
}

// Replication must never replace a different artifact unless override is set.
func TestCopyArtifactOverrideRules(t *testing.T) {
	t.Run("same digest is skipped", func(t *testing.T) {
		src, dst := newRecording(), newRecording()
		src.add("s", "application/vnd.oci.image.manifest.v1+json", tOCI, "v1")
		dst.add("d", "application/vnd.oci.image.manifest.v1+json", tOCI, "v1")
		require.NoError(t, newTransfer(src, dst).copyArtifact("s", "v1", "d", "v1", true, trans.NewOptions()))
		assert.Empty(t, dst.pushedMf)
		assert.Empty(t, dst.pushedBl)
	})
	t.Run("different digest without override is skipped silently", func(t *testing.T) {
		src, dst := newRecording(), newRecording()
		src.add("s", "application/vnd.oci.image.manifest.v1+json", tOCI, "v1")
		dst.add("d", "application/vnd.docker.distribution.manifest.v2+json", tDocker, "v1")
		require.NoError(t, newTransfer(src, dst).copyArtifact("s", "v1", "d", "v1", false, trans.NewOptions()))
		assert.Empty(t, dst.pushedMf)
		assert.Equal(t, tDocker, dst.manifests["d@v1"].payload)
	})
	t.Run("different digest with override replaces", func(t *testing.T) {
		src, dst := newRecording(), newRecording()
		src.add("s", "application/vnd.oci.image.manifest.v1+json", tOCI, "v1")
		dst.add("d", "application/vnd.docker.distribution.manifest.v2+json", tDocker, "v1")
		require.NoError(t, newTransfer(src, dst).copyArtifact("s", "v1", "d", "v1", true, trans.NewOptions()))
		assert.Equal(t, tOCI, dst.manifests["d@v1"].payload)
	})
}
