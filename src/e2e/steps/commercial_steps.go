//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/state"
)

func registerCommercial(sc *godog.ScenarioContext) {
	sc.When(`^the admin opens the Commercial configuration$`, adminOpensCommercialConfig)
	sc.Then(`^project-level identity providers are configurable$`, projectIdentityProvidersConfigurable)
}

func adminOpensCommercialConfig(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	resp, err := s.Client.Get("/api/v2.0/configurations")
	captureResp(s, resp, err)
	return ctx, err
}

func projectIdentityProvidersConfigurable(ctx context.Context) error {
	s := state.Get(ctx)
	if err := mustStatus(s.LastResp, http.StatusOK); err != nil {
		return err
	}

	var cfg map[string]struct {
		Value    any  `json:"value"`
		Editable bool `json:"editable"`
	}
	if err := json.Unmarshal(s.LastBody, &cfg); err != nil {
		return fmt.Errorf("decode configurations: %w", err)
	}

	field, ok := cfg["enable_project_federated_idp"]
	if !ok {
		return fmt.Errorf("configuration missing enable_project_federated_idp")
	}
	if _, ok := field.Value.(bool); !ok {
		return fmt.Errorf("enable_project_federated_idp value has type %T, want bool", field.Value)
	}
	if !field.Editable {
		return fmt.Errorf("enable_project_federated_idp is not editable")
	}
	return nil
}
