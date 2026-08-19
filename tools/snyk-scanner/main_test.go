package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVulnerabilityReportUsesHarborShape(t *testing.T) {
	s := &server{version: "test-version"}
	score := 7.5
	report := s.vulnerabilityReport(&artifact{
		Repository: "npm/vite",
		Digest:     "sha256:test",
		MimeType:   ociManifestType,
	}, []snykVulnerability{{
		ID:          "SNYK-JS-VITE-1",
		Title:       "test vuln",
		PackageName: "vite",
		Version:     "8.1.1",
		Severity:    "high",
		FixedIn:     []string{"8.1.2"},
		CVSSv3:      "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
		CVSSScore:   &score,
		URL:         "https://security.snyk.io/vuln/SNYK-JS-VITE-1",
		Identifiers: struct {
			CVE []string `json:"CVE"`
			CWE []string `json:"CWE"`
		}{
			CVE: []string{"CVE-2026-0001"},
			CWE: []string{"CWE-79"},
		},
	}})

	if report.GeneratedAt == "" {
		t.Fatal("GeneratedAt is empty")
	}
	if report.Artifact.Repository != "npm/vite" || report.Artifact.Digest != "sha256:test" || report.Artifact.MimeType != ociManifestType {
		t.Fatalf("unexpected artifact: %#v", report.Artifact)
	}
	if report.Scanner.Name != "Snyk" || report.Scanner.Vendor != "Snyk" || report.Scanner.Version != "test-version" {
		t.Fatalf("unexpected scanner: %#v", report.Scanner)
	}
	if report.Severity != "High" {
		t.Fatalf("severity = %q, want High", report.Severity)
	}
	if len(report.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities length = %d, want 1", len(report.Vulnerabilities))
	}
	vuln := report.Vulnerabilities[0]
	if vuln.ID != "CVE-2026-0001" || vuln.Package != "vite" || vuln.Version != "8.1.1" || vuln.FixVersion != "8.1.2" {
		t.Fatalf("unexpected vulnerability: %#v", vuln)
	}
	if vuln.Status != "" {
		t.Fatalf("status = %q, want empty", vuln.Status)
	}
	if vuln.CVSSDetails.ScoreV3 == nil || *vuln.CVSSDetails.ScoreV3 != score || vuln.CVSSDetails.VectorV3 == "" {
		t.Fatalf("unexpected CVSS details: %#v", vuln.CVSSDetails)
	}
	if len(vuln.CWEIDs) != 1 || vuln.CWEIDs[0] != "CWE-79" {
		t.Fatalf("unexpected CWE IDs: %#v", vuln.CWEIDs)
	}
}

func TestRawSBOMReportUsesHarborShape(t *testing.T) {
	report := rawSBOMReport{
		GeneratedAt: "2026-07-01T00:00:00Z",
		Scanner:     scanner{Name: "Snyk", Vendor: "Snyk", Version: "test-version"},
		MediaType:   spdxJSONMediaType,
		SBOM:        map[string]any{"SPDXID": "SPDXRef-DOCUMENT"},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded struct {
		GeneratedAt string         `json:"generated_at"`
		Scanner     scanner        `json:"scanner"`
		MediaType   string         `json:"media_type"`
		SBOM        map[string]any `json:"sbom"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if decoded.GeneratedAt == "" || decoded.Scanner.Name != "Snyk" || decoded.MediaType != spdxJSONMediaType {
		t.Fatalf("unexpected SBOM report: %#v", decoded)
	}
	if decoded.SBOM["SPDXID"] != "SPDXRef-DOCUMENT" {
		t.Fatalf("unexpected SBOM content: %#v", decoded.SBOM)
	}
}

func TestHandleReportUsesStoredReportType(t *testing.T) {
	s := &server{
		jobs: map[string]*scanJob{
			"test-id": {
				state:      "done",
				report:     `{"vulnerabilities":[]}`,
				reportType: vulnReportType,
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scan/test-id/report", nil)
	rec := httptest.NewRecorder()

	s.handleReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != vulnReportType {
		t.Fatalf("Content-Type = %q, want %q", got, vulnReportType)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"vulnerabilities":[]}` {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
