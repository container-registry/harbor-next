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

package npm

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	ps "github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/controller/artifact/processor/base"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/multiformat/dao"
)

const (
	// ArtifactTypeNPM is the artifact type label shown in the UI.
	ArtifactTypeNPM = "NPM"

	// AdditionTypeReadme / AdditionTypeVersions expose the npm package README and
	// the cross-version listing as addition tabs.
	AdditionTypeReadme   = "README.MD"
	AdditionTypeVersions = "VERSIONS"

	// npmFormat is the multiformat format token for npm packages, used to load the
	// PG projection state. Reused locally to avoid importing controller/multiformat
	// (which would create an import cycle).
	npmFormat = "npm"

	// npmConfigMediaType mirrors controller/multiformat.NpmConfigMediaType. It is
	// duplicated here (rather than imported) because controller/multiformat already
	// depends on controller/artifact, so importing it would create a cycle.
	npmConfigMediaType = "application/vnd.harbor.npm.config.v1+json"

	// annDistTags / annYanked mirror controller/multiformat.AnnDistTags / AnnYanked.
	annDistTags = "vnd.harbor.multiformat.dist-tags"
	annYanked   = "vnd.harbor.multiformat.yanked"
)

func init() {
	pc := &Processor{}
	pc.ManifestProcessor = base.NewManifestProcessor()
	if err := ps.Register(pc, npmConfigMediaType); err != nil {
		log.Errorf("failed to register processor for media type %s: %v", npmConfigMediaType, err)
		return
	}
}

// Processor abstracts metadata from a multiformat npm per-version manifest.
type Processor struct {
	*base.ManifestProcessor
}

func (p *Processor) AbstractMetadata(ctx context.Context, art *artifact.Artifact, manifestBody []byte) error {
	art.ExtraAttrs = map[string]any{}

	// The npm config blob is the per-version metadata document (package.json-ish:
	// name, version, dependencies, ...). Surface the load-bearing fields.
	config := map[string]any{}
	if err := p.UnmarshalConfig(ctx, art.RepositoryName, manifestBody, &config); err != nil {
		return err
	}
	if v, ok := config["name"]; ok {
		art.ExtraAttrs["name"] = v
	}
	if v, ok := config["version"]; ok {
		art.ExtraAttrs["version"] = v
	}
	if v, ok := config["description"]; ok {
		art.ExtraAttrs["description"] = v
	}
	// npm package.json may carry a "readme" field; surface it so README is served
	// from extra_attrs without a registry round-trip.
	if v, ok := config["readme"]; ok {
		art.ExtraAttrs["readme"] = v
	}

	// dist-tags are PG-authoritative and stamped onto the per-version manifest
	// annotation when published; expose them for the version that carries them.
	manifest := struct {
		Annotations map[string]string `json:"annotations"`
	}{}
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		return err
	}
	if raw, ok := manifest.Annotations[annDistTags]; ok && raw != "" {
		distTags := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &distTags); err == nil {
			art.ExtraAttrs["dist-tags"] = distTags
		}
	}
	// Surface the PG-authoritative yanked fact only when the per-version manifest
	// carries the annotation, so non-multiformat artifacts are unaffected.
	if raw, ok := manifest.Annotations[annYanked]; ok {
		art.ExtraAttrs["yanked"] = raw == "true"
	}
	return nil
}

func (p *Processor) GetArtifactType(_ context.Context, _ *artifact.Artifact) string {
	return ArtifactTypeNPM
}

// ListAdditionTypes advertises the README and Versions additions so the
// controller auto-builds the addition_links entries.
func (p *Processor) ListAdditionTypes(_ context.Context, _ *artifact.Artifact) []string {
	return []string{AdditionTypeReadme, AdditionTypeVersions}
}

// versionDTO is one row in the Versions addition response.
type versionDTO struct {
	Version string `json:"version"`
	Created string `json:"created"`
	Yanked  bool   `json:"yanked"`
	Size    int64  `json:"size"`
}

// versionsDTO is the deterministic Versions addition payload.
type versionsDTO struct {
	Versions []versionDTO      `json:"versions"`
	DistTags map[string]string `json:"distTags"`
}

// AbstractAddition serves the README (markdown) and Versions (cross-version
// listing from the PG projection) additions for an npm package.
func (p *Processor) AbstractAddition(ctx context.Context, art *artifact.Artifact, addition string) (*ps.Addition, error) {
	switch addition {
	case AdditionTypeReadme:
		md, _ := art.ExtraAttrs["readme"].(string)
		if md == "" {
			md, _ = art.ExtraAttrs["description"].(string)
		}
		return &ps.Addition{
			Content:     []byte(md),
			ContentType: "text/markdown; charset=utf-8",
		}, nil
	case AdditionTypeVersions:
		dto := versionsDTO{Versions: []versionDTO{}, DistTags: map[string]string{}}
		name, _ := art.ExtraAttrs["name"].(string)
		if name == "" {
			return marshalVersions(dto)
		}
		st, ok, err := dao.New().LoadState(ctx, art.ProjectID, npmFormat, name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return marshalVersions(dto)
		}
		for _, v := range st.Versions {
			dto.Versions = append(dto.Versions, versionDTO{
				Version: v.Version,
				Created: v.Created.UTC().Format(time.RFC3339),
				Yanked:  v.Yanked,
				Size:    v.PayloadSize,
			})
		}
		sort.Slice(dto.Versions, func(i, j int) bool {
			return dto.Versions[i].Version < dto.Versions[j].Version
		})
		if st.DistTags != nil {
			dto.DistTags = st.DistTags
		}
		return marshalVersions(dto)
	default:
		return p.ManifestProcessor.AbstractAddition(ctx, art, addition)
	}
}

func marshalVersions(dto versionsDTO) (*ps.Addition, error) {
	content, err := json.Marshal(dto)
	if err != nil {
		return nil, err
	}
	return &ps.Addition{
		Content:     content,
		ContentType: "application/json; charset=utf-8",
	}, nil
}
