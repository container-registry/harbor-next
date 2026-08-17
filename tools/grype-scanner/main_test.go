package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestVulnerabilityReportFiltersToRPM(t *testing.T) {
	s := &server{version: "0.115.0"}
	score := 10.0
	report := s.vulnerabilityReport(&artifact{
		Repository: "bluefin/bluefin-bootc",
		Digest:     "sha256:test",
		MimeType:   ociManifestType,
	}, &grypeReport{
		Distro: grypeDistro{Name: "fedora", Version: "44"},
		Matches: []grypeMatch{
			{
				Vulnerability: grypeVulnerability{
					ID:          "CVE-2025-10230",
					DataSource:  "https://bodhi.fedoraproject.org/updates/FEDORA-2025-3ddbddd7e2",
					Namespace:   "fedora:distro:fedora:44",
					Severity:    "Critical",
					Description: "critical rpm vuln",
					URLs:        []string{"https://nvd.nist.gov/vuln/detail/CVE-2025-10230"},
					CVSS: []grypeCVSS{{
						Version: "3.1",
						Vector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H",
						Metrics: struct {
							BaseScore *float64 `json:"baseScore"`
						}{BaseScore: &score},
					}},
					CWEs: []grypeCWE{{CWE: "CWE-78"}},
					Fix:  grypeFix{Versions: []string{"2:4.23.2-1.fc44"}, State: "fixed"},
				},
				Artifact: grypeArtifact{
					Name:    "samba",
					Version: "4.24.3-1.fc44",
					Type:    "rpm",
					PURL:    "pkg:rpm/fedora/samba@4.24.3-1.fc44?arch=x86_64&distro=fedora-44",
				},
				MatchDetails: []grypeMatchDetail{{
					Matcher: "rpm-matcher",
					Found: struct {
						VersionConstraint string `json:"versionConstraint"`
					}{VersionConstraint: "< 2:4.23.2-1.fc44 (rpm)"},
					Fix: struct {
						SuggestedVersion string `json:"suggestedVersion"`
					}{SuggestedVersion: "2:4.23.2-1.fc44"},
				}},
			},
			{
				Vulnerability: grypeVulnerability{ID: "GHSA-test", Severity: "High"},
				Artifact:      grypeArtifact{Name: "golang.org/x/net", Version: "v0.1.0", Type: "go-module"},
			},
		},
	})

	if report.Scanner.Name != "Grype" || report.Scanner.Vendor != "Anchore" || report.Scanner.Version != "0.115.0" {
		t.Fatalf("unexpected scanner: %#v", report.Scanner)
	}
	if report.Severity != "Critical" {
		t.Fatalf("severity = %q, want Critical", report.Severity)
	}
	if len(report.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities length = %d, want 1", len(report.Vulnerabilities))
	}
	vuln := report.Vulnerabilities[0]
	if vuln.ID != "CVE-2025-10230" || vuln.Package != "samba" || vuln.Version != "4.24.3-1.fc44" {
		t.Fatalf("unexpected vulnerability: %#v", vuln)
	}
	if vuln.FixVersion != "2:4.23.2-1.fc44" || vuln.Status != "fixed" {
		t.Fatalf("unexpected fix info: %#v", vuln)
	}
	if vuln.CVSSDetails.ScoreV3 == nil || *vuln.CVSSDetails.ScoreV3 != score || vuln.CVSSDetails.VectorV3 == "" {
		t.Fatalf("unexpected CVSS: %#v", vuln.CVSSDetails)
	}
	if len(vuln.CWEIDs) != 1 || vuln.CWEIDs[0] != "CWE-78" {
		t.Fatalf("unexpected CWE IDs: %#v", vuln.CWEIDs)
	}
	if vuln.VendorAttributes["package_type"] != "rpm" || vuln.VendorAttributes["distro"] != "fedora 44" {
		t.Fatalf("unexpected vendor attrs: %#v", vuln.VendorAttributes)
	}
}

func TestVulnerabilityReportIncludesALPM(t *testing.T) {
	s := &server{version: "0.115.0"}
	report := s.vulnerabilityReport(&artifact{Repository: "arch-bootc/arch-bootc", Digest: "sha256:test"}, &grypeReport{
		Distro: grypeDistro{Name: "archlinux", Version: "rolling"},
		Matches: []grypeMatch{versionedGrypeMatch(
			"CVE-2026-1234",
			"openssl",
			"3.5.0-1",
			"alpm",
			"< 3.5.1-1 (unknown)",
		)},
	})
	if len(report.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities length = %d, want 1", len(report.Vulnerabilities))
	}
	if got := report.Vulnerabilities[0].VendorAttributes["package_type"]; got != "alpm" {
		t.Fatalf("package_type = %#v, want alpm", got)
	}
}

func TestVulnerabilityReportDeduplicatesCVEs(t *testing.T) {
	s := &server{version: "0.115.0"}
	report := s.vulnerabilityReport(&artifact{Repository: "bluefin/bluefin-lts", Digest: "sha256:test"}, &grypeReport{
		Distro: grypeDistro{Name: "centos", Version: "10"},
		Matches: []grypeMatch{
			versionedGrypeMatch("CVE-2026-1234", "kernel", "6.12.0-248.el10", "rpm", "< 6.12.0-249.el10 (rpm)"),
			versionedGrypeMatch("CVE-2026-1234", "kernel-modules", "6.12.0-248.el10", "rpm", "< 6.12.0-249.el10 (rpm)"),
			versionedGrypeMatch("CVE-2026-5678", "openssl", "3.5.1-1.el10", "rpm", "< 3.5.2-1.el10 (rpm)"),
		},
	})

	if len(report.Vulnerabilities) != 2 {
		t.Fatalf("vulnerabilities length = %d, want 2", len(report.Vulnerabilities))
	}
	if got := report.Vulnerabilities[0]; got.ID != "CVE-2026-1234" || got.Package != "kernel" {
		t.Fatalf("first vulnerability = %#v, want first CVE match", got)
	}
	if got := report.Vulnerabilities[1].ID; got != "CVE-2026-5678" {
		t.Fatalf("second vulnerability ID = %q, want CVE-2026-5678", got)
	}
}

func TestVulnerabilityReportRequiresAffectedVersion(t *testing.T) {
	s := &server{version: "0.115.0"}
	report := s.vulnerabilityReport(&artifact{Repository: "almalinux/almalinux-bootc", Digest: "sha256:test"}, &grypeReport{
		Distro: grypeDistro{Name: "almalinux", Version: "10"},
		Matches: []grypeMatch{
			versionedGrypeMatch("CVE-2026-1234", "openssl", "3.5.1-1.el10", "rpm", "< 3.5.2-1.el10 (rpm)"),
			versionedGrypeMatch("CVE-2026-5678", "kernel", "6.12.0-248.el10", "rpm", "none (unknown)"),
			{
				Vulnerability: grypeVulnerability{ID: "CVE-2026-9012", Severity: "High"},
				Artifact:      grypeArtifact{Name: "systemd", Version: "257-10.el10", Type: "rpm"},
			},
		},
	})

	if len(report.Vulnerabilities) != 1 {
		t.Fatalf("vulnerabilities length = %d, want 1", len(report.Vulnerabilities))
	}
	if got := report.Vulnerabilities[0]; got.ID != "CVE-2026-1234" || got.Version != "3.5.1-1.el10" {
		t.Fatalf("vulnerability = %#v, want version-matched CVE-2026-1234", got)
	}
}

func TestGrypeArgsUseCVEAliases(t *testing.T) {
	args := strings.Join(grypeArgs("/work/sbom.json"), "\n")
	if !strings.Contains(args, "--by-cve") {
		t.Fatalf("grype args missing --by-cve:\n%s", args)
	}
}

func versionedGrypeMatch(id, name, version, packageType, constraint string) grypeMatch {
	detail := grypeMatchDetail{Matcher: packageType + "-matcher"}
	detail.Found.VersionConstraint = constraint
	return grypeMatch{
		Vulnerability: grypeVulnerability{ID: id, Severity: "High"},
		Artifact:      grypeArtifact{Name: name, Version: version, Type: packageType},
		MatchDetails:  []grypeMatchDetail{detail},
	}
}

func TestRegistryImageRef(t *testing.T) {
	ref, err := registryImageRef(&registry{URL: "http://registry:5000"}, &artifact{
		Repository: "bluefin/bluefin-bootc",
		Digest:     "sha256:abc",
	})
	if err != nil {
		t.Fatalf("registryImageRef: %v", err)
	}
	if ref != "registry:5000/bluefin/bluefin-bootc@sha256:abc" {
		t.Fatalf("ref = %q", ref)
	}
}

func TestRegistryAuthEnvBasic(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("robot:secret"))
	env := registryAuthEnv("registry:5000", "Basic "+encoded)
	want := []string{
		"SYFT_REGISTRY_AUTH_AUTHORITY=registry:5000",
		"SYFT_REGISTRY_AUTH_USERNAME=robot",
		"SYFT_REGISTRY_AUTH_PASSWORD=secret",
	}
	if strings.Join(env, "\n") != strings.Join(want, "\n") {
		t.Fatalf("env = %#v, want %#v", env, want)
	}
}

func TestNormalizeSBOMBlobAcceptsRawJSON(t *testing.T) {
	data, err := normalizeSBOMBlob([]byte("  {\"SPDXID\":\"SPDXRef-DOCUMENT\"}\n"))
	if err != nil {
		t.Fatalf("normalizeSBOMBlob: %v", err)
	}
	if string(data) != `{"SPDXID":"SPDXRef-DOCUMENT"}` {
		t.Fatalf("data = %q", string(data))
	}
}

func TestSelectSBOMDescriptorPrefersHarborSBOM(t *testing.T) {
	got, ok := selectSBOMDescriptor([]descriptor{
		{ArtifactType: attachedSPDXType, Digest: "sha256:attached"},
		{ArtifactType: harborSBOMType, Digest: "sha256:old-harbor"},
		{ArtifactType: harborSBOMType, Digest: "sha256:new-harbor"},
	})
	if !ok || got.Digest != "sha256:new-harbor" {
		t.Fatalf("selectSBOMDescriptor() = %#v, %v; want newest Harbor SBOM", got, ok)
	}
}

func TestSelectSBOMDescriptorAcceptsAttachedSPDX(t *testing.T) {
	got, ok := selectSBOMDescriptor([]descriptor{{ArtifactType: attachedSPDXType, Digest: "sha256:attached"}})
	if !ok || got.Digest != "sha256:attached" {
		t.Fatalf("selectSBOMDescriptor() = %#v, %v; want attached SPDX", got, ok)
	}
}

func TestSelectAttachedSBOMDescriptorPrefersStandardAttachment(t *testing.T) {
	got, ok := selectAttachedSBOMDescriptor([]descriptor{
		{ArtifactType: attachedSPDXType, Digest: "sha256:attached"},
		{ArtifactType: harborSBOMType, Digest: "sha256:harbor"},
	})
	if !ok || got.Digest != "sha256:attached" {
		t.Fatalf("selectAttachedSBOMDescriptor() = %#v, %v; want standard attachment", got, ok)
	}
}

func TestDecodeSPDXRejectsMislabeledSyftJSON(t *testing.T) {
	path := t.TempDir() + "/sbom.json"
	if err := os.WriteFile(path, []byte(`{"artifacts":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSPDX(path); err == nil {
		t.Fatal("decodeSPDX() returned nil error for Syft JSON")
	}
}

func TestFilterPackageDBArtifactsRemovesInferredAndLanguagePackages(t *testing.T) {
	dir := t.TempDir()
	source := dir + "/sbom.json"
	data := `{"artifacts":[` +
		`{"name":"bash","version":"5.3-1.fc44","type":"rpm","foundBy":"rpm-db-cataloger"},` +
		`{"name":"bash","type":"rpm","foundBy":"elf-binary-package-cataloger"},` +
		`{"name":"example","type":"go-module","foundBy":"go-module-binary-cataloger"}` +
		`]}`
	if err := os.WriteFile(source, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := filterPackageDBArtifacts(dir, source)
	if err != nil {
		t.Fatalf("filterPackageDBArtifacts: %v", err)
	}
	filtered, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Distro    syftDistro `json:"distro"`
		Artifacts []struct {
			Name    string `json:"name"`
			FoundBy string `json:"foundBy"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(filtered, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Artifacts) != 1 || document.Artifacts[0].Name != "bash" || document.Artifacts[0].FoundBy != "rpm-db-cataloger" {
		t.Fatalf("filtered artifacts = %#v", document.Artifacts)
	}
	if document.Distro.ID != "fedora" || document.Distro.VersionID != "44" {
		t.Fatalf("filtered distro = %#v", document.Distro)
	}
}

func TestFilterSPDXPackageManagerInventoryPrefersALPM(t *testing.T) {
	dir := t.TempDir()
	source := dir + "/sbom.json"
	data := `{"spdxVersion":"SPDX-2.3","SPDXID":"SPDXRef-DOCUMENT","packages":[` +
		`{"name":"bash","externalRefs":[{"referenceLocator":"pkg:alpm/arch/bash@5.3"}]},` +
		`{"name":"python","externalRefs":[{"referenceLocator":"pkg:generic/python@3.13"}]}` +
		`]}`
	if err := os.WriteFile(source, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := filterSPDXPackageManagerInventory(dir, source)
	if err != nil {
		t.Fatalf("filterSPDXPackageManagerInventory: %v", err)
	}
	filtered, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(filtered, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Packages) != 1 || document.Packages[0].Name != "bash" {
		t.Fatalf("filtered packages = %#v", document.Packages)
	}
}

func TestMostCommonVersion(t *testing.T) {
	if got := mostCommonVersion(map[string]int{"43": 2, "44": 10}); got != "44" {
		t.Fatalf("mostCommonVersion() = %q, want 44", got)
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

func TestMetadataAdvertisesSBOMAndVulnerability(t *testing.T) {
	s := &server{version: "0.115.0"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metadata", nil)
	rec := httptest.NewRecorder()

	s.handleMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var meta metadata
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	got := map[string][]string{}
	for _, capability := range meta.Capabilities {
		got[capability.Type] = capability.ProducesMimeTypes
	}
	if strings.Join(got[scanTypeVuln], ",") != vulnReportType {
		t.Fatalf("vulnerability produces = %#v, want %q", got[scanTypeVuln], vulnReportType)
	}
	if strings.Join(got[scanTypeSBOM], ",") != sbomReportType {
		t.Fatalf("SBOM produces = %#v, want %q", got[scanTypeSBOM], sbomReportType)
	}
}

func TestScanRequestSupportsSBOMAndVulnerability(t *testing.T) {
	tests := []struct {
		name     string
		scanType string
		want     bool
	}{
		{name: "vulnerability", scanType: scanTypeVuln, want: true},
		{name: "sbom", scanType: scanTypeSBOM, want: true},
		{name: "unsupported", scanType: "license", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &scanRequest{Types: []*scanType{{Type: tt.scanType}}}
			if got := req.requestsSupportedScan(); got != tt.want {
				t.Fatalf("requestsSupportedScan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRawSBOMReportUsesHarborShape(t *testing.T) {
	report := rawSBOMReport{
		GeneratedAt: "2026-07-09T00:00:00Z",
		Scanner:     scanner{Name: "Grype", Vendor: "Anchore", Version: "0.115.0"},
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
	if decoded.GeneratedAt == "" || decoded.Scanner.Name != "Grype" || decoded.MediaType != spdxJSONMediaType {
		t.Fatalf("unexpected SBOM report: %#v", decoded)
	}
}

func TestSyftArgsLimitCatalogingToOSPackages(t *testing.T) {
	args := strings.Join(syftArgs("registry.example/repo@sha256:abc", "spdx-json", "/work/sbom.json"), "\n")
	for _, want := range []string{
		"rpm-db-cataloger",
		"alpm-db-cataloger",
		"dpkg-db-cataloger",
		"apk-db-cataloger",
		"-file",
		"--parallelism\n1",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("syft args missing %q:\n%s", want, args)
		}
	}
}

func TestSBOMScanUsesScanSlot(t *testing.T) {
	t.Setenv("SCANNER_GRYPE_CACHE_DIR", t.TempDir())
	s := &server{
		version: "0.115.0",
		tmpDir:  t.TempDir(),
		timeout: time.Millisecond,
		scanSem: make(chan struct{}, 1),
	}
	s.scanSem <- struct{}{}

	_, _, err := s.scan(&scanRequest{Types: []*scanType{{Type: scanTypeSBOM}}})
	if err == nil {
		t.Fatal("scan returned nil error, want scan slot timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "wait for Grype scan slot") {
		t.Fatalf("error = %v, want scan slot timeout", err)
	}
}
