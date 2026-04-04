package artifactrash

import (
	"context"
	"time"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/pkg/artifactrash/model"
)

type fakeDao struct {
	CreateFunc func(ctx context.Context, artifactrsh *model.ArtifactTrash) (int64, error)
	DeleteFunc func(ctx context.Context, id int64) error
	FilterFunc func(ctx context.Context, timeWindow time.Time) ([]model.ArtifactTrash, error)
	FlushFunc  func(ctx context.Context, timeWindow time.Time) error
}

func (f *fakeDao) Create(ctx context.Context, artifactrsh *model.ArtifactTrash) (int64, error) {
	return f.CreateFunc(ctx, artifactrsh)
}
func (f *fakeDao) Delete(ctx context.Context, id int64) error {
	return f.DeleteFunc(ctx, id)
}
func (f *fakeDao) Filter(ctx context.Context, timeWindow time.Time) ([]model.ArtifactTrash, error) {
	return f.FilterFunc(ctx, timeWindow)
}
func (f *fakeDao) Flush(ctx context.Context, timeWindow time.Time) error {
	return f.FlushFunc(ctx, timeWindow)
}

type managerTestSuite struct {
	suite.Suite
	mgr *manager
	dao *fakeDao
}

func (m *managerTestSuite) SetupTest() {
	m.dao = &fakeDao{}
	m.mgr = &manager{
		dao: m.dao,
	}
}

func (m *managerTestSuite) TestCreate() {
	m.dao.CreateFunc = func(_ context.Context, _ *model.ArtifactTrash) (int64, error) {
		return 1, nil
	}
	id, err := m.mgr.Create(nil, &model.ArtifactTrash{
		ManifestMediaType: v1.MediaTypeImageManifest,
		RepositoryName:    "test/hello-world",
		Digest:            "5678",
	})
	m.Require().Nil(err)
	m.Equal(int64(1), id)
}

func (m *managerTestSuite) TestDelete() {
	m.dao.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := m.mgr.Delete(nil, 1)
	m.Require().Nil(err)
}

func (m *managerTestSuite) TestFilter() {
	m.dao.FilterFunc = func(_ context.Context, _ time.Time) ([]model.ArtifactTrash, error) {
		return []model.ArtifactTrash{
			{
				ManifestMediaType: v1.MediaTypeImageManifest,
				RepositoryName:    "test/hello-world",
				Digest:            "5678",
			},
		}, nil
	}
	arts, err := m.mgr.Filter(nil, 0)
	m.Require().Nil(err)
	m.Equal(len(arts), 1)
}

func (m *managerTestSuite) TestFlush() {
	m.dao.FlushFunc = func(_ context.Context, _ time.Time) error {
		return nil
	}
	err := m.mgr.Flush(nil, 0)
	m.Require().Nil(err)
}
