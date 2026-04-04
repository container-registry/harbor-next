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

package tag

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/tag/model/tag"
)

type fakeDao struct {
	CountFunc           func(ctx context.Context, query *q.Query) (int64, error)
	ListFunc            func(ctx context.Context, query *q.Query) ([]*tag.Tag, error)
	GetFunc             func(ctx context.Context, id int64) (*tag.Tag, error)
	CreateFunc          func(ctx context.Context, t *tag.Tag) (int64, error)
	UpdateFunc          func(ctx context.Context, t *tag.Tag, props ...string) error
	DeleteFunc          func(ctx context.Context, id int64) error
	DeleteOfArtifactFunc func(ctx context.Context, artifactID int64) error
}

func (f *fakeDao) Count(ctx context.Context, query *q.Query) (int64, error) {
	return f.CountFunc(ctx, query)
}
func (f *fakeDao) List(ctx context.Context, query *q.Query) ([]*tag.Tag, error) {
	return f.ListFunc(ctx, query)
}
func (f *fakeDao) Get(ctx context.Context, id int64) (*tag.Tag, error) {
	return f.GetFunc(ctx, id)
}
func (f *fakeDao) Create(ctx context.Context, t *tag.Tag) (int64, error) {
	return f.CreateFunc(ctx, t)
}
func (f *fakeDao) Update(ctx context.Context, t *tag.Tag, props ...string) error {
	return f.UpdateFunc(ctx, t, props...)
}
func (f *fakeDao) Delete(ctx context.Context, id int64) error {
	return f.DeleteFunc(ctx, id)
}
func (f *fakeDao) DeleteOfArtifact(ctx context.Context, artifactID int64) error {
	return f.DeleteOfArtifactFunc(ctx, artifactID)
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

func (m *managerTestSuite) TestCount() {
	m.dao.CountFunc = func(_ context.Context, _ *q.Query) (int64, error) {
		return int64(1), nil
	}
	total, err := m.mgr.Count(nil, nil)
	m.Require().Nil(err)
	m.Equal(int64(1), total)
}

func (m *managerTestSuite) TestList() {
	tg := &tag.Tag{
		ID:           1,
		RepositoryID: 1,
		ArtifactID:   1,
		Name:         "latest",
		PushTime:     time.Now(),
		PullTime:     time.Now(),
	}
	m.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*tag.Tag, error) {
		return []*tag.Tag{tg}, nil
	}
	tags, err := m.mgr.List(nil, nil)
	m.Require().Nil(err)
	m.Equal(1, len(tags))
	m.Equal(tg.ID, tags[0].ID)
}

func (m *managerTestSuite) TestGet() {
	m.dao.GetFunc = func(_ context.Context, _ int64) (*tag.Tag, error) {
		return &tag.Tag{}, nil
	}
	_, err := m.mgr.Get(nil, 1)
	m.Require().Nil(err)
}

func (m *managerTestSuite) TestCreate() {
	m.dao.CreateFunc = func(_ context.Context, _ *tag.Tag) (int64, error) {
		return int64(1), nil
	}
	_, err := m.mgr.Create(nil, nil)
	m.Require().Nil(err)
}

func (m *managerTestSuite) TestUpdate() {
	m.dao.UpdateFunc = func(_ context.Context, _ *tag.Tag, _ ...string) error {
		return nil
	}
	err := m.mgr.Update(nil, nil)
	m.Require().Nil(err)
}

func (m *managerTestSuite) TestDelete() {
	m.dao.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := m.mgr.Delete(nil, 1)
	m.Require().Nil(err)
}

func (m *managerTestSuite) TestDeleteOfArtifact() {
	m.dao.DeleteOfArtifactFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := m.mgr.DeleteOfArtifact(nil, 1)
	m.Require().Nil(err)
}

func TestManager(t *testing.T) {
	suite.Run(t, &managerTestSuite{})
}
