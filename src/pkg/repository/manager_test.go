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

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/repository/model"
	"github.com/goharbor/harbor/src/testing/moq/pkg/repository/dao"
)

type managerTestSuite struct {
	suite.Suite
	mgr *manager
	dao *dao.DAO
}

func (m *managerTestSuite) SetupTest() {
	m.dao = &dao.DAO{}
	m.mgr = &manager{
		dao: m.dao,
	}
}

func (m *managerTestSuite) TestCount() {
	m.dao.CountFunc = func(_ context.Context, _ *q.Query) (int64, error) {
		return int64(1), nil
	}
	n, err := m.mgr.Count(context.Background(), nil)
	m.Nil(err)
	m.Equal(int64(1), n)
}

func (m *managerTestSuite) TestList() {
	repository := &model.RepoRecord{
		RepositoryID: 1,
		ProjectID:    1,
		Name:         "library/hello-world",
	}
	m.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*model.RepoRecord, error) {
		return []*model.RepoRecord{repository}, nil
	}
	rpers, err := m.mgr.List(context.Background(), nil)
	m.Nil(err)
	m.Equal(1, len(rpers))
}

func (m *managerTestSuite) TestGet() {
	repository := &model.RepoRecord{
		RepositoryID: 1,
		ProjectID:    1,
		Name:         "library/hello-world",
	}
	m.dao.GetFunc = func(_ context.Context, _ int64) (*model.RepoRecord, error) {
		return repository, nil
	}
	repo, err := m.mgr.Get(context.Background(), 1)
	m.Require().Nil(err)
	m.Require().NotNil(repo)
	m.Equal(repository.RepositoryID, repo.RepositoryID)
}

func (m *managerTestSuite) TestGetByName() {
	repository := &model.RepoRecord{
		RepositoryID: 1,
		ProjectID:    1,
		Name:         "library/hello-world",
	}
	m.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*model.RepoRecord, error) {
		return []*model.RepoRecord{repository}, nil
	}
	repo, err := m.mgr.GetByName(context.Background(), "library/hello-world")
	m.Require().Nil(err)
	m.Require().NotNil(repo)
	m.Equal(repository.RepositoryID, repo.RepositoryID)
}

func (m *managerTestSuite) TestCreate() {
	m.dao.CreateFunc = func(_ context.Context, _ *model.RepoRecord) (int64, error) {
		return int64(1), nil
	}
	_, err := m.mgr.Create(context.Background(), &model.RepoRecord{})
	m.Nil(err)
}

func (m *managerTestSuite) TestDelete() {
	m.dao.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := m.mgr.Delete(context.Background(), 1)
	m.Nil(err)
}

func (m *managerTestSuite) TestUpdate() {
	m.dao.UpdateFunc = func(_ context.Context, _ *model.RepoRecord, _ ...string) error {
		return nil
	}
	err := m.mgr.Update(context.Background(), &model.RepoRecord{})
	m.Nil(err)
}

func (m *managerTestSuite) TestAddPullCount() {
	m.dao.AddPullCountFunc = func(_ context.Context, _ int64, _ uint64) error {
		return nil
	}
	err := m.mgr.AddPullCount(context.Background(), 1, 1)
	m.Require().Nil(err)
}

func (m *managerTestSuite) TestNonEmptyRepos() {
	repository := &model.RepoRecord{
		RepositoryID: 1,
		ProjectID:    1,
		Name:         "library/hello-world",
	}
	m.dao.NonEmptyReposFunc = func(_ context.Context) ([]*model.RepoRecord, error) {
		return []*model.RepoRecord{repository}, nil
	}
	repo, err := m.mgr.NonEmptyRepos(nil)
	m.Require().Nil(err)
	m.Equal(repository.RepositoryID, repo[0].RepositoryID)
}

func TestManager(t *testing.T) {
	suite.Run(t, &managerTestSuite{})
}
