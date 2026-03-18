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

	"github.com/docker/distribution/manifest/schema2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/controller/artifact/processor/wasm"
	"github.com/goharbor/harbor/src/lib/log"
	pkgartifact "github.com/goharbor/harbor/src/pkg/artifact"
)

type manifestV2Abstractor struct{}

func (m *manifestV2Abstractor) AbstractMetadata(_ context.Context, art *pkgartifact.Artifact, content []byte) error {
	manifest := &v1.Manifest{}
	if err := json.Unmarshal(content, manifest); err != nil {
		return err
	}

	art.MediaType = manifest.Config.MediaType
	if manifest.Annotations[wasm.AnnotationVariantKey] == wasm.AnnotationVariantValue || manifest.Annotations[wasm.AnnotationHandlerKey] == wasm.AnnotationHandlerValue {
		art.MediaType = wasm.MediaType
	}
	if manifest.ArtifactType != "" {
		art.ArtifactType = manifest.ArtifactType
	} else {
		art.ArtifactType = manifest.Config.MediaType
	}

	art.Size = int64(len(content)) + manifest.Config.Size
	for _, layer := range manifest.Layers {
		art.Size += layer.Size
	}
	art.Annotations = manifest.Annotations
	return nil
}

func init() {
	mediaTypes := []string{v1.MediaTypeImageManifest, schema2.MediaTypeManifest}
	if err := registerManifestAbstractor(func(*abstractor) manifestAbstractor {
		return &manifestV2Abstractor{}
	}, mediaTypes...); err != nil {
		log.Errorf("failed to register manifest abstractor for media type %v: %v", mediaTypes, err)
	}
}
