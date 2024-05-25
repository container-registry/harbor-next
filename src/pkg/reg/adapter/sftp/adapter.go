package sftp

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/distribution"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/reg/adapter"
	sftpdriver "github.com/goharbor/harbor/src/pkg/reg/adapter/sftp/driver"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/opencontainers/go-digest"
	"io"
	"strings"
)

func init() {
	err := adapter.RegisterFactory(model.RegistryTypeSFTP, &factory{})
	if err != nil {
		log.Errorf("failed to register factory for dtr: %v", err)
		return
	}
	log.Infof("the factory of SFTP sftpAdapter was registered")
}

type factory struct {
}

// Create ...
func (f *factory) Create(r *model.Registry) (adapter.Adapter, error) {
	driver := sftpdriver.NewDriver(r)
	ns, err := storage.NewRegistry(context.TODO(), driver)
	if err != nil {
		return nil, err
	}
	return &sftpAdapter{
		regModel: r,
		registry: ns,
	}, nil
}

// AdapterPattern ...
func (f *factory) AdapterPattern() *model.AdapterPattern {
	return nil
}

var (
	_ adapter.Adapter          = (*sftpAdapter)(nil)
	_ adapter.ArtifactRegistry = (*sftpAdapter)(nil)
)

type sftpAdapter struct {
	regModel *model.Registry
	driver   storagedriver.StorageDriver
	registry distribution.Namespace
}

func (a *sftpAdapter) FetchArtifacts(_ []*model.Filter) ([]*model.Resource, error) {
	ctx := context.Background()
	var repos = make([]string, 1000)

	_, err := a.registry.Repositories(ctx, repos, "")
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("unable to get repositories: %v", err)
	}
	var resources []*model.Resource

	for _, r := range repos {
		if r == "" {
			continue
		}

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

		resources = append(resources, &model.Resource{
			Type:     model.ResourceTypeImage,
			Registry: a.regModel,
			Metadata: &model.ResourceMetadata{
				Repository: &model.Repository{
					Name: r,
				},
				Artifacts: []*model.Artifact{
					{
						Tags: tags,
					},
				},
			},
		})
	}
	return resources, nil
}

func (a *sftpAdapter) ManifestExist(repository, ref string) (exist bool, desc *distribution.Descriptor, err error) {
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

func (a *sftpAdapter) PullManifest(repository, ref string, _ ...string) (distribution.Manifest, string, error) {
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
func (a *sftpAdapter) PushManifest(repository, ref, mediaType string, payload []byte) (string, error) {
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

func (a *sftpAdapter) DeleteManifest(repository, ref string) error {
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

func (a *sftpAdapter) BlobExist(repository, d string) (exist bool, err error) {
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

func (a *sftpAdapter) PullBlob(repository, d string) (int64, io.ReadCloser, error) {
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

func (a *sftpAdapter) PullBlobChunk(_, _ string, _, _, _ int64) (size int64, blob io.ReadCloser, err error) {
	return 0, nil, fmt.Errorf("PullBlobChunk is not implemented")
}

func (a *sftpAdapter) PushBlobChunk(_, _ string, _ int64, _ io.Reader, _, _ int64, _ string) (nextUploadLocation string, endRange int64, err error) {
	return "", 0, fmt.Errorf("PushBlobChunk is not implemented")
}

func (a *sftpAdapter) PushBlob(repository, d string, size int64, r io.Reader) error {
	fmt.Printf("push blob %s %d", d, size)

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

func (a *sftpAdapter) MountBlob(_, _, _ string) (err error) {
	return fmt.Errorf("MountBlob is not implemented")
}

func (a *sftpAdapter) CanBeMount(_ string) (mount bool, repository string, err error) {
	return false, "", nil
}

func (a *sftpAdapter) DeleteTag(r, tag string) error {
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

func (a *sftpAdapter) ListTags(r string) ([]string, error) {
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

func (a *sftpAdapter) PrepareForPush(_ []*model.Resource) error {
	return nil
}

func (a *sftpAdapter) HealthCheck() (string, error) {
	return model.Healthy, nil
}

func (a *sftpAdapter) getRepo(ctx context.Context, repository, ref string) (distribution.Repository, error) {
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

func (a *sftpAdapter) Info() (*model.RegistryInfo, error) {
	return &model.RegistryInfo{
		Type: model.RegistryTypeSFTP,
		SupportedResourceTypes: []string{
			model.ResourceTypeImage,
		},
		SupportedResourceFilters: []*model.FilterStyle{},
		SupportedTriggers: []string{
			model.TriggerTypeManual,
			model.TriggerTypeScheduled,
		},
	}, nil
}
