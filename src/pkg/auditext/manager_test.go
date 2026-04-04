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

package auditext

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/auditext/model"
	_ "github.com/goharbor/harbor/src/pkg/config/db"
	mockDAO "github.com/goharbor/harbor/src/testing/moq/pkg/auditext/dao"
)

type managerTestSuite struct {
	suite.Suite
	mgr *manager
	dao *mockDAO.DAO
}

func (m *managerTestSuite) SetupTest() {
	m.dao = &mockDAO.DAO{}
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
	audit := &model.AuditLogExt{
		ProjectID:    1,
		Resource:     "library/hello-world",
		ResourceType: "artifact",
	}
	m.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*model.AuditLogExt, error) {
		return []*model.AuditLogExt{audit}, nil
	}
	auditLogs, err := m.mgr.List(nil, nil)
	m.Require().Nil(err)
	m.Equal(1, len(auditLogs))
	m.Equal(audit.Resource, auditLogs[0].Resource)
}

func (m *managerTestSuite) TestGet() {
	audit := &model.AuditLogExt{
		ProjectID:    1,
		Resource:     "library/hello-world",
		ResourceType: "artifact",
	}
	m.dao.GetFunc = func(_ context.Context, _ int64) (*model.AuditLogExt, error) {
		return audit, nil
	}
	au, err := m.mgr.Get(nil, 1)
	m.Require().Nil(err)
	m.Require().NotNil(au)
	m.Equal(audit.Resource, au.Resource)
}

func (m *managerTestSuite) TestCreate() {
	m.dao.CreateFunc = func(_ context.Context, _ *model.AuditLogExt) (int64, error) {
		return int64(1), nil
	}
	id, err := m.mgr.Create(nil, &model.AuditLogExt{
		ProjectID:    1,
		Resource:     "library/hello-world",
		ResourceType: "artifact",
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

func (m *managerTestSuite) TestPurge() {
	m.dao.PurgeFunc = func(_ context.Context, _ int, _ []string, _ bool) (int64, error) {
		return int64(1), nil
	}
	total, err := m.mgr.Purge(nil, 1, nil, false)
	m.Require().Nil(err)
	m.Equal(int64(1), total)
}

func TestManager(t *testing.T) {
	suite.Run(t, &managerTestSuite{})
}
