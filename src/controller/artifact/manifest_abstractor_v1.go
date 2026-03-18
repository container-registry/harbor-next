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

	"github.com/docker/distribution/manifest/schema1"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
	pkgartifact "github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/blob"
)

type manifestV1Abstractor struct {
	blobMgr blob.Manager
}

func (m *manifestV1Abstractor) AbstractMetadata(ctx context.Context, art *pkgartifact.Artifact, content []byte) error {
	art.ManifestMediaType = schema1.MediaTypeSignedManifest
	art.MediaType = schema1.MediaTypeSignedManifest

	manifest := &schema1.Manifest{}
	if err := json.Unmarshal(content, manifest); err != nil {
		return err
	}

	var digests q.OrList
	for _, fsLayer := range manifest.FSLayers {
		digests.Values = append(digests.Values, fsLayer.BlobSum.String())
	}

	blobs, err := m.blobMgr.List(ctx, q.New(q.KeyWords{"digest": &digests}))
	if err != nil {
		log.G(ctx).Errorf("failed to get blobs of the artifact %s, error %v", art.Digest, err)
		return err
	}

	art.Size = int64(len(content))
	for _, b := range blobs {
		art.Size += b.Size
	}

	return nil
}

func init() {
	mediaTypes := []string{"", "application/json", schema1.MediaTypeSignedManifest}
	if err := registerManifestAbstractor(func(a *abstractor) manifestAbstractor {
		return &manifestV1Abstractor{blobMgr: a.blobMgr}
	}, mediaTypes...); err != nil {
		log.Errorf("failed to register manifest abstractor for media type %v: %v", mediaTypes, err)
	}
}
