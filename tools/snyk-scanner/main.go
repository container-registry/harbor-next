package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	adapterMetaType      = "application/vnd.scanner.adapter.metadata+json; version=1.0"
	scanRequestType      = "application/vnd.scanner.adapter.scan.request+json; version=1.0"
	scanResponseType     = "application/vnd.scanner.adapter.scan.response+json; version=1.0"
	sbomReportType       = "application/vnd.security.sbom.report+json; version=1.0"
	vulnReportType       = "application/vnd.security.vulnerability.report; version=1.1"
	ociManifestType      = "application/vnd.oci.image.manifest.v1+json"
	dockerManifestType   = "application/vnd.docker.distribution.manifest.v2+json"
	npmArtifactType      = "application/vnd.harbor.npm.package.v1"
	npmPackageLayerType  = "application/vnd.npm.package.tgz"
	spdxJSONMediaType    = "application/spdx+json"
	defaultListenAddr    = ":8080"
	defaultCheckInterval = 5
	defaultScanTimeout   = 10 * time.Minute
	artifactTypeAnnot    = "org.opencontainers.artifactType"
	scannerTypeProperty  = "harbor.scanner-adapter/scanner-type"
	snykTokenConfigured  = "env.SNYK_TOKEN.configured"
	snykOrgProperty      = "env.SNYK_ORG"
	scanTypeSBOM         = "sbom"
	scanTypeVuln         = "vulnerability"
)

type server struct {
	jobs    map[string]*scanJob
	jobsMu  sync.RWMutex
	version string
	tmpDir  string
	timeout time.Duration
}

type scanJob struct {
	mu         sync.RWMutex
	state      string
	report     string
	reportType string
	err        error
}

type scanner struct {
	Name    string `json:"name"`
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
}

type capability struct {
	Type              string         `json:"type"`
	ConsumesMimeTypes []string       `json:"consumes_mime_types"`
	ProducesMimeTypes []string       `json:"produces_mime_types"`
	Attributes        map[string]any `json:"additional_attributes,omitempty"`
}

type metadata struct {
	Scanner      scanner           `json:"scanner"`
	Capabilities []capability      `json:"capabilities"`
	Properties   map[string]string `json:"properties"`
}

type registry struct {
	URL           string `json:"url"`
	Authorization string `json:"authorization"`
	Insecure      bool   `json:"insecure"`
}

type artifact struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Digest     string `json:"digest"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
}

type scanType struct {
	Type              string         `json:"type"`
	ProducesMimeTypes []string       `json:"produces_mime_types"`
	Parameters        map[string]any `json:"parameters"`
}

type scanRequest struct {
	Registry *registry   `json:"registry"`
	Artifact *artifact   `json:"artifact"`
	Types    []*scanType `json:"enabled_capabilities"`
}

type scanResponse struct {
	ID string `json:"id"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Annotations map[string]string `json:"annotations"`
}

type manifest struct {
	MediaType    string            `json:"mediaType"`
	ArtifactType string            `json:"artifactType"`
	Annotations  map[string]string `json:"annotations"`
	Layers       []descriptor      `json:"layers"`
}

type rawSBOMReport struct {
	GeneratedAt string  `json:"generated_at"`
	Scanner     scanner `json:"scanner"`
	MediaType   string  `json:"media_type"`
	SBOM        any     `json:"sbom"`
}

type vulnerabilityReport struct {
	GeneratedAt     string              `json:"generated_at"`
	Artifact        reportArtifact      `json:"artifact"`
	Scanner         scanner             `json:"scanner"`
	Severity        string              `json:"severity"`
	Vulnerabilities []vulnerabilityItem `json:"vulnerabilities"`
}

type reportArtifact struct {
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	MimeType   string `json:"mime_type"`
}

type vulnerabilityItem struct {
	ID               string         `json:"id"`
	Package          string         `json:"package"`
	Version          string         `json:"version"`
	FixVersion       string         `json:"fix_version"`
	Severity         string         `json:"severity"`
	Status           string         `json:"status"`
	Description      string         `json:"description"`
	Links            []string       `json:"links"`
	CVSSDetails      cvssDetails    `json:"preferred_cvss"`
	CWEIDs           []string       `json:"cwe_ids"`
	VendorAttributes map[string]any `json:"vendor_attributes"`
}

type cvssDetails struct {
	ScoreV3  *float64 `json:"score_v3,omitempty"`
	VectorV3 string   `json:"vector_v3,omitempty"`
}

type snykTestResult struct {
	Vulnerabilities []snykVulnerability `json:"vulnerabilities"`
}

type snykVulnerability struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	PackageName string   `json:"packageName"`
	Version     string   `json:"version"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
	FixedIn     []string `json:"fixedIn"`
	CVSSv3      string   `json:"CVSSv3"`
	CVSSScore   *float64 `json:"cvssScore"`
	URL         string   `json:"url"`
	References  []struct {
		URL string `json:"url"`
	} `json:"references"`
	Identifiers struct {
		CVE []string `json:"CVE"`
		CWE []string `json:"CWE"`
	} `json:"identifiers"`
}

func main() {
	addr := envOrDefault("SCANNER_API_SERVER_ADDR", defaultListenAddr)
	tmpDir := envOrDefault("SCANNER_SNYK_CACHE_DIR", os.TempDir())
	timeout := defaultScanTimeout
	if raw := os.Getenv("SCANNER_SNYK_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			log.Fatalf("invalid SCANNER_SNYK_TIMEOUT: %v", err)
		}
		timeout = parsed
	}

	s := &server{
		jobs:    map[string]*scanJob{},
		version: snykVersion(),
		tmpDir:  tmpDir,
		timeout: timeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/probe/healthy", s.handleHealth)
	mux.HandleFunc("/probe/ready", s.handleHealth)
	mux.HandleFunc("/api/v1/metadata", s.handleMetadata)
	mux.HandleFunc("/api/v1/scan", s.handleScan)
	mux.HandleFunc("/api/v1/scan/", s.handleReport)

	log.Printf("starting Snyk scanner adapter on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	if _, err := exec.LookPath("snyk"); err != nil {
		writeError(w, http.StatusServiceUnavailable, "snyk CLI not found")
		return
	}
	if os.Getenv("SNYK_TOKEN") == "" {
		writeError(w, http.StatusServiceUnavailable, "SNYK_TOKEN is not configured")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	props := map[string]string{
		scannerTypeProperty: "npm-dependency-scanner",
		snykTokenConfigured: fmt.Sprintf("%t", os.Getenv("SNYK_TOKEN") != ""),
	}
	if org := os.Getenv("SNYK_ORG"); org != "" {
		props[snykOrgProperty] = org
	}

	writeJSON(w, http.StatusOK, adapterMetaType, metadata{
		Scanner: s.scanner(),
		Capabilities: []capability{
			{
				Type: scanTypeVuln,
				ConsumesMimeTypes: []string{
					ociManifestType,
					dockerManifestType,
					npmArtifactType,
				},
				ProducesMimeTypes: []string{vulnReportType},
			},
			{
				Type: scanTypeSBOM,
				ConsumesMimeTypes: []string{
					ociManifestType,
					dockerManifestType,
					npmArtifactType,
				},
				ProducesMimeTypes: []string{sbomReportType},
				Attributes: map[string]any{
					"sbom_media_types": []string{spdxJSONMediaType},
				},
			},
		},
		Properties: props,
	})
}

func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode scan request: %v", err))
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.requestsSupportedScan() {
		writeError(w, http.StatusBadRequest, "Snyk scanner only supports vulnerability and SBOM scans")
		return
	}

	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job := &scanJob{state: "running"}
	s.jobsMu.Lock()
	s.jobs[id] = job
	s.jobsMu.Unlock()

	go s.runScan(id, job, &req)
	writeJSON(w, http.StatusAccepted, scanResponseType, scanResponse{ID: id})
}

func (s *server) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, ok := reportID(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "scan report not found")
		return
	}

	s.jobsMu.RLock()
	job := s.jobs[id]
	s.jobsMu.RUnlock()
	if job == nil {
		writeError(w, http.StatusNotFound, "scan report not found")
		return
	}

	job.mu.RLock()
	state, report, reportType, err := job.state, job.report, job.reportType, job.err
	job.mu.RUnlock()
	switch state {
	case "running":
		w.Header().Set("Refresh-After", fmt.Sprintf("%d", defaultCheckInterval))
		w.WriteHeader(http.StatusFound)
	case "failed":
		writeError(w, http.StatusInternalServerError, err.Error())
	case "done":
		w.Header().Set("Content-Type", reportType)
		_, _ = w.Write([]byte(report))
	default:
		writeError(w, http.StatusInternalServerError, "unknown scan job state")
	}
}

func (s *server) runScan(id string, job *scanJob, req *scanRequest) {
	report, reportType, err := s.scan(req)
	job.mu.Lock()
	defer job.mu.Unlock()
	if err != nil {
		log.Printf("scan %s failed: %v", id, err)
		job.state = "failed"
		job.err = err
		return
	}
	job.state = "done"
	job.report = report
	job.reportType = reportType
}

func (s *server) scan(req *scanRequest) (string, string, error) {
	if os.Getenv("SNYK_TOKEN") == "" {
		return "", "", errors.New("SNYK_TOKEN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	workDir, err := os.MkdirTemp(s.tmpDir, "snyk-npm-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	m, err := fetchManifest(ctx, req.Registry, req.Artifact)
	if err != nil {
		return "", "", err
	}
	if !isNPMManifest(m) {
		return "", "", errors.New("artifact is not an NPM package")
	}

	layer, ok := npmPackageLayer(m)
	if !ok {
		return "", "", errors.New("NPM package layer not found")
	}
	if err := downloadAndExtractLayer(ctx, req.Registry, req.Artifact.Repository, layer.Digest, workDir); err != nil {
		return "", "", err
	}

	packageDir, err := packageDir(workDir)
	if err != nil {
		return "", "", err
	}
	var report []byte
	reportType := sbomReportType
	switch req.scanType() {
	case scanTypeVuln:
		vulns, err := runSnykTest(ctx, packageDir)
		if err != nil {
			return "", "", err
		}
		report, err = json.Marshal(s.vulnerabilityReport(req.Artifact, vulns))
		if err != nil {
			return "", "", fmt.Errorf("marshal vulnerability report: %w", err)
		}
		reportType = vulnReportType
	default:
		sbom, err := runSnykSBOM(ctx, packageDir)
		if err != nil {
			return "", "", err
		}
		report, err = json.Marshal(rawSBOMReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Scanner:     s.scanner(),
			MediaType:   spdxJSONMediaType,
			SBOM:        sbom,
		})
		if err != nil {
			return "", "", fmt.Errorf("marshal SBOM report: %w", err)
		}
	}

	return string(report), reportType, nil
}

func (s *server) vulnerabilityReport(art *artifact, vulns []snykVulnerability) vulnerabilityReport {
	items := make([]vulnerabilityItem, 0, len(vulns))
	severity := "None"
	for _, v := range vulns {
		itemSeverity := normalizeSeverity(v.Severity)
		if severityCode(itemSeverity) > severityCode(severity) {
			severity = itemSeverity
		}
		items = append(items, vulnerabilityItem{
			ID:          firstNonEmpty(firstString(v.Identifiers.CVE), v.ID),
			Package:     v.PackageName,
			Version:     v.Version,
			FixVersion:  strings.Join(v.FixedIn, ","),
			Severity:    itemSeverity,
			Description: firstNonEmpty(v.Description, v.Title),
			Links:       snykLinks(v),
			CVSSDetails: cvssDetails{
				ScoreV3:  v.CVSSScore,
				VectorV3: v.CVSSv3,
			},
			CWEIDs: v.Identifiers.CWE,
			VendorAttributes: map[string]any{
				"snyk_id": v.ID,
			},
		})
	}

	return vulnerabilityReport{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Artifact:        vulnerabilityReportArtifact(art),
		Scanner:         s.scanner(),
		Severity:        severity,
		Vulnerabilities: items,
	}
}

func vulnerabilityReportArtifact(art *artifact) reportArtifact {
	if art == nil {
		return reportArtifact{}
	}
	return reportArtifact{
		Repository: art.Repository,
		Digest:     art.Digest,
		MimeType:   art.MimeType,
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func snykLinks(v snykVulnerability) []string {
	links := make([]string, 0, len(v.References)+1)
	if v.URL != "" {
		links = append(links, v.URL)
	}
	for _, ref := range v.References {
		if ref.URL != "" {
			links = append(links, ref.URL)
		}
	}
	return links
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "Critical"
	case "high":
		return "High"
	case "medium", "moderate":
		return "Medium"
	case "low":
		return "Low"
	default:
		return "Unknown"
	}
}

func severityCode(severity string) int {
	switch severity {
	case "Critical":
		return 5
	case "High":
		return 4
	case "Medium":
		return 3
	case "Low":
		return 2
	case "Unknown":
		return 1
	default:
		return 0
	}
}

func (r *scanRequest) scanType() string {
	for _, t := range r.Types {
		if t != nil && t.Type != "" {
			return t.Type
		}
	}
	return scanTypeVuln
}

func (r *scanRequest) requestsSupportedScan() bool {
	switch r.scanType() {
	case scanTypeSBOM, scanTypeVuln:
		return true
	default:
		return false
	}
}

func (s *server) scanner() scanner {
	return scanner{Name: "Snyk", Vendor: "Snyk", Version: s.version}
}

func (r *scanRequest) validate() error {
	if r.Registry == nil || r.Registry.URL == "" {
		return errors.New("scan request: invalid registry")
	}
	if r.Artifact == nil || r.Artifact.Repository == "" || r.Artifact.Digest == "" || r.Artifact.MimeType == "" {
		return errors.New("scan request: invalid artifact")
	}
	return nil
}

func fetchManifest(ctx context.Context, reg *registry, art *artifact) (*manifest, error) {
	req, err := newRegistryRequest(ctx, http.MethodGet, reg, art.Repository, "manifests", art.Digest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", strings.Join([]string{ociManifestType, dockerManifestType}, ", "))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("fetch manifest", resp)
	}

	var m manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &m, nil
}

func downloadAndExtractLayer(ctx context.Context, reg *registry, repo, digest, dest string) error {
	req, err := newRegistryRequest(ctx, http.MethodGet, reg, repo, "blobs", digest, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download package layer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError("download package layer", resp)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("read package gzip: %w", err)
	}
	defer gz.Close()
	return unpackTar(gz, dest)
}

func newRegistryRequest(ctx context.Context, method string, reg *registry, repo, resource, ref string, body io.Reader) (*http.Request, error) {
	base := strings.TrimRight(reg.URL, "/")
	u := fmt.Sprintf("%s/v2/%s/%s/%s", base, escapePath(repo), resource, url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, fmt.Errorf("create registry request: %w", err)
	}
	if reg.Authorization != "" {
		req.Header.Set("Authorization", reg.Authorization)
	}
	return req, nil
}

func isNPMManifest(m *manifest) bool {
	if m.ArtifactType == npmArtifactType || m.Annotations[artifactTypeAnnot] == npmArtifactType {
		return true
	}
	_, ok := npmPackageLayer(m)
	return ok
}

func npmPackageLayer(m *manifest) (descriptor, bool) {
	for _, l := range m.Layers {
		if l.MediaType == npmPackageLayerType {
			return l, true
		}
	}
	return descriptor{}, false
}

func packageDir(root string) (string, error) {
	npmDir := filepath.Join(root, "package")
	if _, err := os.Stat(filepath.Join(npmDir, "package.json")); err == nil {
		return npmDir, nil
	}
	if _, err := os.Stat(filepath.Join(root, "package.json")); err == nil {
		return root, nil
	}
	return "", errors.New("package.json not found in NPM package")
}

func runSnykTest(ctx context.Context, dir string) ([]snykVulnerability, error) {
	args := []string{
		"test",
		"--json",
		"--file=package.json",
		"--package-manager=npm",
		"--strict-out-of-sync=false",
	}
	if org := os.Getenv("SNYK_ORG"); org != "" {
		args = append(args, "--org="+org)
	}
	args = append(args, dir)

	cmd := exec.CommandContext(ctx, "snyk", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("run snyk test: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}

	var result snykTestResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("decode Snyk vulnerability report: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return result.Vulnerabilities, nil
}

func runSnykSBOM(ctx context.Context, dir string) (any, error) {
	out := filepath.Join(dir, "snyk-spdx.json")
	args := []string{
		"sbom",
		"--format=spdx2.3+json",
		"--json-file-output=" + out,
		"--file=package.json",
		"--strict-out-of-sync=false",
	}
	if org := os.Getenv("SNYK_ORG"); org != "" {
		args = append(args, "--org="+org)
	}
	args = append(args, dir)

	cmd := exec.CommandContext(ctx, "snyk", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run snyk sbom: %w: %s", err, strings.TrimSpace(string(output)))
	}

	f, err := os.Open(out)
	if err != nil {
		return nil, fmt.Errorf("open Snyk SBOM: %w", err)
	}
	defer f.Close()

	var sbom any
	if err := json.NewDecoder(f).Decode(&sbom); err != nil {
		return nil, fmt.Errorf("decode Snyk SBOM: %w", err)
	}
	return sbom, nil
}

func unpackTar(r io.Reader, root string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve extraction root: %w", err)
	}

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read package tar: %w", err)
		}

		name := filepath.Clean(header.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe package path %q", header.Name)
		}

		dest := filepath.Join(rootAbs, name)
		if !strings.HasPrefix(dest, rootAbs+string(os.PathSeparator)) && dest != rootAbs {
			return fmt.Errorf("unsafe package path %q", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}

func reportID(p string) (string, bool) {
	const prefix = "/api/v1/scan/"
	const suffix = "/report"
	if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(p, prefix), suffix)
	return id, id != ""
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate scan ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func snykVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "snyk", "--version").Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(output))
}

func statusError(op string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("%s: unexpected status %d: %s", op, resp.StatusCode, strings.TrimSpace(string(body)))
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	var resp errorResponse
	resp.Error.Message = message
	writeJSON(w, status, "application/json", resp)
}
