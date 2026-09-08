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

package systemquota

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/cache"
	_ "github.com/goharbor/harbor/src/lib/cache/memory"
	liberrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/systemquota/dao"
	"github.com/goharbor/harbor/src/pkg/systemquota/model"
	blobtesting "github.com/goharbor/harbor/src/testing/controller/blob"
	"github.com/goharbor/harbor/src/testing/mock"
	systemartifacttesting "github.com/goharbor/harbor/src/testing/pkg/systemartifact"
	systemquotatesting "github.com/goharbor/harbor/src/testing/pkg/systemquota"
)

type fakeMeasurer struct {
	m   *Measurement
	err error
}

func (f fakeMeasurer) Measure(_ context.Context) (*Measurement, error) {
	return f.m, f.err
}

type ControllerTestSuite struct {
	suite.Suite
	mgr     *systemquotatesting.Manager
	blobCtl *blobtesting.Controller
	sysMgr  *systemartifacttesting.Manager
	ctl     *controller
	ctx     context.Context
}

func (s *ControllerTestSuite) SetupTest() {
	s.mgr = &systemquotatesting.Manager{}
	s.blobCtl = &blobtesting.Controller{}
	s.sysMgr = &systemartifacttesting.Manager{}
	s.ctx = context.Background()
	s.ctl = &controller{
		mgr:            s.mgr,
		blobCtl:        s.blobCtl,
		sysArtifactMgr: s.sysMgr,
		cache:          func() cache.Cache { return nil },
	}
}

func (s *ControllerTestSuite) withMemoryCache() cache.Cache {
	c, err := cache.New("memory")
	s.Require().NoError(err)
	s.ctl.cache = func() cache.Cache { return c }
	return c
}

func (s *ControllerTestSuite) quotaRow(hard int64, enforce bool) {
	s.mgr.On("Get", mock.Anything).Return(&model.SystemQuota{ID: 1, Hard: hard, Enforce: enforce, UpdateTime: time.Now()}, nil)
}

func (s *ControllerTestSuite) accounted(blobs, sys int64) {
	s.blobCtl.On("CalculateTotalSize", mock.Anything, true).Return(blobs, nil)
	s.sysMgr.On("GetStorageSize", mock.Anything).Return(sys, nil)
}

func (s *ControllerTestSuite) allocation(allocated, unlimited int64) {
	s.mgr.On("ProjectAllocation", mock.Anything).Return(&dao.ProjectAllocation{Allocated: allocated, Unlimited: unlimited}, nil)
}

func (s *ControllerTestSuite) TestGetNotSet() {
	s.mgr.On("Get", mock.Anything).Return(nil, liberrors.NotFoundError(nil))
	_, err := s.ctl.Get(s.ctx)
	s.True(liberrors.IsNotFoundErr(err))
}

func (s *ControllerTestSuite) TestGetAccounted() {
	s.quotaRow(1000, false)
	s.accounted(600, 50)
	s.allocation(800, 2)

	st, err := s.ctl.Get(s.ctx)
	s.Require().NoError(err)
	s.Equal(int64(1000), st.Hard)
	s.Equal(int64(650), st.Used)
	s.Equal(int64(350), st.Free)
	s.Equal(UsedSourceAccounted, st.UsedSource)
	s.Nil(st.MeasuredAt)
	s.Equal(int64(800), st.Allocated)
	s.Equal(int64(2), st.UnlimitedProjects)
	s.False(st.Enforce)
}

func (s *ControllerTestSuite) TestGetMeasuredWins() {
	s.quotaRow(1000, true)
	s.allocation(0, 0)
	at := time.Now().Add(-time.Minute)
	s.ctl.measurer = fakeMeasurer{m: &Measurement{Used: 900, At: at}}

	st, err := s.ctl.Get(s.ctx)
	s.Require().NoError(err)
	s.Equal(int64(900), st.Used)
	s.Equal(UsedSourceMeasured, st.UsedSource)
	s.Equal(at, *st.MeasuredAt)
	s.blobCtl.AssertNotCalled(s.T(), "CalculateTotalSize", mock.Anything, mock.Anything)
}

func (s *ControllerTestSuite) TestGetMeasurerFailureFallsBack() {
	s.quotaRow(1000, true)
	s.accounted(100, 0)
	s.allocation(0, 0)
	s.ctl.measurer = fakeMeasurer{err: errors.New("registryctl unreachable")}

	st, err := s.ctl.Get(s.ctx)
	s.Require().NoError(err)
	s.Equal(UsedSourceAccounted, st.UsedSource)
	s.Equal(int64(100), st.Used)
}

func (s *ControllerTestSuite) TestFreeNeverNegative() {
	s.quotaRow(100, false)
	s.accounted(150, 0)
	s.allocation(0, 0)

	st, err := s.ctl.Get(s.ctx)
	s.Require().NoError(err)
	s.Equal(int64(0), st.Free)
}

func (s *ControllerTestSuite) TestAccountedIsCachedAndSingleFlight() {
	s.withMemoryCache()
	s.quotaRow(1000, false)
	s.allocation(0, 0)
	s.blobCtl.On("CalculateTotalSize", mock.Anything, true).Return(int64(10), nil).Once()
	s.sysMgr.On("GetStorageSize", mock.Anything).Return(int64(0), nil).Once()

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := s.ctl.Get(s.ctx)
			s.NoError(err)
			s.Equal(int64(10), st.Used)
		}()
	}
	wg.Wait()
	s.blobCtl.AssertNumberOfCalls(s.T(), "CalculateTotalSize", 1)
}

func (s *ControllerTestSuite) TestUpdateValidation() {
	s.Error(s.ctl.Update(s.ctx, -1, false))
	s.Error(s.ctl.Update(s.ctx, 0, false))
	s.Error(s.ctl.Update(s.ctx, 1125899906842624+1, false))
	s.mgr.AssertNotCalled(s.T(), "Upsert", mock.Anything, mock.Anything)
}

func (s *ControllerTestSuite) TestUpdateInvalidatesCache() {
	c := s.withMemoryCache()
	s.quotaRow(1000, false)
	_, err := s.ctl.quota(s.ctx)
	s.Require().NoError(err)
	s.NoError(c.Fetch(s.ctx, quotaCacheKey, &model.SystemQuota{}))

	s.mgr.On("Upsert", mock.Anything, testifymock.MatchedBy(func(q *model.SystemQuota) bool {
		return q.Hard == 2000 && q.Enforce
	})).Return(nil)
	s.Require().NoError(s.ctl.Update(s.ctx, 2000, true))
	s.True(errors.Is(c.Fetch(s.ctx, quotaCacheKey, &model.SystemQuota{}), cache.ErrNotFound))
}

func (s *ControllerTestSuite) TestDelete() {
	s.mgr.On("Delete", mock.Anything).Return(nil)
	s.NoError(s.ctl.Delete(s.ctx))
}

func (s *ControllerTestSuite) TestEnforceEnabled() {
	s.mgr.On("Get", mock.Anything).Return(nil, liberrors.NotFoundError(nil)).Once()
	s.False(s.ctl.EnforceEnabled(s.ctx))

	s.mgr.On("Get", mock.Anything).Return(&model.SystemQuota{Hard: 10, Enforce: true}, nil).Once()
	s.True(s.ctl.EnforceEnabled(s.ctx))
}

func (s *ControllerTestSuite) TestCheckCapacity() {
	s.quotaRow(1000, true)
	s.accounted(900, 0)
	s.allocation(0, 0)

	s.NoError(s.ctl.CheckCapacity(s.ctx, 0))
	s.NoError(s.ctl.CheckCapacity(s.ctx, 100))
	err := s.ctl.CheckCapacity(s.ctx, 101)
	s.Require().Error(err)
	s.True(liberrors.IsErr(err, liberrors.DENIED))
	s.Contains(err.Error(), "global storage quota exceeded")
}

func (s *ControllerTestSuite) TestCheckCapacityNotEnforced() {
	s.quotaRow(1, false)
	s.NoError(s.ctl.CheckCapacity(s.ctx, 1<<40))
	s.blobCtl.AssertNotCalled(s.T(), "CalculateTotalSize", mock.Anything, mock.Anything)
}

func (s *ControllerTestSuite) TestCheckCapacityFailsOpen() {
	s.quotaRow(1000, true)
	s.blobCtl.On("CalculateTotalSize", mock.Anything, true).Return(int64(0), errors.New("db down"))
	s.NoError(s.ctl.CheckCapacity(s.ctx, 1<<40))
}

func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, &ControllerTestSuite{})
}
