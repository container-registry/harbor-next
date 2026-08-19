//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/harborclient"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

// registerBranding installs step definitions that exercise the
// /api/v2.0/systeminfo/branding endpoint added by patch 0001.
func registerBranding(sc *godog.ScenarioContext) {
	sc.When(`^I request the branding configuration$`, requestBrandingConfig)
	sc.When(`^I update the branding product name to "([^"]+)"$`, updateBrandingProductName)
	sc.When(`^I update the branding product name to "([^"]+)" as user "([^"]+)"$`, updateBrandingProductNameAsUser)
	sc.Then(`^the branding config contains a "([^"]+)" field$`, brandingContainsField)
	sc.Then(`^the branding product name is "([^"]+)"$`, brandingProductNameIs)
}

// ============================================================================
// When
// ============================================================================

func requestBrandingConfig(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Get("/api/v2.0/systeminfo/branding")
	captureResp(s, resp, err)
	if err != nil {
		return ctx, err
	}

	// Decode into a generic map for later assertions.
	var cfg map[string]any
	if jsonErr := json.Unmarshal(resp.Body, &cfg); jsonErr == nil {
		s.Mu.Lock()
		s.LastBrandingConfig = cfg
		s.Mu.Unlock()
	}
	return ctx, nil
}

func updateBrandingProductName(ctx context.Context, name string) (context.Context, error) {
	s := state.Get(ctx)
	return ctx, postBrandingProductName(s, s.Client, name)
}

func updateBrandingProductNameAsUser(ctx context.Context, name, alias string) (context.Context, error) {
	s := state.Get(ctx)
	u, ok := s.UsersByAlias[alias]
	if !ok {
		return ctx, fmt.Errorf("no user %q registered in scenario", alias)
	}
	uc := s.Client.WithCredentials(u.Name, u.Password)
	return ctx, postBrandingProductName(s, uc, name)
}

// postBrandingProductName issues POST /api/v2.0/systeminfo/branding with the
// given product name. On the first admin call it also registers a teardown
// that restores the original product name.
func postBrandingProductName(s *state.Scenario, c *harborclient.Client, name string) error {
	// Capture original on first update so teardown can restore it.
	s.Mu.Lock()
	if s.BrandingRestore == nil && s.LastBrandingConfig != nil {
		orig := s.LastBrandingConfig
		adminClient := s.Client
		s.BrandingRestore = func() error {
			_, err := adminClient.Post("/api/v2.0/systeminfo/branding", orig)
			return err
		}
	}
	s.Mu.Unlock()

	body := buildBrandingBody(name)
	resp, err := c.Post("/api/v2.0/systeminfo/branding", body)
	captureResp(s, resp, err)
	return err
}

// buildBrandingBody constructs a BrandingConfig payload with only the product
// name set. All other fields are omitted (nil/omitempty).
func buildBrandingBody(productName string) map[string]any {
	return map[string]any{
		"product": map[string]any{
			"name": productName,
		},
	}
}

// ============================================================================
// Then
// ============================================================================

func brandingContainsField(ctx context.Context, field string) error {
	s := state.Get(ctx)
	s.Mu.Lock()
	cfg := s.LastBrandingConfig
	s.Mu.Unlock()
	if cfg == nil {
		return fmt.Errorf("no branding config captured; run 'I request the branding configuration' first")
	}
	if _, ok := cfg[field]; !ok {
		return fmt.Errorf("branding config missing field %q; got keys: %v", field, mapKeys(cfg))
	}
	return nil
}

func brandingProductNameIs(ctx context.Context, want string) error {
	s := state.Get(ctx)
	s.Mu.Lock()
	cfg := s.LastBrandingConfig
	s.Mu.Unlock()
	if cfg == nil {
		return fmt.Errorf("no branding config captured; run 'I request the branding configuration' first")
	}
	product, ok := cfg["product"].(map[string]any)
	if !ok {
		return fmt.Errorf("branding config has no 'product' object; raw: %v", cfg)
	}
	got, _ := product["name"].(string)
	if got != want {
		return fmt.Errorf("branding product name %q (want %q)", got, want)
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
