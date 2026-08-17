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

package cargo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

const (
	cargoConfigMediaType   = "application/vnd.harbor.package.config.v1+json"
	cargoArtifactMediaType = "application/vnd.harbor.cargo.crate.v1"
	cargoCrateMediaType    = "application/vnd.harbor.cargo.crate.layer.v1"
	cargoIndexMediaType    = "application/vnd.harbor.cargo.index.v1+json"
	cargoFilesMediaType    = "application/vnd.harbor.package.files.v1+json"
	layerTitleAnnotation   = "org.opencontainers.image.title"
	layerRoleAnnotation    = "io.goharbor.package.role"
	crateLayerTitle        = "crate.tar.gz"
	indexLayerTitle        = "index.json"
	filesLayerTitle        = "files.json"
)

var cargoFormat = pkgstore.Format{
	RepositoryPrefix: "cargo/",
	VersionTagPrefix: "cargo-",
	ConfigMediaType:  cargoConfigMediaType,
	LayerMediaType:   cargoCrateMediaType,
	ArtifactType:     cargoArtifactMediaType,
}

type packageStore interface {
	Publish(ctx context.Context, projectID int64, project string, publish publishRequest) error
	Index(ctx context.Context, project, name string) ([]indexEntry, error)
	OpenCrate(ctx context.Context, project, name, version string) (*pkgstore.Content, error)
	SetYanked(ctx context.Context, projectID int64, project, name, version string, yanked bool) error
}

type registryPackageStore struct {
	store *pkgstore.Store
}

type publishRequest struct {
	Metadata crateMetadata
	Content  []byte
}

type publishedCrateMetadata struct {
	Name          string                `json:"name"`
	Version       string                `json:"vers"`
	Deps          []publishedDependency `json:"deps"`
	Features      map[string][]string   `json:"features"`
	Authors       []string              `json:"authors"`
	Description   string                `json:"description"`
	Documentation string                `json:"documentation"`
	Homepage      string                `json:"homepage"`
	Readme        string                `json:"readme"`
	ReadmeFile    string                `json:"readme_file"`
	Keywords      []string              `json:"keywords"`
	Categories    []string              `json:"categories"`
	License       string                `json:"license"`
	LicenseFile   string                `json:"license_file"`
	Repository    string                `json:"repository"`
	Links         string                `json:"links"`
	RustVersion   string                `json:"rust_version"`
}

type publishedDependency struct {
	Name            string   `json:"name"`
	VersionReq      string   `json:"version_req"`
	Features        []string `json:"features"`
	Optional        bool     `json:"optional"`
	DefaultFeatures bool     `json:"default_features"`
	Target          *string  `json:"target"`
	Kind            string   `json:"kind"`
	Registry        *string  `json:"registry"`
	ExplicitName    *string  `json:"explicit_name_in_toml"`
}

type crateMetadata struct {
	Name          string              `json:"name"`
	Version       string              `json:"vers"`
	Deps          []dependency        `json:"deps"`
	Features      map[string][]string `json:"features"`
	Authors       []string            `json:"authors"`
	Description   string              `json:"description"`
	Documentation string              `json:"documentation"`
	Homepage      string              `json:"homepage"`
	Readme        string              `json:"readme"`
	ReadmeFile    string              `json:"readme_file"`
	Keywords      []string            `json:"keywords"`
	Categories    []string            `json:"categories"`
	License       string              `json:"license"`
	LicenseFile   string              `json:"license_file"`
	Repository    string              `json:"repository"`
	Links         string              `json:"links"`
	RustVersion   string              `json:"rust_version"`
}

type dependency struct {
	Name            string   `json:"name"`
	VersionReq      string   `json:"req"`
	Features        []string `json:"features"`
	Optional        bool     `json:"optional"`
	DefaultFeatures bool     `json:"default_features"`
	Target          string   `json:"target"`
	Kind            string   `json:"kind"`
	Registry        string   `json:"registry"`
	ExplicitName    string   `json:"explicit_name_in_toml,omitempty"`
	Package         string   `json:"package,omitempty"`
}

type versionMetadata struct {
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Checksum    string        `json:"checksum"`
	Metadata    crateMetadata `json:"metadata"`
	Files       []fileEntry   `json:"files,omitempty"`
	PublishedAt time.Time     `json:"published_at"`
	Yanked      bool          `json:"yanked"`
}

type indexEntry struct {
	Name        string              `json:"name"`
	Version     string              `json:"vers"`
	Deps        []dependency        `json:"deps"`
	Checksum    string              `json:"cksum"`
	Features    map[string][]string `json:"features"`
	Yanked      bool                `json:"yanked"`
	Links       string              `json:"links,omitempty"`
	RustVersion string              `json:"rust_version,omitempty"`
}

type fileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func newPackageStore() packageStore {
	return &registryPackageStore{store: pkgstore.New(cargoFormat)}
}

func (s *registryPackageStore) Publish(ctx context.Context, projectID int64, project string, publish publishRequest) error {
	name := normalizeCrate(publish.Metadata.Name)
	sum := sha256.Sum256(publish.Content)
	files, _ := crateFiles(bytes.NewReader(publish.Content))
	entry := indexEntry{
		Name:        name,
		Version:     publish.Metadata.Version,
		Deps:        publish.Metadata.Deps,
		Checksum:    hex.EncodeToString(sum[:]),
		Features:    publish.Metadata.Features,
		Yanked:      false,
		Links:       publish.Metadata.Links,
		RustVersion: publish.Metadata.RustVersion,
	}
	entryPayload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	filesPayload, err := json.Marshal(files)
	if err != nil {
		return err
	}
	metadataPayload, err := json.Marshal(versionMetadata{
		Name:        name,
		Version:     publish.Metadata.Version,
		Checksum:    entry.Checksum,
		Metadata:    publish.Metadata,
		Files:       files,
		PublishedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return s.store.Publish(ctx, pkgstore.PublishRequest{
		ProjectID:   projectID,
		Project:     project,
		PackageName: name,
		Version:     publish.Metadata.Version,
		Metadata:    metadataPayload,
		Layers: []pkgstore.Layer{{
			MediaType: cargoCrateMediaType,
			Content:   publish.Content,
			Annotations: map[string]string{
				layerTitleAnnotation: crateLayerTitle,
				layerRoleAnnotation:  "distribution",
			},
		}, {
			MediaType: cargoIndexMediaType,
			Content:   entryPayload,
			Annotations: map[string]string{
				layerTitleAnnotation: indexLayerTitle,
				layerRoleAnnotation:  "index",
			},
		}, {
			MediaType: cargoFilesMediaType,
			Content:   filesPayload,
			Annotations: map[string]string{
				layerTitleAnnotation: filesLayerTitle,
				layerRoleAnnotation:  "files",
			},
		}},
	})
}

func (s *registryPackageStore) Index(ctx context.Context, project, name string) ([]indexEntry, error) {
	versions, err := s.store.List(ctx, project, name)
	if err != nil {
		return nil, err
	}
	entries := make([]indexEntry, 0, len(versions))
	for _, stored := range versions {
		metadata := &versionMetadata{}
		if err := json.Unmarshal(stored.Metadata, metadata); err != nil {
			return nil, err
		}
		entries = append(entries, indexEntry{
			Name:        metadata.Name,
			Version:     metadata.Version,
			Deps:        metadata.Metadata.Deps,
			Checksum:    metadata.Checksum,
			Features:    metadata.Metadata.Features,
			Yanked:      metadata.Yanked,
			Links:       metadata.Metadata.Links,
			RustVersion: metadata.Metadata.RustVersion,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Version < entries[j].Version })
	return entries, nil
}

func (s *registryPackageStore) OpenCrate(ctx context.Context, project, name, version string) (*pkgstore.Content, error) {
	return s.store.OpenLayer(ctx, project, name, version)
}

func (s *registryPackageStore) SetYanked(ctx context.Context, projectID int64, project, name, version string, yanked bool) error {
	stored, err := s.store.Get(ctx, project, name, version)
	if err != nil {
		return err
	}
	metadata := &versionMetadata{}
	if err := json.Unmarshal(stored.Metadata, metadata); err != nil {
		return err
	}
	if metadata.Yanked == yanked {
		return nil
	}
	metadata.Yanked = yanked
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return s.store.Upsert(ctx, pkgstore.PublishRequest{
		ProjectID:   projectID,
		Project:     project,
		PackageName: name,
		Version:     version,
		Metadata:    payload,
	})
}

func parsePublish(body []byte) (publishRequest, error) {
	if len(body) < 8 {
		return publishRequest{}, fmt.Errorf("publish body too short")
	}
	metaLen := int(binary.LittleEndian.Uint32(body[:4]))
	if metaLen <= 0 || len(body) < 4+metaLen+4 {
		return publishRequest{}, fmt.Errorf("invalid metadata length")
	}
	var published publishedCrateMetadata
	if err := json.Unmarshal(body[4:4+metaLen], &published); err != nil {
		return publishRequest{}, fmt.Errorf("parse cargo metadata: %w", err)
	}
	metadata := published.indexMetadata()
	crateLenOffset := 4 + metaLen
	crateLen := int(binary.LittleEndian.Uint32(body[crateLenOffset : crateLenOffset+4]))
	payloadOffset := crateLenOffset + 4
	if crateLen <= 0 || len(body) != payloadOffset+crateLen {
		return publishRequest{}, fmt.Errorf("invalid crate payload length")
	}
	if metadata.Name == "" || metadata.Version == "" {
		return publishRequest{}, fmt.Errorf("publish metadata missing name or version")
	}
	if metadata.Features == nil {
		metadata.Features = map[string][]string{}
	}
	return publishRequest{Metadata: metadata, Content: append([]byte(nil), body[payloadOffset:]...)}, nil
}

func (m publishedCrateMetadata) indexMetadata() crateMetadata {
	deps := make([]dependency, 0, len(m.Deps))
	for _, published := range m.Deps {
		name := published.Name
		packageName := ""
		if published.ExplicitName != nil && *published.ExplicitName != "" {
			name = *published.ExplicitName
			packageName = published.Name
		}
		target := ""
		if published.Target != nil {
			target = *published.Target
		}
		registry := ""
		if published.Registry != nil {
			registry = *published.Registry
		}
		deps = append(deps, dependency{
			Name:            name,
			VersionReq:      published.VersionReq,
			Features:        published.Features,
			Optional:        published.Optional,
			DefaultFeatures: published.DefaultFeatures,
			Target:          target,
			Kind:            published.Kind,
			Registry:        registry,
			Package:         packageName,
		})
	}
	return crateMetadata{
		Name:          m.Name,
		Version:       m.Version,
		Deps:          deps,
		Features:      m.Features,
		Authors:       m.Authors,
		Description:   m.Description,
		Documentation: m.Documentation,
		Homepage:      m.Homepage,
		Readme:        m.Readme,
		ReadmeFile:    m.ReadmeFile,
		Keywords:      m.Keywords,
		Categories:    m.Categories,
		License:       m.License,
		LicenseFile:   m.LicenseFile,
		Repository:    m.Repository,
		Links:         m.Links,
		RustVersion:   m.RustVersion,
	}
}

func crateFiles(reader io.Reader) ([]fileEntry, error) {
	gr, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	files := make([]fileEntry, 0)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		files = append(files, fileEntry{Path: header.Name, Size: header.Size})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
