// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-openapi/runtime/middleware"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/controller/config"
	"github.com/goharbor/harbor/src/lib"
	cfgModels "github.com/goharbor/harbor/src/lib/config/models"
	"github.com/goharbor/harbor/src/lib/errors"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi/operations/configure"
)

type configAPI struct {
	BaseAPI
	controller config.Controller
}

func newConfigAPI() *configAPI {
	return &configAPI{controller: config.Ctl}
}

func (c *configAPI) GetConfigurations(ctx context.Context, _ configure.GetConfigurationsParams) middleware.Responder {
	if sec, exist := security.FromContext(ctx); exist {
		if sec.IsSolutionUser() {
			cfg, err := c.controller.AllConfigs(ctx)
			if err != nil {
				return c.SendError(ctx, err)
			}
			var result []byte
			result, err = json.Marshal(cfg)
			if err != nil {
				return c.SendError(ctx, err)
			}
			var cfgResp models.ConfigurationsResponse
			err = json.Unmarshal(result, &cfgResp)
			if err != nil {
				return c.SendError(ctx, err)
			}
			return configure.NewGetConfigurationsOK().WithPayload(&cfgResp)
		}
	}
	if err := c.RequireSystemAccess(ctx, rbac.ActionRead, rbac.ResourceConfiguration); err != nil {
		return c.SendError(ctx, err)
	}
	cfg, err := c.controller.UserConfigs(ctx)
	if err != nil {
		return c.SendError(ctx, err)
	}
	payload, err := toResponseModel(cfg)
	if err != nil {
		c.SendError(ctx, err)
	}
	return configure.NewGetConfigurationsOK().WithPayload(payload)
}

func (c *configAPI) UpdateConfigurations(ctx context.Context, params configure.UpdateConfigurationsParams) middleware.Responder {
	if err := c.RequireSystemAccess(ctx, rbac.ActionUpdate, rbac.ResourceConfiguration); err != nil {
		return c.SendError(ctx, err)
	}
	if params.Configurations == nil {
		return c.SendError(ctx, errors.BadRequestError(nil).WithMessage("Missing configuration item"))
	}
	if keys, _ := ctx.Value(featureKeysKey{}).([]string); len(keys) > 0 {
		return c.SendError(ctx, errors.BadRequestError(nil).
			WithMessagef("%s is a feature toggle: use PUT /api/v2.0/features/%s", keys[0], keys[0]))
	}
	conf := params.Configurations
	cfgMap, err := toCfgMap(conf)
	if err != nil {
		return c.SendError(ctx, err)
	}
	err = c.controller.UpdateUserConfigs(ctx, cfgMap)
	if err != nil {
		return c.SendError(ctx, err)
	}
	return configure.NewUpdateConfigurationsOK()
}

type featureKeysKey struct{}

// maxConfigBodySize is far above any real configuration update.
const maxConfigBodySize = 1 << 20

// captureFeatureKeys records the feature keys a configuration update names,
// since binding the typed body would drop them and answer 200.
func captureFeatureKeys(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := lib.ReadRequestBody(r, maxConfigBodySize)
		if err != nil {
			lib_http.SendError(w, err)
			return
		}
		// a malformed body is left to the binding to report
		var raw map[string]json.RawMessage
		if json.Unmarshal(body, &raw) == nil {
			var keys []string
			for _, def := range feature.Definitions() {
				if _, ok := raw[def.ConfigKey]; ok {
					keys = append(keys, def.ConfigKey)
				}
			}
			if len(keys) > 0 {
				sort.Strings(keys)
				r = r.WithContext(context.WithValue(r.Context(), featureKeysKey{}, keys))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func toCfgMap(conf *models.Configurations) (map[string]any, error) {
	var cfgMap map[string]any
	buf, err := json.Marshal(conf)
	if err != nil {
		return cfgMap, err
	}
	err = json.Unmarshal(buf, &cfgMap)
	if err != nil {
		return cfgMap, err
	}
	return cfgMap, nil
}

func (c *configAPI) GetInternalconfig(ctx context.Context, _ configure.GetInternalconfigParams) middleware.Responder {
	if err := c.RequireSolutionUserAccess(ctx); err != nil {
		return c.SendError(ctx, err)
	}
	cfg, err := c.controller.AllConfigs(ctx)
	if err != nil {
		return c.SendError(ctx, err)
	}
	resultCfg, err := c.controller.ConvertForGet(ctx, cfg, true)
	if err != nil {
		return c.SendError(ctx, err)
	}

	payload := make(models.InternalConfigurationsResponse, len(resultCfg))
	for key, cfg := range resultCfg {
		payload[key] = models.InternalConfigurationValue{Value: cfg.Val, Editable: cfg.Editable}
	}

	return configure.NewGetInternalconfigOK().WithPayload(payload)
}

func toResponseModel(cfg map[string]*cfgModels.Value) (*models.ConfigurationsResponse, error) {
	var result []byte
	result, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var cfgResp models.ConfigurationsResponse
	err = json.Unmarshal(result, &cfgResp)
	if err != nil {
		return nil, err
	}
	return &cfgResp, nil
}
