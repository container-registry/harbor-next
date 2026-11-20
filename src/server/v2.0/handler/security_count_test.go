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

package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/pkg/scan/scanner"
	secHubModel "github.com/goharbor/harbor/src/pkg/securityhub/model"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	securityhubtesting "github.com/goharbor/harbor/src/testing/controller/securityhub"
	"github.com/goharbor/harbor/src/testing/mock"
	scannertesting "github.com/goharbor/harbor/src/testing/pkg/scan/scanner"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

func TestTotalFromPage(t *testing.T) {
	tests := []struct {
		name      string
		page      int64
		pageSize  int64
		rows      int
		tuneCount bool
		want      int64
		known     bool
	}{
		{"empty first page", 1, 100, 0, true, 0, true},
		{"partial first page", 1, 100, 90, true, 90, true},
		{"partial later page", 3, 100, 42, false, 242, true},
		{"full page needs a count", 2, 100, 100, true, 0, false},
		{"empty later page needs a count", 4, 100, 0, true, 0, false},
		{"no page size needs a count", 1, 0, 5, true, 0, false},
		{"full last page below the tune_count limit", 10, 100, 100, true, 0, false},
		{"partial page just below the tune_count limit", 10, 100, 99, true, 999, true},
		{"partial page above the tune_count limit", 11, 100, 7, true, -1, true},
		{"partial page above the limit without tune_count", 11, 100, 7, false, 1007, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := totalFromPage(tt.page, tt.pageSize, tt.rows, tt.tuneCount)
			assert.Equal(t, tt.known, known)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLinkTotal(t *testing.T) {
	assert.Equal(t, int64(42), linkTotal(42, 1, 100, 42), "a known total is used as is")
	assert.Equal(t, int64(101), linkTotal(-1, 1, 100, 100), "a full first page with a hidden total has a next page")
	assert.Equal(t, int64(501), linkTotal(-1, 5, 100, 100), "a full later page has prev and next")
	assert.Equal(t, int64(430), linkTotal(-1, 5, 100, 30), "a partial page has prev only")
}

type securityVulSuite struct {
	htesting.Suite
	ctl        *securityhubtesting.Controller
	scannerMgr *scannertesting.Manager
	origMgr    scanner.Manager
}

func (s *securityVulSuite) SetupSuite() {
	s.origMgr = scanner.Mgr
}

func (s *securityVulSuite) TearDownSuite() {
	scanner.Mgr = s.origMgr
	s.Suite.TearDownSuite()
}

func (s *securityVulSuite) SetupTest() {
	s.ctl = &securityhubtesting.Controller{}
	s.scannerMgr = &scannertesting.Manager{}
	scanner.Mgr = s.scannerMgr
	s.Config = &restapi.Config{SecurityhubAPI: &securityAPI{controller: s.ctl}}
	s.Suite.SetupSuite()
	s.Security.On("IsAuthenticated").Return(true)
	s.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)
	mock.OnAnything(s.scannerMgr, "DefaultScannerUUID").Return("ruuid", nil)
}

func vulRows(n int) []*secHubModel.VulnerabilityItem {
	rows := make([]*secHubModel.VulnerabilityItem, n)
	for i := range rows {
		rows[i] = &secHubModel.VulnerabilityItem{}
	}
	return rows
}

func (s *securityVulSuite) TestCountRunsOnlyWhenPageIsFull() {
	tests := []struct {
		name      string
		query     string
		rows      int
		count     int64
		wantCount bool
		wantTotal string
		wantNext  bool
		wantPrev  bool
	}{
		{"partial first page", "page=1&page_size=100&tune_count=true", 90, 0, false, "90", false, false},
		{"partial later page", "page=3&page_size=100", 42, 0, false, "242", false, true},
		{"full page counts", "page=1&page_size=100", 100, 250, true, "250", true, false},
		{"empty later page counts", "page=4&page_size=100&tune_count=true", 0, 90, true, "90", false, false},
		{"hidden total keeps next and prev", "page=2&page_size=100&tune_count=true", 100, -1, true, "-1", true, true},
		{"partial page above the limit", "page=11&page_size=100&tune_count=true", 7, 0, false, "-1", false, true},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.ctl.ExpectedCalls = nil
			s.ctl.Calls = nil
			mock.OnAnything(s.ctl, "ListVuls").Return(vulRows(tt.rows), nil)
			if tt.wantCount {
				mock.OnAnything(s.ctl, "CountVuls").Return(tt.count, nil).Once()
			}
			res, err := s.DoReq(http.MethodGet, fmt.Sprintf("/security/vul?%s", tt.query), nil)
			s.Require().NoError(err)
			s.Equal(http.StatusOK, res.StatusCode)
			s.Equal(tt.wantTotal, res.Header.Get("X-Total-Count"))
			link := res.Header.Get("Link")
			s.Equal(tt.wantNext, strings.Contains(link, `rel="next"`), "Link: %s", link)
			s.Equal(tt.wantPrev, strings.Contains(link, `rel="prev"`), "Link: %s", link)
			if tt.wantCount {
				s.ctl.AssertNumberOfCalls(s.T(), "CountVuls", 1)
			} else {
				s.ctl.AssertNotCalled(s.T(), "CountVuls", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestSecurityVulSuite(t *testing.T) {
	suite.Run(t, &securityVulSuite{})
}
