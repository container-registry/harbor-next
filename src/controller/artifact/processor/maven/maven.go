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

package maven

import (
	"context"
	"encoding/json"
	"strings"

	ps "github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/controller/artifact/processor/base"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
)

const (
	// ArtifactTypeMaven is the artifact type label shown in the UI.
	ArtifactTypeMaven = "MAVEN"

	// AdditionTypeFiles exposes the GAV's member files (classifiers, checksums,
	// SNAPSHOT-vs-RELEASE) as a Files addition tab.
	AdditionTypeFiles = "FILES"

	// mavenConfigMediaType mirrors controller/multiformat.MavenConfigMediaType. It is
	// duplicated here (rather than imported) because controller/multiformat already
	// depends on controller/artifact, so importing it would create a cycle.
	mavenConfigMediaType = "application/vnd.harbor.maven.config.v1+json"

	// annNativeName / annVersion / annYanked mirror controller/multiformat.AnnNativeName / AnnVersion / AnnYanked.
	annNativeName = "vnd.harbor.multiformat.native-name"
	annVersion    = "vnd.harbor.multiformat.version"
	annYanked     = "vnd.harbor.multiformat.yanked"
)

func init() {
	pc := &Processor{}
	pc.ManifestProcessor = base.NewManifestProcessor()
	if err := ps.Register(pc, mavenConfigMediaType); err != nil {
		log.Errorf("failed to register processor for media type %s: %v", mavenConfigMediaType, err)
		return
	}
}

// Processor abstracts metadata from a multiformat Maven per-GAV manifest.
type Processor struct {
	*base.ManifestProcessor
}

func (p *Processor) AbstractMetadata(ctx context.Context, art *artifact.Artifact, manifestBody []byte) error {
	art.ExtraAttrs = map[string]any{}

	// groupId:artifactId + version live on the manifest annotations.
	manifest := struct {
		Annotations map[string]string `json:"annotations"`
	}{}
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		return err
	}
	if ga, ok := manifest.Annotations[annNativeName]; ok {
		group, artifactID, found := strings.Cut(ga, ":")
		if found {
			art.ExtraAttrs["groupId"] = group
			art.ExtraAttrs["artifactId"] = artifactID
		} else {
			art.ExtraAttrs["artifactId"] = ga
		}
	}
	if v, ok := manifest.Annotations[annVersion]; ok {
		art.ExtraAttrs["version"] = v
	}
	// Surface the PG-authoritative yanked fact only when the per-version manifest
	// carries the annotation, so non-multiformat artifacts are unaffected.
	if raw, ok := manifest.Annotations[annYanked]; ok {
		art.ExtraAttrs["yanked"] = raw == "true"
	}

	// The Maven config blob is the deterministic []FileRef list of the GAV.
	files := []model.FileRef{}
	if err := p.UnmarshalConfig(ctx, art.RepositoryName, manifestBody, &files); err != nil {
		return err
	}
	art.ExtraAttrs["files"] = files
	return nil
}

func (p *Processor) GetArtifactType(_ context.Context, _ *artifact.Artifact) string {
	return ArtifactTypeMaven
}

// ListAdditionTypes advertises the Files addition so the controller auto-builds
// the addition_links entry.
func (p *Processor) ListAdditionTypes(_ context.Context, _ *artifact.Artifact) []string {
	return []string{AdditionTypeFiles}
}

// AbstractAddition returns the GAV member-file list (the canonical []FileRef
// config blob) for the Files addition.
func (p *Processor) AbstractAddition(ctx context.Context, art *artifact.Artifact, addition string) (*ps.Addition, error) {
	if addition != AdditionTypeFiles {
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("addition %s isn't supported for %s", addition, ArtifactTypeMaven)
	}

	m, _, err := p.RegCli.PullManifest(art.RepositoryName, art.Digest)
	if err != nil {
		return nil, err
	}
	_, manifestBody, err := m.Payload()
	if err != nil {
		return nil, err
	}
	// The config blob is the canonical []FileRef JSON; re-marshal for stable ordering.
	files := []model.FileRef{}
	if err := p.UnmarshalConfig(ctx, art.RepositoryName, manifestBody, &files); err != nil {
		return nil, err
	}
	content, err := json.Marshal(files)
	if err != nil {
		return nil, err
	}
	return &ps.Addition{
		Content:     content,
		ContentType: "application/json; charset=utf-8",
	}, nil
}
