//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/state"
)

func registerWebhook(sc *godog.ScenarioContext) {
	// Given
	sc.Given(`^an in-process webhook listener$`, inProcessWebhookListener)
	sc.Given(`^a webhook policy on "([^"]+)" for push events targeting the listener$`, webhookPolicyForPushEvents)
	sc.Given(`^the policy is disabled$`, policyDisabled)

	// When
	sc.When(`^the listener receives the event$`, listenerReceivesEvent)

	// Then
	sc.Then(`^the listener receives a push event whose digest matches the pushed artifact$`, listenerReceivesPushMatching)
	sc.Then(`^the listener receives no event within (\d+) seconds$`, listenerReceivesNothing)
	sc.Then(`^the webhook delivery history lists at least one successful job$`, deliveryHistorySuccessful)
}

// ============================================================================
// Given
// ============================================================================

func inProcessWebhookListener(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	s.WebhookEvents = make(chan state.WebhookEvent, 16)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		evt := state.WebhookEvent{
			Type:   r.Header.Get("X-Harbor-Event-Type"),
			Body:   body,
			Header: r.Header.Clone(),
		}
		select {
		case s.WebhookEvents <- evt:
		default:
		}
		w.WriteHeader(http.StatusOK)
	})

	// Bind on all interfaces so a remote Harbor (e.g. running in a k3s pod
	// while the tests run on the cluster host) can reach back to us. The
	// default httptest.Server binds 127.0.0.1 which is unreachable from
	// anywhere but the test process itself.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return ctx, fmt.Errorf("bind webhook listener: %w", err)
	}
	ts := httptest.NewUnstartedServer(handler)
	_ = ts.Listener.Close()
	ts.Listener = listener
	ts.Start()

	// Rewrite the advertised URL so Harbor gets a host it can route to.
	// Priority: E2E_WEBHOOK_HOST env → auto-detected outbound IP to Harbor
	// → whatever httptest chose (127.0.0.1, localhost-only fallback).
	port := listener.Addr().(*net.TCPAddr).Port
	host := resolveWebhookHost(s.Client.BaseURL)
	if host != "" {
		ts.URL = fmt.Sprintf("http://%s:%d", host, port)
	}
	s.HTTPMock = ts
	return ctx, nil
}

// resolveWebhookHost returns the host/IP that the target Harbor should use to
// call back to this test process. `E2E_WEBHOOK_HOST` wins when set (useful
// for overriding with a tunnel address). Otherwise we open a UDP "connection"
// to the Harbor URL — no packet is sent, but the kernel assigns a local
// address on the interface that would be used to reach that target, which is
// almost always the right answer.
func resolveWebhookHost(harborURL string) string {
	if h := os.Getenv("E2E_WEBHOOK_HOST"); h != "" {
		return h
	}
	u, err := url.Parse(harborURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := net.Dial("udp", net.JoinHostPort(host, port))
	if err != nil {
		return ""
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local == nil {
		return ""
	}
	return local.IP.String()
}

func webhookPolicyForPushEvents(ctx context.Context, alias string) (context.Context, error) {
	s := state.Get(ctx)
	if s.HTTPMock == nil {
		return ctx, fmt.Errorf("no listener staged — missing 'an in-process webhook listener'")
	}
	name := projectName(s, alias)
	body := webhookPolicyBody(name, s.HTTPMock.URL, []string{"PUSH_ARTIFACT"}, true)
	resp, err := s.Client.Post(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies", name), body)
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}
	if resp.StatusCode != 201 {
		return ctx, fmt.Errorf("create webhook policy: %d %s", resp.StatusCode, truncate(resp.Body))
	}
	if id, ok := idFromLocation(resp.Header.Get("Location")); ok {
		s.CreatedWebhookIDs[name] = append(s.CreatedWebhookIDs[name], id)
	}
	return ctx, nil
}

func policyDisabled(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	project, id, ok := latestWebhookID(s)
	if !ok {
		return ctx, fmt.Errorf("no webhook policy staged in scenario")
	}
	// Fetch current policy body and toggle enabled=false.
	resp, err := s.Client.Get(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies/%d", project, id))
	if err != nil {
		return ctx, err
	}
	var policy map[string]any
	_ = json.Unmarshal(resp.Body, &policy)
	policy["enabled"] = false
	putResp, err := s.Client.Put(fmt.Sprintf("/api/v2.0/projects/%s/webhook/policies/%d", project, id), policy)
	captureResp(s, putResp, err)
	if err != nil {
		return ctx, err
	}
	if putResp.StatusCode != 200 {
		return ctx, fmt.Errorf("disable webhook: %d %s", putResp.StatusCode, truncate(putResp.Body))
	}
	return ctx, nil
}

// ============================================================================
// When
// ============================================================================

func listenerReceivesEvent(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	select {
	case <-s.WebhookEvents:
		return ctx, nil
	case <-time.After(15 * time.Second):
		return ctx, fmt.Errorf("listener did not receive any event within 15s")
	}
}

// ============================================================================
// Then
// ============================================================================

func listenerReceivesPushMatching(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastImageErr != nil {
		return fmt.Errorf("prior push failed (webhook cannot fire): %v", s.LastImageErr)
	}
	if s.PushedDigest == "" {
		return fmt.Errorf("no pushed digest recorded — push step did not run")
	}
	deadline := time.After(15 * time.Second)
	for {
		select {
		case evt := <-s.WebhookEvents:
			var payload map[string]any
			_ = json.Unmarshal(evt.Body, &payload)
			typ, _ := payload["type"].(string)
			if !strings.Contains(strings.ToUpper(typ), "PUSH") {
				continue
			}
			ev, _ := payload["event_data"].(map[string]any)
			res, _ := ev["resources"].([]any)
			for _, r := range res {
				m, _ := r.(map[string]any)
				if d, _ := m["digest"].(string); d == s.PushedDigest {
					return nil
				}
			}
			// Not matching — keep listening.
		case <-deadline:
			return fmt.Errorf("no matching push event with digest %s within 15s", s.PushedDigest)
		}
	}
}

func listenerReceivesNothing(ctx context.Context, secs int) error {
	s := state.Get(ctx)
	select {
	case evt := <-s.WebhookEvents:
		return fmt.Errorf("received unexpected event: %s", truncate(evt.Body))
	case <-time.After(time.Duration(secs) * time.Second):
		return nil
	}
}

func deliveryHistorySuccessful(ctx context.Context) error {
	s := state.Get(ctx)
	project, id, ok := latestWebhookID(s)
	if !ok {
		return fmt.Errorf("no webhook policy staged")
	}
	// Harbor v2 exposes delivery history under .../webhook/policies/{id}/executions
	// (each execution has Status plus nested tasks); the older .../jobs path
	// was removed. We accept any terminal success-like status.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := s.Client.Get(fmt.Sprintf(
			"/api/v2.0/projects/%s/webhook/policies/%d/executions?page=1&page_size=20", project, id))
		if err != nil {
			return err
		}
		if resp.StatusCode == 200 {
			var execs []struct {
				Status  string `json:"status"`
				Metrics *struct {
					SuccessTaskCount int64 `json:"success_task_count"`
				} `json:"metrics"`
			}
			_ = json.Unmarshal(resp.Body, &execs)
			for _, e := range execs {
				if isWebhookSuccess(e.Status) || e.Metrics != nil && e.Metrics.SuccessTaskCount > 0 {
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("no successful webhook delivery recorded within 30s")
}

func isWebhookSuccess(s string) bool {
	switch strings.ToLower(s) {
	case "success", "succeed", "finished":
		return true
	}
	return false
}

// ============================================================================
// helpers
// ============================================================================

func latestWebhookID(s *state.Scenario) (string, int64, bool) {
	for project, ids := range s.CreatedWebhookIDs {
		if len(ids) == 0 {
			continue
		}
		return project, ids[len(ids)-1], true
	}
	return "", 0, false
}
