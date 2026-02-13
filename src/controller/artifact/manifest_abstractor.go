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
	"fmt"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
)

// ManifestAbstractor abstracts metadata from a specific manifest format
type ManifestAbstractor interface {
	// AbstractManifest extracts metadata from the manifest content into the artifact model
	AbstractManifest(ctx context.Context, artifact *artifact.Artifact, content []byte) error
}

// ManifestAbstractorRegistry holds registered manifest abstractors keyed by media type
var ManifestAbstractorRegistry = map[string]ManifestAbstractor{}

// RegisterManifestAbstractor registers a manifest abstractor for the given media types.
// One abstractor can handle multiple media types.
func RegisterManifestAbstractor(a ManifestAbstractor, mediaTypes ...string) error {
	for _, mediaType := range mediaTypes {
		if _, exists := ManifestAbstractorRegistry[mediaType]; exists {
			return fmt.Errorf("manifest abstractor for media type %s already registered", mediaType)
		}
		ManifestAbstractorRegistry[mediaType] = a
		log.Infof("manifest abstractor for media type %s registered", mediaType)
	}
	return nil
}

// GetManifestAbstractor returns the manifest abstractor registered for the given media type.
// Returns nil if no abstractor is registered for the media type.
func GetManifestAbstractor(mediaType string) ManifestAbstractor {
	return ManifestAbstractorRegistry[mediaType]
}
