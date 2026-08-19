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

package pypi

import (
	"context"
	"encoding/json"
	"sort"

	ps "github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/controller/artifact/processor/base"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
)

const (
	// ArtifactTypePyPI defines the artifact type for PyPI packages.
	ArtifactTypePyPI = "PYPI"

	AdditionTypeDependencies = "DEPENDENCIES"
	AdditionTypeFiles        = "FILES"

	artifactMediaType = "application/vnd.harbor.pypi.package.v1"
	contentTypeJSON   = "application/json; charset=utf-8"
)

func init() {
	pc := &processor{ManifestProcessor: base.NewManifestProcessor()}
	if err := ps.Register(pc, artifactMediaType, ArtifactTypePyPI); err != nil {
		log.Errorf("failed to register processor for PyPI artifact: %v", err)
		return
	}
}

type processor struct {
	*base.ManifestProcessor
}

type versionMetadata struct {
	RequiresDist  []string               `json:"requires_dist,omitempty"`
	Distributions []distributionMetadata `json:"distributions,omitempty"`
}

type distributionMetadata struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type fileEntry struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
	Size int64  `json:"size"`
}

type dependency struct {
	Ecosystem string `json:"ecosystem"`
	Package   string `json:"package"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Type      string `json:"type"`
}

func (p *processor) AbstractAddition(_ context.Context, artifact *artifact.Artifact, addition string) (*ps.Addition, error) {
	switch addition {
	case AdditionTypeDependencies:
		dependencies, err := p.dependencies(artifact)
		if err != nil {
			return nil, err
		}
		return &ps.Addition{ContentType: contentTypeJSON, Content: dependencies}, nil
	case AdditionTypeFiles:
		files, err := p.files(artifact)
		if err != nil {
			return nil, err
		}
		return &ps.Addition{ContentType: contentTypeJSON, Content: files}, nil
	default:
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("addition %s isn't supported for %s", addition, ArtifactTypePyPI)
	}
}

func (p *processor) GetArtifactType(_ context.Context, _ *artifact.Artifact) string {
	return ArtifactTypePyPI
}

func (p *processor) ListAdditionTypes(_ context.Context, _ *artifact.Artifact) []string {
	return []string{AdditionTypeFiles, AdditionTypeDependencies}
}

func (p *processor) dependencies(artifact *artifact.Artifact) ([]byte, error) {
	metadata, err := metadataFromArtifact(artifact)
	if err != nil {
		return nil, err
	}
	deps := make([]dependency, 0, len(metadata.RequiresDist))
	for _, req := range metadata.RequiresDist {
		deps = append(deps, dependency{
			Ecosystem: "pypi",
			Package:   req,
			Name:      req,
			Version:   req,
			Type:      "Direct",
		})
	}
	return json.Marshal(deps)
}

func (p *processor) files(artifact *artifact.Artifact) ([]byte, error) {
	metadata, err := metadataFromArtifact(artifact)
	if err != nil {
		return nil, err
	}
	files := make([]fileEntry, 0, len(metadata.Distributions))
	for _, dist := range metadata.Distributions {
		files = append(files, fileEntry{
			Path: dist.Filename,
			Name: dist.Filename,
			Size: dist.Size,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return json.Marshal(files)
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
