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
	"sort"
	"strings"

	"github.com/go-openapi/runtime/middleware"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/controller/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	"github.com/goharbor/harbor/src/server/v2.0/restapi/operations/configure"
)

const configOverwriteJSON = "CONFIG_OVERWRITE_JSON"

func (c *configAPI) ListFeatures(ctx context.Context, _ configure.ListFeaturesParams) middleware.Responder {
	if err := c.RequireSystemAccess(ctx, rbac.ActionRead, rbac.ResourceConfiguration); err != nil {
		return c.SendError(ctx, err)
	}
	return configure.NewListFeaturesOK().WithPayload(featureList(ctx, config.OverwrittenByEnv()))
}

func (c *configAPI) UpdateFeature(ctx context.Context, params configure.UpdateFeatureParams) middleware.Responder {
	if err := c.RequireSystemAccess(ctx, rbac.ActionUpdate, rbac.ResourceConfiguration); err != nil {
		return c.SendError(ctx, err)
	}
	if _, _, known := feature.Resolve(ctx, params.FeatureKey); !known {
		return c.SendError(ctx, errors.NotFoundError(nil).WithMessagef("feature %s not found", params.FeatureKey))
	}
	if params.Feature == nil || params.Feature.Enabled == nil {
		return c.SendError(ctx, errors.BadRequestError(nil).WithMessage("enabled is required"))
	}
	// Going through the config controller keeps the env lock, the disable
	// guards, CONFIG_OVERWRITE_JSON and the change notification in one place.
	if err := c.controller.UpdateUserConfigs(ctx, map[string]any{params.FeatureKey: *params.Feature.Enabled}); err != nil {
		return c.SendError(ctx, err)
	}
	return configure.NewUpdateFeatureOK()
}

func featureList(ctx context.Context, overwritten bool) *models.FeatureList {
	// Registration order follows Go package initialization, which users
	// should not see; sort by display name for a stable list.
	definitions := feature.Definitions()
	sort.SliceStable(definitions, func(i, j int) bool {
		return strings.ToLower(definitions[i].Name) < strings.ToLower(definitions[j].Name)
	})
	enabledBySection := map[string][]feature.Definition{}
	features := make([]*models.Feature, 0, len(definitions))
	for _, def := range definitions {
		enabled, _, _ := feature.Resolve(ctx, def.ConfigKey)
		if enabled {
			enabledBySection[def.Section] = append(enabledBySection[def.Section], def)
		}
		lockedBy := ""
		if envKey, overridden := feature.EnvironmentKey(def.ConfigKey); overridden {
			lockedBy = envKey
		} else if overwritten {
			lockedBy = configOverwriteJSON
		}
		features = append(features, &models.Feature{
			Key:         def.ConfigKey,
			Section:     def.Section,
			Name:        def.Name,
			Description: def.Description,
			Beta:        def.Beta,
			Enabled:     enabled,
			Editable:    lockedBy == "",
			LockedBy:    lockedBy,
		})
	}
	sections := make([]*models.FeatureSection, 0)
	for _, section := range feature.Sections() {
		link := ""
		if section.Link != nil {
			link = section.Link(ctx, enabledBySection[section.ID])
		}
		sections = append(sections, &models.FeatureSection{
			ID:          section.ID,
			Title:       section.Title,
			Description: section.Description,
			LinkText:    section.LinkText,
			Link:        link,
		})
	}
	return &models.FeatureList{Sections: sections, Features: features}
}
