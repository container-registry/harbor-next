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

package cargo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	ps "github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/controller/artifact/processor/base"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
)

const (
	// ArtifactTypeCargo defines the artifact type for Cargo crates.
	ArtifactTypeCargo = "CARGO"

	AdditionTypeDependencies = "DEPENDENCIES"
	AdditionTypeFiles        = "FILES"

	artifactMediaType = "application/vnd.harbor.cargo.crate.v1"
	filesMediaType    = "application/vnd.harbor.package.files.v1+json"
	contentTypeJSON   = "application/json; charset=utf-8"
)

func init() {
	pc := &processor{ManifestProcessor: base.NewManifestProcessor()}
	if err := ps.Register(pc, artifactMediaType, ArtifactTypeCargo); err != nil {
		log.Errorf("failed to register processor for Cargo artifact: %v", err)
		return
	}
}

type processor struct {
	*base.ManifestProcessor
}

type versionMetadata struct {
	Metadata crateMetadata `json:"metadata"`
}

type crateMetadata struct {
	Deps []crateDependency `json:"deps"`
}

type crateDependency struct {
	Name       string `json:"name"`
	VersionReq string `json:"req"`
	Kind       string `json:"kind"`
}

type dependency struct {
	Ecosystem string `json:"ecosystem"`
	Package   string `json:"package"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Type      string `json:"type"`
}

func (p *processor) AbstractAddition(ctx context.Context, artifact *artifact.Artifact, addition string) (*ps.Addition, error) {
	switch addition {
	case AdditionTypeDependencies:
		dependencies, err := p.dependencies(artifact)
		if err != nil {
			return nil, err
		}
		return &ps.Addition{ContentType: contentTypeJSON, Content: dependencies}, nil
	case AdditionTypeFiles:
		files, err := p.layerByMediaType(ctx, artifact, filesMediaType)
		if err != nil {
			return nil, err
		}
		return &ps.Addition{ContentType: contentTypeJSON, Content: files}, nil
	default:
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("addition %s isn't supported for %s", addition, ArtifactTypeCargo)
	}
}

func (p *processor) GetArtifactType(_ context.Context, _ *artifact.Artifact) string {
	return ArtifactTypeCargo
}

func (p *processor) ListAdditionTypes(_ context.Context, _ *artifact.Artifact) []string {
	return []string{AdditionTypeFiles, AdditionTypeDependencies}
}

func (p *processor) dependencies(artifact *artifact.Artifact) ([]byte, error) {
	metadata, err := metadataFromArtifact(artifact)
	if err != nil {
		return nil, err
	}
	deps := make([]dependency, 0, len(metadata.Metadata.Deps))
	for _, dep := range metadata.Metadata.Deps {
		depType := "Direct"
		if dep.Kind == "dev" || dep.Kind == "build" {
			depType = "Development"
		}
		deps = append(deps, dependency{
			Ecosystem: "cargo",
			Package:   dep.Name,
			Name:      dep.Name,
			Version:   dep.VersionReq,
			Type:      depType,
		})
	}
	return json.Marshal(deps)
}

func metadataFromArtifact(artifact *artifact.Artifact) (*versionMetadata, error) {
	if artifact == nil {
		return &versionMetadata{}, nil
	}
	raw, ok := artifact.ExtraAttrs["metadata"]
	if !ok {
		return &versionMetadata{}, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	metadata := &versionMetadata{}
	if err := json.Unmarshal(payload, metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (p *processor) layerByMediaType(_ context.Context, artifact *artifact.Artifact, mediaType string) ([]byte, error) {
	manifest, err := p.manifest(artifact)
	if err != nil {
		return nil, err
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType != mediaType {
			continue
		}
		_, blob, err := p.RegCli.PullBlob(artifact.RepositoryName, layer.Digest.String())
		if err != nil {
			return nil, err
		}
		defer blob.Close()
		return io.ReadAll(blob)
	}
	return nil, errors.NotFoundError(fmt.Errorf("%s layer not found", mediaType))
}

func (p *processor) manifest(artifact *artifact.Artifact) (*v1.Manifest, error) {
	mf, _, err := p.RegCli.PullManifest(artifact.RepositoryName, artifact.Digest)
	if err != nil {
		return nil, err
	}
	_, payload, err := mf.Payload()
	if err != nil {
		return nil, err
	}
	manifest := &v1.Manifest{}
	if err := json.Unmarshal(payload, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}
