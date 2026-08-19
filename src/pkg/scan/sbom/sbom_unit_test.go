package sbom

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/controller/artifact"
	art "github.com/goharbor/harbor/src/pkg/artifact"
	v1 "github.com/goharbor/harbor/src/pkg/scan/rest/v1"
	sbomModel "github.com/goharbor/harbor/src/pkg/scan/sbom/model"
	"github.com/goharbor/harbor/src/pkg/task"
	"github.com/goharbor/harbor/src/testing/mock"
	sbomTesting "github.com/goharbor/harbor/src/testing/pkg/scan/sbom"
	taskTesting "github.com/goharbor/harbor/src/testing/pkg/task"
)

func TestGetSummaryPrefersSuccessfulSBOMReportAcrossScanners(t *testing.T) {
	sbomManager := &sbomTesting.Manager{}
	taskManager := &taskTesting.Manager{}
	handler := &scanHandler{
		SBOMMgrFunc: func() Manager { return sbomManager },
		TaskMgrFunc: func() task.Manager {
			return taskManager
		},
	}
	artifact := &artifact.Artifact{Artifact: art.Artifact{ID: 1, ProjectID: 2}}
	reports := []*sbomModel.Report{
		{
			UUID:             "old-grype-report",
			RegistrationUUID: "grype",
			MimeType:         v1.MimeTypeSBOMReport,
			MediaType:        sbomMediaTypeSpdx,
			ReportSummary:    `{"scan_status":"Error","report_id":"old-grype-report"}`,
		},
		{
			UUID:             "new-trivy-report",
			RegistrationUUID: "trivy",
			MimeType:         v1.MimeTypeSBOMReport,
			MediaType:        sbomMediaTypeSpdx,
			ReportSummary:    `{"scan_status":"Success","sbom_digest":"sha256:sbom"}`,
		},
	}
	sbomManager.On("GetBy", mock.Anything, int64(1), "", v1.MimeTypeSBOMReport, sbomMediaTypeSpdx).Return(reports, nil).Once()

	summary, err := handler.GetSummary(context.Background(), artifact, []string{v1.MimeTypeSBOMReport})
	require.NoError(t, err)
	require.Equal(t, "Success", summary[sbomModel.ScanStatus])
	require.Equal(t, "sha256:sbom", summary[sbomModel.SBOMDigest])
	require.Equal(t, "new-trivy-report", summary[sbomModel.ReportID])
}
