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

package pypi

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"strings"
	"sync"
	"time"

	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

const (
	pypiConfigMediaType       = "application/vnd.harbor.package.config.v1+json"
	pypiArtifactMediaType     = "application/vnd.harbor.pypi.package.v1"
	pypiDistributionMediaType = "application/vnd.harbor.pypi.distribution.v1"
	pypiMetadataMediaType     = "application/vnd.harbor.pypi.core-metadata.v1"
	layerTitleAnnotation      = "org.opencontainers.image.title"
	layerRoleAnnotation       = "io.goharbor.package.role"
	maxCoreMetadataSize       = 10 << 20
)

var (
	pypiFormat = pkgstore.Format{
		RepositoryPrefix: "pypi/",
		VersionTagPrefix: "pypi-",
		ConfigMediaType:  pypiConfigMediaType,
		LayerMediaType:   pypiDistributionMediaType,
		ArtifactType:     pypiArtifactMediaType,
	}
	upsertLocks sync.Map
)

type packageStore interface {
	Publish(ctx context.Context, projectID int64, project string, upload uploadRequest) error
	Load(ctx context.Context, project, name string) (*storedPackage, error)
	ListPackages(ctx context.Context, project string) ([]string, error)
	OpenDistribution(ctx context.Context, project, name, version, filename string) (*pkgstore.Content, error)
	OpenMetadata(ctx context.Context, project, name, version, filename string) (*pkgstore.Content, error)
}

type registryPackageStore struct {
	store *pkgstore.Store
}

type uploadRequest struct {
	Name           string
	NormalizedName string
	Version        string
	Filename       string
	ContentType    string
	Summary        string
	RequiresPython string
	RequiresDist   []string
	Classifiers    []string
	Metadata       map[string][]string
	Content        []byte
}

type versionMetadata struct {
	Name           string                 `json:"name"`
	NormalizedName string                 `json:"normalized_name"`
	Version        string                 `json:"version"`
	Summary        string                 `json:"summary,omitempty"`
	RequiresPython string                 `json:"requires_python,omitempty"`
	RequiresDist   []string               `json:"requires_dist,omitempty"`
	Classifiers    []string               `json:"classifiers,omitempty"`
	Distributions  []distributionMetadata `json:"distributions"`
	Metadata       map[string][]string    `json:"metadata,omitempty"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type distributionMetadata struct {
	Filename       string    `json:"filename"`
	ContentType    string    `json:"content_type"`
	Size           int64     `json:"size"`
	SHA256         string    `json:"sha256"`
	MetadataSHA256 string    `json:"metadata_sha256,omitempty"`
	UploadedAt     time.Time `json:"uploaded_at"`
}

type storedPackage struct {
	Name     string
	Versions []storedVersion
}

type storedVersion struct {
	Version        string
	RequiresPython string
	Distributions  []distributionMetadata
	PushTime       time.Time
}

func newPackageStore() packageStore {
	return &registryPackageStore{store: pkgstore.New(pypiFormat)}
}

func (s *registryPackageStore) Publish(ctx context.Context, projectID int64, project string, upload uploadRequest) error {
	unlock := lockUpsert(project, upload.NormalizedName, upload.Version)
	defer unlock()

	metadata := &versionMetadata{}
	if existing, err := s.loadVersion(ctx, project, upload.NormalizedName, upload.Version); err == nil {
		metadata = existing
	} else if !harborerrors.IsNotFoundErr(err) {
		return err
	}
	if distributionExists(metadata.Distributions, upload.Filename) {
		return fmt.Errorf("pypi distribution already exists: %s", upload.Filename)
	}

	now := time.Now().UTC()
	sum := sha256.Sum256(upload.Content)
	coreMetadata := coreMetadataFromUpload(upload)
	metadataSum := sha256.Sum256(coreMetadata)
	metadata.Name = upload.Name
	metadata.NormalizedName = upload.NormalizedName
	metadata.Version = upload.Version
	metadata.Summary = upload.Summary
	metadata.RequiresPython = upload.RequiresPython
	metadata.RequiresDist = append([]string(nil), upload.RequiresDist...)
	metadata.Classifiers = append([]string(nil), upload.Classifiers...)
	metadata.Metadata = cloneForm(upload.Metadata)
	metadata.UpdatedAt = now
	metadata.Distributions = append(metadata.Distributions, distributionMetadata{
		Filename:       upload.Filename,
		ContentType:    upload.ContentType,
		Size:           int64(len(upload.Content)),
		SHA256:         hex.EncodeToString(sum[:]),
		MetadataSHA256: hex.EncodeToString(metadataSum[:]),
		UploadedAt:     now,
	})
	sort.Slice(metadata.Distributions, func(i, j int) bool {
		return metadata.Distributions[i].Filename < metadata.Distributions[j].Filename
	})
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return s.store.Upsert(ctx, pkgstore.PublishRequest{
		ProjectID:   projectID,
		Project:     project,
		PackageName: upload.NormalizedName,
		Version:     upload.Version,
		Metadata:    payload,
		Layers: []pkgstore.Layer{{
			MediaType: pypiDistributionMediaType,
			Content:   upload.Content,
			Annotations: map[string]string{
				layerTitleAnnotation: upload.Filename,
				layerRoleAnnotation:  "distribution",
			},
		}, {
			MediaType: pypiMetadataMediaType,
			Content:   coreMetadata,
			Annotations: map[string]string{
				layerTitleAnnotation: upload.Filename + ".metadata",
				layerRoleAnnotation:  "metadata",
			},
		}},
	})
}

func (s *registryPackageStore) Load(ctx context.Context, project, name string) (*storedPackage, error) {
	versions, err := s.store.List(ctx, project, name)
	if err != nil {
		return nil, err
	}
	pkg := &storedPackage{Name: name}
	for _, version := range versions {
		metadata := &versionMetadata{}
		if err := json.Unmarshal(version.Metadata, metadata); err != nil {
			return nil, err
		}
		if metadata.NormalizedName != "" {
			pkg.Name = metadata.NormalizedName
		}
		pkg.Versions = append(pkg.Versions, storedVersion{
			Version:        version.Version,
			RequiresPython: metadata.RequiresPython,
			Distributions:  metadata.Distributions,
			PushTime:       version.PushTime,
		})
	}
	if len(pkg.Versions) == 0 {
		return nil, harborerrors.NotFoundError(nil).WithMessage("package not found")
	}
	sort.Slice(pkg.Versions, func(i, j int) bool { return pkg.Versions[i].Version < pkg.Versions[j].Version })
	return pkg, nil
}

func (s *registryPackageStore) ListPackages(ctx context.Context, project string) ([]string, error) {
	return s.store.PackageNames(ctx, project)
}

func (s *registryPackageStore) OpenDistribution(ctx context.Context, project, name, version, filename string) (*pkgstore.Content, error) {
	return s.store.OpenLayerByAnnotation(ctx, project, name, version, layerTitleAnnotation, filename)
}

func (s *registryPackageStore) OpenMetadata(ctx context.Context, project, name, version, filename string) (*pkgstore.Content, error) {
	return s.store.OpenLayerByAnnotation(ctx, project, name, version, layerTitleAnnotation, filename+".metadata")
}

func (s *registryPackageStore) loadVersion(ctx context.Context, project, name, version string) (*versionMetadata, error) {
	versions, err := s.store.List(ctx, project, name)
	if err != nil {
		return nil, err
	}
	for _, stored := range versions {
		if stored.Version != version {
			continue
		}
		metadata := &versionMetadata{}
		if err := json.Unmarshal(stored.Metadata, metadata); err != nil {
			return nil, err
		}
		return metadata, nil
	}
	return nil, harborerrors.NotFoundError(nil).WithMessage("version not found")
}

func lockUpsert(project, packageName, version string) func() {
	key := project + "/" + packageName + "@" + version
	value, _ := upsertLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return func() { mu.Unlock() }
}

func distributionExists(distributions []distributionMetadata, filename string) bool {
	for _, dist := range distributions {
		if dist.Filename == filename {
			return true
		}
	}
	return false
}

func uploadFromMultipart(form *multipart.Form) (uploadRequest, error) {
	fileHeaders := form.File["content"]
	if len(fileHeaders) == 0 {
		return uploadRequest{}, fmt.Errorf("upload missing content file")
	}
	fileHeader := fileHeaders[0]
	file, err := fileHeader.Open()
	if err != nil {
		return uploadRequest{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return uploadRequest{}, err
	}
	name := first(form.Value, "name")
	version := first(form.Value, "version")
	if name == "" || version == "" {
		return uploadRequest{}, fmt.Errorf("upload missing name or version")
	}
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return uploadRequest{
		Name:           name,
		NormalizedName: normalizeName(name),
		Version:        version,
		Filename:       fileHeader.Filename,
		ContentType:    contentType,
		Summary:        first(form.Value, "summary"),
		RequiresPython: first(form.Value, "requires_python"),
		RequiresDist:   values(form.Value, "requires_dist"),
		Classifiers:    append(values(form.Value, "classifiers"), values(form.Value, "classifier")...),
		Metadata:       cloneForm(form.Value),
		Content:        content,
	}, nil
}

func first(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func values(source map[string][]string, key string) []string {
	out := append([]string(nil), source[key]...)
	sort.Strings(out)
	return out
}

func cloneForm(source map[string][]string) map[string][]string {
	clone := make(map[string][]string, len(source))
	for key, values := range source {
		if strings.HasPrefix(key, ":") || key == "content" {
			continue
		}
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func coreMetadataFromUpload(upload uploadRequest) []byte {
	if metadata, ok := coreMetadataFromDistribution(upload.Filename, upload.Content); ok {
		return metadata
	}
	var b strings.Builder
	writeMetadataField(&b, "Metadata-Version", first(upload.Metadata, "metadata_version"), "2.1")
	writeMetadataField(&b, "Name", upload.Name, "")
	writeMetadataField(&b, "Version", upload.Version, "")
	writeMetadataField(&b, "Summary", upload.Summary, "")
	writeMetadataField(&b, "Requires-Python", upload.RequiresPython, "")
	for _, classifier := range upload.Classifiers {
		writeMetadataField(&b, "Classifier", classifier, "")
	}
	for _, req := range upload.RequiresDist {
		writeMetadataField(&b, "Requires-Dist", req, "")
	}
	b.WriteByte('\n')
	return []byte(b.String())
}

func coreMetadataFromDistribution(filename string, content []byte) ([]byte, bool) {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".whl") || strings.HasSuffix(lower, ".zip"):
		reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err != nil {
			return nil, false
		}
		for _, file := range reader.File {
			path := strings.ReplaceAll(file.Name, "\\", "/")
			if !strings.HasSuffix(path, ".dist-info/METADATA") && !strings.HasSuffix(path, "/PKG-INFO") {
				continue
			}
			body, err := file.Open()
			if err != nil {
				return nil, false
			}
			metadata, ok := readCoreMetadata(body)
			_ = body.Close()
			return metadata, ok
		}
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		compressed, err := gzip.NewReader(bytes.NewReader(content))
		if err != nil {
			return nil, false
		}
		defer compressed.Close()
		reader := tar.NewReader(compressed)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, false
			}
			path := strings.ReplaceAll(header.Name, "\\", "/")
			if (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || !strings.HasSuffix(path, "/PKG-INFO") {
				continue
			}
			return readCoreMetadata(reader)
		}
	}
	return nil, false
}

func readCoreMetadata(reader io.Reader) ([]byte, bool) {
	metadata, err := io.ReadAll(io.LimitReader(reader, maxCoreMetadataSize+1))
	if err != nil || len(metadata) == 0 || len(metadata) > maxCoreMetadataSize {
		return nil, false
	}
	return metadata, true
}

func writeMetadataField(b *strings.Builder, name, value, fallback string) {
	if value == "" {
		value = fallback
	}
	if value == "" {
		return
	}
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "\n ")
	_, _ = fmt.Fprintf(b, "%s: %s\n", name, value)
}
