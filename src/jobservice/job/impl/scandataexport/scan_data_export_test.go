//go:build db

package scandataexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact"
	"github.com/goharbor/harbor/src/jobservice/job"
	artpkg "github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/scan/export"
	sysartmodel "github.com/goharbor/harbor/src/pkg/systemartifact/model"
	"github.com/goharbor/harbor/src/pkg/task"
	htesting "github.com/goharbor/harbor/src/testing"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
	export2 "github.com/goharbor/harbor/src/testing/moq/pkg/scan/export"
	systemartifacttesting "github.com/goharbor/harbor/src/testing/moq/pkg/systemartifact"
	tasktesting "github.com/goharbor/harbor/src/testing/moq/pkg/task"
)

const (
	ExecID     int64 = 1000000
	JobId            = "1000000"
	MockDigest       = "mockDigest"
)

type ScanDataExportJobTestSuite struct {
	htesting.Suite
	execMgr          *tasktesting.ExecutionManager
	job              *ScanDataExport
	exportMgr        *export2.Manager
	digestCalculator *export2.ArtifactDigestCalculator
	filterProcessor  *export2.FilterProcessor
	sysArtifactMgr   *systemartifacttesting.Manager
}

func (suite *ScanDataExportJobTestSuite) SetupSuite() {
}

func (suite *ScanDataExportJobTestSuite) SetupTest() {
	suite.execMgr = &tasktesting.ExecutionManager{}
	suite.exportMgr = &export2.Manager{}
	suite.digestCalculator = &export2.ArtifactDigestCalculator{}
	suite.filterProcessor = &export2.FilterProcessor{}
	suite.sysArtifactMgr = &systemartifacttesting.Manager{}
	suite.job = &ScanDataExport{
		execMgr:               suite.execMgr,
		exportMgr:             suite.exportMgr,
		scanDataExportDirPath: "/tmp",
		digestCalculator:      suite.digestCalculator,
		filterProcessor:       suite.filterProcessor,
		sysArtifactMgr:        suite.sysArtifactMgr,
	}

	suite.execMgr.UpdateExtraAttrsFunc = func(_ context.Context, _ int64, _ map[string]any) error {
		return nil
	}
	// all BLOB related operations succeed
	suite.sysArtifactMgr.CreateFunc = func(_ context.Context, _ *sysartmodel.SystemArtifact, _ io.Reader) (int64, error) {
		return int64(1), nil
	}
}

func (suite *ScanDataExportJobTestSuite) TestRun() {

	data := suite.createDataRecords(3)
	suite.exportMgr.FetchFunc = func(_ context.Context, _ export.Params) ([]export.Data, error) {
		return data, nil
	}
	suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
		return digest.Digest(MockDigest), nil
	}
	suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
		return []int64{1}, nil
	}
	suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
		return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
	}
	suite.filterProcessor.ProcessLabelFilterFunc = func(_ context.Context, _ []int64, _ []*artifact.Artifact) ([]*artifact.Artifact, error) {
		return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
	}

	execAttrs := make(map[string]any)
	execAttrs[export.JobNameAttribute] = "test-job"
	execAttrs[export.UserNameAttribute] = "test-user"
	suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
		return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
	}

	params := job.Parameters{}
	params[export.JobModeKey] = export.JobModeExport
	params["JobId"] = JobId
	params["Request"] = map[string]any{
		"projects": []int64{1},
	}
	ctx := &mockjobservice.MockJobContext{}

	err := suite.job.Run(ctx, params)
	suite.NoError(err)
	// Verify sysArtifactMgr.Create was called with matching record
	createCalls := suite.sysArtifactMgr.CreateCalls()
	suite.NotEmpty(createCalls)
	lastCreate := createCalls[len(createCalls)-1]
	suite.Equal("scandata_export_1000000", lastCreate.ArtifactRecord.Repository)
	suite.Equal(strings.ToLower(export.Vendor), lastCreate.ArtifactRecord.Vendor)
	suite.Equal(MockDigest, lastCreate.ArtifactRecord.Digest)

	// Verify UpdateExtraAttrs was called with correct digest and attrs
	updateCalls := suite.execMgr.UpdateExtraAttrsCalls()
	suite.NotEmpty(updateCalls)
	found := false
	for _, c := range updateCalls {
		if c.ID == ExecID {
			if c.ExtraAttrs[export.DigestKey] == MockDigest &&
				c.ExtraAttrs[export.JobNameAttribute] == "test-job" &&
				c.ExtraAttrs[export.UserNameAttribute] == "test-user" {
				found = true
			}
		}
	}
	suite.True(found, "Expected UpdateExtraAttrs call with correct attrs")
	_, err = os.Stat("/tmp/scandata_export_1000000.csv")
	suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")

}

func (suite *ScanDataExportJobTestSuite) TestRunWithEmptyData() {
	var data []export.Data
	suite.exportMgr.FetchFunc = func(_ context.Context, _ export.Params) ([]export.Data, error) {
		return data, nil
	}
	suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
		return digest.Digest(MockDigest), nil
	}

	execAttrs := make(map[string]any)
	execAttrs[export.JobNameAttribute] = "test-job"
	execAttrs[export.UserNameAttribute] = "test-user"
	suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
		return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
	}

	params := job.Parameters{}
	params[export.JobModeKey] = export.JobModeExport
	params["JobId"] = JobId
	ctx := &mockjobservice.MockJobContext{}

	err := suite.job.Run(ctx, params)
	suite.NoError(err)

	// Verify UpdateExtraAttrs was called with "No vulnerabilities" message
	updateCalls := suite.execMgr.UpdateExtraAttrsCalls()
	found := false
	for _, c := range updateCalls {
		if c.ExtraAttrs["status_message"] == "No vulnerabilities found or matched" &&
			c.ExtraAttrs[export.JobNameAttribute] == "test-job" &&
			c.ExtraAttrs[export.UserNameAttribute] == "test-user" {
			found = true
		}
	}
	suite.True(found, "Expected UpdateExtraAttrs call with 'No vulnerabilities' message")
}

func (suite *ScanDataExportJobTestSuite) TestRunAttributeUpdateError() {

	data := suite.createDataRecords(3)
	suite.exportMgr.FetchFunc = func(_ context.Context, _ export.Params) ([]export.Data, error) {
		return data, nil
	}
	suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
		return []int64{1}, nil
	}
	suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
		return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
	}
	suite.filterProcessor.ProcessLabelFilterFunc = func(_ context.Context, _ []int64, _ []*artifact.Artifact) ([]*artifact.Artifact, error) {
		return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
	}
	suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
		return digest.Digest(MockDigest), nil
	}

	suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
		return nil, errors.New("test-error")
	}

	params := job.Parameters{}
	params[export.JobModeKey] = export.JobModeExport
	params["JobId"] = JobId
	params["Request"] = map[string]any{
		"projects": []int{1},
	}
	ctx := &mockjobservice.MockJobContext{}

	err := suite.job.Run(ctx, params)
	suite.Error(err)
	// Create should have been called (for sys artifact)
	suite.NotEmpty(suite.sysArtifactMgr.CreateCalls())

	_, err = os.Stat("/tmp/scandata_export_1000000.csv")
	suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")

}

func (suite *ScanDataExportJobTestSuite) TestExtractCriteria() {
	// empty request should return error
	_, err := suite.job.extractCriteria(job.Parameters{})
	suite.Error(err)
	// invalid request should return error
	_, err = suite.job.extractCriteria(job.Parameters{"Request": ""})
	suite.Error(err)
	// valid request should not return error and trim space
	c, err := suite.job.extractCriteria(job.Parameters{"Request": map[string]any{
		"CVEIds":       "CVE-123, CVE-456 ",
		"Repositories": " test-repo1 ",
		"Tags":         "test-tag1, test-tag2",
	}})
	suite.NoError(err)
	suite.Equal("CVE-123,CVE-456", c.CVEIds)
	suite.Equal("test-repo1", c.Repositories)
	suite.Equal("test-tag1,test-tag2", c.Tags)
}

func (suite *ScanDataExportJobTestSuite) TestRunWithCriteria() {
	{
		data := suite.createDataRecords(3)

		fetchCallCount := 0
		suite.exportMgr.FetchFunc = func(_ context.Context, p export.Params) ([]export.Data, error) {
			fetchCallCount++
			if fetchCallCount == 1 {
				// Verify CVEIds in params
				assert.Equal(suite.T(), "CVE-123", p.CVEIds)
				return data, nil
			}
			return make([]export.Data, 0), nil
		}
		suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
			return digest.Digest(MockDigest), nil
		}
		execAttrs := make(map[string]any)
		execAttrs[export.JobNameAttribute] = "test-job"
		execAttrs[export.UserNameAttribute] = "test-user"
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
		}

		repoCandidates := []int64{1}
		artCandidates := []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1, Digest: "digest1"}}}
		suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
			return repoCandidates, nil
		}
		suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
			return artCandidates, nil
		}
		suite.filterProcessor.ProcessLabelFilterFunc = func(_ context.Context, _ []int64, _ []*artifact.Artifact) ([]*artifact.Artifact, error) {
			return artCandidates, nil
		}

		criteria := export.Request{
			CVEIds:       "CVE-123",
			Labels:       []int64{1},
			Projects:     []int64{1},
			Repositories: "test-repo",
			Tags:         "test-tag",
		}
		criteriaMap := make(map[string]any)
		bytes, _ := json.Marshal(criteria)
		json.Unmarshal(bytes, &criteriaMap)
		params := job.Parameters{}
		params[export.JobModeKey] = export.JobModeExport
		params["JobId"] = JobId
		params["Request"] = criteriaMap

		ctx := &mockjobservice.MockJobContext{}

		err := suite.job.Run(ctx, params)
		suite.NoError(err)
		// Verify sysArtifactMgr.Create was called with matching record
		createCalls := suite.sysArtifactMgr.CreateCalls()
		suite.NotEmpty(createCalls)
		lastCreate := createCalls[len(createCalls)-1]
		suite.Equal("scandata_export_1000000", lastCreate.ArtifactRecord.Repository)
		suite.Equal(strings.ToLower(export.Vendor), lastCreate.ArtifactRecord.Vendor)
		suite.Equal(MockDigest, lastCreate.ArtifactRecord.Digest)

		// Verify UpdateExtraAttrs was called with correct digest
		updateCalls := suite.execMgr.UpdateExtraAttrsCalls()
		found := false
		for _, c := range updateCalls {
			if c.ID == ExecID && c.ExtraAttrs[export.DigestKey] == MockDigest {
				found = true
			}
		}
		suite.True(found, "Expected UpdateExtraAttrs call with correct digest")
		_, err = os.Stat("/tmp/scandata_export_1000000.csv")
		suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
	}

	{
		// Reset mocks for second sub-test
		suite.SetupTest()

		data := suite.createDataRecords(3)
		fetchCallCount := 0
		suite.exportMgr.FetchFunc = func(_ context.Context, p export.Params) ([]export.Data, error) {
			fetchCallCount++
			if fetchCallCount == 1 {
				assert.Equal(suite.T(), "CVE-123", p.CVEIds)
				return data, nil
			}
			return make([]export.Data, 0), nil
		}
		suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
			return digest.Digest(MockDigest), nil
		}
		execAttrs := make(map[string]any)
		execAttrs[export.JobNameAttribute] = "test-job"
		execAttrs[export.UserNameAttribute] = "test-user"
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
		}

		suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
			return []int64{1}, nil
		}
		suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
			return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
		}
		suite.filterProcessor.ProcessLabelFilterFunc = func(_ context.Context, _ []int64, _ []*artifact.Artifact) ([]*artifact.Artifact, error) {
			return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
		}

		criteria := export.Request{
			CVEIds:   "CVE-123",
			Labels:   []int64{1},
			Projects: []int64{1},
			Tags:     "test-tag",
		}
		criteriaMap := make(map[string]any)
		bytes, _ := json.Marshal(criteria)
		json.Unmarshal(bytes, &criteriaMap)
		params := job.Parameters{}
		params[export.JobModeKey] = export.JobModeExport
		params["JobId"] = JobId
		params["Request"] = criteriaMap

		ctx := &mockjobservice.MockJobContext{}

		err := suite.job.Run(ctx, params)
		suite.NoError(err)
		createCalls := suite.sysArtifactMgr.CreateCalls()
		suite.NotEmpty(createCalls)
		lastCreate := createCalls[len(createCalls)-1]
		suite.Equal("scandata_export_1000000", lastCreate.ArtifactRecord.Repository)
		suite.Equal(strings.ToLower(export.Vendor), lastCreate.ArtifactRecord.Vendor)
		suite.Equal(MockDigest, lastCreate.ArtifactRecord.Digest)

		updateCalls := suite.execMgr.UpdateExtraAttrsCalls()
		found := false
		for _, c := range updateCalls {
			if c.ID == ExecID && c.ExtraAttrs[export.DigestKey] == MockDigest {
				found = true
			}
		}
		suite.True(found)
		_, err = os.Stat("/tmp/scandata_export_1000000.csv")
		suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
	}
}

func (suite *ScanDataExportJobTestSuite) TestRunWithCriteriaForRepositoryIdFilter() {
	{
		suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
			return digest.Digest(MockDigest), nil
		}
		execAttrs := make(map[string]any)
		execAttrs[export.JobNameAttribute] = "test-job"
		execAttrs[export.UserNameAttribute] = "test-user"
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
		}

		suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
			return []int64{1}, errors.New("test error")
		}
		suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
			return []*artifact.Artifact{{Artifact: artpkg.Artifact{ID: 1}}}, nil
		}

		criteria := export.Request{
			CVEIds:       "CVE-123",
			Labels:       []int64{1},
			Projects:     []int64{1},
			Repositories: "test-repo",
			Tags:         "test-tag",
		}
		criteriaMap := make(map[string]any)
		bytes, _ := json.Marshal(criteria)
		json.Unmarshal(bytes, &criteriaMap)
		params := job.Parameters{}
		params[export.JobModeKey] = export.JobModeExport
		params["JobId"] = JobId
		params["Request"] = criteriaMap

		ctx := &mockjobservice.MockJobContext{}

		err := suite.job.Run(ctx, params)
		suite.Error(err)
		// sysArtifactMgr.Create should NOT have been called (beyond SetupTest)
		// exportMgr.Fetch should NOT have been called
		suite.Empty(suite.exportMgr.FetchCalls())
		_, err = os.Stat("/tmp/scandata_export_1000000.csv")
		suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
	}

	// empty list of repo ids
	{
		suite.SetupTest()

		suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
			return digest.Digest(MockDigest), nil
		}
		execAttrs := make(map[string]any)
		execAttrs[export.JobNameAttribute] = "test-job"
		execAttrs[export.UserNameAttribute] = "test-user"
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
		}

		suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
			return []int64{}, nil
		}
		suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
			return []*artifact.Artifact{}, nil
		}

		criteria := export.Request{
			CVEIds:       "CVE-123",
			Labels:       []int64{1},
			Projects:     []int64{1},
			Repositories: "test-repo",
			Tags:         "test-tag",
		}
		criteriaMap := make(map[string]any)
		bytes, _ := json.Marshal(criteria)
		json.Unmarshal(bytes, &criteriaMap)
		params := job.Parameters{}
		params[export.JobModeKey] = export.JobModeExport
		params["JobId"] = JobId
		params["Request"] = criteriaMap

		ctx := &mockjobservice.MockJobContext{}

		err := suite.job.Run(ctx, params)
		suite.NoError(err)
		// UpdateExtraAttrs should have been called
		suite.NotEmpty(suite.execMgr.UpdateExtraAttrsCalls())
		// exportMgr.Fetch should NOT have been called
		suite.Empty(suite.exportMgr.FetchCalls())
		_, err = os.Stat("/tmp/scandata_export_1000000.csv")
		suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
	}

}

func (suite *ScanDataExportJobTestSuite) TestRunWithCriteriaForRepositoryIdWithTagFilter() {
	{
		suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
			return digest.Digest(MockDigest), nil
		}
		execAttrs := make(map[string]any)
		execAttrs[export.JobNameAttribute] = "test-job"
		execAttrs[export.UserNameAttribute] = "test-user"
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
		}

		suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
			return []int64{1}, nil
		}
		suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
			return nil, errors.New("test error")
		}

		criteria := export.Request{
			CVEIds:       "CVE-123",
			Labels:       []int64{1},
			Projects:     []int64{1},
			Repositories: "test-repo",
			Tags:         "test-tag",
		}
		criteriaMap := make(map[string]any)
		bytes, _ := json.Marshal(criteria)
		json.Unmarshal(bytes, &criteriaMap)
		params := job.Parameters{}
		params[export.JobModeKey] = export.JobModeExport
		params["JobId"] = JobId
		params["Request"] = criteriaMap

		ctx := &mockjobservice.MockJobContext{}

		err := suite.job.Run(ctx, params)
		suite.Error(err)
		suite.Empty(suite.exportMgr.FetchCalls())
		_, err = os.Stat("/tmp/scandata_export_1000000.csv")
		suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
	}

	// empty list of repo ids after applying tag filters
	{
		suite.SetupTest()

		suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
			return digest.Digest(MockDigest), nil
		}
		execAttrs := make(map[string]any)
		execAttrs[export.JobNameAttribute] = "test-job"
		execAttrs[export.UserNameAttribute] = "test-user"
		suite.execMgr.GetFunc = func(_ context.Context, _ int64) (*task.Execution, error) {
			return &task.Execution{ID: ExecID, ExtraAttrs: execAttrs}, nil
		}

		suite.filterProcessor.ProcessRepositoryFilterFunc = func(_ context.Context, _ string, _ []int64) ([]int64, error) {
			return []int64{}, nil
		}
		suite.filterProcessor.ProcessTagFilterFunc = func(_ context.Context, _ string, _ []int64) ([]*artifact.Artifact, error) {
			return nil, nil
		}

		criteria := export.Request{
			CVEIds:       "CVE-123",
			Labels:       []int64{1},
			Projects:     []int64{1},
			Repositories: "test-repo",
			Tags:         "test-tag",
		}
		criteriaMap := make(map[string]any)
		bytes, _ := json.Marshal(criteria)
		json.Unmarshal(bytes, &criteriaMap)
		params := job.Parameters{}
		params[export.JobModeKey] = export.JobModeExport
		params["JobId"] = JobId
		params["Request"] = criteriaMap

		ctx := &mockjobservice.MockJobContext{}

		err := suite.job.Run(ctx, params)
		suite.NoError(err)
		suite.NotEmpty(suite.execMgr.UpdateExtraAttrsCalls())
		suite.Empty(suite.exportMgr.FetchCalls())
		_, err = os.Stat("/tmp/scandata_export_1000000.csv")
		suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
	}

}

func (suite *ScanDataExportJobTestSuite) TestExportDigestCalculationErrorsOut() {
	data := suite.createDataRecords(3)
	suite.exportMgr.FetchFunc = func(_ context.Context, _ export.Params) ([]export.Data, error) {
		return data, nil
	}
	suite.digestCalculator.CalculateFunc = func(_ string) (digest.Digest, error) {
		return digest.Digest(""), errors.New("test error")
	}
	params := job.Parameters{}
	params[export.JobModeKey] = export.JobModeExport
	params["JobId"] = JobId
	ctx := &mockjobservice.MockJobContext{}

	err := suite.job.Run(ctx, params)
	suite.Error(err)
	_, err = os.Stat("/tmp/scandata_export_1000000.csv")
	suite.Truef(os.IsNotExist(err), "Expected CSV file to be deleted")
}

func (suite *ScanDataExportJobTestSuite) TearDownTest() {
	path := fmt.Sprintf("/tmp/scandata_export_%v.csv", JobId)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return
	}
	err = os.Remove(path)
	suite.NoError(err)
}

func (suite *ScanDataExportJobTestSuite) createDataRecords(numRecs int) []export.Data {
	data := make([]export.Data, 0)
	for i := 1; i <= numRecs; i++ {
		dataRec := export.Data{
			ScannerName:    fmt.Sprintf("TestScanner%d", i),
			Repository:     fmt.Sprintf("Repository%d", i),
			ArtifactDigest: fmt.Sprintf("Digest%d", i),
			CVEId:          fmt.Sprintf("CVEId-%d", i),
			Package:        fmt.Sprintf("Package%d", i),
			Version:        fmt.Sprintf("Version%d", i),
			FixVersion:     fmt.Sprintf("FixVersion%d", i),
			Severity:       fmt.Sprintf("Severity%d", i),
			CWEIds:         "",
		}
		data = append(data, dataRec)
	}
	return data
}
func TestScanDataExportJobSuite(t *testing.T) {
	suite.Run(t, &ScanDataExportJobTestSuite{})
}
