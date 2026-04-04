package instance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/q"
	providerModel "github.com/goharbor/harbor/src/pkg/p2p/preheat/models/provider"
)

type fakeDao struct {
	CreateFunc    func(ctx context.Context, instance *providerModel.Instance) (int64, error)
	GetFunc       func(ctx context.Context, id int64) (*providerModel.Instance, error)
	GetByNameFunc func(ctx context.Context, name string) (*providerModel.Instance, error)
	UpdateFunc    func(ctx context.Context, instance *providerModel.Instance, props ...string) error
	DeleteFunc    func(ctx context.Context, id int64) error
	CountFunc     func(ctx context.Context, query *q.Query) (int64, error)
	ListFunc      func(ctx context.Context, query *q.Query) ([]*providerModel.Instance, error)
}

func (d *fakeDao) Create(ctx context.Context, instance *providerModel.Instance) (int64, error) {
	return d.CreateFunc(ctx, instance)
}
func (d *fakeDao) Get(ctx context.Context, id int64) (*providerModel.Instance, error) {
	return d.GetFunc(ctx, id)
}
func (d *fakeDao) GetByName(ctx context.Context, name string) (*providerModel.Instance, error) {
	return d.GetByNameFunc(ctx, name)
}
func (d *fakeDao) Update(ctx context.Context, instance *providerModel.Instance, props ...string) error {
	return d.UpdateFunc(ctx, instance, props...)
}
func (d *fakeDao) Delete(ctx context.Context, id int64) error {
	return d.DeleteFunc(ctx, id)
}
func (d *fakeDao) Count(ctx context.Context, query *q.Query) (int64, error) {
	return d.CountFunc(ctx, query)
}
func (d *fakeDao) List(ctx context.Context, query *q.Query) ([]*providerModel.Instance, error) {
	return d.ListFunc(ctx, query)
}

var lists = []*providerModel.Instance{
	{Name: "abc"},
}

type instanceManagerSuite struct {
	suite.Suite
	dao     *fakeDao
	ctx     context.Context
	manager Manager
}

func (im *instanceManagerSuite) SetupSuite() {
	im.dao = &fakeDao{}
	im.manager = &manager{dao: im.dao}
	im.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*providerModel.Instance, error) {
		return lists, nil
	}
}

func (im *instanceManagerSuite) TestSave() {
	im.dao.CreateFunc = func(_ context.Context, _ *providerModel.Instance) (int64, error) {
		return int64(1), nil
	}
	id, err := im.manager.Save(im.ctx, nil)
	im.Require().Nil(err)
	im.Require().Equal(int64(1), id)
}

func (im *instanceManagerSuite) TestDelete() {
	im.dao.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := im.manager.Delete(im.ctx, 1)
	im.Require().Nil(err)
}

func (im *instanceManagerSuite) TestUpdate() {
	im.dao.UpdateFunc = func(_ context.Context, _ *providerModel.Instance, _ ...string) error {
		return nil
	}
	err := im.manager.Update(im.ctx, nil)
	im.Require().Nil(err)
}

func (im *instanceManagerSuite) TestGet() {
	ins := &providerModel.Instance{Name: "abc"}
	im.dao.GetFunc = func(_ context.Context, _ int64) (*providerModel.Instance, error) {
		return ins, nil
	}
	res, err := im.manager.Get(im.ctx, 1)
	im.Require().Nil(err)
	im.Require().Equal(ins, res)
}

func (im *instanceManagerSuite) TestGetByName() {
	im.dao.GetByNameFunc = func(_ context.Context, _ string) (*providerModel.Instance, error) {
		return lists[0], nil
	}
	res, err := im.manager.GetByName(im.ctx, "abc")
	im.Require().Nil(err)
	im.Require().Equal(lists[0], res)
}

func (im *instanceManagerSuite) TestCount() {
	im.dao.CountFunc = func(_ context.Context, _ *q.Query) (int64, error) {
		return int64(2), nil
	}
	count, err := im.manager.Count(im.ctx, nil)
	assert.Nil(im.T(), err)
	assert.Equal(im.T(), int64(2), count)
}

func (im *instanceManagerSuite) TestList() {
	l := []*providerModel.Instance{
		{Name: "abc"},
	}
	im.dao.ListFunc = func(_ context.Context, _ *q.Query) ([]*providerModel.Instance, error) {
		return l, nil
	}
	res, err := im.manager.List(im.ctx, nil)
	assert.Nil(im.T(), err)
	assert.Len(im.T(), res, 1)
	assert.Equal(im.T(), l, res)
}

func TestInstanceManager(t *testing.T) {
	suite.Run(t, &instanceManagerSuite{})
}
