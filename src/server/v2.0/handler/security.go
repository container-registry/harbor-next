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
	"strings"

	"github.com/go-openapi/runtime/middleware"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/pkg/scan/scanner"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	securityModel "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/securityhub"

	"github.com/goharbor/harbor/src/controller/securityhub"
	"github.com/goharbor/harbor/src/pkg/scan/dao/scan"
	secHubModel "github.com/goharbor/harbor/src/pkg/securityhub/model"
)

func newSecurityAPI() *securityAPI {
	return &securityAPI{
		controller: securityhub.Ctl,
	}
}

type securityAPI struct {
	BaseAPI
	controller securityhub.Controller
}

func (s *securityAPI) GetSecuritySummary(ctx context.Context,
	params securityModel.GetSecuritySummaryParams) middleware.Responder {
	if err := s.RequireSystemAccess(ctx, rbac.ActionRead, rbac.ResourceSecurityHub); err != nil {
		return s.SendError(ctx, err)
	}
	summary, err := s.controller.SecuritySummary(ctx, 0, securityhub.WithCVE(*params.WithDangerousCVE), securityhub.WithArtifact(*params.WithDangerousArtifact))
	if err != nil {
		return s.SendError(ctx, err)
	}
	sum := toSecuritySummaryModel(summary)
	return securityModel.NewGetSecuritySummaryOK().WithPayload(sum)
}

func toSecuritySummaryModel(summary *secHubModel.Summary) *models.SecuritySummary {
	return &models.SecuritySummary{
		CriticalCnt:        summary.CriticalCnt,
		HighCnt:            summary.HighCnt,
		MediumCnt:          summary.MediumCnt,
		LowCnt:             summary.LowCnt,
		NoneCnt:            summary.NoneCnt,
		UnknownCnt:         summary.UnknownCnt,
		FixableCnt:         summary.FixableCnt,
		TotalVuls:          summary.CriticalCnt + summary.HighCnt + summary.MediumCnt + summary.LowCnt + summary.NoneCnt + summary.UnknownCnt,
		TotalArtifact:      summary.TotalArtifactCnt,
		ScannedCnt:         summary.ScannedCnt,
		DangerousCves:      toDangerousCves(summary.DangerousCVEs),
		DangerousArtifacts: toDangerousArtifacts(summary.DangerousArtifacts),
	}
}
func toDangerousArtifacts(artifacts []*secHubModel.DangerousArtifact) []*models.DangerousArtifact {
	var result []*models.DangerousArtifact
	for _, artifact := range artifacts {
		result = append(result, &models.DangerousArtifact{
			ProjectID:      artifact.Project,
			RepositoryName: artifact.Repository,
			Digest:         artifact.Digest,
			CriticalCnt:    artifact.CriticalCnt,
			HighCnt:        artifact.HighCnt,
			MediumCnt:      artifact.MediumCnt,
		})
	}
	return result
}

func toDangerousCves(cves []*scan.VulnerabilityRecord) []*models.DangerousCVE {
	var result []*models.DangerousCVE
	for _, vul := range cves {
		result = append(result, &models.DangerousCVE{
			CVEID:       vul.CVEID,
			Package:     vul.Package,
			Version:     vul.PackageVersion,
			Severity:    vul.Severity,
			CvssScoreV3: *vul.CVE3Score,
		})
	}
	return result
}

func (s *securityAPI) ListVulnerabilities(ctx context.Context, params securityModel.ListVulnerabilitiesParams) middleware.Responder {
	if err := s.RequireSystemAccess(ctx, rbac.ActionList, rbac.ResourceSecurityHub); err != nil {
		return s.SendError(ctx, err)
	}
	query, err := s.BuildQuery(ctx, params.Q, nil, params.Page, params.PageSize)
	if err != nil {
		return s.SendError(ctx, err)
	}
	scannerUUID, err := scanner.Mgr.DefaultScannerUUID(ctx)
	if err != nil || len(scannerUUID) == 0 {
		return securityModel.NewListVulnerabilitiesOK().WithPayload([]*models.VulnerabilityItem{}).WithXTotalCount(0)
	}
	vuls, err := s.controller.ListVuls(ctx, scannerUUID, 0, *params.WithTag, query)
	if err != nil {
		return s.SendError(ctx, err)
	}
	cnt, known := totalFromPage(query.PageNumber, query.PageSize, len(vuls), *params.TuneCount)
	if !known {
		cnt, err = s.controller.CountVuls(ctx, scannerUUID, 0, *params.TuneCount, query)
		if err != nil {
			return s.SendError(ctx, err)
		}
	}
	link := s.Links(ctx, params.HTTPRequest.URL, linkTotal(cnt, query.PageNumber, query.PageSize, len(vuls)), query.PageNumber, query.PageSize).String()
	return securityModel.NewListVulnerabilitiesOK().WithPayload(toVulnerabilities(vuls)).WithLink(link).WithXTotalCount(cnt)
}

// totalFromPage derives the total from a page that is not full, so the count query, which joins the same
// tables as the list query, only runs when the total cannot be known from the page itself.
func totalFromPage(pageNumber, pageSize int64, rows int, tuneCount bool) (int64, bool) {
	if pageSize <= 0 || int64(rows) >= pageSize || (rows == 0 && pageNumber > 1) {
		return 0, false
	}
	total := int64(rows)
	if pageNumber > 1 {
		total += (pageNumber - 1) * pageSize
	}
	if tuneCount {
		return secHubModel.TunedTotal(total), true
	}
	return total, true
}

// linkTotal returns the total that Links needs to emit prev and next. When tune_count hides the total (-1),
// a full page implies a next page and any page after the first implies a previous one.
func linkTotal(total, pageNumber, pageSize int64, rows int) int64 {
	if total >= 0 {
		return total
	}
	if pageNumber < 1 {
		pageNumber = 1
	}
	known := (pageNumber-1)*pageSize + int64(rows)
	if int64(rows) >= pageSize {
		known++
	}
	return known
}

func toVulnerabilities(vuls []*secHubModel.VulnerabilityItem) []*models.VulnerabilityItem {
	result := make([]*models.VulnerabilityItem, 0)
	for _, item := range vuls {
		score := float32(0)
		if item.CVE3Score != nil {
			score = float32(*item.CVE3Score)
		}
		result = append(result, &models.VulnerabilityItem{
			ProjectID:      item.ProjectID,
			RepositoryName: item.RepositoryName,
			Digest:         item.Digest,
			CVEID:          item.CVEID,
			Severity:       item.Severity,
			Status:         item.Status,
			Package:        item.Package,
			Tags:           item.Tags,
			Version:        item.PackageVersion,
			FixedVersion:   item.Fix,
			Desc:           item.Description,
			CvssV3Score:    score,
			Links:          strings.Split(item.URLs, "|"),
		})
	}
	return result
}
