package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	adapterMetaType       = "application/vnd.scanner.adapter.metadata+json; version=1.0"
	scanRequestType       = "application/vnd.scanner.adapter.scan.request+json; version=1.0"
	scanResponseType      = "application/vnd.scanner.adapter.scan.response+json; version=1.0"
	sbomReportType        = "application/vnd.security.sbom.report+json; version=1.0"
	vulnReportType        = "application/vnd.security.vulnerability.report; version=1.1"
	harborSBOMType        = "application/vnd.goharbor.harbor.sbom.v1"
	attachedSPDXType      = "application/vnd.spdx+json"
	ociManifestType       = "application/vnd.oci.image.manifest.v1+json"
	ociIndexType          = "application/vnd.oci.image.index.v1+json"
	dockerManifestType    = "application/vnd.docker.distribution.manifest.v2+json"
	spdxJSONMediaType     = "application/spdx+json"
	defaultListenAddr     = ":8080"
	defaultCheckInterval  = 5
	defaultScanTimeout    = 30 * time.Minute
	scannerTypeProperty   = "harbor.scanner-adapter/scanner-type"
	scanTypeSBOM          = "sbom"
	scanTypeVuln          = "vulnerability"
	packageTypeRPM        = "rpm"
	defaultSyftCatalogers = "rpm-db-cataloger,alpm-db-cataloger,dpkg-db-cataloger,apk-db-cataloger"
)

type server struct {
	jobs    map[string]*scanJob
	jobsMu  sync.RWMutex
	version string
	tmpDir  string
	timeout time.Duration
	scanSem chan struct{}
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
	MediaType    string            `json:"mediaType"`
	Digest       string            `json:"digest"`
	Size         int64             `json:"size"`
	ArtifactType string            `json:"artifactType"`
	Annotations  map[string]string `json:"annotations"`
}

type referrersIndex struct {
	MediaType string       `json:"mediaType"`
	Manifests []descriptor `json:"manifests"`
}

type manifest struct {
	MediaType    string       `json:"mediaType"`
	ArtifactType string       `json:"artifactType"`
	Config       descriptor   `json:"config"`
	Layers       []descriptor `json:"layers"`
}

type imageConfig struct {
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"config"`
	History []imageHistory `json:"history"`
}

type imageHistory struct {
	CreatedBy  string `json:"created_by"`
	Comment    string `json:"comment"`
	EmptyLayer bool   `json:"empty_layer"`
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

type grypeReport struct {
	Matches    []grypeMatch `json:"matches"`
	Distro     grypeDistro  `json:"distro"`
	Descriptor struct {
		Version string `json:"version"`
	} `json:"descriptor"`
}

type grypeDistro struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type grypeMatch struct {
	Vulnerability          grypeVulnerability   `json:"vulnerability"`
	RelatedVulnerabilities []grypeVulnerability `json:"relatedVulnerabilities"`
	Artifact               grypeArtifact        `json:"artifact"`
	MatchDetails           []grypeMatchDetail   `json:"matchDetails"`
}

type grypeVulnerability struct {
	ID          string          `json:"id"`
	DataSource  string          `json:"dataSource"`
	Namespace   string          `json:"namespace"`
	Severity    string          `json:"severity"`
	URLs        []string        `json:"urls"`
	Description string          `json:"description"`
	CVSS        []grypeCVSS     `json:"cvss"`
	CWEs        []grypeCWE      `json:"cwes"`
	Fix         grypeFix        `json:"fix"`
	Advisories  []grypeAdvisory `json:"advisories"`
	Risk        float64         `json:"risk"`
}

type grypeCVSS struct {
	Version string `json:"version"`
	Vector  string `json:"vector"`
	Metrics struct {
		BaseScore *float64 `json:"baseScore"`
	} `json:"metrics"`
}

type grypeCWE struct {
	CWE string `json:"cwe"`
}

type grypeFix struct {
	Versions []string `json:"versions"`
	State    string   `json:"state"`
}

type grypeAdvisory struct {
	ID   string `json:"id"`
	Link string `json:"link"`
}

type grypeArtifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	PURL    string `json:"purl"`
}

type grypeMatchDetail struct {
	Matcher string `json:"matcher"`
	Found   struct {
		VersionConstraint string `json:"versionConstraint"`
	} `json:"found"`
	Fix struct {
		SuggestedVersion string `json:"suggestedVersion"`
	} `json:"fix"`
}

func main() {
	addr := envOrDefault("SCANNER_API_SERVER_ADDR", defaultListenAddr)
	tmpDir := envOrDefault("SCANNER_GRYPE_CACHE_DIR", os.TempDir())
	timeout := defaultScanTimeout
	if raw := os.Getenv("SCANNER_GRYPE_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			log.Fatalf("invalid SCANNER_GRYPE_TIMEOUT: %v", err)
		}
		timeout = parsed
	}
	maxConcurrency := envIntOrDefault("SCANNER_GRYPE_MAX_CONCURRENCY", 1)
	if maxConcurrency < 1 {
		log.Fatalf("SCANNER_GRYPE_MAX_CONCURRENCY must be greater than 0")
	}

	s := &server{
		jobs:    map[string]*scanJob{},
		version: grypeVersion(),
		tmpDir:  tmpDir,
		timeout: timeout,
		scanSem: make(chan struct{}, maxConcurrency),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/probe/healthy", s.handleHealth)
	mux.HandleFunc("/probe/ready", s.handleHealth)
	mux.HandleFunc("/api/v1/metadata", s.handleMetadata)
	mux.HandleFunc("/api/v1/scan", s.handleScan)
	mux.HandleFunc("/api/v1/scan/", s.handleReport)

	log.Printf("starting Grype scanner adapter on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	if _, err := exec.LookPath("syft"); err != nil {
		writeError(w, http.StatusServiceUnavailable, "syft CLI not found")
		return
	}
	if _, err := exec.LookPath("grype"); err != nil {
		writeError(w, http.StatusServiceUnavailable, "grype CLI not found")
		return
	}
	if _, err := exec.LookPath("zstd"); err != nil {
		writeError(w, http.StatusServiceUnavailable, "zstd CLI not found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, adapterMetaType, metadata{
		Scanner: s.scanner(),
		Capabilities: []capability{
			{
				Type: scanTypeVuln,
				ConsumesMimeTypes: []string{
					ociManifestType,
					dockerManifestType,
				},
				ProducesMimeTypes: []string{vulnReportType},
			},
			{
				Type: scanTypeSBOM,
				ConsumesMimeTypes: []string{
					ociManifestType,
					dockerManifestType,
				},
				ProducesMimeTypes: []string{sbomReportType},
			},
		},
		Properties: map[string]string{
			scannerTypeProperty: "bootc-native-package-vulnerability-scanner",
		},
	})
}

func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, scanRequestType) {
		log.Printf("unexpected scan request content type %q", contentType)
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
		writeError(w, http.StatusBadRequest, "Grype scanner only supports vulnerability and SBOM scans")
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
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	if err := s.acquireScanSlot(ctx); err != nil {
		return "", "", err
	}
	defer s.releaseScanSlot()

	workDir, err := os.MkdirTemp(s.tmpDir, "grype-scan-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	imageRef, err := registryImageRef(req.Registry, req.Artifact)
	if err != nil {
		return "", "", err
	}

	switch req.scanType() {
	case scanTypeSBOM:
		sbom, err := sbomSPDX(ctx, workDir, imageRef, req.Registry, req.Artifact)
		if err != nil {
			return "", "", err
		}
		report, err := json.Marshal(rawSBOMReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Scanner:     s.scanner(),
			MediaType:   spdxJSONMediaType,
			SBOM:        sbom,
		})
		if err != nil {
			return "", "", fmt.Errorf("marshal SBOM report: %w", err)
		}
		return string(report), sbomReportType, nil
	default:
		sbomPath, err := vulnerabilitySBOM(ctx, workDir, imageRef, req.Registry, req.Artifact)
		if err != nil {
			return "", "", err
		}
		grypeReport, err := runGrype(ctx, workDir, sbomPath)
		if err != nil {
			return "", "", err
		}
		report, err := json.Marshal(s.vulnerabilityReport(req.Artifact, grypeReport))
		if err != nil {
			return "", "", fmt.Errorf("marshal vulnerability report: %w", err)
		}
		return string(report), vulnReportType, nil
	}
}

func sbomSPDX(ctx context.Context, workDir, imageRef string, reg *registry, art *artifact) (any, error) {
	attachedPath, attached, err := fetchAttachedSBOM(ctx, workDir, reg, art)
	if err != nil {
		return nil, err
	}
	if attached {
		if sbom, err := decodeSPDX(attachedPath); err == nil {
			log.Printf("using attached SPDX SBOM for %s@%s", art.Repository, art.Digest)
			return sbom, nil
		}
		log.Printf("converting attached package database inventory to SPDX for %s@%s", art.Repository, art.Digest)
		filtered, err := filterPackageDBArtifacts(workDir, attachedPath)
		if err != nil {
			return nil, err
		}
		return convertToSPDX(ctx, workDir, filtered)
	}

	if inventoryPath, bootc, err := bootcInventorySBOM(ctx, workDir, reg, art); bootc {
		if err == nil {
			return convertToSPDX(ctx, workDir, inventoryPath)
		}
		log.Printf("content-driven inventory unavailable for %s@%s: %v", art.Repository, art.Digest, err)
	} else if err != nil {
		return nil, err
	}

	sbomPath, found, err := fetchHarborSBOM(ctx, workDir, reg, art)
	if err != nil {
		return nil, err
	}
	if found {
		if sbom, err := decodeSPDX(sbomPath); err == nil {
			log.Printf("using Harbor SPDX SBOM for %s@%s", art.Repository, art.Digest)
			return sbom, nil
		}
		log.Printf("converting attached package database inventory to SPDX for %s@%s", art.Repository, art.Digest)
		filtered, err := filterPackageDBArtifacts(workDir, sbomPath)
		if err != nil {
			return nil, err
		}
		return convertToSPDX(ctx, workDir, filtered)
	}

	return runSyftSPDX(ctx, workDir, imageRef, reg)
}

func vulnerabilitySBOM(ctx context.Context, workDir, imageRef string, reg *registry, art *artifact) (string, error) {
	if sbomPath, bootc, err := bootcInventorySBOM(ctx, workDir, reg, art); bootc {
		if err == nil {
			return sbomPath, nil
		}
		log.Printf("content-driven vulnerability inventory unavailable for %s@%s: %v", art.Repository, art.Digest, err)
	} else if err != nil {
		return "", err
	}

	sbomPath, found, err := fetchAttachedSBOM(ctx, workDir, reg, art)
	if err != nil {
		return "", err
	}
	if found {
		if _, err := decodeSPDX(sbomPath); err == nil {
			return filterSPDXPackageManagerInventory(workDir, sbomPath)
		}
		return filterPackageDBArtifacts(workDir, sbomPath)
	}

	sbomPath, found, err = fetchHarborSBOM(ctx, workDir, reg, art)
	if err != nil {
		return "", err
	}
	if found {
		return sbomPath, nil
	}

	log.Printf("usable SBOM referrer not found for %s@%s; generating Syft SBOM for vulnerability scan", art.Repository, art.Digest)
	return runSyftJSON(ctx, workDir, imageRef, reg)
}

func (s *server) acquireScanSlot(ctx context.Context) error {
	if s.scanSem == nil {
		return nil
	}
	select {
	case s.scanSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for Grype scan slot: %w", ctx.Err())
	}
}

func (s *server) releaseScanSlot() {
	if s.scanSem == nil {
		return
	}
	<-s.scanSem
}

func (s *server) vulnerabilityReport(art *artifact, report *grypeReport) vulnerabilityReport {
	items := make([]vulnerabilityItem, 0, len(report.Matches))
	seen := make(map[string]struct{}, len(report.Matches))
	severity := "None"

	for _, m := range report.Matches {
		if !isNativePackageType(m.Artifact.Type) {
			continue
		}
		matchDetail, ok := versionedMatchDetail(m)
		if !ok {
			continue
		}
		if _, ok := seen[m.Vulnerability.ID]; ok {
			continue
		}
		seen[m.Vulnerability.ID] = struct{}{}

		itemSeverity := normalizeSeverity(m.Vulnerability.Severity)
		if severityCode(itemSeverity) > severityCode(severity) {
			severity = itemSeverity
		}

		attrs := map[string]any{
			"grype_vulnerability_id": m.Vulnerability.ID,
			"namespace":              m.Vulnerability.Namespace,
			"package_type":           m.Artifact.Type,
			"purl":                   m.Artifact.PURL,
			"fix_state":              m.Vulnerability.Fix.State,
		}
		if report.Distro.Name != "" || report.Distro.Version != "" {
			attrs["distro"] = strings.TrimSpace(report.Distro.Name + " " + report.Distro.Version)
		}
		attrs["matcher"] = matchDetail.Matcher
		attrs["version_constraint"] = matchDetail.Found.VersionConstraint

		items = append(items, vulnerabilityItem{
			ID:               m.Vulnerability.ID,
			Package:          m.Artifact.Name,
			Version:          m.Artifact.Version,
			FixVersion:       fixVersion(m),
			Severity:         itemSeverity,
			Status:           m.Vulnerability.Fix.State,
			Description:      vulnerabilityDescription(m),
			Links:            vulnerabilityLinks(m),
			CVSSDetails:      preferredCVSS(m),
			CWEIDs:           vulnerabilityCWEs(m),
			VendorAttributes: attrs,
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

func versionedMatchDetail(m grypeMatch) (grypeMatchDetail, bool) {
	for _, detail := range m.MatchDetails {
		constraint := strings.TrimSpace(detail.Found.VersionConstraint)
		if constraint != "" && !strings.EqualFold(constraint, "none (unknown)") {
			return detail, true
		}
	}
	return grypeMatchDetail{}, false
}

func fetchHarborSBOM(ctx context.Context, workDir string, reg *registry, art *artifact) (string, bool, error) {
	return fetchSBOM(ctx, workDir, reg, art, selectSBOMDescriptor)
}

func fetchAttachedSBOM(ctx context.Context, workDir string, reg *registry, art *artifact) (string, bool, error) {
	return fetchSBOM(ctx, workDir, reg, art, selectAttachedSBOMDescriptor)
}

func fetchSBOM(ctx context.Context, workDir string, reg *registry, art *artifact, selectDescriptor func([]descriptor) (descriptor, bool)) (string, bool, error) {
	referrers, err := fetchReferrers(ctx, reg, art)
	if err != nil {
		return "", false, nil
	}
	sbomDescriptor, ok := selectDescriptor(referrers.Manifests)
	if !ok {
		return "", false, nil
	}

	manifest, err := fetchManifest(ctx, reg, art.Repository, sbomDescriptor.Digest)
	if err != nil {
		return "", true, err
	}
	if len(manifest.Layers) == 0 {
		return "", true, errors.New("Harbor SBOM manifest has no layers")
	}

	blob, err := fetchBlob(ctx, reg, art.Repository, manifest.Layers[0].Digest)
	if err != nil {
		return "", true, err
	}

	out := filepath.Join(workDir, "harbor-sbom.json")
	sbom, err := normalizeSBOMBlob(blob)
	if err != nil {
		return "", true, err
	}
	if err := os.WriteFile(out, sbom, 0o600); err != nil {
		return "", true, fmt.Errorf("write Harbor SBOM: %w", err)
	}
	return out, true, nil
}

func fetchReferrers(ctx context.Context, reg *registry, art *artifact) (*referrersIndex, error) {
	req, err := newRegistryRequest(ctx, http.MethodGet, reg, art.Repository, "referrers", art.Digest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", strings.Join([]string{ociIndexType, "application/json"}, ", "))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch referrers: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("fetch referrers", resp)
	}

	var index referrersIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("decode referrers: %w", err)
	}
	return &index, nil
}

func fetchManifest(ctx context.Context, reg *registry, repo, digest string) (*manifest, error) {
	req, err := newRegistryRequest(ctx, http.MethodGet, reg, repo, "manifests", digest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", strings.Join([]string{ociManifestType, dockerManifestType, "application/vnd.oci.artifact.manifest.v1+json", "application/json"}, ", "))

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

func fetchBlob(ctx context.Context, reg *registry, repo, digest string) ([]byte, error) {
	req, err := newRegistryRequest(ctx, http.MethodGet, reg, repo, "blobs", digest, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch SBOM blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("fetch SBOM blob", resp)
	}
	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read SBOM blob: %w", err)
	}
	return blob, nil
}

func selectSBOMDescriptor(manifests []descriptor) (descriptor, bool) {
	for i := len(manifests) - 1; i >= 0; i-- {
		descriptor := manifests[i]
		if descriptor.ArtifactType == harborSBOMType {
			return descriptor, true
		}
	}
	for i := len(manifests) - 1; i >= 0; i-- {
		descriptor := manifests[i]
		if descriptor.ArtifactType == attachedSPDXType {
			return descriptor, true
		}
	}
	for i := len(manifests) - 1; i >= 0; i-- {
		descriptor := manifests[i]
		if strings.Contains(strings.ToLower(descriptor.Annotations["org.opencontainers.artifact.description"]), "sbom") {
			return descriptor, true
		}
	}
	return descriptor{}, false
}

func selectAttachedSBOMDescriptor(manifests []descriptor) (descriptor, bool) {
	for i := len(manifests) - 1; i >= 0; i-- {
		if manifests[i].ArtifactType == attachedSPDXType {
			return manifests[i], true
		}
	}
	return descriptor{}, false
}

func normalizeSBOMBlob(blob []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(blob)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return trimmed, nil
	}

	tr := tar.NewReader(bytes.NewReader(blob))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read SBOM tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(strings.ToLower(header.Name), ".json") {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read SBOM tar entry: %w", err)
		}
		return bytes.TrimSpace(data), nil
	}
	return nil, errors.New("no JSON SBOM found in blob")
}

func runSyftJSON(ctx context.Context, workDir, imageRef string, reg *registry) (string, error) {
	out := filepath.Join(workDir, "sbom.syft.json")
	args := syftArgs(imageRef, "syft-json", out)
	cmd := exec.CommandContext(ctx, "syft", args...)
	cmd.Env = scannerEnv(workDir, reg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run syft: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return out, nil
}

func runSyftSPDX(ctx context.Context, workDir, imageRef string, reg *registry) (any, error) {
	out := filepath.Join(workDir, "sbom.spdx.json")
	args := syftArgs(imageRef, "spdx-json", out)
	cmd := exec.CommandContext(ctx, "syft", args...)
	cmd.Env = scannerEnv(workDir, reg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run syft spdx: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return decodeSPDX(out)
}

func convertToSPDX(ctx context.Context, workDir, source string) (any, error) {
	out := filepath.Join(workDir, "sbom.spdx.json")
	cmd := exec.CommandContext(ctx, "syft", "convert", source, "-q", "-o", "spdx-json="+out)
	cmd.Env = scannerEnv(workDir, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("convert Syft SBOM to SPDX: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return decodeSPDX(out)
}

func filterPackageDBArtifacts(workDir, source string) (string, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read attached Syft SBOM: %w", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return "", fmt.Errorf("decode attached Syft SBOM: %w", err)
	}
	var artifacts []json.RawMessage
	if err := json.Unmarshal(document["artifacts"], &artifacts); err != nil {
		return "", fmt.Errorf("decode attached Syft artifacts: %w", err)
	}
	filtered := make([]json.RawMessage, 0, len(artifacts))
	fedoraVersions := map[string]int{}
	for _, raw := range artifacts {
		var artifact struct {
			Type    string `json:"type"`
			FoundBy string `json:"foundBy"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return "", fmt.Errorf("decode attached Syft artifact: %w", err)
		}
		if isPackageDBArtifact(artifact.Type, artifact.FoundBy) {
			filtered = append(filtered, raw)
			if artifact.Type == packageTypeRPM {
				if match := fedoraVersionPattern.FindStringSubmatch(artifact.Version); len(match) == 2 {
					fedoraVersions[match[1]]++
				}
			}
		}
	}
	if len(filtered) == 0 {
		return "", errors.New("attached Syft SBOM has no package database artifacts")
	}
	document["artifacts"], err = json.Marshal(filtered)
	if err != nil {
		return "", fmt.Errorf("encode filtered Syft artifacts: %w", err)
	}
	if version := mostCommonVersion(fedoraVersions); version != "" {
		document["distro"], err = json.Marshal(syftDistro{
			PrettyName: "Fedora Linux " + version,
			Name:       "Fedora Linux",
			ID:         "fedora",
			VersionID:  version,
			IDLike:     []string{"fedora"},
			CPEName:    "cpe:/o:fedoraproject:fedora:" + version,
		})
		if err != nil {
			return "", fmt.Errorf("encode filtered Syft distro: %w", err)
		}
	}
	filteredData, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode filtered Syft SBOM: %w", err)
	}
	out := filepath.Join(workDir, "sbom.package-db.syft.json")
	if err := os.WriteFile(out, filteredData, 0o600); err != nil {
		return "", fmt.Errorf("write filtered Syft SBOM: %w", err)
	}
	return out, nil
}

func filterSPDXPackageManagerInventory(workDir, source string) (string, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read attached SPDX SBOM: %w", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return "", fmt.Errorf("decode attached SPDX SBOM: %w", err)
	}
	var packages []json.RawMessage
	if err := json.Unmarshal(document["packages"], &packages); err != nil {
		return "", fmt.Errorf("decode attached SPDX packages: %w", err)
	}

	selectedPrefix := ""
	for _, prefix := range nativePURLPrefixes {
		for _, raw := range packages {
			if spdxPackageHasPURL(raw, prefix) {
				selectedPrefix = prefix
				break
			}
		}
		if selectedPrefix != "" {
			break
		}
	}
	if selectedPrefix == "" {
		return source, nil
	}

	filtered := make([]json.RawMessage, 0, len(packages))
	for _, raw := range packages {
		if spdxPackageHasPURL(raw, selectedPrefix) {
			filtered = append(filtered, raw)
		}
	}
	document["packages"], err = json.Marshal(filtered)
	if err != nil {
		return "", fmt.Errorf("encode filtered SPDX packages: %w", err)
	}
	filteredData, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode filtered SPDX SBOM: %w", err)
	}
	out := filepath.Join(workDir, "sbom.package-db.spdx.json")
	if err := os.WriteFile(out, filteredData, 0o600); err != nil {
		return "", fmt.Errorf("write filtered SPDX SBOM: %w", err)
	}
	return out, nil
}

func spdxPackageHasPURL(raw json.RawMessage, prefix string) bool {
	var pkg struct {
		ExternalRefs []struct {
			ReferenceLocator string `json:"referenceLocator"`
		} `json:"externalRefs"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return false
	}
	for _, ref := range pkg.ExternalRefs {
		if strings.HasPrefix(strings.ToLower(ref.ReferenceLocator), prefix) {
			return true
		}
	}
	return false
}

func mostCommonVersion(versions map[string]int) string {
	var selected string
	for version, count := range versions {
		if count > versions[selected] || count == versions[selected] && version > selected {
			selected = version
		}
	}
	return selected
}

func isPackageDBArtifact(packageType, foundBy string) bool {
	if _, ok := nativePackageTypes[packageType]; !ok || packageType == "binary" {
		return false
	}
	switch foundBy {
	case "rpm-db-cataloger", "alpm-db-cataloger", "dpkg-db-cataloger", "apk-db-cataloger", "nix-cataloger":
		return true
	default:
		return false
	}
}

func decodeSPDX(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open SPDX SBOM: %w", err)
	}
	defer f.Close()

	var sbom map[string]any
	if err := json.NewDecoder(f).Decode(&sbom); err != nil {
		return nil, fmt.Errorf("decode SPDX SBOM: %w", err)
	}
	if _, ok := sbom["spdxVersion"].(string); !ok {
		return nil, errors.New("SPDX SBOM has no spdxVersion")
	}
	if _, ok := sbom["SPDXID"].(string); !ok {
		return nil, errors.New("SPDX SBOM has no SPDXID")
	}
	return sbom, nil
}

func syftArgs(imageRef, format, out string) []string {
	args := []string{
		"scan",
		"registry:" + imageRef,
		"-q",
		"-o", format + "=" + out,
		"--scope", envOrDefault("SCANNER_GRYPE_SCOPE", "squashed"),
		"--parallelism", envOrDefault("SCANNER_SYFT_PARALLELISM", "1"),
	}
	for _, cataloger := range strings.Split(envOrDefault("SCANNER_SYFT_CATALOGERS", defaultSyftCatalogers), ",") {
		cataloger = strings.TrimSpace(cataloger)
		if cataloger == "" {
			continue
		}
		args = append(args, "--override-default-catalogers", cataloger)
	}
	args = append(args, "--select-catalogers", "-file")
	return args
}

func runGrype(ctx context.Context, workDir, sbomPath string) (*grypeReport, error) {
	args := grypeArgs(sbomPath)
	cmd := exec.CommandContext(ctx, "grype", args...)
	cmd.Env = scannerEnv(workDir, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("run grype: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}

	var report grypeReport
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, fmt.Errorf("decode Grype report: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return &report, nil
}

func grypeArgs(sbomPath string) []string {
	return []string{
		"sbom:" + sbomPath,
		"-q",
		"-o", "json",
		"--sort-by", "risk",
		"--by-cve",
	}
}

func scannerEnv(workDir string, reg *registry) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"TMPDIR="+workDir,
		"SYFT_CACHE_DIR="+filepath.Join(workDir, "syft-cache"),
		"GRYPE_DB_CACHE_DIR="+envOrDefault("SCANNER_GRYPE_DB_CACHE_DIR", filepath.Join(workDir, "grype-db")),
	)
	if reg == nil {
		return env
	}

	base, err := url.Parse(reg.URL)
	if err == nil {
		if reg.Insecure || base.Scheme == "http" {
			env = append(env,
				"SYFT_REGISTRY_INSECURE_USE_HTTP=true",
				"SYFT_REGISTRY_INSECURE_SKIP_TLS_VERIFY=true",
			)
		}
		if base.Host != "" {
			env = append(env, registryAuthEnv(base.Host, reg.Authorization)...)
		}
	}
	return env
}

func registryAuthEnv(authority, authorization string) []string {
	scheme, value, ok := strings.Cut(strings.TrimSpace(authorization), " ")
	if !ok || value == "" {
		return nil
	}

	switch strings.ToLower(scheme) {
	case "basic":
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil
		}
		username, password, ok := strings.Cut(string(decoded), ":")
		if !ok {
			return nil
		}
		return []string{
			"SYFT_REGISTRY_AUTH_AUTHORITY=" + authority,
			"SYFT_REGISTRY_AUTH_USERNAME=" + username,
			"SYFT_REGISTRY_AUTH_PASSWORD=" + password,
		}
	case "bearer":
		return []string{
			"SYFT_REGISTRY_AUTH_AUTHORITY=" + authority,
			"SYFT_REGISTRY_AUTH_TOKEN=" + value,
		}
	default:
		return nil
	}
}

func registryImageRef(reg *registry, art *artifact) (string, error) {
	base, err := url.Parse(reg.URL)
	if err != nil {
		return "", fmt.Errorf("parse registry URL: %w", err)
	}
	host := base.Host
	if host == "" {
		host = strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(reg.URL, "/"), "http://"), "https://")
	}
	if host == "" {
		return "", errors.New("empty registry host")
	}
	return fmt.Sprintf("%s/%s@%s", host, art.Repository, art.Digest), nil
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

func vulnerabilityDescription(m grypeMatch) string {
	if m.Vulnerability.Description != "" {
		return m.Vulnerability.Description
	}
	for _, related := range m.RelatedVulnerabilities {
		if related.ID == m.Vulnerability.ID && related.Description != "" {
			return related.Description
		}
	}
	return ""
}

func vulnerabilityLinks(m grypeMatch) []string {
	seen := map[string]struct{}{}
	var links []string
	add := func(link string) {
		if link == "" {
			return
		}
		if _, ok := seen[link]; ok {
			return
		}
		seen[link] = struct{}{}
		links = append(links, link)
	}

	add(m.Vulnerability.DataSource)
	for _, link := range m.Vulnerability.URLs {
		add(link)
	}
	for _, advisory := range m.Vulnerability.Advisories {
		add(advisory.Link)
	}
	for _, related := range m.RelatedVulnerabilities {
		if related.ID != m.Vulnerability.ID {
			continue
		}
		add(related.DataSource)
		for _, link := range related.URLs {
			add(link)
		}
	}
	return links
}

func vulnerabilityCWEs(m grypeMatch) []string {
	seen := map[string]struct{}{}
	var cwes []string
	add := func(cwe string) {
		if cwe == "" {
			return
		}
		if _, ok := seen[cwe]; ok {
			return
		}
		seen[cwe] = struct{}{}
		cwes = append(cwes, cwe)
	}
	for _, cwe := range m.Vulnerability.CWEs {
		add(cwe.CWE)
	}
	for _, related := range m.RelatedVulnerabilities {
		if related.ID != m.Vulnerability.ID {
			continue
		}
		for _, cwe := range related.CWEs {
			add(cwe.CWE)
		}
	}
	return cwes
}

func preferredCVSS(m grypeMatch) cvssDetails {
	best := cvssDetails{}
	var bestScore float64
	for _, cvss := range append(m.Vulnerability.CVSS, relatedCVSS(m.RelatedVulnerabilities, m.Vulnerability.ID)...) {
		if !strings.HasPrefix(cvss.Version, "3") || cvss.Metrics.BaseScore == nil {
			continue
		}
		if best.ScoreV3 == nil || *cvss.Metrics.BaseScore > bestScore {
			score := *cvss.Metrics.BaseScore
			bestScore = score
			best.ScoreV3 = &score
			best.VectorV3 = cvss.Vector
		}
	}
	return best
}

func relatedCVSS(vulns []grypeVulnerability, id string) []grypeCVSS {
	var cvss []grypeCVSS
	for _, vuln := range vulns {
		if vuln.ID == id {
			cvss = append(cvss, vuln.CVSS...)
		}
	}
	return cvss
}

func fixVersion(m grypeMatch) string {
	for _, detail := range m.MatchDetails {
		if detail.Fix.SuggestedVersion != "" {
			return detail.Fix.SuggestedVersion
		}
	}
	return strings.Join(m.Vulnerability.Fix.Versions, ",")
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
	case "negligible":
		return "Negligible"
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
	case "Negligible", "Unknown":
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
	case scanTypeVuln, scanTypeSBOM:
		return true
	default:
		return false
	}
}

func (r *scanRequest) validate() error {
	if r.Registry == nil || r.Registry.URL == "" {
		return errors.New("scan request: invalid registry")
	}
	if r.Artifact == nil || r.Artifact.Repository == "" || r.Artifact.Digest == "" || r.Artifact.MimeType == "" {
		return errors.New("scan request: invalid artifact")
	}
	if !slices.Contains([]string{ociManifestType, dockerManifestType}, r.Artifact.MimeType) {
		return fmt.Errorf("scan request: unsupported artifact MIME type %q", r.Artifact.MimeType)
	}
	return nil
}

func (s *server) scanner() scanner {
	return scanner{Name: "Grype", Vendor: "Anchore", Version: s.version}
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

func grypeVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "grype", "version", "-o", "json").Output()
	if err == nil {
		var decoded struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(output, &decoded) == nil && decoded.Version != "" {
			return decoded.Version
		}
	}

	output, err = exec.CommandContext(ctx, "grype", "version").Output()
	if err != nil {
		return "Unknown"
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		}
	}
	return strings.TrimSpace(string(output))
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("invalid %s: %v", name, err)
	}
	return parsed
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
