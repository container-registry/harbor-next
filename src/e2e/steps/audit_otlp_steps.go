//go:build e2e

package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/harborclient"
	"github.com/goharbor/harbor/src/e2e/internal/otlpreceiver"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

const (
	otlpAuditTimeout = 20 * time.Second
	manifestAccept   = "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
)

func registerAuditOTLP(sc *godog.ScenarioContext) {
	sc.Given("^an in-process OTLP audit collector$", inProcessOTLPAuditCollector)
	sc.When("^the admin enables OTLP audit forwarding from client IP \"([^\"]+)\" and user agent \"([^\"]+)\"$", enableOTLPAuditForwarding)
	sc.When("^a client pulls manifest \"([^\"]+)\" from client IP \"([^\"]+)\" and user agent \"([^\"]+)\"$", pullManifestWithSource)
	sc.Then("^the collector receives audit event \"([^\"]+)\" with client IP \"([^\"]+)\" and user agent \"([^\"]+)\"$", collectorReceivesAuditSource)
	sc.Then("^the collector receives a pull audit for \"([^\"]+)\" with client IP \"([^\"]+)\" and user agent \"([^\"]+)\"$", collectorReceivesPullAudit)
}

func inProcessOTLPAuditCollector(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	host := resolveWebhookHost(s.Client.BaseURL)
	if host == "" {
		return ctx, fmt.Errorf("cannot determine callback host for OTLP collector")
	}
	receiver, err := otlpreceiver.New(host)
	if err != nil {
		return ctx, err
	}
	s.OTLPReceiver = receiver
	return ctx, nil
}

func enableOTLPAuditForwarding(ctx context.Context, clientIP, userAgent string) (context.Context, error) {
	s := state.Get(ctx)
	if s.OTLPReceiver == nil {
		return ctx, fmt.Errorf("no OTLP collector staged")
	}
	s.AuditOTLPRestore = func() error {
		resp, err := s.Client.Put("/api/v2.0/configurations", otlpConfiguration(""))
		if err != nil {
			return err
		}
		return harborclient.Expect(resp, http.StatusOK)
	}

	resp, err := doJSONWithSource(
		s.Client,
		http.MethodPut,
		"/api/v2.0/configurations",
		otlpConfiguration(s.OTLPReceiver.Endpoint()),
		clientIP,
		userAgent,
		true,
	)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if err := harborclient.Expect(resp, http.StatusOK); err != nil {
		return ctx, fmt.Errorf("configure OTLP audit forwarding: %w", err)
	}
	return ctx, nil
}

func otlpConfiguration(endpoint string) map[string]any {
	return map[string]any{
		"audit_log_forward_otlp_endpoint":       endpoint,
		"audit_log_forward_otlp_authentication": "none",
		"audit_log_forward_otlp_username":       "",
		"audit_log_forward_otlp_password":       "",
		"gdpr_audit_logs":                       false,
	}
}

func pullManifestWithSource(ctx context.Context, ref, clientIP, userAgent string) (context.Context, error) {
	s := state.Get(ctx)
	_, project, repo, tag, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	path := fmt.Sprintf("/v2/%s/%s/manifests/%s", project, repo, tag)
	resp, err := doJSONWithSource(s.Client, http.MethodGet, path, nil, clientIP, userAgent, false)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if err := harborclient.Expect(resp, http.StatusOK); err != nil {
		return ctx, fmt.Errorf("pull manifest %s: %w", ref, err)
	}
	return ctx, nil
}

func doJSONWithSource(c *harborclient.Client, method, path string, body any, clientIP, userAgent string, auth bool) (*harborclient.Response, error) {
	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		requestBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	if auth {
		req.SetBasicAuth(c.Username, c.Password)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Forwarded-For", clientIP)
	req.Header.Set("User-Agent", userAgent)
	if method == http.MethodGet && strings.Contains(path, "/manifests/") {
		req.Header.Set("Accept", manifestAccept)
	}

	response, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return &harborclient.Response{StatusCode: response.StatusCode, Header: response.Header, Body: responseBody}, nil
}

func collectorReceivesAuditSource(ctx context.Context, eventName, clientIP, userAgent string) error {
	return waitForOTLPRecord(ctx, state.Get(ctx), eventName, func(record otlpreceiver.Record) error {
		return requireAuditAttributes(record, map[string]string{
			"client.address":      clientIP,
			"user_agent.original": userAgent,
		})
	})
}

func collectorReceivesPullAudit(ctx context.Context, ref, clientIP, userAgent string) error {
	s := state.Get(ctx)
	_, project, repo, tag, err := registryRef(s, ref)
	if err != nil {
		return err
	}
	return waitForOTLPRecord(ctx, s, "harbor.audit.pull_artifact", func(record otlpreceiver.Record) error {
		return requireAuditAttributes(record, map[string]string{
			"client.address":          clientIP,
			"user_agent.original":     userAgent,
			"oci.artifact.repository": project + "/" + repo,
			"oci.artifact.tag":        tag,
			"oci.manifest.digest":     s.PushedDigest,
		})
	})
}

func waitForOTLPRecord(ctx context.Context, s *state.Scenario, eventName string, validate func(otlpreceiver.Record) error) error {
	if s.OTLPReceiver == nil {
		return fmt.Errorf("no OTLP collector staged")
	}
	timer := time.NewTimer(otlpAuditTimeout)
	defer timer.Stop()
	var validationErr error
	for {
		select {
		case record := <-s.OTLPReceiver.Records():
			if record.EventName != eventName {
				continue
			}
			if err := validate(record); err == nil {
				return nil
			} else {
				validationErr = err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if validationErr != nil {
				return fmt.Errorf("OTLP event %q had unexpected attributes: %w", eventName, validationErr)
			}
			return fmt.Errorf("OTLP event %q not received within %s", eventName, otlpAuditTimeout)
		}
	}
}

func requireAuditAttributes(record otlpreceiver.Record, expected map[string]string) error {
	for key, want := range expected {
		if got := record.Attributes[key]; got != want {
			return fmt.Errorf("attribute %q = %q, want %q", key, got, want)
		}
	}
	return nil
}
