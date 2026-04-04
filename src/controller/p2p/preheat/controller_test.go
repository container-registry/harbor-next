package preheat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/p2p/preheat/models/policy"
	providerModel "github.com/goharbor/harbor/src/pkg/p2p/preheat/models/provider"
	"github.com/goharbor/harbor/src/pkg/p2p/preheat/provider"
	"github.com/goharbor/harbor/src/pkg/p2p/preheat/provider/auth"
	taskModel "github.com/goharbor/harbor/src/pkg/task"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
	"github.com/goharbor/harbor/src/testing/moq/pkg/p2p/preheat/instance"
	pmocks "github.com/goharbor/harbor/src/testing/moq/pkg/p2p/preheat/policy"
	smocks "github.com/goharbor/harbor/src/testing/moq/pkg/scheduler"
	tmocks "github.com/goharbor/harbor/src/testing/moq/pkg/task"
)

type preheatSuite struct {
	suite.Suite
	ctx                context.Context
	controller         Controller
	fakeInstanceMgr    *instance.Manager
	fakePolicyMgr      *pmocks.Manager
	fakeScheduler      *smocks.Scheduler
	mockInstanceServer *httptest.Server
	fakeExecutionMgr   *tmocks.ExecutionManager
}

func TestPreheatSuite(t *testing.T) {
	t.Log("Start TestPreheatSuite")
	fakeInstanceMgr := &instance.Manager{}
	fakePolicyMgr := &pmocks.Manager{}
	fakeScheduler := &smocks.Scheduler{}
	fakeExecutionMgr := &tmocks.ExecutionManager{}

	var c = &controller{
		iManager:     fakeInstanceMgr,
		pManager:     fakePolicyMgr,
		scheduler:    fakeScheduler,
		executionMgr: fakeExecutionMgr,
	}
	assert.NotNil(t, c)

	ctx := orm.NewContext(context.TODO(), &ormtesting.FakeOrmer{})
	suite.Run(t, &preheatSuite{
		ctx:              ctx,
		controller:       c,
		fakeInstanceMgr:  fakeInstanceMgr,
		fakePolicyMgr:    fakePolicyMgr,
		fakeScheduler:    fakeScheduler,
		fakeExecutionMgr: fakeExecutionMgr,
	})
}

func TestNewController(t *testing.T) {
	c := NewController()
	assert.NotNil(t, c)
}

func (s *preheatSuite) SetupSuite() {
	config.Init()

	s.fakeInstanceMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*providerModel.Instance, error) {
		return []*providerModel.Instance{
			{
				ID:       1,
				Vendor:   "dragonfly",
				Endpoint: "http://localhost",
				Status:   provider.DriverStatusHealthy,
				Enabled:  true,
			},
		}, nil
	}
	s.fakeInstanceMgr.SaveFunc = func(_ context.Context, _ *providerModel.Instance) (int64, error) {
		return int64(1), nil
	}
	s.fakeInstanceMgr.CountFunc = func(_ context.Context, query *q.Query) (int64, error) {
		if query != nil && query.Keywords != nil {
			if ep, ok := query.Keywords["endpoint"]; ok && ep == "http://localhost" {
				return int64(1), nil
			}
		}
		return int64(0), nil
	}
	s.fakeInstanceMgr.DeleteFunc = func(_ context.Context, id int64) error {
		if id == 0 {
			return errors.New("not found")
		}
		return nil
	}
	s.fakeInstanceMgr.GetFunc = func(_ context.Context, id int64) (*providerModel.Instance, error) {
		switch id {
		case 1:
			return &providerModel.Instance{
				ID:       1,
				Endpoint: "http://localhost",
			}, nil
		case 0:
			return nil, errors.New("not found")
		case 2:
			return &providerModel.Instance{ID: 2}, nil
		case 1000:
			return &providerModel.Instance{ID: 1000}, nil
		case 1001:
			return &providerModel.Instance{ID: 1001}, nil
		case 1002:
			return &providerModel.Instance{ID: 1002}, nil
		case 1003:
			return &providerModel.Instance{ID: 1003, Vendor: "dragonfly"}, nil
		default:
			return &providerModel.Instance{ID: id}, nil
		}
	}

	// mock server for check health
	s.mockInstanceServer = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/_ping":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusNotImplemented)
				return
			}

			w.WriteHeader(http.StatusOK)
		}
	}))
	s.mockInstanceServer.Start()
}

// TearDownSuite clears the env.
func (s *preheatSuite) TearDownSuite() {
	s.mockInstanceServer.Close()
}

func (s *preheatSuite) TestGetAvailableProviders() {
	providers, err := s.controller.GetAvailableProviders()
	s.Equal(2, len(providers))
	expectProviders := map[string]any{}
	expectProviders["dragonfly"] = nil
	expectProviders["kraken"] = nil
	_, ok := expectProviders[providers[0].ID]
	s.True(ok)
	_, ok = expectProviders[providers[1].ID]
	s.True(ok)
	s.NoError(err)
}

func (s *preheatSuite) TestListInstance() {
	instances, err := s.controller.ListInstance(s.ctx, nil)
	s.NoError(err)
	s.Equal(1, len(instances))
	s.Equal(int64(1), instances[0].ID)
}

func (s *preheatSuite) TestCreateInstance() {
	// Case: nil instance, expect error.
	id, err := s.controller.CreateInstance(s.ctx, nil)
	s.Empty(id)
	s.Error(err)

	// Case: instance with already existed endpoint, expect conflict.
	id, err = s.controller.CreateInstance(s.ctx, &providerModel.Instance{
		Endpoint: "http://localhost",
	})
	s.Equal(ErrorConflict, err)
	s.Empty(id)

	// Case: instance with invalid provider, expect error.
	id, err = s.controller.CreateInstance(s.ctx, &providerModel.Instance{
		Endpoint: "http://foo.bar",
		Status:   "healthy",
		Vendor:   "none",
	})
	s.NoError(err)
	s.Equal(int64(1), id)

	// Case: instance with valid provider, expect ok.
	id, err = s.controller.CreateInstance(s.ctx, &providerModel.Instance{
		Endpoint: "http://foo.bar",
		Status:   "healthy",
		Vendor:   "dragonfly",
	})
	s.NoError(err)
	s.Equal(int64(1), id)

	id, err = s.controller.CreateInstance(s.ctx, &providerModel.Instance{
		Endpoint: "http://foo.bar2",
		Status:   "healthy",
		Vendor:   "kraken",
	})
	s.NoError(err)
	s.Equal(int64(1), id)
}

func (s *preheatSuite) TestDeleteInstance() {
	// instance be used should not be deleted
	s.fakePolicyMgr.ListPoliciesFunc = func(_ context.Context, query *q.Query) ([]*policy.Schema, error) {
		if query != nil && query.Keywords != nil {
			if pID, ok := query.Keywords["provider_id"]; ok {
				switch pID.(int64) {
				case 1:
					return []*policy.Schema{{ProviderID: 1}}, nil
				case 2:
					return []*policy.Schema{}, nil
				}
			}
		}
		return []*policy.Schema{}, nil
	}
	s.fakeInstanceMgr.DeleteFunc = func(_ context.Context, id int64) error {
		if id == 0 {
			return errors.New("not found")
		}
		return nil
	}
	err := s.controller.DeleteInstance(s.ctx, int64(1))
	s.Error(err, "instance should not be deleted")

	err = s.controller.DeleteInstance(s.ctx, int64(2))
	s.NoError(err, "instance can be deleted")
}

func (s *preheatSuite) TestUpdateInstance() {
	s.fakeInstanceMgr.UpdateFunc = func(_ context.Context, inst *providerModel.Instance, _ ...string) error {
		return nil
	}
	s.fakePolicyMgr.ListPoliciesFunc = func(_ context.Context, query *q.Query) ([]*policy.Schema, error) {
		if query != nil && query.Keywords != nil {
			if pID, ok := query.Keywords["provider_id"]; ok {
				switch pID.(int64) {
				case 1001:
					return []*policy.Schema{{ProviderID: 1001}}, nil
				}
			}
		}
		return []*policy.Schema{}, nil
	}

	// normal update
	err := s.controller.UpdateInstance(s.ctx, &providerModel.Instance{ID: 1000, Enabled: true})
	s.NoError(err, "instance can be updated")

	// disable instance should error due to with policy used
	err = s.controller.UpdateInstance(s.ctx, &providerModel.Instance{ID: 1001})
	s.Error(err, "instance should not be disabled")

	// disable instance can be deleted if no policy used
	err = s.controller.UpdateInstance(s.ctx, &providerModel.Instance{ID: 1002})
	s.NoError(err, "instance can be disabled")

	// not support change vendor type
	err = s.controller.UpdateInstance(s.ctx, &providerModel.Instance{ID: 1003, Vendor: "kraken"})
	s.Error(err, "provider vendor cannot be changed")
}

func (s *preheatSuite) TestGetInstance() {
	inst, err := s.controller.GetInstance(s.ctx, 1)
	s.NoError(err)
	s.NotNil(inst)
}

func (s *preheatSuite) TestCountPolicy() {
	s.fakePolicyMgr.CountFunc = func(_ context.Context, _ *q.Query) (int64, error) {
		return int64(1), nil
	}
	id, err := s.controller.CountPolicy(s.ctx, nil)
	s.NoError(err)
	s.Equal(int64(1), id)
}

func (s *preheatSuite) TestCreatePolicy() {
	pol := &policy.Schema{
		Name:       "test",
		FiltersStr: `[{"type":"repository","value":"harbor*"},{"type":"tag","value":"2*"}]`,
		TriggerStr: fmt.Sprintf(`{"type":"%s", "trigger_setting":{"cron":"0 * * * * */1"}}`, policy.TriggerTypeScheduled),
	}
	s.fakeScheduler.ScheduleFunc = func(_ context.Context, _ string, _ int64, _ string, _ string, _ string, _ any, _ map[string]any) (int64, error) {
		return int64(1), nil
	}
	s.fakePolicyMgr.CreateFunc = func(_ context.Context, schema *policy.Schema) (int64, error) {
		return int64(1), nil
	}
	s.fakePolicyMgr.UpdateFunc = func(_ context.Context, _ *policy.Schema, _ ...string) error {
		return nil
	}
	s.fakeScheduler.UnScheduleByVendorFunc = func(_ context.Context, _ string, _ int64) error {
		return nil
	}
	id, err := s.controller.CreatePolicy(s.ctx, pol)
	s.NoError(err)
	s.Equal(int64(1), id)
	s.False(pol.CreatedAt.IsZero())
	s.False(pol.UpdatedTime.IsZero())
}

func (s *preheatSuite) TestGetPolicy() {
	s.fakePolicyMgr.GetFunc = func(_ context.Context, id int64) (*policy.Schema, error) {
		if id == 1 {
			return &policy.Schema{Name: "test"}, nil
		}
		return nil, nil
	}
	p, err := s.controller.GetPolicy(s.ctx, 1)
	s.NoError(err)
	s.Equal("test", p.Name)
}

func (s *preheatSuite) TestGetPolicyByName() {
	s.fakePolicyMgr.GetByNameFunc = func(_ context.Context, projectID int64, name string) (*policy.Schema, error) {
		if projectID == 1 && name == "test" {
			return &policy.Schema{Name: "test"}, nil
		}
		return nil, nil
	}
	p, err := s.controller.GetPolicyByName(s.ctx, 1, "test")
	s.NoError(err)
	s.Equal("test", p.Name)
}

func (s *preheatSuite) TestUpdatePolicy() {
	var p0 = &policy.Schema{Name: "test", Trigger: &policy.Trigger{Type: policy.TriggerTypeScheduled}}
	p0.Trigger.Settings.Cron = "0 * * * * */1"
	p0.Filters = []*policy.Filter{
		{
			Type:  policy.FilterTypeRepository,
			Value: "harbor*",
		},
		{
			Type:  policy.FilterTypeTag,
			Value: "2*",
		},
	}
	s.fakePolicyMgr.GetFunc = func(_ context.Context, id int64) (*policy.Schema, error) {
		if id == 1 {
			return p0, nil
		}
		return nil, nil
	}
	s.fakeScheduler.UnScheduleByVendorFunc = func(_ context.Context, _ string, _ int64) error {
		return nil
	}
	s.fakeScheduler.ScheduleFunc = func(_ context.Context, _ string, _ int64, _ string, _ string, _ string, _ any, _ map[string]any) (int64, error) {
		return int64(1), nil
	}

	// need change to schedule
	p1 := &policy.Schema{
		ID:         1,
		Name:       "test",
		FiltersStr: `[{"type":"repository","value":"harbor*"},{"type":"tag","value":"2*"}]`,
		TriggerStr: fmt.Sprintf(`{"type":"%s", "trigger_setting":{}}`, policy.TriggerTypeManual),
	}
	s.fakePolicyMgr.UpdateFunc = func(_ context.Context, _ *policy.Schema, _ ...string) error {
		return nil
	}
	err := s.controller.UpdatePolicy(s.ctx, p1, "")
	s.NoError(err)
	s.False(p1.UpdatedTime.IsZero())

	// need update schedule
	p2 := &policy.Schema{
		ID:         1,
		Name:       "test",
		FiltersStr: `[{"type":"repository","value":"harbor*"},{"type":"tag","value":"2*"}]`,
		TriggerStr: fmt.Sprintf(`{"type":"%s", "trigger_setting":{"cron":"0 * * * * */2"}}`, policy.TriggerTypeScheduled),
	}
	err = s.controller.UpdatePolicy(s.ctx, p2, "")
	s.NoError(err)
	s.False(p2.UpdatedTime.IsZero())
}

func (s *preheatSuite) TestDeletePolicy() {
	var p0 = &policy.Schema{Name: "test", Trigger: &policy.Trigger{Type: policy.TriggerTypeScheduled}}
	s.fakePolicyMgr.GetFunc = func(_ context.Context, id int64) (*policy.Schema, error) {
		if id == 1 {
			return p0, nil
		}
		return nil, nil
	}
	s.fakeExecutionMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*taskModel.Execution, error) {
		return []*taskModel.Execution{
			{ID: 1},
			{ID: 2},
		}, nil
	}
	s.fakeExecutionMgr.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	s.fakePolicyMgr.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	err := s.controller.DeletePolicy(s.ctx, 1)
	s.NoError(err)
}

func (s *preheatSuite) TestListPolicies() {
	s.fakePolicyMgr.ListPoliciesFunc = func(_ context.Context, _ *q.Query) ([]*policy.Schema, error) {
		return []*policy.Schema{}, nil
	}
	p, err := s.controller.ListPolicies(s.ctx, &q.Query{})
	s.NoError(err)
	s.NotNil(p)
}

func (s *preheatSuite) TestListPoliciesByProject() {
	s.fakePolicyMgr.ListPoliciesByProjectFunc = func(_ context.Context, _ int64, _ *q.Query) ([]*policy.Schema, error) {
		return []*policy.Schema{}, nil
	}
	p, err := s.controller.ListPoliciesByProject(s.ctx, 1, nil)
	s.NoError(err)
	s.NotNil(p)
}

func (s *preheatSuite) TestDeletePoliciesOfProject() {
	fakePolicies := []*policy.Schema{
		{ID: 1000, Name: "1-should-delete", ProjectID: 10},
		{ID: 1001, Name: "2-should-delete", ProjectID: 10},
	}
	s.fakePolicyMgr.ListPoliciesByProjectFunc = func(_ context.Context, project int64, _ *q.Query) ([]*policy.Schema, error) {
		if project == 10 {
			return fakePolicies, nil
		}
		return nil, nil
	}
	s.fakePolicyMgr.GetFunc = func(_ context.Context, id int64) (*policy.Schema, error) {
		for _, p := range fakePolicies {
			if p.ID == id {
				return p, nil
			}
		}
		return nil, nil
	}
	s.fakePolicyMgr.DeleteFunc = func(_ context.Context, _ int64) error {
		return nil
	}
	s.fakeExecutionMgr.ListFunc = func(_ context.Context, _ *q.Query) ([]*taskModel.Execution, error) {
		return []*taskModel.Execution{}, nil
	}

	err := s.controller.DeletePoliciesOfProject(s.ctx, 10)
	s.NoError(err)
}

func (s *preheatSuite) TestCheckHealth() {
	// if instance is nil
	var inst *providerModel.Instance
	err := s.controller.CheckHealth(s.ctx, inst)
	s.Error(err)

	// unknown vendor
	inst = &providerModel.Instance{
		ID:       1,
		Name:     "test-instance",
		Vendor:   "unknown",
		Endpoint: "http://127.0.0.1",
		AuthMode: auth.AuthModeNone,
		Enabled:  true,
		Default:  true,
		Insecure: true,
		Status:   "Unknown",
	}
	err = s.controller.CheckHealth(s.ctx, inst)
	s.Error(err)

	// not health
	// health
	inst = &providerModel.Instance{
		ID:       1,
		Name:     "test-instance",
		Vendor:   provider.DriverDragonfly,
		Endpoint: "http://127.0.0.1",
		AuthMode: auth.AuthModeNone,
		Enabled:  true,
		Default:  true,
		Insecure: true,
		Status:   "Unknown",
	}
	err = s.controller.CheckHealth(s.ctx, inst)
	s.Error(err)

	// health
	inst = &providerModel.Instance{
		ID:       1,
		Name:     "test-instance",
		Vendor:   provider.DriverDragonfly,
		Endpoint: s.mockInstanceServer.URL,
		AuthMode: auth.AuthModeNone,
		Enabled:  true,
		Default:  true,
		Insecure: true,
		Status:   "Unknown",
	}
	err = s.controller.CheckHealth(s.ctx, inst)
	s.NoError(err)
}
