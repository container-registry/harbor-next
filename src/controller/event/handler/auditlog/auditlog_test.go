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

//go:build db

package auditlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	common_dao "github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/audit/model"
	_ "github.com/goharbor/harbor/src/pkg/config/db"
	"github.com/goharbor/harbor/src/pkg/notifier"
	ne "github.com/goharbor/harbor/src/pkg/notifier/event"
)

type MockAuditLogManager struct {
	CountFunc  func(ctx context.Context, query *q.Query) (int64, error)
	CreateFunc func(ctx context.Context, audit *model.AuditLog) (int64, error)
	DeleteFunc func(ctx context.Context, id int64) error
	GetFunc    func(ctx context.Context, id int64) (*model.AuditLog, error)
	ListFunc   func(ctx context.Context, query *q.Query) ([]*model.AuditLog, error)
}

func (m *MockAuditLogManager) Count(ctx context.Context, query *q.Query) (int64, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx, query)
	}
	return 0, nil
}

func (m *MockAuditLogManager) Create(ctx context.Context, audit *model.AuditLog) (int64, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, audit)
	}
	return 0, nil
}

func (m *MockAuditLogManager) Delete(ctx context.Context, id int64) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *MockAuditLogManager) Get(ctx context.Context, id int64) (*model.AuditLog, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockAuditLogManager) List(ctx context.Context, query *q.Query) ([]*model.AuditLog, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, query)
	}
	return nil, nil
}

type AuditLogHandlerTestSuite struct {
	suite.Suite
	auditLogHandler *Handler
	logMgr          *MockAuditLogManager
}

func (suite *AuditLogHandlerTestSuite) SetupSuite() {
	common_dao.PrepareTestForPostgresSQL()
	suite.logMgr = &MockAuditLogManager{}
	suite.auditLogHandler = &Handler{}
}

func (suite *AuditLogHandlerTestSuite) TestSubscribeTagEvent() {

	suite.logMgr.CreateFunc = func(_ context.Context, _ *model.AuditLog) (int64, error) {
		return 1, nil
	}
	suite.logMgr.CountFunc = func(_ context.Context, _ *q.Query) (int64, error) {
		return 1, nil
	}

	// sample code to use the event framework.

	notifier.Subscribe(event.TopicCreateProject, suite.auditLogHandler)
	// event data should implement the interface TopicEvent
	ne.BuildAndPublish(context.TODO(), &metadata.CreateProjectEventMetadata{
		ProjectID: 1,
		Project:   "test",
		Operator:  "admin",
	})
	cnt, err := suite.logMgr.Count(nil, nil)

	suite.Nil(err)
	suite.Equal(int64(1), cnt)

}

func (suite *AuditLogHandlerTestSuite) TestName() {
	suite.Equal("AuditLog", suite.auditLogHandler.Name())
}

func TestAuditLogHandlerTestSuite(t *testing.T) {
	suite.Run(t, &AuditLogHandlerTestSuite{})
}
