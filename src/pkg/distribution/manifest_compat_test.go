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

package distribution

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cfgDigest   = "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7"
	layerDigest = "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
	mfDigestA   = "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	mfDigestB   = "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"
	subjDigest  = "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"
)

// Odd spacing and key order on purpose: a parser that re-encodes would change the digest.
var (
	ociManifest = `{"schemaVersion":2,  "mediaType":"application/vnd.oci.image.manifest.v1+json",
 "config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"` + cfgDigest + `","size":7023},
 "layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"` + layerDigest + `","size":32654,
   "annotations":{"org.opencontainers.image.title":"layer.tgz"}}],
 "subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + subjDigest + `","size":527},
 "annotations":{"z":"1","a":"2"}}`

	ociManifestNoMediaType = `{"schemaVersion":2,
 "config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"` + cfgDigest + `","size":7023},
 "layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"` + layerDigest + `","size":32654}]}`

	ociArtifactManifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json",
 "artifactType":"application/vnd.example.sbom",
 "config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},
 "layers":[{"mediaType":"application/spdx+json","digest":"` + layerDigest + `","size":120}]}`

	ociIndex = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json",
 "manifests":[
  {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + mfDigestA + `","size":527,"platform":{"architecture":"amd64","os":"linux"}},
  {"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + mfDigestB + `","size":528,"platform":{"architecture":"arm64","os":"linux","variant":"v8"}}],
 "annotations":{"k":"v"}}`

	dockerManifest = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {"mediaType": "application/vnd.docker.container.image.v1+json","size": 1510,"digest": "` + cfgDigest + `"},
   "layers": [
      {"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip","size": 977,"digest": "` + layerDigest + `"},
      {"mediaType": "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip","size": 1000,"digest": "` + subjDigest + `","urls":["https://example.com/layer"]}
   ]
}`

	dockerList = `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.list.v2+json",
 "manifests":[
  {"mediaType":"application/vnd.docker.distribution.manifest.v2+json","digest":"` + mfDigestA + `","size":527,"platform":{"architecture":"amd64","os":"linux"}},
  {"mediaType":"application/vnd.docker.distribution.manifest.v2+json","digest":"` + mfDigestB + `","size":527,"platform":{"architecture":"arm","os":"linux","variant":"v7"}}]}`
)

type mref struct {
	mediaType string
	digest    string
	size      int64
}

func TestUnmarshalManifestRoundTrip(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		payload     string
		mediaType   string
		refs        []mref
	}{
		{
			name: "oci manifest", contentType: "application/vnd.oci.image.manifest.v1+json", payload: ociManifest,
			mediaType: "application/vnd.oci.image.manifest.v1+json",
			refs: []mref{
				{"application/vnd.oci.image.config.v1+json", cfgDigest, 7023},
				{"application/vnd.oci.image.layer.v1.tar+gzip", layerDigest, 32654},
			},
		},
		{
			name: "oci manifest with content-type parameters", contentType: "application/vnd.oci.image.manifest.v1+json; charset=utf-8", payload: ociManifest,
			mediaType: "application/vnd.oci.image.manifest.v1+json",
			refs: []mref{
				{"application/vnd.oci.image.config.v1+json", cfgDigest, 7023},
				{"application/vnd.oci.image.layer.v1.tar+gzip", layerDigest, 32654},
			},
		},
		{
			name: "oci manifest without mediaType field", contentType: "application/vnd.oci.image.manifest.v1+json", payload: ociManifestNoMediaType,
			mediaType: "application/vnd.oci.image.manifest.v1+json",
			refs: []mref{
				{"application/vnd.oci.image.config.v1+json", cfgDigest, 7023},
				{"application/vnd.oci.image.layer.v1.tar+gzip", layerDigest, 32654},
			},
		},
		{
			name: "oci artifact manifest", contentType: "application/vnd.oci.image.manifest.v1+json", payload: ociArtifactManifest,
			mediaType: "application/vnd.oci.image.manifest.v1+json",
			refs: []mref{
				{"application/vnd.oci.empty.v1+json", "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", 2},
				{"application/spdx+json", layerDigest, 120},
			},
		},
		{
			name: "oci index", contentType: "application/vnd.oci.image.index.v1+json", payload: ociIndex,
			mediaType: "application/vnd.oci.image.index.v1+json",
			refs: []mref{
				{"application/vnd.oci.image.manifest.v1+json", mfDigestA, 527},
				{"application/vnd.oci.image.manifest.v1+json", mfDigestB, 528},
			},
		},
		{
			name: "docker schema2", contentType: "application/vnd.docker.distribution.manifest.v2+json", payload: dockerManifest,
			mediaType: "application/vnd.docker.distribution.manifest.v2+json",
			refs: []mref{
				{"application/vnd.docker.container.image.v1+json", cfgDigest, 1510},
				{"application/vnd.docker.image.rootfs.diff.tar.gzip", layerDigest, 977},
				{"application/vnd.docker.image.rootfs.foreign.diff.tar.gzip", subjDigest, 1000},
			},
		},
		{
			name: "docker manifest list", contentType: "application/vnd.docker.distribution.manifest.list.v2+json", payload: dockerList,
			mediaType: "application/vnd.docker.distribution.manifest.list.v2+json",
			refs: []mref{
				{"application/vnd.docker.distribution.manifest.v2+json", mfDigestA, 527},
				{"application/vnd.docker.distribution.manifest.v2+json", mfDigestB, 527},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, desc, err := UnmarshalManifest(tc.contentType, []byte(tc.payload))
			require.NoError(t, err)

			wantDigest := digest.FromString(tc.payload)
			assert.Equal(t, wantDigest, desc.Digest, "descriptor digest must hash the original bytes")
			assert.Equal(t, int64(len(tc.payload)), desc.Size)
			assert.Equal(t, tc.mediaType, desc.MediaType)

			mt, payload, err := m.Payload()
			require.NoError(t, err)
			assert.Equal(t, tc.mediaType, mt)
			assert.Equal(t, tc.payload, string(payload), "payload must be byte-identical")

			var got []mref
			for _, r := range m.References() {
				got = append(got, mref{r.MediaType, r.Digest.String(), r.Size})
			}
			assert.Equal(t, tc.refs, got)
		})
	}
}

// Platform is not asserted: v2 drops it from References() and Harbor reads
// platforms by decoding the index itself.
func TestUnmarshalManifestKeepsURLs(t *testing.T) {
	m, _, err := UnmarshalManifest("application/vnd.docker.distribution.manifest.v2+json", []byte(dockerManifest))
	require.NoError(t, err)
	refs := m.References()
	require.Len(t, refs, 3)
	assert.Equal(t, []string{"https://example.com/layer"}, refs[2].URLs)
}

// Inputs that must fail cleanly, never panic or parse as something else.
func TestUnmarshalManifestRejects(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		payload     string
	}{
		{"truncated json", "application/vnd.oci.image.manifest.v1+json", `{"schemaVersion":2,`},
		{"not json", "application/vnd.docker.distribution.manifest.v2+json", `hello`},
		{"oci payload under docker content type", "application/vnd.docker.distribution.manifest.v2+json", ociManifest},
		{"docker payload under oci content type", "application/vnd.oci.image.manifest.v1+json", dockerManifest},
		{"index payload under manifest content type", "application/vnd.oci.image.manifest.v1+json", ociIndex},
		{"list payload under index content type", "application/vnd.oci.image.index.v1+json", dockerList},
		{"bad content type syntax", "application/vnd.oci.image.manifest.v1+json; =", ociManifest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_, _, err := UnmarshalManifest(tc.contentType, []byte(tc.payload))
				assert.Error(t, err)
			})
		})
	}
}
