package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout = 45 * time.Minute
)

type config struct {
	harborURL  string
	registry   string
	project    string
	repository string
	reference  string
	targetTag  string
	username   string
	password   string
	workDir    string
	authFile   string
	buildkit   string
	filemap    string
	tlsVerify  bool
	mode       string
	timeout    time.Duration
	skipScan   bool
	skipSBOM   bool
}

type artifact struct {
	Digest       string         `json:"digest"`
	ScanOverview map[string]any `json:"scan_overview"`
	SBOMOverview map[string]any `json:"sbom_overview"`
}

type vulnerabilityReport struct {
	Vulnerabilities []vulnerability `json:"vulnerabilities"`
}

type vulnerability struct {
	ID             string   `json:"id"`
	Package        string   `json:"package"`
	Version        string   `json:"version"`
	FixVersion     string   `json:"fix_version"`
	Severity       string   `json:"severity"`
	Status         string   `json:"status"`
	Description    string   `json:"description"`
	Links          []string `json:"links"`
	ArtifactDigest string   `json:"artifact_digest"`
}

type summary struct {
	Total   int `json:"total"`
	Fixable int `json:"fixable"`
}

type trivyReport struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Target          string               `json:"Target"`
	Type            string               `json:"Type"`
	Vulnerabilities []trivyVulnerability `json:"Vulnerabilities"`
}

type trivyVulnerability struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Severity         string `json:"Severity"`
}

type sourceTargetPlan struct {
	Owner    string
	Target   string
	Type     string
	Total    int
	Fixable  int
	Packages map[string]*sourcePackagePlan
}

type sourcePackagePlan struct {
	Name             string
	InstalledVersion string
	FixedVersion     string
	Total            int
	Severities       map[string]bool
}

func main() {
	ctx := context.Background()
	cfg := parseFlags()
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "bootc-patcher: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.harborURL, "harbor-url", env("HARBOR_URL", "http://100.100.156.26:18085"), "Harbor API base URL")
	flag.StringVar(&cfg.registry, "registry", env("HARBOR_REGISTRY", "100.100.156.26:18085"), "Harbor registry host[:port]")
	flag.StringVar(&cfg.project, "project", env("HARBOR_PROJECT", "bluefin"), "Harbor project")
	flag.StringVar(&cfg.repository, "repository", env("HARBOR_REPOSITORY", "dakota-bootc"), "Harbor repository name")
	flag.StringVar(&cfg.reference, "reference", env("HARBOR_REFERENCE", "latest"), "Artifact tag or digest")
	flag.StringVar(&cfg.targetTag, "target-tag", env("PATCHED_TAG", "patched"), "Tag for the patched image")
	flag.StringVar(&cfg.username, "username", env("HARBOR_USERNAME", "admin"), "Harbor username")
	flag.StringVar(&cfg.password, "password", env("HARBOR_PASSWORD", ""), "Harbor password")
	flag.StringVar(&cfg.workDir, "workdir", env("PATCH_WORKDIR", "temp/harbor-patch"), "Project-local working directory")
	flag.StringVar(&cfg.authFile, "authfile", env("REGISTRY_AUTH_FILE", "temp/harbor-push/auth.json"), "Containers auth file for skopeo/trivy/copa")
	flag.StringVar(&cfg.buildkit, "buildkit-addr", env("BUILDKIT_HOST", ""), "BuildKit address for Copa, for example unix:///path/to/buildkitd.sock")
	flag.StringVar(&cfg.filemap, "filemap", env("DAKOTA_FILEMAP", ""), "Dakota fakecap manifest TSV for mapping files back to BuildStream elements")
	flag.BoolVar(&cfg.tlsVerify, "tls-verify", boolEnv("REGISTRY_TLS_VERIFY", false), "Verify registry TLS when Podman pulls or pushes")
	flag.StringVar(&cfg.mode, "mode", env("PATCH_MODE", "plan"), "Mode: plan, patch")
	flag.DurationVar(&cfg.timeout, "timeout", defaultTimeout, "Timeout for scan polling and patch command")
	flag.BoolVar(&cfg.skipScan, "skip-scan", false, "Do not trigger Harbor vulnerability scan")
	flag.BoolVar(&cfg.skipSBOM, "skip-sbom", false, "Do not trigger Harbor SBOM generation")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config) error {
	if cfg.password == "" {
		return errors.New("password is required; set HARBOR_PASSWORD or pass -password")
	}
	if cfg.mode != "plan" && cfg.mode != "patch" {
		return fmt.Errorf("unsupported mode %q; expected plan or patch", cfg.mode)
	}
	if err := os.MkdirAll(cfg.workDir, 0o755); err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	client := &harborClient{
		baseURL:  strings.TrimRight(cfg.harborURL, "/"),
		username: cfg.username,
		password: cfg.password,
		client:   &http.Client{Timeout: 60 * time.Second},
	}

	if !cfg.skipScan {
		if err := client.triggerScan(ctx, cfg.project, cfg.repository, cfg.reference, "vulnerability"); err != nil {
			return err
		}
	}
	if !cfg.skipSBOM {
		if err := client.triggerScan(ctx, cfg.project, cfg.repository, cfg.reference, "sbom"); err != nil {
			return err
		}
	}

	art, err := client.waitForScans(ctx, cfg.project, cfg.repository, cfg.reference, !cfg.skipScan, !cfg.skipSBOM)
	if err != nil {
		return err
	}
	fmt.Printf("artifact digest: %s\n", art.Digest)

	raw, err := client.vulnerabilityAddition(ctx, cfg.project, cfg.repository, cfg.reference)
	if err != nil {
		return err
	}
	harborReportPath := filepath.Join(cfg.workDir, "harbor-vulnerabilities.json")
	if err := os.WriteFile(harborReportPath, raw, 0o644); err != nil {
		return fmt.Errorf("write Harbor vulnerability report: %w", err)
	}

	report, err := parseVulnerabilities(raw)
	if err != nil {
		return err
	}
	sum := summarize(report)
	fmt.Printf("harbor vulnerabilities: total=%d fixable=%d\n", sum.Total, sum.Fixable)

	imageRef := fmt.Sprintf("%s/%s/%s:%s", cfg.registry, cfg.project, cfg.repository, cfg.reference)
	patchedRef := fmt.Sprintf("%s/%s/%s:%s", cfg.registry, cfg.project, cfg.repository, cfg.targetTag)
	trivyReportPath := filepath.Join(cfg.workDir, "trivy-report.json")

	if err := runTrivy(ctx, cfg, imageRef, trivyReportPath); err != nil {
		return err
	}
	if cfg.filemap != "" {
		sourcePlanPath := filepath.Join(cfg.workDir, "source-remediation.md")
		if err := writeSourcePlan(trivyReportPath, cfg.filemap, sourcePlanPath); err != nil {
			return err
		}
		fmt.Printf("source remediation plan: %s\n", sourcePlanPath)
	}
	if cfg.mode == "plan" {
		fmt.Printf("plan complete; Trivy Copa report: %s\n", trivyReportPath)
		fmt.Printf("next patch target: %s -> %s\n", imageRef, patchedRef)
		return nil
	}

	if err := runCopa(ctx, cfg, imageRef, patchedRef, trivyReportPath); err != nil {
		return err
	}

	fmt.Printf("patched image pushed: %s\n", patchedRef)
	fmt.Println("triggering Harbor scans for patched image")
	if err := client.triggerScan(ctx, cfg.project, cfg.repository, cfg.targetTag, "vulnerability"); err != nil {
		return err
	}
	if err := client.triggerScan(ctx, cfg.project, cfg.repository, cfg.targetTag, "sbom"); err != nil {
		return err
	}
	_, err = client.waitForScans(ctx, cfg.project, cfg.repository, cfg.targetTag, true, true)
	return err
}

type harborClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

func (c *harborClient) triggerScan(ctx context.Context, project, repo, ref, scanType string) error {
	body := []byte(fmt.Sprintf(`{"scan_type":%q}`, scanType))
	req, err := c.request(ctx, http.MethodPost, artifactPath(c.baseURL, project, repo, ref)+"/scan", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("trigger %s scan: %w", scanType, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusAccepted || res.StatusCode == http.StatusConflict {
		fmt.Printf("%s scan accepted for %s/%s:%s\n", scanType, project, repo, ref)
		return nil
	}
	return responseError("trigger "+scanType+" scan", res)
}

func (c *harborClient) waitForScans(ctx context.Context, project, repo, ref string, waitVuln, waitSBOM bool) (*artifact, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		art, err := c.artifact(ctx, project, repo, ref)
		if err != nil {
			return nil, err
		}
		vulnDone := !waitVuln || overviewDone(art.ScanOverview)
		sbomDone := !waitSBOM || overviewDone(art.SBOMOverview)
		fmt.Printf("scan status: vulnerability=%s sbom=%s\n", overviewStatus(art.ScanOverview), overviewStatus(art.SBOMOverview))
		if vulnDone && sbomDone {
			return art, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for Harbor scans: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *harborClient) artifact(ctx context.Context, project, repo, ref string) (*artifact, error) {
	req, err := c.request(ctx, http.MethodGet, artifactPath(c.baseURL, project, repo, ref)+"?with_scan_overview=true&with_sbom_overview=true", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, responseError("get artifact", res)
	}
	var art artifact
	if err := json.NewDecoder(res.Body).Decode(&art); err != nil {
		return nil, fmt.Errorf("decode artifact: %w", err)
	}
	return &art, nil
}

func (c *harborClient) vulnerabilityAddition(ctx context.Context, project, repo, ref string) ([]byte, error) {
	req, err := c.request(ctx, http.MethodGet, artifactPath(c.baseURL, project, repo, ref)+"/additions/vulnerabilities", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get vulnerabilities addition: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, responseError("get vulnerabilities addition", res)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read vulnerabilities addition: %w", err)
	}
	return raw, nil
}

func (c *harborClient) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	return req, nil
}

func runTrivy(ctx context.Context, cfg config, imageRef, out string) error {
	args := []string{
		"image",
		"--format", "json",
		"--output", out,
		"--scanners", "vuln",
		"--ignore-unfixed=false",
		"--username", cfg.username,
		"--password", cfg.password,
	}
	if !cfg.tlsVerify {
		args = append(args, "--insecure")
	}
	args = append(args, imageRef)
	env := os.Environ()
	if cfg.authFile != "" {
		env = append(env, "REGISTRY_AUTH_FILE="+cfg.authFile)
	}
	fmt.Printf("running trivy for Copa input: %s\n", out)
	return runCommand(ctx, env, "trivy", args...)
}

func runCopa(ctx context.Context, cfg config, imageRef, patchedRef, report string) error {
	if _, err := exec.LookPath("copa"); err != nil {
		return errors.New("copa is not installed; install project-copacetic/copacetic and rerun with -mode patch")
	}
	if _, err := exec.LookPath("podman"); err != nil {
		return errors.New("podman is not installed; patch mode pulls and pushes through Podman")
	}
	sourceLocal := fmt.Sprintf("localhost/%s:copa-source-%s", imageName(cfg), safeTag(cfg.reference))
	patchedLocal := fmt.Sprintf("localhost/%s:%s", imageName(cfg), safeTag(cfg.targetTag))

	env := os.Environ()
	if cfg.authFile != "" {
		env = append(env, "REGISTRY_AUTH_FILE="+cfg.authFile)
	}

	if err := runPodman(ctx, env, cfg, "pull", imageRef); err != nil {
		return err
	}
	if err := runCommand(ctx, env, "podman", "tag", imageRef, sourceLocal); err != nil {
		return err
	}

	args := []string{
		"patch",
		"-i", sourceLocal,
		"-r", report,
		"-t", patchedLocal,
		"--loader", "podman",
		"--timeout", cfg.timeout.String(),
	}
	if cfg.buildkit != "" {
		args = append(args, "--addr", cfg.buildkit)
	}
	fmt.Printf("running copa patch: %s -> %s\n", sourceLocal, patchedLocal)
	if err := runCommand(ctx, env, "copa", args...); err != nil {
		return err
	}
	if err := runCommand(ctx, env, "podman", "image", "exists", patchedLocal); err != nil {
		return fmt.Errorf("copa produced no patched image for %s; this usually means the report has no OS package-manager updates for Copa to apply: %w", sourceLocal, err)
	}
	if err := runCommand(ctx, env, "podman", "tag", patchedLocal, patchedRef); err != nil {
		return err
	}
	return runPodman(ctx, env, cfg, "push", patchedRef)
}

func runPodman(ctx context.Context, env []string, cfg config, action, imageRef string) error {
	args := []string{action}
	if cfg.authFile != "" {
		args = append(args, "--authfile", cfg.authFile)
	}
	args = append(args, "--tls-verify="+fmt.Sprint(cfg.tlsVerify), imageRef)
	return runCommand(ctx, env, "podman", args...)
}

func imageName(cfg config) string {
	return "bootc-patcher-" + safeTag(cfg.project+"-"+cfg.repository)
}

func safeTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	safe := strings.Trim(b.String(), ".-")
	if safe == "" {
		return "empty"
	}
	return safe
}

func writeSourcePlan(trivyPath, filemapPath, out string) error {
	raw, err := os.ReadFile(trivyPath)
	if err != nil {
		return fmt.Errorf("read Trivy report: %w", err)
	}
	var report trivyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return fmt.Errorf("decode Trivy report: %w", err)
	}
	filemap, err := loadFilemap(filemapPath)
	if err != nil {
		return err
	}

	plans := make(map[string]*sourceTargetPlan)
	for _, result := range report.Results {
		if len(result.Vulnerabilities) == 0 {
			continue
		}
		target := normalizeTarget(result.Target)
		owner := filemap[target]
		if owner == "" {
			owner = "(unmapped)"
		}
		key := owner + "\x00" + target + "\x00" + result.Type
		plan := plans[key]
		if plan == nil {
			plan = &sourceTargetPlan{
				Owner:    owner,
				Target:   target,
				Type:     result.Type,
				Packages: make(map[string]*sourcePackagePlan),
			}
			plans[key] = plan
		}
		for _, vuln := range result.Vulnerabilities {
			plan.Total++
			if vuln.FixedVersion != "" {
				plan.Fixable++
			}
			pkgKey := strings.Join([]string{vuln.PkgName, vuln.InstalledVersion, vuln.FixedVersion}, "\x00")
			pkgPlan := plan.Packages[pkgKey]
			if pkgPlan == nil {
				pkgPlan = &sourcePackagePlan{
					Name:             vuln.PkgName,
					InstalledVersion: vuln.InstalledVersion,
					FixedVersion:     vuln.FixedVersion,
					Severities:       make(map[string]bool),
				}
				plan.Packages[pkgKey] = pkgPlan
			}
			pkgPlan.Total++
			if vuln.Severity != "" {
				pkgPlan.Severities[vuln.Severity] = true
			}
		}
	}

	var ordered []*sourceTargetPlan
	for _, plan := range plans {
		ordered = append(ordered, plan)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Owner != ordered[j].Owner {
			return ordered[i].Owner < ordered[j].Owner
		}
		return ordered[i].Target < ordered[j].Target
	})

	var b strings.Builder
	b.WriteString("# Dakota Source Remediation Plan\n\n")
	b.WriteString("This report maps Trivy dependency findings back to Dakota BuildStream outputs.\n\n")
	b.WriteString("Copa can patch OS package-manager findings. The current Dakota findings are `gobinary` and `python-pkg` dependency findings, so durable remediation is to update the owning `.bst` source refs or junctions, rebuild, rechunk with Chunkah, push a new bootc tag, and rescan in Harbor.\n\n")
	b.WriteString("Counts below are local Trivy finding instances by target. Harbor may show a lower artifact total after normalizing or deduplicating findings.\n\n")
	b.WriteString("## Target Summary\n\n")
	b.WriteString("| Owner | Target | Type | Total | Fixable |\n")
	b.WriteString("| --- | --- | --- | ---: | ---: |\n")
	for _, plan := range ordered {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d |\n",
			escapeMarkdown(plan.Owner),
			escapeMarkdown(plan.Target),
			escapeMarkdown(plan.Type),
			plan.Total,
			plan.Fixable,
		)
	}
	b.WriteString("\n## Package Fix Floors\n\n")
	for _, plan := range ordered {
		fmt.Fprintf(&b, "### `%s` -> `%s`\n\n", plan.Target, plan.Owner)
		b.WriteString("| Package | Installed | Fixed | Findings | Severities |\n")
		b.WriteString("| --- | --- | --- | ---: | --- |\n")
		for _, pkg := range orderedPackages(plan.Packages) {
			fmt.Fprintf(&b, "| %s | %s | %s | %d | %s |\n",
				escapeMarkdown(pkg.Name),
				escapeMarkdown(pkg.InstalledVersion),
				escapeMarkdown(pkg.FixedVersion),
				pkg.Total,
				escapeMarkdown(joinSet(pkg.Severities)),
			)
		}
		b.WriteString("\n")
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("create source plan directory: %w", err)
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write source remediation plan: %w", err)
	}
	return nil
}

func loadFilemap(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open filemap: %w", err)
	}
	defer file.Close()

	filemap := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			fields = strings.Fields(line)
		}
		if len(fields) < 2 {
			continue
		}
		filemap[normalizeTarget(fields[0])] = fields[1]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read filemap: %w", err)
	}
	return filemap, nil
}

func normalizeTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return "(unknown)"
	}
	if strings.HasPrefix(target, "/") {
		return filepath.Clean(target)
	}
	if strings.Contains(target, "/") {
		return filepath.Clean("/" + target)
	}
	return target
}

func orderedPackages(packages map[string]*sourcePackagePlan) []*sourcePackagePlan {
	ordered := make([]*sourcePackagePlan, 0, len(packages))
	for _, pkg := range packages {
		ordered = append(ordered, pkg)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		if ordered[i].InstalledVersion != ordered[j].InstalledVersion {
			return ordered[i].InstalledVersion < ordered[j].InstalledVersion
		}
		return ordered[i].FixedVersion < ordered[j].FixedVersion
	})
	return ordered
}

func joinSet(values map[string]bool) string {
	ordered := make([]string, 0, len(values))
	for value := range values {
		ordered = append(ordered, value)
	}
	sort.Strings(ordered)
	return strings.Join(ordered, ", ")
}

func escapeMarkdown(value string) string {
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}

func runCommand(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func parseVulnerabilities(raw []byte) (*vulnerabilityReport, error) {
	var direct vulnerabilityReport
	if err := json.Unmarshal(raw, &direct); err == nil && direct.Vulnerabilities != nil {
		return &direct, nil
	}
	var wrapped map[string]vulnerabilityReport
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("decode vulnerability report: %w", err)
	}
	for _, report := range wrapped {
		return &report, nil
	}
	return &vulnerabilityReport{}, nil
}

func summarize(report *vulnerabilityReport) summary {
	var s summary
	for _, v := range report.Vulnerabilities {
		s.Total++
		if v.FixVersion != "" || strings.EqualFold(v.Status, "fixed") {
			s.Fixable++
		}
	}
	return s
}

func overviewDone(overview map[string]any) bool {
	if len(overview) == 0 {
		return false
	}
	if status, _ := overview["scan_status"].(string); status != "" {
		return terminalStatus(status)
	}
	for _, value := range overview {
		m, ok := value.(map[string]any)
		if !ok {
			continue
		}
		status, _ := m["scan_status"].(string)
		if terminalStatus(status) {
			return true
		}
	}
	return false
}

func overviewStatus(overview map[string]any) string {
	if len(overview) == 0 {
		return "none"
	}
	if status, _ := overview["scan_status"].(string); status != "" {
		return status
	}
	var statuses []string
	for _, value := range overview {
		m, ok := value.(map[string]any)
		if !ok {
			continue
		}
		status, _ := m["scan_status"].(string)
		if status == "" {
			status = "unknown"
		}
		statuses = append(statuses, status)
	}
	if len(statuses) == 0 {
		return "unknown"
	}
	return strings.Join(statuses, ",")
}

func terminalStatus(status string) bool {
	return strings.EqualFold(status, "success") ||
		strings.EqualFold(status, "finished") ||
		strings.EqualFold(status, "completed") ||
		strings.EqualFold(status, "error") ||
		strings.EqualFold(status, "failed") ||
		strings.EqualFold(status, "stopped")
}

func artifactPath(baseURL, project, repo, ref string) string {
	return fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s",
		baseURL,
		url.PathEscape(project),
		escapeRepository(repo),
		url.PathEscape(ref),
	)
}

func escapeRepository(repo string) string {
	return strings.ReplaceAll(url.PathEscape(repo), "%2F", "%252F")
}

func responseError(action string, res *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		msg = res.Status
	}
	return fmt.Errorf("%s: HTTP %d: %s", action, res.StatusCode, msg)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
