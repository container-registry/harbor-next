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

	"github.com/goharbor/harbor/src/controller/artifact/manifest"
	"github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/registry"
)

// Abstractor abstracts the metadata of artifact
type Abstractor interface {
	// AbstractMetadata abstracts the metadata for the specific artifact type into the artifact model,
	AbstractMetadata(ctx context.Context, artifact *artifact.Artifact) error
}

// NewAbstractor creates a new abstractor
func NewAbstractor() Abstractor {
	return &abstractor{
		regCli: registry.Cli,
	}
}

type abstractor struct {
	regCli registry.Client
}

func (a *abstractor) AbstractMetadata(ctx context.Context, art *artifact.Artifact) error {
	// read manifest content
	mf, _, err := a.regCli.PullManifest(art.RepositoryName, art.Digest)
	if err != nil {
		return err
	}
	manifestMediaType, content, err := mf.Payload()
	if err != nil {
		return err
	}
	art.ManifestMediaType = manifestMediaType

	abs, err := manifest.Get(art.ManifestMediaType)
	if err != nil {
		return err
	}
	if err = abs.Abstract(ctx, art, content); err != nil {
		return err
	}

<<<<<<< HEAD
	artifact.Size = int64(len(content))
	for _, blob := range blobs {
		artifact.Size += blob.Size
	}

	return nil
}

// the artifact is enveloped by OCI manifest or docker manifest v2
func (a *abstractor) abstractManifestV2Metadata(artifact *artifact.Artifact, content []byte) error {
	manifest := &v1.Manifest{}
	if err := json.Unmarshal(content, manifest); err != nil {
		return err
	}
	// use the "manifest.config.mediatype" as the media type of the artifact
	artifact.MediaType = manifest.Config.MediaType
	if manifest.Annotations[wasm.AnnotationVariantKey] == wasm.AnnotationVariantValue || manifest.Annotations[wasm.AnnotationHandlerKey] == wasm.AnnotationHandlerValue {
		artifact.MediaType = wasm.MediaType
	}
	/*
		https://github.com/opencontainers/distribution-spec/blob/v1.1.0/spec.md#listing-referrers
		For referrers list, if the artifactType is empty or missing in the image manifest, the value of artifactType MUST be set to the config descriptor mediaType value
	*/
	if manifest.ArtifactType != "" {
		artifact.ArtifactType = manifest.ArtifactType
	} else {
		artifact.ArtifactType = manifest.Config.MediaType
	}

	// set size
	artifact.Size = int64(len(content)) + manifest.Config.Size
	for _, layer := range manifest.Layers {
		artifact.Size += layer.Size
	}
	// set annotations
	artifact.Annotations = manifest.Annotations
	return nil
}

// the artifact is enveloped by OCI index or docker manifest list
func (a *abstractor) abstractIndexMetadata(ctx context.Context, art *artifact.Artifact, content []byte) error {
	// the identity of index is still in progress, we use the manifest mediaType
	// as the media type of artifact
	art.MediaType = art.ManifestMediaType

	index := &v1.Index{}
	if err := json.Unmarshal(content, index); err != nil {
		return err
	}

	/*
		https://github.com/opencontainers/distribution-spec/blob/v1.1.0/spec.md#listing-referrers
		For referrers list, If the artifactType is empty or missing in an index, the artifactType MUST be omitted.
	*/
	if index.ArtifactType != "" {
		art.ArtifactType = index.ArtifactType
	} else {
		art.ArtifactType = ""
	}

	// set annotations
	art.Annotations = index.Annotations

	art.Size += int64(len(content))
	// populate the referenced artifacts
	for _, mani := range index.Manifests {
		candidate, err := a.toBuildKitAttestationCandidate(ctx, art.RepositoryName, mani, index.Manifests)
		if err != nil {
			return err
		}
		if candidate != nil {
			art.Size += candidate.Size
			art.AccessoryCandidates = append(art.AccessoryCandidates, candidate)
			continue
		}

		digest := mani.Digest.String()
		// make sure the child artifact exist
		ar, err := a.artMgr.GetByDigest(ctx, art.RepositoryName, digest)
		if err != nil {
			return err
		}
		art.Size += ar.Size
		art.References = append(art.References, &artifact.Reference{
			ChildID:     ar.ID,
			ChildDigest: digest,
			Platform:    mani.Platform,
			URLs:        mani.URLs,
			Annotations: mani.Annotations,
		})
	}

	// Currently, CNAB put its media type inside the annotations
	// try to parse the artifact media type from the annotations
	if art.Annotations != nil {
		mediaType := art.Annotations["org.opencontainers.artifactType"]
		if len(mediaType) > 0 {
			art.MediaType = mediaType
		}
	}

	return nil
=======
	return processor.Get(art.ResolveArtifactType()).AbstractMetadata(ctx, art, content)
>>>>>>> 93de6ff8f (Refactor manifest abstraction into a registry (#23647))
}
