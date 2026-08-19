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

package gomod

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

const (
	goConfigMediaType    = "application/vnd.harbor.package.config.v1+json"
	goArtifactMediaType  = "application/vnd.harbor.go.module.v1"
	goInfoMediaType      = "application/vnd.harbor.go.info.v1+json"
	goModMediaType       = "application/vnd.harbor.go.mod.v1"
	goZipMediaType       = "application/vnd.harbor.go.zip.v1"
	layerTitleAnnotation = "org.opencontainers.image.title"
	infoLayerTitle       = "module.info"
	modLayerTitle        = "go.mod"
	zipLayerTitle        = "module.zip"
)

var goFormat = pkgstore.Format{
	RepositoryPrefix: "go/",
	VersionTagPrefix: "go-",
	ConfigMediaType:  goConfigMediaType,
	LayerMediaType:   goZipMediaType,
	ArtifactType:     goArtifactMediaType,
}

var upsertLocks sync.Map

type packageStore interface {
	Publish(ctx context.Context, projectID int64, project, modulePath, escapedModule, version string, typ requestType, content []byte) error
	Open(ctx context.Context, project, escapedModule, version, title string) (*pkgstore.Content, error)
}

type registryPackageStore struct {
	store *pkgstore.Store
}

type versionMetadata struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}

func newPackageStore() packageStore {
	return &registryPackageStore{store: pkgstore.New(goFormat)}
}

func (s *registryPackageStore) Publish(ctx context.Context, projectID int64, project, modulePath, escapedModule, version string, typ requestType, content []byte) error {
	unlock := lockUpsert(project, escapedModule, version)
	defer unlock()

	metadata, err := json.Marshal(versionMetadata{Module: modulePath, Version: version})
	if err != nil {
		return err
	}
	mediaType, title := layerFor(typ)
	err = s.store.Upsert(ctx, pkgstore.PublishRequest{
		ProjectID:   projectID,
		Project:     project,
		PackageName: storagePackageName(escapedModule),
		Version:     version,
		Metadata:    metadata,
		Layers: []pkgstore.Layer{{
			MediaType: mediaType,
			Content:   content,
			Annotations: map[string]string{
				layerTitleAnnotation: title,
			},
		}},
	})
	if err != nil && !harborerrors.IsConflictErr(err) {
		return err
	}
	return nil
}

func lockUpsert(project, escapedModule, version string) func() {
	key := project + "/" + escapedModule + "@" + version
	value, _ := upsertLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *registryPackageStore) Open(ctx context.Context, project, escapedModule, version, title string) (*pkgstore.Content, error) {
	return s.store.OpenLayerByAnnotation(ctx, project, storagePackageName(escapedModule), version, layerTitleAnnotation, title)
}

func storagePackageName(escapedModule string) string {
	sum := sha256.Sum256([]byte(escapedModule))
	return fmt.Sprintf("module-%x", sum)
}

func layerFor(typ requestType) (string, string) {
	switch typ {
	case requestInfo:
		return goInfoMediaType, infoLayerTitle
	case requestMod:
		return goModMediaType, modLayerTitle
	default:
		return goZipMediaType, zipLayerTitle
	}
}
