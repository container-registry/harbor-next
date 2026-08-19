//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/state"
)

const pgxMonitoringProjectAlias = "pgx-monitoring"

func registerPgxMonitoring(sc *godog.ScenarioContext) {
	sc.When(`^I exercise PostgreSQL-backed Harbor APIs$`, exercisePostgreSQLBackedHarborAPIs)
	sc.When(`^I run a PostgreSQL-backed jobservice task$`, runPostgreSQLBackedJobserviceTask)
	sc.When(`^I scrape database metrics from core and jobservice$`, scrapeDatabaseMetrics)

	sc.Then(`^both services expose pgx pool metrics$`, bothServicesExposePgxPoolMetrics)
	sc.Then(`^both services expose pgx database operation metrics$`, bothServicesExposePgxDatabaseOperationMetrics)
	sc.Then(`^pgx metrics do not expose scenario resource names$`, pgxMetricsDoNotExposeScenarioResourceNames)
}

// ============================================================================
// When
// ============================================================================

func exercisePostgreSQLBackedHarborAPIs(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	project := projectName(s, pgxMonitoringProjectAlias)
	if err := createProject(s, project, true, nil); err != nil {
		return ctx, err
	}

	paths := []string{
		"/api/v2.0/users/current",
		"/api/v2.0/projects/" + project,
		"/api/v2.0/projects?page=1&page_size=10",
		"/api/v2.0/configurations",
	}
	for _, path := range paths {
		resp, err := s.Client.Get(path)
		captureResp(s, resp, err)
		if err != nil {
			return ctx, err
		}
		if resp.StatusCode != http.StatusOK {
			return ctx, fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, truncate(resp.Body))
		}
	}
	return ctx, nil
}

func runPostgreSQLBackedJobserviceTask(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	body := map[string]any{
		"schedule": map[string]any{"type": "Manual"},
		"parameters": map[string]any{
			"audit_retention_hour": 1,
			"include_event_types":  "create_artifact",
			"dry_run":              true,
		},
	}
	resp, err := s.Client.Post("/api/v2.0/system/purgeaudit/schedule", body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != http.StatusCreated {
		return ctx, fmt.Errorf("trigger purge audit job: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	id, ok := idFromLocation(resp.Header.Get("Location"))
	if !ok {
		return ctx, fmt.Errorf("purge audit job response missing execution id in Location %q", resp.Header.Get("Location"))
	}
	return ctx, waitForPurgeAuditJob(ctx, s, id)
}

func scrapeDatabaseMetrics(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	urls := map[string]string{
		"core":       envDefault("CORE_METRICS_URL", "http://127.0.0.1:9090/metrics"),
		"jobservice": envDefault("JOBSERVICE_METRICS_URL", "http://127.0.0.1:9091/metrics"),
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for service, url := range urls {
		body, err := scrapeMetricEndpoint(ctx, client, service, url)
		if err != nil {
			return ctx, err
		}
		s.MetricsByService[service] = body
	}
	return ctx, nil
}

// ============================================================================
// Then
// ============================================================================

func bothServicesExposePgxPoolMetrics(ctx context.Context) error {
	return requireMetricFromEveryService(ctx, "pgx pool", func(body []byte) bool {
		return metricBodyContainsAny(body,
			"pgx_pool_",
			"db_client_connections_usage",
			"db_client_connection_max",
			"db_client_connections_pending_requests",
		)
	})
}

func bothServicesExposePgxDatabaseOperationMetrics(ctx context.Context) error {
	return requireMetricFromEveryService(ctx, "pgx database operation", func(body []byte) bool {
		return metricBodyContainsAny(body, "db_client_operation_duration")
	})
}

func pgxMetricsDoNotExposeScenarioResourceNames(ctx context.Context) error {
	s := state.Get(ctx)
	needles := []string{projectName(s, pgxMonitoringProjectAlias), s.Suffix}
	for service, body := range s.MetricsByService {
		lines := databaseMetricLines(body)
		if len(lines) == 0 {
			return fmt.Errorf("%s metrics contained no pgx/db metric lines", service)
		}
		for _, line := range lines {
			for _, needle := range needles {
				if needle != "" && strings.Contains(line, needle) {
					return fmt.Errorf("%s pgx metric line leaked scenario value %q: %s", service, needle, line)
				}
			}
		}
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func waitForPurgeAuditJob(ctx context.Context, s *state.Scenario, id int64) error {
	path := fmt.Sprintf("/api/v2.0/system/purgeaudit/%d", id)
	return pollUntil(ctx, 2*time.Second, 60*time.Second, func() (bool, error) {
		resp, err := s.Client.Get(path)
		captureResp(s, resp, err)
		if err != nil {
			return false, err
		}
		if resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, truncate(resp.Body))
		}
		var job struct {
			Status    string `json:"status"`
			JobStatus string `json:"job_status"`
		}
		if err := json.Unmarshal(resp.Body, &job); err != nil {
			return false, fmt.Errorf("decode purge audit job status: %w", err)
		}
		status := job.Status
		if status == "" {
			status = job.JobStatus
		}
		if !terminalJobStatus(status) {
			return false, nil
		}
		if !successfulJobStatus(status) {
			return false, fmt.Errorf("purge audit job %d status = %s", id, status)
		}
		return true, nil
	})
}

func successfulJobStatus(status string) bool {
	switch strings.ToLower(status) {
	case "finished", "success", "succeed", "succeeded":
		return true
	}
	return false
}

func scrapeMetricEndpoint(ctx context.Context, client *http.Client, service, url string) ([]byte, error) {
	var lastErr error
	deadline := time.Now().Add(30 * time.Second)
	for {
		body, err := fetchMetricEndpoint(ctx, client, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("scrape %s metrics at %s: %w", service, url, lastErr)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func fetchMetricEndpoint(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new metrics request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read metrics response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, truncate(body))
	}
	return body, nil
}

func requireMetricFromEveryService(ctx context.Context, label string, contains func([]byte) bool) error {
	s := state.Get(ctx)
	for _, service := range []string{"core", "jobservice"} {
		body, ok := s.MetricsByService[service]
		if !ok || len(body) == 0 {
			return fmt.Errorf("no metrics captured for %s", service)
		}
		if !contains(body) {
			return fmt.Errorf("%s metrics missing %s metric; db metric families: %s", service, label, strings.Join(metricFamilyNames(body), ", "))
		}
	}
	return nil
}

func metricBodyContainsAny(body []byte, needles ...string) bool {
	text := string(body)
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func databaseMetricLines(body []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(body), "\n") {
		if isDatabaseMetricLine(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

func isDatabaseMetricLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	return strings.Contains(line, "db_client_") ||
		strings.Contains(line, "pgx_pool_") ||
		strings.Contains(line, "db.client.") ||
		strings.Contains(line, "pgx.pool.")
}

func metricFamilyNames(body []byte) []string {
	seen := map[string]bool{}
	var names []string
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "# HELP ") && !strings.HasPrefix(line, "# TYPE ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !isDatabaseMetricLine(fields[2]) || seen[fields[2]] {
			continue
		}
		seen[fields[2]] = true
		names = append(names, fields[2])
	}
	return names
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
