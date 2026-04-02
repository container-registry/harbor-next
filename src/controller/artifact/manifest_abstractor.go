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
	"sync"

	pkgartifact "github.com/goharbor/harbor/src/pkg/artifact"
)

type manifestAbstractor interface {
	AbstractMetadata(ctx context.Context, art *pkgartifact.Artifact, content []byte) error
}

type manifestAbstractorFactory func(*abstractor) manifestAbstractor

var (
	manifestAbstractorFactories = map[string]manifestAbstractorFactory{}
	manifestAbstractorLock      sync.RWMutex
)

func registerManifestAbstractor(factory manifestAbstractorFactory, mediaTypes ...string) error {
	manifestAbstractorLock.Lock()
	defer manifestAbstractorLock.Unlock()

	for _, mediaType := range mediaTypes {
		if _, ok := manifestAbstractorFactories[mediaType]; ok {
			return fmt.Errorf("manifest abstractor for media type %s already exists", mediaType)
		}
	}
	for _, mediaType := range mediaTypes {
		manifestAbstractorFactories[mediaType] = factory
	}

	return nil
}

func getManifestAbstractor(artAbstractor *abstractor, mediaType string) manifestAbstractor {
	manifestAbstractorLock.RLock()
	factory := manifestAbstractorFactories[mediaType]
	manifestAbstractorLock.RUnlock()
	if factory == nil {
		return nil
	}

	return factory(artAbstractor)
}
