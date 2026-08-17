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

package pkgstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/distribution"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	artifactcontroller "github.com/goharbor/harbor/src/controller/artifact"
	blobcontroller "github.com/goharbor/harbor/src/controller/blob"
	quotacontroller "github.com/goharbor/harbor/src/controller/quota"
	repositorycontroller "github.com/goharbor/harbor/src/controller/repository"
	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	artifactmodel "github.com/goharbor/harbor/src/pkg/artifact"
	quotatypes "github.com/goharbor/harbor/src/pkg/quota/types"
	"github.com/goharbor/harbor/src/pkg/registry"
	repositorymodel "github.com/goharbor/harbor/src/pkg/repository/model"
)

// Format describes how one package ecosystem is represented as OCI artifacts.
type Format struct {
	RepositoryPrefix string
	VersionTagPrefix string
	ConfigMediaType  string
	LayerMediaType   string
	ArtifactType     string
}

// Version is one published package version loaded from Harbor artifact metadata.
type Version struct {
	PackageName string
	Version     string
	Tags        []string
	Metadata    json.RawMessage
	LayerDigest string
	LayerSize   int64
	Layers      []LayerInfo
	PushTime    time.Time
}

// Layer contains one OCI layer to publish with a package version.
type Layer struct {
	MediaType   string
	Content     []byte
	Annotations map[string]string
}

// LayerInfo describes one stored package layer.
type LayerInfo struct {
	MediaType   string            `json:"media_type,omitempty"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PublishRequest contains a package version and its primary content layer.
type PublishRequest struct {
	ProjectID   int64
	Project     string
	PackageName string
	Version     string
	Tags        []string
	Metadata    json.RawMessage
	Layer       []byte
	Layers      []Layer
}

// Content is a registry blob stream. The caller must close Body.
type Content struct {
	Size int64
	Body io.ReadCloser
}

// Store persists package ecosystem content as Harbor-managed OCI artifacts.
type Store struct {
	Format    Format
	Registry  RegistryClient
	Artifacts ArtifactController
	Repos     RepositoryController
	Blobs     BlobController
	Quota     QuotaController
}

// RegistryClient is the registry operations Store needs.
type RegistryClient interface {
	PullBlob(repository, digest string) (int64, io.ReadCloser, error)
	PushBlob(repository, digest string, size int64, blob io.Reader) error
	PushManifest(repository, reference, mediaType string, payload []byte) (string, error)
}

// ArtifactController is the artifact controller operations Store needs.
type ArtifactController interface {
	Ensure(ctx context.Context, repository, digest string, option *artifactcontroller.ArtOption) (bool, int64, error)
	Get(ctx context.Context, id int64, option *artifactcontroller.Option) (*artifactcontroller.Artifact, error)
	GetByReference(ctx context.Context, repository, reference string, option *artifactcontroller.Option) (*artifactcontroller.Artifact, error)
	List(ctx context.Context, query *q.Query, option *artifactcontroller.Option) ([]*artifactcontroller.Artifact, error)
	Delete(ctx context.Context, id int64) error
}

// RepositoryController is the repository controller operations Store needs.
type RepositoryController interface {
	Ensure(ctx context.Context, name string) (bool, int64, error)
}

type repositoryLister interface {
	List(ctx context.Context, query *q.Query) ([]*repositorymodel.RepoRecord, error)
}

// BlobController is the blob controller operations Store needs.
type BlobController interface {
	Sync(ctx context.Context, references []distribution.Descriptor) error
	Ensure(ctx context.Context, digest string, contentType string, size int64) (int64, error)
	AssociateWithProjectByDigest(ctx context.Context, blobDigest string, projectID int64) error
	AssociateWithProjectByID(ctx context.Context, blobID int64, projectID int64) error
	AssociateWithArtifact(ctx context.Context, blobDigests []string, artifactDigest string) error
}

// QuotaController is the quota controller operation Store needs.
type QuotaController interface {
	Request(ctx context.Context, reference, referenceID string, resources quotatypes.ResourceList, f func() error) error
}

type config struct {
	PackageName string          `json:"package_name"`
	Version     string          `json:"version"`
	Metadata    json.RawMessage `json:"metadata"`
	Layer       LayerInfo       `json:"layer"`
	Layers      []LayerInfo     `json:"layers,omitempty"`
}

// New creates a production Store backed by Harbor's registry and controllers.
func New(format Format) *Store {
	return &Store{
		Format:    format,
		Registry:  registry.Cli,
		Artifacts: artifactcontroller.Ctl,
		Repos:     repositorycontroller.Ctl,
		Blobs:     blobcontroller.Ctl,
		Quota:     quotacontroller.Ctl,
	}
}

// Publish writes the package layer, config, manifest, and Harbor metadata.
func (s *Store) Publish(ctx context.Context, req PublishRequest) error {
	if err := s.validate(); err != nil {
		return err
	}
	repo := s.Repository(req.Project, req.PackageName)
	tags, err := s.publishTags(req.Version, req.Tags)
	if err != nil {
		return err
	}
	tag := tags[0]
	if _, err := s.Artifacts.GetByReference(ctx, repo, tag, nil); err == nil {
		return fmt.Errorf("version %s already exists", req.Version)
	} else if !harborerrors.IsNotFoundErr(err) {
		return err
	}

	if _, _, err := s.Repos.Ensure(ctx, repo); err != nil {
		return err
	}

	layers := req.Layers
	if len(layers) == 0 {
		layers = []Layer{{MediaType: s.Format.LayerMediaType, Content: req.Layer}}
	}
	for i := range layers {
		if layers[i].MediaType == "" {
			layers[i].MediaType = s.Format.LayerMediaType
		}
	}
	return s.write(ctx, repo, req, tags, nil, nil)
}

// Upsert writes a package version, preserving existing layers unless a new
// layer has the same annotated title.
func (s *Store) Upsert(ctx context.Context, req PublishRequest) error {
	if err := s.validate(); err != nil {
		return err
	}
	repo := s.Repository(req.Project, req.PackageName)
	tags, err := s.publishTags(req.Version, req.Tags)
	if err != nil {
		return err
	}
	if _, _, err := s.Repos.Ensure(ctx, repo); err != nil {
		return err
	}

	var existing []LayerInfo
	var superseded []int64
	for i, tag := range tags {
		art, err := s.Artifacts.GetByReference(ctx, repo, tag, nil)
		if err != nil {
			if harborerrors.IsNotFoundErr(err) {
				continue
			}
			return err
		}
		if cfg, ok := configFromArtifact(art.Artifact); ok && cfg.PackageName == req.PackageName && cfg.Version == req.Version {
			if i == 0 {
				existing = cfg.Layers
			}
			superseded = appendUniqueID(superseded, art.ID)
		}
	}
	return s.write(ctx, repo, req, tags, existing, superseded)
}

func (s *Store) write(ctx context.Context, repo string, req PublishRequest, tags []string, existing []LayerInfo, superseded []int64) error {
	layers := req.Layers
	if len(layers) == 0 && (len(existing) == 0 || req.Layer != nil) {
		layers = []Layer{{MediaType: s.Format.LayerMediaType, Content: req.Layer}}
	}
	for i := range layers {
		if layers[i].MediaType == "" {
			layers[i].MediaType = s.Format.LayerMediaType
		}
	}
	layerInfos, layerDescriptors, newLayers := mergeLayers(existing, layers)
	if len(layerInfos) == 0 {
		return fmt.Errorf("registry package store publish request has no layers")
	}
	cfgPayload, err := json.Marshal(config{
		PackageName: req.PackageName,
		Version:     req.Version,
		Metadata:    req.Metadata,
		Layer:       layerInfos[0],
		Layers:      layerInfos,
	})
	if err != nil {
		return err
	}
	cfgDigest := digest.FromBytes(cfgPayload).String()

	manifestPayload, err := s.manifest(req.PackageName, req.Version, cfgDigest, int64(len(cfgPayload)), layerDescriptors)
	if err != nil {
		return err
	}

	resourceStorage := int64(len(cfgPayload) + len(manifestPayload))
	for _, layer := range newLayers {
		resourceStorage += int64(len(layer.Content))
	}
	resources := quotatypes.ResourceList{quotatypes.ResourceStorage: resourceStorage}
	return s.Quota.Request(ctx, "project", strconv.FormatInt(req.ProjectID, 10), resources, func() error {
		for _, layer := range newLayers {
			layerDigest := digest.FromBytes(layer.Content).String()
			if err := s.Registry.PushBlob(repo, layerDigest, int64(len(layer.Content)), bytes.NewReader(layer.Content)); err != nil {
				return err
			}
		}
		if err := s.Registry.PushBlob(repo, cfgDigest, int64(len(cfgPayload)), bytes.NewReader(cfgPayload)); err != nil {
			return err
		}
		var manifestDigest string
		for _, tag := range tags {
			pushedDigest, err := s.Registry.PushManifest(repo, tag, v1.MediaTypeImageManifest, manifestPayload)
			if err != nil {
				return err
			}
			if manifestDigest == "" {
				manifestDigest = pushedDigest
			}
		}
		if manifestDigest == "" {
			manifestDigest = digest.FromBytes(manifestPayload).String()
		}
		if err := s.syncBlobs(ctx, req.ProjectID, manifestDigest, manifestPayload, cfgDigest, int64(len(cfgPayload)), layerDescriptors); err != nil {
			return err
		}
		_, artifactID, err := s.Artifacts.Ensure(ctx, repo, manifestDigest, &artifactcontroller.ArtOption{Tags: tags})
		if err != nil {
			return err
		}
		return s.deleteSuperseded(ctx, superseded, artifactID)
	})
}

func (s *Store) deleteSuperseded(ctx context.Context, artifactIDs []int64, currentID int64) error {
	for _, id := range artifactIDs {
		if id == 0 || id == currentID {
			continue
		}
		art, err := s.Artifacts.Get(ctx, id, &artifactcontroller.Option{WithTag: true})
		if err != nil {
			if harborerrors.IsNotFoundErr(err) {
				continue
			}
			return err
		}
		if len(art.Tags) > 0 {
			continue
		}
		if err := s.Artifacts.Delete(ctx, id); err != nil && !harborerrors.IsNotFoundErr(err) {
			return err
		}
	}
	return nil
}

// List returns all versions of a package stored in Harbor artifacts.
func (s *Store) List(ctx context.Context, project, packageName string) ([]Version, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	arts, err := s.Artifacts.List(ctx, q.New(q.KeyWords{"RepositoryName": s.Repository(project, packageName)}), &artifactcontroller.Option{WithTag: true})
	if err != nil {
		return nil, err
	}
	var versions []Version
	for _, art := range arts {
		cfg, ok := configFromArtifact(art.Artifact)
		if !ok || cfg.PackageName != packageName || cfg.Version == "" {
			continue
		}
		tags := artifactTags(art)
		if !hasTag(tags, s.VersionTag(cfg.Version)) {
			continue
		}
		versions = append(versions, Version{
			PackageName: cfg.PackageName,
			Version:     cfg.Version,
			Tags:        tags,
			Metadata:    cfg.Metadata,
			LayerDigest: cfg.Layer.Digest,
			LayerSize:   cfg.Layer.Size,
			Layers:      cfg.Layers,
			PushTime:    art.PushTime,
		})
	}
	if len(versions) == 0 {
		return nil, harborerrors.NotFoundError(nil).WithMessage("package not found")
	}
	return versions, nil
}

// Get returns one package version by its deterministic OCI version tag.
func (s *Store) Get(ctx context.Context, project, packageName, version string) (*Version, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	artifact, err := s.Artifacts.GetByReference(
		ctx,
		s.Repository(project, packageName),
		s.VersionTag(version),
		&artifactcontroller.Option{WithTag: true},
	)
	if err != nil {
		return nil, err
	}
	cfg, ok := configFromArtifact(artifact.Artifact)
	if !ok || cfg.PackageName != packageName || cfg.Version != version {
		return nil, harborerrors.NotFoundError(nil).WithMessage("package version not found")
	}
	return &Version{
		PackageName: cfg.PackageName,
		Version:     cfg.Version,
		Tags:        artifactTags(artifact),
		Metadata:    cfg.Metadata,
		LayerDigest: cfg.Layer.Digest,
		LayerSize:   cfg.Layer.Size,
		Layers:      cfg.Layers,
		PushTime:    artifact.PushTime,
	}, nil
}

// PackageNames returns all package names for this format in one project.
func (s *Store) PackageNames(ctx context.Context, project string) ([]string, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	lister, ok := s.Repos.(repositoryLister)
	if !ok {
		return nil, fmt.Errorf("registry package store repository controller cannot list repositories")
	}
	prefix := project + "/" + s.Format.RepositoryPrefix
	repos, err := lister.List(ctx, q.New(q.KeyWords{"Name__icontains": prefix}))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(repos))
	seen := map[string]struct{}{}
	for _, repo := range repos {
		if !strings.HasPrefix(repo.Name, prefix) {
			continue
		}
		name := strings.TrimPrefix(repo.Name, prefix)
		if name == "" {
			continue
		}
		versions, err := s.List(ctx, project, name)
		if err != nil || len(versions) == 0 {
			continue
		}
		packageName := versions[0].PackageName
		if packageName == "" {
			packageName = name
		}
		if _, ok := seen[packageName]; ok {
			continue
		}
		seen[packageName] = struct{}{}
		names = append(names, packageName)
	}
	sort.Strings(names)
	return names, nil
}

// OpenLayer opens the package content layer for one package version.
func (s *Store) OpenLayer(ctx context.Context, project, packageName, version string) (*Content, error) {
	stored, err := s.Get(ctx, project, packageName, version)
	if err != nil {
		return nil, err
	}
	size, body, err := s.Registry.PullBlob(s.Repository(project, packageName), stored.LayerDigest)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		size = stored.LayerSize
	}
	return &Content{Size: size, Body: body}, nil
}

// OpenLayerByAnnotation opens a stored layer selected by an annotation.
func (s *Store) OpenLayerByAnnotation(ctx context.Context, project, packageName, version, key, value string) (*Content, error) {
	stored, err := s.Get(ctx, project, packageName, version)
	if err != nil {
		return nil, err
	}
	for _, layer := range stored.Layers {
		if layer.Annotations[key] != value {
			continue
		}
		size, body, err := s.Registry.PullBlob(s.Repository(project, packageName), layer.Digest)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			size = layer.Size
		}
		return &Content{Size: size, Body: body}, nil
	}
	return nil, harborerrors.NotFoundError(nil).WithMessage("layer not found")
}

// Repository returns the Harbor repository name for a package.
func (s *Store) Repository(project, packageName string) string {
	return project + "/" + s.Format.RepositoryPrefix + repositoryPackagePath(packageName)
}

func repositoryPackagePath(packageName string) string {
	packageName = strings.ToLower(packageName)
	packageName = strings.TrimPrefix(packageName, "@")
	return strings.ReplaceAll(packageName, "/@", "/")
}

func artifactTags(artifact *artifactcontroller.Artifact) []string {
	tags := make([]string, 0, len(artifact.Tags))
	for _, tag := range artifact.Tags {
		tags = append(tags, tag.Name)
	}
	return tags
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func appendUniqueID(ids []int64, id int64) []int64 {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// VersionTag returns the OCI tag for a package version.
func (s *Store) VersionTag(version string) string {
	if validTag(version) {
		return version
	}
	sum := sha256.Sum256([]byte(version))
	prefix := s.Format.VersionTagPrefix
	if prefix == "" {
		prefix = "pkg-"
	}
	return prefix + hex.EncodeToString(sum[:])[:24]
}

func (s *Store) publishTags(version string, aliases []string) ([]string, error) {
	versionTag := s.VersionTag(version)
	if !validTag(versionTag) {
		return nil, fmt.Errorf("invalid OCI tag %q", versionTag)
	}
	tags := make([]string, 0, len(aliases)+1)
	seen := map[string]struct{}{}
	for _, tag := range append([]string{versionTag}, aliases...) {
		if _, ok := seen[tag]; ok {
			continue
		}
		if !validTag(tag) {
			return nil, fmt.Errorf("invalid OCI tag %q", tag)
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags, nil
}

func validTag(tag string) bool {
	if tag == "" || len(tag) > 128 || !validTagStart(tag[0]) {
		return false
	}
	for i := 1; i < len(tag); i++ {
		if !validTagChar(tag[i]) {
			return false
		}
	}
	return true
}

func validTagStart(c byte) bool {
	return isAlphaNum(c) || c == '_'
}

func validTagChar(c byte) bool {
	return validTagStart(c) || c == '.' || c == '-'
}

func isAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func cloneAnnotations(annotations map[string]string) map[string]string {
	clone := make(map[string]string, len(annotations))
	for key, value := range annotations {
		clone[key] = value
	}
	return clone
}

func mergeLayers(existing []LayerInfo, layers []Layer) ([]LayerInfo, []v1.Descriptor, []Layer) {
	merged := make([]LayerInfo, 0, len(existing)+len(layers))
	positions := map[string]int{}
	for _, layer := range existing {
		key := layerIdentity(layer.MediaType, layer.Annotations)
		if key != "" {
			positions[key] = len(merged)
		}
		merged = append(merged, layer)
	}

	newLayers := make([]Layer, 0, len(layers))
	for _, layer := range layers {
		layerDigest := digest.FromBytes(layer.Content).String()
		info := LayerInfo{
			MediaType:   layer.MediaType,
			Digest:      layerDigest,
			Size:        int64(len(layer.Content)),
			Annotations: cloneAnnotations(layer.Annotations),
		}
		key := layerIdentity(info.MediaType, info.Annotations)
		if pos, ok := positions[key]; ok && key != "" {
			merged[pos] = info
		} else {
			if key != "" {
				positions[key] = len(merged)
			}
			merged = append(merged, info)
		}
		newLayers = append(newLayers, layer)
	}

	descriptors := make([]v1.Descriptor, 0, len(merged))
	for _, layer := range merged {
		desc := v1.Descriptor{MediaType: layer.MediaType, Digest: digest.Digest(layer.Digest), Size: layer.Size}
		if len(layer.Annotations) > 0 {
			desc.Annotations = cloneAnnotations(layer.Annotations)
		}
		descriptors = append(descriptors, desc)
	}
	return merged, descriptors, newLayers
}

func layerIdentity(mediaType string, annotations map[string]string) string {
	if title := annotations["org.opencontainers.image.title"]; title != "" {
		return title
	}
	return mediaType + "@" + annotations["org.opencontainers.image.version"]
}

func (s *Store) validate() error {
	if s.Format.VersionTagPrefix == "" || s.Format.ConfigMediaType == "" || s.Format.LayerMediaType == "" || s.Format.ArtifactType == "" {
		return fmt.Errorf("registry package store format is incomplete")
	}
	if s.Registry == nil || s.Artifacts == nil || s.Repos == nil || s.Blobs == nil || s.Quota == nil {
		return fmt.Errorf("registry package store dependencies are incomplete")
	}
	return nil
}

func (s *Store) manifest(packageName, version, configDigest string, configSize int64, layers []v1.Descriptor) ([]byte, error) {
	return json.Marshal(v1.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    v1.MediaTypeImageManifest,
		ArtifactType: s.Format.ArtifactType,
		Config: v1.Descriptor{
			MediaType: s.Format.ConfigMediaType,
			Digest:    digest.Digest(configDigest),
			Size:      configSize,
		},
		Layers: layers,
		Annotations: map[string]string{
			"org.opencontainers.image.title":   packageName,
			"org.opencontainers.image.version": version,
			"org.opencontainers.artifactType":  s.Format.ArtifactType,
		},
	})
}

func (s *Store) syncBlobs(ctx context.Context, projectID int64, manifestDigest string, manifestPayload []byte, configDigest string, configSize int64, layers []v1.Descriptor) error {
	references := []distribution.Descriptor{
		{Digest: digest.Digest(configDigest), MediaType: s.Format.ConfigMediaType, Size: configSize},
	}
	for _, layer := range layers {
		references = append(references, distribution.Descriptor{Digest: layer.Digest, MediaType: layer.MediaType, Size: layer.Size})
	}
	if err := s.Blobs.Sync(ctx, references); err != nil {
		return err
	}
	for _, ref := range references {
		if err := s.Blobs.AssociateWithProjectByDigest(ctx, ref.Digest.String(), projectID); err != nil {
			return err
		}
	}
	manifestBlobID, err := s.Blobs.Ensure(ctx, manifestDigest, v1.MediaTypeImageManifest, int64(len(manifestPayload)))
	if err != nil {
		return err
	}
	if err := s.Blobs.AssociateWithProjectByID(ctx, manifestBlobID, projectID); err != nil {
		return err
	}
	blobs := make([]string, 0, len(references))
	for _, ref := range references {
		blobs = append(blobs, ref.Digest.String())
	}
	return s.Blobs.AssociateWithArtifact(ctx, blobs, manifestDigest)
}

func configFromArtifact(art artifactmodel.Artifact) (config, bool) {
	if len(art.ExtraAttrs) == 0 {
		return config{}, false
	}
	payload, err := json.Marshal(art.ExtraAttrs)
	if err != nil {
		return config{}, false
	}
	var cfg config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return config{}, false
	}
	return cfg, cfg.PackageName != ""
}
