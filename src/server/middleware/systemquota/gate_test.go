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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/systemquota"
	"github.com/goharbor/harbor/src/lib/errors"
	systemquotatesting "github.com/goharbor/harbor/src/testing/controller/systemquota"
	"github.com/goharbor/harbor/src/testing/mock"
)

type GateTestSuite struct {
	suite.Suite
	original func() systemquota.Controller
	mockCtl  *systemquotatesting.Controller
	next     http.Handler
	called   bool
}

func (s *GateTestSuite) SetupTest() {
	s.original = ctl
	s.mockCtl = &systemquotatesting.Controller{}
	ctl = func() systemquota.Controller { return s.mockCtl }
	s.called = false
	s.next = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.called = true
		w.WriteHeader(http.StatusAccepted)
	})
}

func (s *GateTestSuite) TearDownTest() {
	ctl = s.original
}

func (s *GateTestSuite) TestAllowed() {
	s.mockCtl.On("CheckCapacity", mock.Anything, int64(0)).Return(nil)
	req := httptest.NewRequest(http.MethodPut, "/v2/library/hello/manifests/latest", nil)
	rec := httptest.NewRecorder()
	Gate()(s.next).ServeHTTP(rec, req)
	s.Equal(http.StatusAccepted, rec.Code)
	s.True(s.called)
}

func (s *GateTestSuite) TestDenied() {
	s.mockCtl.On("CheckCapacity", mock.Anything, int64(0)).Return(errors.DeniedError(nil).WithMessage("global storage quota exceeded"))
	req := httptest.NewRequest(http.MethodPost, "/v2/library/hello/blobs/uploads", nil)
	rec := httptest.NewRecorder()
	Gate()(s.next).ServeHTTP(rec, req)
	s.Equal(http.StatusForbidden, rec.Code)
	s.Contains(rec.Body.String(), "global storage quota exceeded")
	s.False(s.called)
}

func (s *GateTestSuite) TestContentLengthCountsForUploads() {
	s.mockCtl.On("CheckCapacity", mock.Anything, int64(5)).Return(nil)
	req := httptest.NewRequest(http.MethodPatch, "/v2/library/hello/blobs/uploads/abc", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	Gate()(s.next).ServeHTTP(rec, req)
	s.Equal(http.StatusAccepted, rec.Code)
	s.mockCtl.AssertCalled(s.T(), "CheckCapacity", mock.Anything, int64(5))
}

func (s *GateTestSuite) TestManifestBodyIsNotCounted() {
	s.mockCtl.On("CheckCapacity", mock.Anything, int64(0)).Return(nil)
	req := httptest.NewRequest(http.MethodPut, "/v2/library/hello/manifests/latest", strings.NewReader(`{"schemaVersion":2}`))
	rec := httptest.NewRecorder()
	Gate()(s.next).ServeHTTP(rec, req)
	s.Equal(http.StatusAccepted, rec.Code)
	s.mockCtl.AssertCalled(s.T(), "CheckCapacity", mock.Anything, int64(0))
}

func (s *GateTestSuite) TestSkipper() {
	req := httptest.NewRequest(http.MethodPut, "/v2/library/hello/manifests/latest", nil)
	rec := httptest.NewRecorder()
	Gate(func(*http.Request) bool { return true })(s.next).ServeHTTP(rec, req)
	s.Equal(http.StatusAccepted, rec.Code)
	s.mockCtl.AssertNotCalled(s.T(), "CheckCapacity", mock.Anything, mock.Anything)
}

func TestGateTestSuite(t *testing.T) {
	suite.Run(t, &GateTestSuite{})
}
