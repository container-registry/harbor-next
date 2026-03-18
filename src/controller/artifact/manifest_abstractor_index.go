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
	"github.com/docker/distribution/manifest/schema2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/lib/log"
	accessorymodel "github.com/goharbor/harbor/src/pkg/accessory/model"
	pkgartifact "github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/registry"
)

const (
	buildKitReferenceTypeAnnotation   = "vnd.docker.reference.type"
	buildKitReferenceDigestAnnotation = "vnd.docker.reference.digest"
	buildKitAttestationManifestType   = "attestation-manifest"
	inTotoLayerMediaType              = "application/vnd.in-toto+json"
)

type indexManifestAbstractor struct {
	artMgr pkgartifact.Manager
	regCli registry.Client
}

func (m *indexManifestAbstractor) AbstractMetadata(ctx context.Context, art *pkgartifact.Artifact, content []byte) error {
	art.MediaType = art.ManifestMediaType

	index := &v1.Index{}
	if err := json.Unmarshal(content, index); err != nil {
		return err
	}

	if index.ArtifactType != "" {
		art.ArtifactType = index.ArtifactType
	} else {
		art.ArtifactType = ""
	}

	art.Annotations = index.Annotations
	art.Size += int64(len(content))

	for _, mani := range index.Manifests {
		candidate, err := m.toBuildKitAttestationCandidate(ctx, art.RepositoryName, mani)
		if err != nil {
			return err
		}
		if candidate != nil {
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

	if art.Annotations != nil {
		mediaType := art.Annotations["org.opencontainers.artifactType"]
		if len(mediaType) > 0 {
			art.MediaType = mediaType
		}
	}

	return nil
}

func (m *indexManifestAbstractor) toBuildKitAttestationCandidate(ctx context.Context, repository string, descriptor v1.Descriptor) (*pkgartifact.AccessoryCandidate, error) {
	if descriptor.MediaType != v1.MediaTypeImageManifest && descriptor.MediaType != schema2.MediaTypeManifest {
		return nil, nil
	}
	if descriptor.Annotations[buildKitReferenceTypeAnnotation] != buildKitAttestationManifestType {
		return nil, nil
	}

	targetDigest := descriptor.Annotations[buildKitReferenceDigestAnnotation]
	if targetDigest == "" {
		return nil, nil
	}

	if !m.isBuildKitAttestationManifest(repository, descriptor.Digest.String()) {
		return nil, nil
	}

	accessoryArt, err := m.artMgr.GetByDigest(ctx, repository, descriptor.Digest.String())
	if err != nil {
		return nil, err
	}
	targetArt, err := m.artMgr.GetByDigest(ctx, repository, targetDigest)
	if err != nil {
		return nil, err
	}

	return &pkgartifact.AccessoryCandidate{
		ArtifactID:        accessoryArt.ID,
		SubArtifactID:     targetArt.ID,
		SubArtifactRepo:   repository,
		SubArtifactDigest: targetDigest,
		Digest:            accessoryArt.Digest,
		Size:              accessoryArt.Size,
		Type:              accessorymodel.TypeBuildKitAttestation,
	}, nil
}

func (m *indexManifestAbstractor) isBuildKitAttestationManifest(repository, digest string) bool {
	manifest, _, err := m.regCli.PullManifest(repository, digest)
	if err != nil {
		return false
	}

	mediaType, content, err := manifest.Payload()
	if err != nil {
		return false
	}
	if mediaType != v1.MediaTypeImageManifest && mediaType != schema2.MediaTypeManifest {
		return false
	}

	imageManifest := &v1.Manifest{}
	if err := json.Unmarshal(content, imageManifest); err != nil {
		return false
	}

	for _, layer := range imageManifest.Layers {
		if layer.MediaType == inTotoLayerMediaType {
			return true
		}
	}

	return false
}

func init() {
	mediaTypes := []string{v1.MediaTypeImageIndex, manifestlist.MediaTypeManifestList}
	if err := registerManifestAbstractor(func(a *abstractor) manifestAbstractor {
		return &indexManifestAbstractor{artMgr: a.artMgr, regCli: a.regCli}
	}, mediaTypes...); err != nil {
		log.Errorf("failed to register manifest abstractor for media type %v: %v", mediaTypes, err)
	}
}
