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

package gosum

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

const (
	configMediaType      = "application/vnd.harbor.package.config.v1+json"
	artifactMediaType    = "application/vnd.harbor.go.sumdb.v1"
	responseMediaType    = "application/vnd.harbor.go.sumdb.response.v1"
	responseLayerTitle   = "response"
	layerTitleAnnotation = "org.opencontainers.image.title"
)

var format = pkgstore.Format{
	RepositoryPrefix: "go-sumdb/",
	VersionTagPrefix: "sumdb-",
	ConfigMediaType:  configMediaType,
	LayerMediaType:   responseMediaType,
	ArtifactType:     artifactMediaType,
}

// Response is an exact checksum database protocol response persisted in Harbor.
type Response struct {
	Body        []byte
	ContentType string
	FetchedAt   time.Time
}

// Database identifies the checksum authority whose responses are mirrored.
type Database struct {
	Name string
	URL  string
}

// Store persists checksum database responses independently of transient caches.
type Store interface {
	Open(ctx context.Context, project string, database Database, requestPath string) (*Response, error)
	Put(ctx context.Context, projectID int64, project string, database Database, requestPath string, response *Response) error
}

type registryStore struct {
	store *pkgstore.Store
}

type responseMetadata struct {
	Database    string    `json:"database"`
	Upstream    string    `json:"upstream"`
	Path        string    `json:"path"`
	ContentType string    `json:"content_type,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// NewStore creates an OCI-backed checksum response store.
func NewStore() Store {
	return &registryStore{store: pkgstore.New(format)}
}

func (s *registryStore) Open(ctx context.Context, project string, database Database, requestPath string) (*Response, error) {
	packageName := databasePackageName(database)
	version, err := s.store.Get(ctx, project, packageName, requestPath)
	if err != nil {
		return nil, err
	}
	var metadata responseMetadata
	if err := json.Unmarshal(version.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("decode checksum response metadata: %w", err)
	}
	if metadata.Database != database.Name || metadata.Upstream != database.URL || metadata.Path != requestPath {
		return nil, harborerrors.NotFoundError(nil).WithMessage("checksum response metadata not found")
	}
	content, err := s.store.OpenLayerByAnnotation(ctx, project, packageName, requestPath, layerTitleAnnotation, responseLayerTitle)
	if err != nil {
		return nil, err
	}
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	if err != nil {
		return nil, err
	}
	return &Response{Body: body, ContentType: metadata.ContentType, FetchedAt: metadata.FetchedAt}, nil
}

func (s *registryStore) Put(ctx context.Context, projectID int64, project string, database Database, requestPath string, response *Response) error {
	metadata, err := json.Marshal(responseMetadata{
		Database:    database.Name,
		Upstream:    database.URL,
		Path:        requestPath,
		ContentType: response.ContentType,
		FetchedAt:   response.FetchedAt,
	})
	if err != nil {
		return err
	}
	return s.store.Upsert(ctx, pkgstore.PublishRequest{
		ProjectID:   projectID,
		Project:     project,
		PackageName: databasePackageName(database),
		Version:     requestPath,
		Metadata:    metadata,
		Layers: []pkgstore.Layer{{
			MediaType: responseMediaType,
			Content:   response.Body,
			Annotations: map[string]string{
				layerTitleAnnotation: responseLayerTitle,
			},
		}},
	})
}

func databasePackageName(database Database) string {
	sum := sha256.Sum256([]byte(database.Name + "\x00" + database.URL))
	return fmt.Sprintf("database-%x", sum)
}
