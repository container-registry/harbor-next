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
	"context"
	"fmt"
	"maps"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/rbac/system"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/pattern"
	pkgmetadata "github.com/goharbor/harbor/src/pkg/project/metadata"
	"github.com/goharbor/harbor/src/pkg/project/models"
	"github.com/goharbor/harbor/src/pkg/scan/dao/scanner"
	v1 "github.com/goharbor/harbor/src/pkg/scan/rest/v1"
	apiModels "github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	repositorytesting "github.com/goharbor/harbor/src/testing/controller/repository"
	scannertesting "github.com/goharbor/harbor/src/testing/controller/scanner"
	"github.com/goharbor/harbor/src/testing/mock"
	metadatatesting "github.com/goharbor/harbor/src/testing/pkg/project/metadata"
	htesting "github.com/goharbor/harbor/src/testing/server/v2.0/handler"
)

type ProjectTestSuite struct {
	htesting.Suite

	projectCtl *projecttesting.Controller
	repoCtl    *repositorytesting.Controller
	scannerCtl *scannertesting.Controller
	metaMgr    *metadatatesting.Manager
	project    *models.Project
	reg        *scanner.Registration
	metadata   *v1.ScannerAdapterMetadata
}

func (suite *ProjectTestSuite) SetupSuite() {
	suite.project = &models.Project{
		ProjectID: 1,
		Name:      "library",
	}

	suite.reg = &scanner.Registration{
		Name: "reg",
		URL:  "http://reg:8080",
		UUID: "uuid",
	}

	suite.projectCtl = &projecttesting.Controller{}
	suite.repoCtl = &repositorytesting.Controller{}
	suite.scannerCtl = &scannertesting.Controller{}
	suite.metaMgr = &metadatatesting.Manager{}

	suite.Config = &restapi.Config{
		ProjectAPI: &projectAPI{
			projectCtl:     suite.projectCtl,
			repositoryCtl:  suite.repoCtl,
			scannerCtl:     suite.scannerCtl,
			metadataMgr:    suite.metaMgr,
			defaultProject: func(context.Context) (string, error) { return "library", nil },
			deleteLock: func(ctx context.Context, _ int64, f func(context.Context) error) error {
				return f(ctx)
			},
		},
	}
	suite.metadata = &v1.ScannerAdapterMetadata{
		Capabilities: []*v1.ScannerCapability{
			{Type: "vulnerability", ProducesMimeTypes: []string{v1.MimeTypeScanResponse}},
			{Type: "sbom", ProducesMimeTypes: []string{v1.MimeTypeSBOMReport}},
		},
	}
	suite.Suite.SetupSuite()
}

func (suite *ProjectTestSuite) TestUpdateProjectProxyCacheFilter() {
	proxy := &models.Project{ProjectID: 2, Name: "proxy", RegistryID: 1, Metadata: map[string]string{}}
	suite.Security.On("IsAuthenticated").Return(true)
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

	// validate the request against stored metadata with the validator the handler passes
	withStored := func(stored map[string]string) func(context.Context, int64, map[string]string, pkgmetadata.Validator) error {
		return func(_ context.Context, _ int64, meta map[string]string, validate pkgmetadata.Validator) error {
			effective := maps.Clone(stored)
			maps.Copy(effective, meta)
			return validate(effective)
		}
	}
	regexStored := map[string]string{
		models.ProMetaProxyCacheFilterPattern: "^foo/.*$",
		models.ProMetaProxyCacheFilterKind:    pattern.KindRegex,
	}
	globStored := map[string]string{
		models.ProMetaProxyCacheFilterPattern: "*",
		models.ProMetaProxyCacheFilterKind:    pattern.KindDoublestar,
	}

	tests := []struct {
		name   string
		stored map[string]string
		req    *apiModels.ProjectMetadata
		filter map[string]string
		status int
	}{
		{
			name:   "pattern-only update incompatible with stored kind",
			stored: regexStored,
			req:    &apiModels.ProjectMetadata{ProxyCacheFilterPattern: stringPtr("*")},
			filter: map[string]string{models.ProMetaProxyCacheFilterPattern: "*"},
			status: 400,
		},
		{
			name:   "kind-only update incompatible with stored pattern",
			stored: globStored,
			req:    &apiModels.ProjectMetadata{ProxyCacheFilterKind: stringPtr(pattern.KindRegex)},
			filter: map[string]string{models.ProMetaProxyCacheFilterKind: pattern.KindRegex},
			status: 400,
		},
		{
			name:   "compatible pattern-only update",
			stored: regexStored,
			req:    &apiModels.ProjectMetadata{ProxyCacheFilterPattern: stringPtr("^bar/.*$"), Public: "true"},
			filter: map[string]string{models.ProMetaProxyCacheFilterPattern: "^bar/.*$"},
			status: 200,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			mock.OnAnything(suite.projectCtl, "Get").Return(proxy, nil).Once()
			suite.metaMgr.On("Get", mock.Anything, int64(2), models.ProMetaProxyCacheFilterPattern, models.ProMetaProxyCacheFilterKind).
				Return(tt.stored, nil).Once()
			if tt.status == 200 {
				// the filter keys are written by UpdateWithValidation only, after projectCtl.Update
				suite.projectCtl.On("Update", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
					_, hasPattern := p.Metadata[models.ProMetaProxyCacheFilterPattern]
					_, hasKind := p.Metadata[models.ProMetaProxyCacheFilterKind]
					return !hasPattern && !hasKind
				})).Return(nil).Once()
				suite.metaMgr.On("UpdateWithValidation", mock.Anything, int64(2), tt.filter, mock.Anything).
					Return(withStored(tt.stored)).Once()
			}

			res, err := suite.PutJSON("/projects/2", &apiModels.ProjectReq{Metadata: tt.req})
			suite.NoError(err)
			suite.Equal(tt.status, res.StatusCode)
		})
	}

	{
		// a failed project update stops before the filter is written
		mock.OnAnything(suite.projectCtl, "Get").Return(proxy, nil).Once()
		suite.metaMgr.On("Get", mock.Anything, int64(2), models.ProMetaProxyCacheFilterPattern, models.ProMetaProxyCacheFilterKind).
			Return(regexStored, nil).Once()
		mock.OnAnything(suite.projectCtl, "Update").Return(errors.BadRequestError(nil).WithMessage("invalid allowlist")).Once()

		res, err := suite.PutJSON("/projects/2", &apiModels.ProjectReq{
			Metadata: &apiModels.ProjectMetadata{ProxyCacheFilterPattern: stringPtr("^baz/.*$")},
		})
		suite.NoError(err)
		suite.Equal(400, res.StatusCode)
	}

	{
		// unrelated metadata skips the filter validation
		mock.OnAnything(suite.projectCtl, "Get").Return(proxy, nil).Once()
		suite.projectCtl.On("Update", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
			return p.Metadata[models.ProMetaAutoScan] == "true"
		})).Return(nil).Once()

		res, err := suite.PutJSON("/projects/2", &apiModels.ProjectReq{
			Metadata: &apiModels.ProjectMetadata{AutoScan: stringPtr("true")},
		})
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
	}

	suite.metaMgr.AssertExpectations(suite.T())
	suite.projectCtl.AssertExpectations(suite.T())
}

func stringPtr(value string) *string {
	return &value
}

func (suite *ProjectTestSuite) TestGetScannerOfProject() {
	times := 4
	suite.Security.On("IsAuthenticated").Return(true).Times(times)
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Times(times)

	{
		// get project failed
		mock.OnAnything(suite.projectCtl, "Get").Return(nil, fmt.Errorf("failed to get project")).Once()

		res, err := suite.Get("/projects/1/scanner")
		suite.NoError(err)
		suite.Equal(500, res.StatusCode)
	}

	{
		// scanner not found
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.scannerCtl, "GetRegistrationByProject").Return(nil, nil).Once()
		mock.OnAnything(suite.scannerCtl, "GetMetadata").Return(suite.metadata, nil).Once()
		mock.OnAnything(suite.scannerCtl, "RetrieveCap").Return(nil).Once()
		res, err := suite.Get("/projects/1/scanner")
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
	}

	{
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.scannerCtl, "GetRegistrationByProject").Return(suite.reg, nil).Once()
		mock.OnAnything(suite.scannerCtl, "GetMetadata").Return(suite.metadata, nil).Once()
		mock.OnAnything(suite.scannerCtl, "RetrieveCap").Return(nil).Once()
		var scanner scanner.Registration
		res, err := suite.GetJSON("/projects/1/scanner", &scanner)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Equal(suite.reg.UUID, scanner.UUID)
	}

	{
		mock.OnAnything(projectCtlMock, "GetByName").Return(suite.project, nil).Once()
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.scannerCtl, "GetMetadata").Return(suite.metadata, nil).Once()
		mock.OnAnything(suite.scannerCtl, "GetRegistrationByProject").Return(suite.reg, nil).Once()
		mock.OnAnything(suite.scannerCtl, "RetrieveCap").Return(nil).Once()

		var scanner scanner.Registration
		res, err := suite.GetJSON("/projects/library/scanner", &scanner)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Equal(suite.reg.UUID, scanner.UUID)
	}
}

func (suite *ProjectTestSuite) TestListScannerCandidatesOfProject() {
	suite.Security.On("IsAuthenticated").Return(true)

	{
		suite.Security.On("IsSysAdmin").Return(false)
		suite.Security.On("Can", mock.Anything, mock.Anything, system.NewNamespace().Resource(rbac.ResourceScanner)).Return(false)
		suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)
		// list scanners failed
		mock.OnAnything(suite.scannerCtl, "GetTotalOfRegistrations").Return(int64(0), fmt.Errorf("failed to count scanners")).Once()

		res, err := suite.Get("/projects/1/scanner/candidates")
		suite.NoError(err)
		suite.Equal(500, res.StatusCode)
	}

	{
		// list scanners failed
		mock.OnAnything(suite.scannerCtl, "GetTotalOfRegistrations").Return(int64(1), nil).Once()
		mock.OnAnything(suite.scannerCtl, "ListRegistrations").Return(nil, fmt.Errorf("failed to list scanners")).Once()

		res, err := suite.Get("/projects/1/scanner/candidates")
		suite.NoError(err)
		suite.Equal(500, res.StatusCode)
	}

	{
		// scanners not found
		mock.OnAnything(suite.scannerCtl, "GetTotalOfRegistrations").Return(int64(0), nil).Once()
		mock.OnAnything(suite.scannerCtl, "ListRegistrations").Return(nil, nil).Once()

		var scanners []any
		res, err := suite.GetJSON("/projects/1/scanner/candidates", &scanners)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Len(scanners, 0)
	}

	{
		// scanners found for non-admin user (URL redacted)
		mock.OnAnything(suite.scannerCtl, "GetTotalOfRegistrations").Return(int64(3), nil).Once()
		mock.OnAnything(suite.scannerCtl, "ListRegistrations").Return([]*scanner.Registration{suite.reg}, nil).Once()

		var scanners []map[string]any
		res, err := suite.GetJSON("/projects/1/scanner/candidates?page_size=1&page=2&name=n&description=d&url=u&ex_name=n&ex_url=u", &scanners)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Len(scanners, 1)
		suite.Nil(scanners[0]["url"])
		suite.Equal("3", res.Header.Get("X-Total-Count"))
		suite.Contains(res.Header, "Link")
		suite.Equal(`</api/v2.0/projects/1/scanner/candidates?description=d&ex_name=n&ex_url=u&name=n&page=1&page_size=1&url=u>; rel="prev" , </api/v2.0/projects/1/scanner/candidates?description=d&ex_name=n&ex_url=u&name=n&page=3&page_size=1&url=u>; rel="next"`, res.Header.Get("Link"))
	}

	{
		// scanners found for sysadmin user (URL kept)
		suite.Security.ExpectedCalls = nil
		suite.Security.On("IsAuthenticated").Return(true)
		suite.Security.On("IsSysAdmin").Return(true)
		suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

		mock.OnAnything(suite.scannerCtl, "GetTotalOfRegistrations").Return(int64(1), nil).Once()
		mock.OnAnything(suite.scannerCtl, "ListRegistrations").Return([]*scanner.Registration{suite.reg}, nil).Once()

		var scanners []map[string]any
		res, err := suite.GetJSON("/projects/1/scanner/candidates", &scanners)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Len(scanners, 1)
		suite.Equal(suite.reg.URL, scanners[0]["url"])
	}

	{
		// scanners found for non-sysadmin user with system scanner list permission (URL kept)
		suite.Security.ExpectedCalls = nil
		suite.Security.On("IsAuthenticated").Return(true)
		suite.Security.On("IsSysAdmin").Return(false)
		suite.Security.On("Can", mock.Anything, rbac.ActionList, system.NewNamespace().Resource(rbac.ResourceScanner)).Return(true)
		suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

		mock.OnAnything(suite.scannerCtl, "GetTotalOfRegistrations").Return(int64(1), nil).Once()
		mock.OnAnything(suite.scannerCtl, "ListRegistrations").Return([]*scanner.Registration{suite.reg}, nil).Once()

		var scanners []map[string]any
		res, err := suite.GetJSON("/projects/1/scanner/candidates", &scanners)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Len(scanners, 1)
		suite.Equal(suite.reg.URL, scanners[0]["url"])
	}
}

func (suite *ProjectTestSuite) TestSetScannerOfProject() {
	times := 3
	suite.Security.On("IsAuthenticated").Return(true).Times(times)
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true).Times(times)

	{
		// get project failed
		mock.OnAnything(suite.projectCtl, "Get").Return(nil, fmt.Errorf("failed to get project")).Once()

		res, err := suite.PutJSON("/projects/1/scanner", map[string]any{"uuid": "uuid"})
		suite.NoError(err)
		suite.Equal(500, res.StatusCode)
	}

	{
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.scannerCtl, "SetRegistrationByProject").Return(nil).Once()

		res, err := suite.PutJSON("/projects/1/scanner", map[string]any{"uuid": "uuid"})
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
	}

	{
		mock.OnAnything(projectCtlMock, "GetByName").Return(suite.project, nil).Once()
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.scannerCtl, "SetRegistrationByProject").Return(nil).Once()

		res, err := suite.PutJSON("/projects/library/scanner", map[string]any{"uuid": "uuid"})
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
	}
}

func (suite *ProjectTestSuite) TestUpdateProjectRegistryID() {
	suite.Security.On("IsAuthenticated").Return(true)
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

	proxyProject := &models.Project{
		ProjectID:  2,
		Name:       "proxy",
		RegistryID: 1,
	}

	{
		// changing registry_id of a proxy cache project is rejected
		mock.OnAnything(suite.projectCtl, "Get").Return(proxyProject, nil).Once()

		res, err := suite.PutJSON("/projects/2", map[string]any{"registry_id": 3})
		suite.NoError(err)
		suite.Equal(400, res.StatusCode)
	}

	{
		// setting registry_id on a normal project is rejected
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()

		res, err := suite.PutJSON("/projects/1", map[string]any{"registry_id": 3})
		suite.NoError(err)
		suite.Equal(400, res.StatusCode)
	}

	{
		// sending the current registry_id back unchanged is allowed
		mock.OnAnything(suite.projectCtl, "Get").Return(proxyProject, nil).Once()
		mock.OnAnything(suite.projectCtl, "Update").Return(nil).Once()

		res, err := suite.PutJSON("/projects/2", map[string]any{"registry_id": 1})
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
	}

	{
		// registry_id 0 on a normal project is allowed
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.projectCtl, "Update").Return(nil).Once()

		res, err := suite.PutJSON("/projects/1", map[string]any{"registry_id": 0})
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
	}
}

func (suite *ProjectTestSuite) TestDeleteDefaultProject() {
	// later tests register narrower Can expectations that this catch-all would shadow
	defer func() { suite.Security.ExpectedCalls = nil }()
	suite.Security.On("IsAuthenticated").Return(true)
	suite.Security.On("Can", mock.Anything, mock.Anything, mock.Anything).Return(true)

	{
		// the default project is reported as not deletable
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.repoCtl, "Count").Return(int64(0), nil).Once()

		var result map[string]any
		res, err := suite.GetJSON("/projects/1/_deletable", &result)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Nil(result["deletable"])
		suite.Contains(result["message"], "default project")
	}

	{
		// deleting the default project is rejected
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Twice()
		mock.OnAnything(suite.repoCtl, "Count").Return(int64(0), nil).Once()

		res, err := suite.Delete("/projects/1")
		suite.NoError(err)
		suite.Equal(412, res.StatusCode)
	}

	{
		// a default project with repositories still names the default project as the blocker
		mock.OnAnything(suite.projectCtl, "Get").Return(suite.project, nil).Once()
		mock.OnAnything(suite.repoCtl, "Count").Return(int64(2), nil).Once()

		var result map[string]any
		res, err := suite.GetJSON("/projects/1/_deletable", &result)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Contains(result["message"], "default project")
	}

	{
		// any other empty project stays deletable
		other := &models.Project{ProjectID: 3, Name: "other"}
		mock.OnAnything(suite.projectCtl, "Get").Return(other, nil).Once()
		mock.OnAnything(suite.repoCtl, "Count").Return(int64(0), nil).Once()

		var result map[string]any
		res, err := suite.GetJSON("/projects/3/_deletable", &result)
		suite.NoError(err)
		suite.Equal(200, res.StatusCode)
		suite.Equal(true, result["deletable"])
	}
}

func TestProjectTestSuite(t *testing.T) {
	suite.Run(t, &ProjectTestSuite{})
}
