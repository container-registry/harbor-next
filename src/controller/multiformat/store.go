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

package multiformat

import (
	"context"
	"encoding/json"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/registry"
)

// store is the OCI backend seam over Harbor's global registry client
// (registry.Cli). It is the only layer that knows OCI wire details. Adapters
// never see it; the mapping layer (mapper, mapper_maven, index_ops) uses it.
// It authenticates as Harbor's internal registry credential (NOT the requesting
// user) — RBAC MUST be enforced upstream before any store call.
type store struct {
	cli registry.Client
}

// newStore builds a store over the given registry client.
func newStore(cli registry.Client) *store {
	return &store{cli: cli}
}

// blobDescriptor computes the content descriptor for data without pushing it.
// Used by the mapping layer to derive layer0's client-pinned digest.
func blobDescriptor(mediaType string, data []byte) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}
}

// PushBlob uploads content and returns its descriptor. The digest is computed
// over the exact bytes provided — for layer0 this is the unmodified artifact,
// so the client-pinned digest is preserved (no recompression). Idempotent:
// re-pushing identical bytes is a no-op by digest.
func (s *store) PushBlob(_ context.Context, repo, mediaType string, data []byte) (ocispec.Descriptor, error) {
	desc := blobDescriptor(mediaType, data)
	exists, err := s.cli.BlobExist(repo, desc.Digest.String())
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "check blob existence")
	}
	if exists {
		return desc, nil
	}
	if err := s.cli.PushBlob(repo, desc.Digest.String(), desc.Size, bytesReader(data)); err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "push blob")
	}
	return desc, nil
}

// FetchBlob streams a blob by descriptor. The caller must Close the reader.
func (s *store) FetchBlob(_ context.Context, repo string, desc ocispec.Descriptor) (io.ReadCloser, error) {
	_, rc, err := s.cli.PullBlob(repo, desc.Digest.String())
	if err != nil {
		return nil, errors.Wrap(err, "pull blob")
	}
	return rc, nil
}

// PushManifest serializes m as v1.MediaTypeImageManifest, pushes+tags it, and
// returns its descriptor. The per-version manifest MUST be ImageManifest or the
// Harbor abstractor rejects it.
func (s *store) PushManifest(_ context.Context, repo, tag string, m ocispec.Manifest) (ocispec.Descriptor, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	dgst, err := s.cli.PushManifest(repo, tag, ocispec.MediaTypeImageManifest, data)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "push manifest %s:%s", repo, tag)
	}
	desc := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: m.ArtifactType,
		Digest:       digest.Digest(dgst),
		Size:         int64(len(data)),
	}
	return desc, nil
}

// PushIndex pushes the canonical _index bytes as v1.MediaTypeImageIndex and
// moves the reserved tag. The caller is responsible for canonical ordering.
func (s *store) PushIndex(_ context.Context, repo, tag string, data []byte) (ocispec.Descriptor, error) {
	dgst, err := s.cli.PushManifest(repo, tag, ocispec.MediaTypeImageIndex, data)
	if err != nil {
		return ocispec.Descriptor{}, errors.Wrapf(err, "push _index %s:%s", repo, tag)
	}
	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.Digest(dgst),
		Size:      int64(len(data)),
	}, nil
}

// FetchIndex fetches the _index control artifact by its reserved tag. ok==false
// (no error) means the package has no _index yet (404). Concurrency on the
// _index RMW is guaranteed SOLELY by the Postgres advisory lock (no registry
// CAS on tags).
func (s *store) FetchIndex(_ context.Context, repo, tag string) (ocispec.Index, ocispec.Descriptor, bool, error) {
	exist, _, err := s.cli.ManifestExist(repo, tag)
	if err != nil {
		return ocispec.Index{}, ocispec.Descriptor{}, false, errors.Wrap(err, "check _index existence")
	}
	if !exist {
		return ocispec.Index{}, ocispec.Descriptor{}, false, nil
	}
	manifest, dgst, err := s.cli.PullManifest(repo, tag, ocispec.MediaTypeImageIndex)
	if err != nil {
		return ocispec.Index{}, ocispec.Descriptor{}, false, errors.Wrap(err, "pull _index")
	}
	_, payload, err := manifest.Payload()
	if err != nil {
		return ocispec.Index{}, ocispec.Descriptor{}, false, err
	}
	var idx ocispec.Index
	if err := json.Unmarshal(payload, &idx); err != nil {
		return ocispec.Index{}, ocispec.Descriptor{}, false, errors.Wrap(err, "decode _index")
	}
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.Digest(dgst),
		Size:      int64(len(payload)),
	}
	return idx, desc, true, nil
}

// FetchManifest fetches and decodes a per-version manifest by tag or digest.
func (s *store) FetchManifest(_ context.Context, repo, ref string) (ocispec.Manifest, ocispec.Descriptor, error) {
	manifest, dgst, err := s.cli.PullManifest(repo, ref, ocispec.MediaTypeImageManifest)
	if err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, errors.Wrap(err, "pull manifest")
	}
	_, payload, err := manifest.Payload()
	if err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, err
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(payload, &m); err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, err
	}
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.Digest(dgst),
		Size:      int64(len(payload)),
	}
	return m, desc, nil
}
