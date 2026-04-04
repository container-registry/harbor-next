//go:build db

package oidc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/q"
	tdao "github.com/goharbor/harbor/src/testing/moq/pkg/oidc/dao"
)

// encrypt "secret1" using key "naa4JtarA1Zsc3uY" (set in helper_test)
var encSecret = "<enc-v1>6FvOrx1O9TKBdalX4gMQrrKNZ99KIyg="

type metaMgrTestSuite struct {
	suite.Suite
	mgr MetaManager
	dao *tdao.MetaDAO
}

func (m *metaMgrTestSuite) SetupTest() {
	m.dao = &tdao.MetaDAO{}
	m.mgr = &metaManager{
		dao: m.dao,
	}
}

func (m *metaMgrTestSuite) TestGetByUserID() {
	{
		m.dao.ListFunc = func(_ context.Context, query *q.Query) ([]*models.OIDCUser, error) {
			if query.Keywords["user_id"] == 8 {
				return []*models.OIDCUser{}, nil
			}
			return []*models.OIDCUser{
				{ID: 1, UserID: 9, Secret: encSecret, Token: "token1"},
				{ID: 2, UserID: 9, Secret: "secret", Token: "token2"},
			}, nil
		}
		_, err := m.mgr.GetByUserID(context.Background(), 8)
		m.NotNil(err)
	}
	{
		ou, err := m.mgr.GetByUserID(context.Background(), 9)
		m.Nil(err)
		m.Equal(encSecret, ou.Secret)
		m.Equal("secret1", ou.PlainSecret)
	}
}

func (m *metaMgrTestSuite) TestUpdateSecret() {
	m.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*models.OIDCUser, error) {
		return []*models.OIDCUser{
			{ID: 1, UserID: 9, Secret: encSecret, Token: "token1"},
		}, nil
	}
	m.dao.UpdateFunc = func(_ context.Context, _ *models.OIDCUser, _ ...string) error {
		return nil
	}
	err := m.mgr.SetCliSecretByUserID(context.Background(), 9, "new")
	m.Nil(err)
}

func TestManager(t *testing.T) {
	suite.Run(t, &metaMgrTestSuite{})
}
