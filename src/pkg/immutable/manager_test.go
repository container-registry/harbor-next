package immutable

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/q"
	dao_model "github.com/goharbor/harbor/src/pkg/immutable/dao/model"
	"github.com/goharbor/harbor/src/pkg/immutable/model"
	"github.com/goharbor/harbor/src/testing/moq/pkg/immutable/dao"
)

type managerTestingSuite struct {
	suite.Suite
	t                *testing.T
	assert           *assert.Assertions
	require          *require.Assertions
	mockImmutableDao *dao.DAO
}

func (m *managerTestingSuite) SetupSuite() {
	m.t = m.T()
	m.assert = assert.New(m.t)
	m.require = require.New(m.t)

	m.T().Setenv("RUN_MODE", "TEST")
}

func (m *managerTestingSuite) SetupTest() {
	m.mockImmutableDao = &dao.DAO{}
	Mgr = &defaultRuleManager{
		dao: m.mockImmutableDao,
	}
}

func TestManagerTestingSuite(t *testing.T) {
	suite.Run(t, &managerTestingSuite{})
}

func (m *managerTestingSuite) TestCreateImmutableRule() {
	m.mockImmutableDao.CreateImmutableRuleFunc = func(_ context.Context, _ *dao_model.ImmutableRule) (int64, error) {
		return int64(1), nil
	}
	id, err := Mgr.CreateImmutableRule(context.Background(), &model.Metadata{})
	m.require.Nil(err)
	m.assert.Equal(int64(1), id)
}

func (m *managerTestingSuite) TestQueryImmutableRuleByProjectID() {
	m.mockImmutableDao.ListImmutableRulesFunc = func(_ context.Context, _ *q.Query) ([]*dao_model.ImmutableRule, error) {
		return []*dao_model.ImmutableRule{
			{
				ID:        1,
				ProjectID: 1,
				Disabled:  false,
				TagFilter: "{\"id\":1, \"project_id\":1,\"priority\":0,\"disabled\":false,\"action\":\"immutable\"," +
					"\"template\":\"immutable_template\"," +
					"\"tag_selectors\":[{\"kind\":\"doublestar\",\"decoration\":\"matches\",\"pattern\":\"**\"}]," +
					"\"scope_selectors\":{\"repository\":[{\"kind\":\"doublestar\",\"decoration\":\"repoMatches\",\"pattern\":\"**\"}]}}",
			},
			{
				ID:        2,
				ProjectID: 1,
				Disabled:  false,
				TagFilter: "{\"id\":2, \"project_id\":1,\"priority\":0,\"disabled\":false,\"action\":\"immutable\"," +
					"\"template\":\"immutable_template\"," +
					"\"tag_selectors\":[{\"kind\":\"doublestar\",\"decoration\":\"matches\",\"pattern\":\"**\"}]," +
					"\"scope_selectors\":{\"repository\":[{\"kind\":\"doublestar\",\"decoration\":\"repoMatches\",\"pattern\":\"**\"}]}}",
			}}, nil
	}
	irs, err := Mgr.ListImmutableRules(context.Background(), &q.Query{})
	m.require.Nil(err)
	m.assert.Equal(len(irs), 2)
	m.assert.Equal(irs[1].Disabled, false)
}

func (m *managerTestingSuite) TestQueryEnabledImmutableRuleByProjectID() {
	m.mockImmutableDao.ListImmutableRulesFunc = func(_ context.Context, _ *q.Query) ([]*dao_model.ImmutableRule, error) {
		return []*dao_model.ImmutableRule{
			{
				ID:        1,
				ProjectID: 1,
				Disabled:  true,
				TagFilter: "{\"id\":1, \"project_id\":1,\"priority\":0,\"disabled\":false,\"action\":\"immutable\"," +
					"\"template\":\"immutable_template\"," +
					"\"tag_selectors\":[{\"kind\":\"doublestar\",\"decoration\":\"matches\",\"pattern\":\"**\"}]," +
					"\"scope_selectors\":{\"repository\":[{\"kind\":\"doublestar\",\"decoration\":\"repoMatches\",\"pattern\":\"**\"}]}}",
			},
			{
				ID:        2,
				ProjectID: 1,
				Disabled:  true,
				TagFilter: "{\"id\":2, \"project_id\":1,\"priority\":0,\"disabled\":false,\"action\":\"immutable\"," +
					"\"template\":\"immutable_template\"," +
					"\"tag_selectors\":[{\"kind\":\"doublestar\",\"decoration\":\"matches\",\"pattern\":\"**\"}]," +
					"\"scope_selectors\":{\"repository\":[{\"kind\":\"doublestar\",\"decoration\":\"repoMatches\",\"pattern\":\"**\"}]}}",
			}}, nil
	}
	irs, err := Mgr.ListImmutableRules(context.Background(), &q.Query{})
	m.require.Nil(err)
	m.assert.Equal(len(irs), 2)
	m.assert.Equal(irs[0].Disabled, true)
}

func (m *managerTestingSuite) TestGetImmutableRule() {
	m.mockImmutableDao.GetImmutableRuleFunc = func(_ context.Context, _ int64) (*dao_model.ImmutableRule, error) {
		return &dao_model.ImmutableRule{
			ID:        1,
			ProjectID: 1,
			Disabled:  true,
			TagFilter: "{\"id\":1, \"project_id\":1,\"priority\":0,\"disabled\":false,\"action\":\"immutable\"," +
				"\"template\":\"immutable_template\"," +
				"\"tag_selectors\":[{\"kind\":\"doublestar\",\"decoration\":\"matches\",\"pattern\":\"**\"}]," +
				"\"scope_selectors\":{\"repository\":[{\"kind\":\"doublestar\",\"decoration\":\"repoMatches\",\"pattern\":\"**\"}]}}",
		}, nil
	}
	ir, err := Mgr.GetImmutableRule(context.Background(), 1)
	m.require.Nil(err)
	m.require.NotNil(ir)
	m.assert.Equal(int64(1), ir.ID)
}

func (m *managerTestingSuite) TestUpdateImmutableRule() {
	m.mockImmutableDao.UpdateImmutableRuleFunc = func(_ context.Context, _ int64, _ *dao_model.ImmutableRule) error {
		return nil
	}
	err := Mgr.UpdateImmutableRule(context.Background(), int64(1), &model.Metadata{})
	m.require.Nil(err)
}

func (m *managerTestingSuite) TestEnableImmutableRule() {
	m.mockImmutableDao.ToggleImmutableRuleFunc = func(_ context.Context, _ int64, _ bool) error {
		return nil
	}
	err := Mgr.EnableImmutableRule(context.Background(), int64(1), true)
	m.require.Nil(err)
}

func (m *managerTestingSuite) TestDeleteImmutableRule() {
	m.mockImmutableDao.DeleteImmutableRuleFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := Mgr.DeleteImmutableRule(context.Background(), int64(1))
	m.require.Nil(err)
}
