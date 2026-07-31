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

package sbom

import (
	"context"
	"encoding/json"
	"io"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/controller/artifact/processor/base"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
)

const (
	// ArtifactTypeSBOM is the artifact type for SBOM, it's scope is only used in the processor
	ArtifactTypeSBOM = "SBOM"
	// processorMediaType is the media type for SBOM, it's scope is only used to register the processor
	processorMediaType = "application/vnd.goharbor.harbor.sbom.v1"
	// MediaTypeSPDX is the SPDX SBOM media type
	MediaTypeSPDX = "application/spdx+json"
	// MediaTypeCycloneDX is the CycloneDX SBOM media type
	MediaTypeCycloneDX = "application/vnd.cyclonedx+json"
)

var SupportedMediaTypes = []string{
	processorMediaType,
	MediaTypeSPDX,
	MediaTypeCycloneDX,
}

func init() {
	pc := &Processor{}
	pc.ManifestProcessor = base.NewManifestProcessor()
	for _, mt := range SupportedMediaTypes {
		if err := processor.Register(pc, mt); err != nil {
			log.Errorf("failed to register processor for media type %s: %v", mt, err)
		}
	}
}

// Processor is the processor for SBOM
type Processor struct {
	*base.ManifestProcessor
}

// AbstractAddition returns the addition for SBOM
func (m *Processor) AbstractAddition(_ context.Context, art *artifact.Artifact, _ string) (*processor.Addition, error) {
	man, _, err := m.RegCli.PullManifest(art.RepositoryName, art.Digest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to pull manifest")
	}
	_, payload, err := man.Payload()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get payload")
	}
	manifest := &v1.Manifest{}
	if err := json.Unmarshal(payload, manifest); err != nil {
		return nil, err
	}
	// SBOM artifact should only have one layer
	if len(manifest.Layers) != 1 {
		return nil, errors.New(nil).WithCode(errors.NotFoundCode).WithMessage("The sbom is not found")
	}
	layerDgst := manifest.Layers[0].Digest.String()
	_, blob, err := m.RegCli.PullBlob(art.RepositoryName, layerDgst)
	if err != nil {
		return nil, errors.Wrap(err, "failed to pull the blob")
	}
	defer blob.Close()
	content, err := io.ReadAll(blob)
	if err != nil {
		return nil, err
	}
	contentType := art.ResolveArtifactType()
	switch contentType {
	case MediaTypeSPDX, MediaTypeCycloneDX:
		// use dynamically resolved type
	default:
		contentType = processorMediaType
	}

	return &processor.Addition{
		Content:     content,
		ContentType: contentType,
	}, nil
}

// GetArtifactType the artifact type is used to display the artifact type in the UI
func (m *Processor) GetArtifactType(_ context.Context, _ *artifact.Artifact) string {
	return ArtifactTypeSBOM
}
