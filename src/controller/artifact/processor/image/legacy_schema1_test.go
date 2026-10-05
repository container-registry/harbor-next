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

package image

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/artifact"
)

// Schema1 artifacts pushed before the v3 registry stay in the database. Listing
// them must keep working: no additions offered, additions refused with 400.
func TestLegacySchema1ArtifactReadPath(t *testing.T) {
	art := &artifact.Artifact{
		Type:              ArtifactTypeImage,
		MediaType:         "application/vnd.docker.distribution.manifest.v1+prettyjws",
		ManifestMediaType: "application/vnd.docker.distribution.manifest.v1+prettyjws",
	}
	p := processor.Get(art.ResolveArtifactType())

	assert.Empty(t, p.ListAdditionTypes(context.Background(), art))
	for _, addition := range []string{AdditionTypeBuildHistory, "vulnerabilities", "readme.md"} {
		_, err := p.AbstractAddition(context.Background(), art, addition)
		assert.True(t, errors.IsErr(err, errors.BadRequestCode), "%s: got %v", addition, err)
	}
}
