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

package artifact

import (
	"context"
	"encoding/json"

	"github.com/docker/distribution/manifest/manifestlist"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/lib/log"
	accessorymodel "github.com/goharbor/harbor/src/pkg/accessory/model"
	pkgartifact "github.com/goharbor/harbor/src/pkg/artifact"
)

const (
	buildKitReferenceTypeAnnotation   = "vnd.docker.reference.type"
	buildKitReferenceDigestAnnotation = "vnd.docker.reference.digest"
	buildKitAttestationManifestType   = "attestation-manifest"
)

type indexManifestAbstractor struct {
	artMgr pkgartifact.Manager
}

func (m *indexManifestAbstractor) AbstractMetadata(ctx context.Context, art *pkgartifact.Artifact, content []byte) error {
	art.MediaType = art.ManifestMediaType

	index := &v1.Index{}
	if err := json.Unmarshal(content, index); err != nil {
		return err
	}

	if index.ArtifactType != "" {
		art.ArtifactType = index.ArtifactType
	}

	art.Annotations = index.Annotations
	art.Size += int64(len(content))

	for _, mani := range index.Manifests {
		if candidate := buildKitAttestationCandidate(ctx, m.artMgr, art.RepositoryName, mani); candidate != nil {
			art.Size += candidate.Size
			art.AccessoryCandidates = append(art.AccessoryCandidates, candidate)
			continue
		}

		digest := mani.Digest.String()
		childArt, err := m.artMgr.GetByDigest(ctx, art.RepositoryName, digest)
		if err != nil {
			return err
		}
		art.Size += childArt.Size
		art.References = append(art.References, &pkgartifact.Reference{
			ChildID:     childArt.ID,
			ChildDigest: digest,
			Platform:    mani.Platform,
			URLs:        mani.URLs,
			Annotations: mani.Annotations,
		})
	}

	// CNAB stores its media type in annotations.
	if mediaType := art.Annotations["org.opencontainers.artifactType"]; mediaType != "" {
		art.MediaType = mediaType
	}

	return nil
}

// buildKitAttestationCandidate checks whether a descriptor is a BuildKit
// attestation manifest (provenance/SBOM) and returns an AccessoryCandidate
// linking it to its subject platform image. Returns nil if not an attestation.
//
// Detection uses the standard BuildKit annotations:
//   - vnd.docker.reference.type == "attestation-manifest"
//   - vnd.docker.reference.digest points to the subject platform image
func buildKitAttestationCandidate(ctx context.Context, artMgr pkgartifact.Manager, repository string, desc v1.Descriptor) *pkgartifact.AccessoryCandidate {
	if desc.Annotations[buildKitReferenceTypeAnnotation] != buildKitAttestationManifestType {
		return nil
	}
	targetDigest := desc.Annotations[buildKitReferenceDigestAnnotation]
	if targetDigest == "" {
		return nil
	}

	accessoryArt, err := artMgr.GetByDigest(ctx, repository, desc.Digest.String())
	if err != nil {
		return nil
	}
	targetArt, err := artMgr.GetByDigest(ctx, repository, targetDigest)
	if err != nil {
		return nil
	}

	return &pkgartifact.AccessoryCandidate{
		ArtifactID:        accessoryArt.ID,
		SubArtifactID:     targetArt.ID,
		SubArtifactRepo:   repository,
		SubArtifactDigest: targetDigest,
		Digest:            accessoryArt.Digest,
		Size:              accessoryArt.Size,
		Type:              accessorymodel.TypeBuildKitAttestation,
	}
}

func init() {
	mediaTypes := []string{v1.MediaTypeImageIndex, manifestlist.MediaTypeManifestList}
	if err := registerManifestAbstractor(func(a *abstractor) manifestAbstractor {
		return &indexManifestAbstractor{artMgr: a.artMgr}
	}, mediaTypes...); err != nil {
		log.Errorf("register index manifest abstractor: %v", err)
	}
}
