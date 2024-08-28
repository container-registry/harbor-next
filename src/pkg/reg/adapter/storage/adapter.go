package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
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
	"time"
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
	ctx := context.Background()
	var repoNames = make([]string, 1000)

	// @todo do iteration using last
	_, err := a.registry.Repositories(ctx, repoNames, "")
	spew.Dump("Repositories", err, repoNames)

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("unable to get repositories: %v", err)
	}

	spew.Dump(repoNames)
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

	spew.Dump("repositories filtered", repositories)

	runner := utils.NewLimitedConcurrentRunner(10)

	var rawResources = make([]*model.Resource, len(repositories))
	for i, r := range repositories {
		repo := r
		index := i

		runner.AddTask(func() error {
			if repo.Name == "" {
				return nil
			}

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

	if err = runner.Wait(); err != nil {
		return nil, err
	}

	var resources []*model.Resource

	for _, r := range rawResources {
		if r == nil {
			continue
		}
		resources = append(resources, r)
	}
	spew.Dump("result", resources)
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
		switch {
		case errors.Is(err, distribution.ErrBlobUnknown):
			return false, nil, nil
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

	_manifests, err := repo.Manifests(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to get manifest service: %v", err)
	}

	manifest, desc, err := distribution.UnmarshalManifest(mediaType, payload)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal manifest: %v", err)
	}

	var options = []distribution.ManifestServiceOption{
		distribution.WithManifestMediaTypes([]string{mediaType}),
	}

	if !strings.HasPrefix(ref, "sha256") {
		options = append(options, distribution.WithTag(ref))
	}

	_, err = _manifests.Put(ctx, manifest, options...)
	if err != nil {
		return "", fmt.Errorf("unable to put manifest: %v", err)
	}

	if strings.HasPrefix(ref, "sha256") {
		return desc.Digest.String(), nil
	}

	tags := repo.Tags(ctx)
	err = tags.Tag(ctx, ref, desc)
	if err != nil {
		return "", fmt.Errorf("unable to tag manifest: %v", err)
	}
	return desc.Digest.String(), nil
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

	d := digest.Digest(ref)
	err = manifests.Delete(ctx, d)
	if err != nil {
		return err
	}

	tagService := repo.Tags(ctx)

	// search tags with digest
	referencedTags, err := tagService.Lookup(ctx, distribution.Descriptor{Digest: d})
	if err != nil {
		return err
	}

	for _, tag := range referencedTags {
		if err := tagService.Untag(ctx, tag); err != nil {
			return err
		}
	}
	return nil
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

func (a *adapter) PullBlobChunk(repository, d string, _, start, end int64) (size int64, blob io.ReadCloser, err error) {

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

	_, err = readSeeker.Seek(end-start, int(start))
	if err != nil {
		return 0, nil, fmt.Errorf("unable to seek blob: %v", err)
	}

	return descriptor.Size, readSeeker, nil
}

func (a *adapter) PushBlobChunk(repository, d string, size int64, chunk io.Reader, start, end int64, location string) (nextUploadLocation string, endRange int64, err error) {

	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, d)
	if err != nil {
		return "", 0, fmt.Errorf("get repo error: %v", err)
	}

	var writer distribution.BlobWriter

	if start == 0 {
		writer, err = repo.Blobs(ctx).Create(ctx)
		if err != nil {
			return "", 0, fmt.Errorf("unable to create blob: %v", err)
		}
	} else {
		writer, err = repo.Blobs(ctx).Resume(ctx, location)
		if err != nil {
			return "", 0, fmt.Errorf("unable to resume blob: %v", err)
		}
	}

	defer writer.Close()

	_, err = writer.ReadFrom(chunk)
	if err != nil {
		return "", 0, fmt.Errorf("unable to read from chunk: %v", err)
	}

	if writer.Size() < size {
		// another chunk needed
		return writer.ID(), writer.Size(), nil
	}

	//done
	_, err = writer.Commit(ctx, distribution.Descriptor{
		Size:   size,
		Digest: digest.Digest(d),
	})
	if err != nil {
		return "", 0, fmt.Errorf("unable to commit blob: %v", err)
	}

	return writer.ID(), writer.Size(), nil
}

func (a *adapter) PushBlob(repository, d string, size int64, r io.Reader) error {
	ctx := context.Background()

	repo, err := a.getRepo(ctx, repository, d)
	if err != nil {
		return fmt.Errorf("get repo error: %v", err)
	}

	writer, err := repo.Blobs(ctx).Create(ctx)
	if err != nil {
		return fmt.Errorf("unable to create blob: %v", err)
	}
	defer func() {
		_ = writer.Cancel(ctx)
	}()

	_, err = writer.ReadFrom(r)
	if err != nil {
		return fmt.Errorf("writer unable to read from reader: %v", err)
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
		return model.Unhealthy, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := checker.Health(ctx); err != nil {
		return model.Unhealthy, nil
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
		SupportedCopyByChunk: true,
	}, nil
}
