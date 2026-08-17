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
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// upsertIndexEntry replaces (or adds) the descriptor for a version in the index,
// stamping the per-version annotations used for reconcile AND the immutability
// check: AnnPayloadDig/AnnPayloadSize are the layer0 (client-pinned) digest, so
// findVersionPayload can compare a re-push's bytes against the stored bytes.
func upsertIndexEntry(idx ocispec.Index, verDesc, payloadDesc ocispec.Descriptor, version string, created time.Time, yanked bool) ocispec.Index {
	if verDesc.Annotations == nil {
		verDesc.Annotations = map[string]string{}
	}
	verDesc.Annotations[AnnVersion] = version
	verDesc.Annotations[AnnCreated] = created.UTC().Format(time.RFC3339)
	verDesc.Annotations[AnnYanked] = strconv.FormatBool(yanked)
	// Carry payload facts forward (layer0 digest + size) so the immutability
	// check discriminates on bytes and reconcile can recover the payload ref.
	verDesc.Annotations[AnnPayloadDig] = payloadDesc.Digest.String()
	verDesc.Annotations[AnnPayloadSize] = strconv.FormatInt(payloadDesc.Size, 10)
	out := idx
	replaced := false
	for i := range out.Manifests {
		if out.Manifests[i].Annotations[AnnVersion] == version {
			out.Manifests[i] = verDesc
			replaced = true
			break
		}
	}
	if !replaced {
		out.Manifests = append(out.Manifests, verDesc)
	}
	return out
}

// findVersionPayload returns the recorded payload digest for a version, if the
// index carries it as an annotation.
func findVersionPayload(idx ocispec.Index, version string) (string, bool) {
	for _, d := range idx.Manifests {
		if d.Annotations[AnnVersion] == version {
			return d.Annotations[AnnPayloadDig], true
		}
	}
	return "", false
}

// setDistTags writes the dist-tags map into the index-level annotations as
// canonical JSON (PG-authoritative mutable state lives here in OCI).
func setDistTags(idx *ocispec.Index, tags map[string]string) {
	if idx.Annotations == nil {
		idx.Annotations = map[string]string{}
	}
	b, _ := json.Marshal(tags)
	idx.Annotations[AnnDistTags] = string(b)
}

// readDistTags decodes the dist-tags map from index-level annotations.
func readDistTags(idx ocispec.Index) map[string]string {
	out := map[string]string{}
	if idx.Annotations == nil {
		return out
	}
	if s, ok := idx.Annotations[AnnDistTags]; ok && s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseDigest(s string) digest.Digest { return digest.Digest(s) }

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func readAll(r io.Reader) []byte {
	b, _ := io.ReadAll(r)
	return b
}

func readAllClose(rc io.ReadCloser) []byte {
	defer rc.Close()
	return readAll(rc)
}

// canonicalIndex serializes the _index deterministically: descriptors sorted by
// version annotation, annotation maps sorted (Go json sorts map keys), stable
// field order. Same logical state -> same bytes -> stable digest, which the
// byte-identical reconcile gate and oci-layout dedup need.
func canonicalIndex(idx ocispec.Index) ([]byte, error) {
	sortManifestsByVersion(idx.Manifests)
	idx.Versioned = specs.Versioned{SchemaVersion: 2}
	idx.MediaType = ocispec.MediaTypeImageIndex
	idx.ArtifactType = IndexArtifactType
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(idx); err != nil {
		return nil, err
	}
	// json.Encoder appends a newline; trim for a stable canonical form.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
