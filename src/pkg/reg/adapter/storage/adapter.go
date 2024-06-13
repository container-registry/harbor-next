package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/distribution"
	"github.com/docker/distribution/reference"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/goharbor/harbor/src/common/utils"
	regadapter "github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
	"github.com/goharbor/harbor/src/pkg/reg/filter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/opencontainers/go-digest"
	"io"
	"strings"
)

var (
	_ regadapter.Adapter          = (*adapter)(nil)
	_ regadapter.ArtifactRegistry = (*adapter)(nil)
)

type adapter struct {
	regModel *model.Registry
	driver   storagedriver.StorageDriver
	registry distribution.Namespace
}

func (a *adapter) FetchArtifacts(filters []*model.Filter) ([]*model.Resource, error) {
	fmt.Println("Fetch artifacts")

	ctx := context.Background()
	var repoNames = make([]string, 1000)

	_, err := a.registry.Repositories(ctx, repoNames, "")
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("unable to get repositories: %v", err)
	}

	if len(repoNames) == 0 {
		return nil, nil
	}

	var repositories []*model.Repository
	for _, repoName := range repoNames {
		repositories = append(repositories, &model.Repository{
			Name: repoName,
		})
	}

	repositories, err = filter.DoFilterRepositories(repositories, filters)
	if err != nil {
		return nil, err
	}

	runner := utils.NewLimitedConcurrentRunner(10)

	var rawResources = make([]*model.Resource, len(repositories))
	for i, r := range repositories {
		repo := r
		index := i

		runner.AddTask(func() error {
			named, err := reference.WithName(repo.Name)
			if err != nil {
				return fmt.Errorf("ref %s error: %v", repo.Name, err)
			}
			repository, err := a.registry.Repository(ctx, named)
			if err != nil {
				return fmt.Errorf("unable to get repo %s: %v", repo.Name, err)
			}

			tags, err := repository.Tags(ctx).All(ctx)
			if err != nil {
				return fmt.Errorf("unable to get all tags for repo %s: %v", r, err)
			}

			artifacts := []*model.Artifact{
				{
					Tags: tags,
				},
			}

			artifacts, err = filter.DoFilterArtifacts(artifacts, filters)
			if err != nil {
				return fmt.Errorf("failed to list artifacts of repository %s: %v", repo, err)
			}

			if len(artifacts) == 0 {
				return nil
			}

			rawResources[index] = &model.Resource{
				Type:     model.ResourceTypeImage,
				Registry: a.regModel,
				Metadata: &model.ResourceMetadata{
					Repository: &model.Repository{
						Name: r.Name,
					},
					Artifacts: artifacts,
				},
			}
			return nil
		})
	}

	var resources []*model.Resource

	for _, r := range rawResources {
		if r == nil {
			continue
		}
		resources = append(resources, r)
	}
	return resources, nil
}

func (a *adapter) ManifestExist(repository, ref string) (exist bool, desc *distribution.Descriptor, err error) {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, ref)
	if err != nil {
		return false, nil, fmt.Errorf("get repo error: %v", err)
	}

	tagService := repo.Tags(ctx)

	var d digest.Digest

	if !strings.HasPrefix(ref, "sha256:") {
		// looks like a tag
		descriptor, err := tagService.Get(ctx, ref)
		if err != nil {
			var errTagUnknown distribution.ErrTagUnknown
			if errors.As(err, &errTagUnknown) {
				return false, nil, nil
			}
			return false, nil, fmt.Errorf("unable to get tag %s: %v", ref, err)
		}
		d = descriptor.Digest
	} else {
		d = digest.Digest(ref)
	}

	blobs := repo.Blobs(ctx)
	descriptor, err := blobs.Stat(ctx, d)
	if err != nil {
		if err != nil {
			switch {
			case errors.Is(err, distribution.ErrBlobUnknown):
				return false, nil, nil
			}
		}
		return false, nil, fmt.Errorf("manifest check (blob sata) error: %v", err)
	}
	return true, &descriptor, nil
}

func (a *adapter) PullManifest(repository, ref string, _ ...string) (distribution.Manifest, string, error) {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, ref)
	if err != nil {
		return nil, "", fmt.Errorf("get repo error: %v", err)
	}

	var (
		opts []distribution.ManifestServiceOption
		d    digest.Digest
	)

	if !strings.HasPrefix(ref, "sha256:") {
		// looks like a tag
		descriptor, err := repo.Tags(ctx).Get(ctx, ref)
		if err != nil {
			return nil, "", fmt.Errorf("unable to get tag: %v", err)
		}
		opts = append(opts, distribution.WithTag(ref))
		d = descriptor.Digest
	} else {
		d = digest.Digest(ref)
	}

	manifestService, err := repo.Manifests(ctx)
	if err != nil {
		return nil, "", err
	}

	manifest, err := manifestService.Get(ctx, d, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("unable to get manifest: %v", err)
	}

	return manifest, d.String(), err
}

// PushManifest manifests are blobs actually
func (a *adapter) PushManifest(repository, ref, mediaType string, payload []byte) (string, error) {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, ref)
	if err != nil {
		return "", fmt.Errorf("get repo error: %v", err)
	}

	descriptor, err := repo.Blobs(ctx).Put(ctx, mediaType, payload)
	if err != nil {
		return "", fmt.Errorf("push manifest error: %v", err)
	}

	if strings.HasPrefix(ref, "sha256") {
		return descriptor.Digest.String(), nil
	}

	if err = repo.Tags(ctx).Tag(ctx, ref, descriptor); err != nil {
		return "", fmt.Errorf("unable to tag %s: %v", ref, err)
	}
	return descriptor.Digest.String(), nil
}

func (a *adapter) DeleteManifest(repository, ref string) error {
	ctx := context.Background()

	named, err := reference.WithName(repository)
	if err != nil {
		return err
	}
	repo, err := a.registry.Repository(ctx, named)
	if err != nil {
		return err
	}

	manifests, err := repo.Manifests(ctx)
	if err != nil {
		return err
	}
	return manifests.Delete(ctx, digest.Digest(ref))
}

func (a *adapter) BlobExist(repository, d string) (exist bool, err error) {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, d)
	if err != nil {
		return false, fmt.Errorf("get repo error: %v", err)
	}

	blobs := repo.Blobs(ctx)
	_, err = blobs.Stat(ctx, digest.Digest(d))
	if err != nil {
		switch {
		case errors.Is(err, distribution.ErrBlobUnknown):
			return false, nil
		}
	}
	return true, nil
}

func (a *adapter) PullBlob(repository, d string) (int64, io.ReadCloser, error) {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, d)
	if err != nil {
		return 0, nil, fmt.Errorf("get repo error: %v", err)
	}

	blobs := repo.Blobs(ctx)

	descriptor, err := blobs.Stat(ctx, digest.Digest(d))
	if err != nil {
		return 0, nil, fmt.Errorf("unable to get blob size: %v", err)
	}

	readSeeker, err := blobs.Open(ctx, digest.Digest(d))
	if err != nil {
		return 0, nil, fmt.Errorf("unable to open blob: %v", err)
	}
	return descriptor.Size, readSeeker, nil
}

func (a *adapter) PullBlobChunk(_, _ string, _, _, _ int64) (size int64, blob io.ReadCloser, err error) {
	return 0, nil, fmt.Errorf("PullBlobChunk is not implemented")
}

func (a *adapter) PushBlobChunk(_, _ string, _ int64, _ io.Reader, _, _ int64, _ string) (nextUploadLocation string, endRange int64, err error) {
	return "", 0, fmt.Errorf("PushBlobChunk is not implemented")
}

func (a *adapter) PushBlob(repository, d string, size int64, r io.Reader) error {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, d)
	if err != nil {
		return fmt.Errorf("get repo error: %v", err)
	}

	blobs := repo.Blobs(ctx)

	writer, err := blobs.Create(ctx)
	if err != nil {
		return fmt.Errorf("unable to create blob: %v", err)
	}
	defer func() {
		_ = writer.Cancel(ctx)
	}()

	_, err = writer.ReadFrom(r)
	if err != nil {
		return fmt.Errorf("writer is unable to write: %v", err)
	}

	_, err = writer.Commit(ctx, distribution.Descriptor{
		Size:   size,
		Digest: digest.Digest(d),
	})
	if err != nil {
		return fmt.Errorf("unable to commit: %v", err)
	}
	return nil
}

func (a *adapter) MountBlob(_, _, _ string) (err error) {
	return fmt.Errorf("MountBlob is not implemented")
}

func (a *adapter) CanBeMount(_ string) (mount bool, repository string, err error) {
	return false, "", nil
}

func (a *adapter) DeleteTag(r, tag string) error {
	ctx := context.Background()
	named, err := reference.WithName(r)
	if err != nil {
		return fmt.Errorf("ref %s error: %v", r, err)
	}
	repo, err := a.registry.Repository(ctx, named)
	if err != nil {
		return fmt.Errorf("unable to get repo %s: %v", r, err)
	}
	return repo.Tags(ctx).Untag(ctx, tag)
}

func (a *adapter) ListTags(r string) ([]string, error) {
	ctx := context.Background()

	named, err := reference.WithName(r)
	if err != nil {
		return nil, fmt.Errorf("ref %s error: %v", r, err)
	}
	repo, err := a.registry.Repository(ctx, named)
	if err != nil {
		return nil, fmt.Errorf("unable to get repo %s: %v", r, err)
	}
	tags, err := repo.Tags(ctx).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get all tags for repo %s: %v", r, err)
	}
	return tags, nil
}

func (a *adapter) PrepareForPush(_ []*model.Resource) error {
	return nil
}

func (a *adapter) HealthCheck() (string, error) {
	checker, ok := a.driver.(health.Checker)
	if !ok {
		return model.Healthy, nil
	}
	if err := checker.Health(context.Background()); err != nil {
		return model.Unhealthy, err
	}
	return model.Healthy, nil
}

func (a *adapter) getRepo(ctx context.Context, repository, ref string) (distribution.Repository, error) {
	named, err := reference.WithName(repository)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(ref, "sha256:") {
		named, err = reference.WithDigest(named, digest.Digest(ref))
	} else {
		named, err = reference.WithTag(named, ref)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to build reference: %v", err)
	}
	return a.registry.Repository(ctx, named)
}

func (a *adapter) Info() (*model.RegistryInfo, error) {
	return &model.RegistryInfo{
		Type: model.RegistryTypeSFTP,
		SupportedResourceTypes: []string{
			model.ResourceTypeImage,
		},
		SupportedResourceFilters: []*model.FilterStyle{
			{
				Type:  model.FilterTypeName,
				Style: model.FilterStyleTypeText,
			},
			{
				Type:  model.FilterTypeTag,
				Style: model.FilterStyleTypeText,
			},
		},
		SupportedTriggers: []string{
			model.TriggerTypeManual,
			model.TriggerTypeScheduled,
		},
	}, nil
}
