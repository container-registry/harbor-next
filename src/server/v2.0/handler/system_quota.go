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

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/controller/systemquota"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/quota/types"
	"github.com/goharbor/harbor/src/server/v2.0/models"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/systemquota"
)

func newSystemQuotaAPI() *systemQuotaAPI {
	return &systemQuotaAPI{ctl: systemquota.Ctl}
}

type systemQuotaAPI struct {
	BaseAPI
	ctl systemquota.Controller
}

func (s *systemQuotaAPI) GetSystemQuota(ctx context.Context, _ operation.GetSystemQuotaParams) middleware.Responder {
	if err := s.RequireSystemAccess(ctx, rbac.ActionRead, rbac.ResourceSystemQuota); err != nil {
		return s.SendError(ctx, err)
	}
	status, err := s.ctl.Get(ctx)
	if err != nil {
		return s.SendError(ctx, err)
	}
	return operation.NewGetSystemQuotaOK().WithPayload(toSystemQuota(status))
}

func (s *systemQuotaAPI) UpdateSystemQuota(ctx context.Context, params operation.UpdateSystemQuotaParams) middleware.Responder {
	if err := s.RequireSystemAccess(ctx, rbac.ActionUpdate, rbac.ResourceSystemQuota); err != nil {
		return s.SendError(ctx, err)
	}
	// go-swagger validates the required fields; the guards keep a malformed
	// body from panicking should that validation ever be bypassed.
	if params.Quota == nil || params.Quota.Hard == nil || params.Quota.Hard.Storage == nil || params.Quota.Enforce == nil {
		return s.SendError(ctx, errors.BadRequestError(nil).WithMessage("hard.storage and enforce are required"))
	}
	if err := s.ctl.Update(ctx, *params.Quota.Hard.Storage, *params.Quota.Enforce); err != nil {
		return s.SendError(ctx, err)
	}
	return operation.NewUpdateSystemQuotaOK()
}

func (s *systemQuotaAPI) DeleteSystemQuota(ctx context.Context, _ operation.DeleteSystemQuotaParams) middleware.Responder {
	if err := s.RequireSystemAccess(ctx, rbac.ActionDelete, rbac.ResourceSystemQuota); err != nil {
		return s.SendError(ctx, err)
	}
	if err := s.ctl.Delete(ctx); err != nil {
		return s.SendError(ctx, err)
	}
	return operation.NewDeleteSystemQuotaOK()
}

func toSystemQuota(status *systemquota.Status) *models.SystemQuota {
	res := &models.SystemQuota{
		Hard:              models.ResourceList{string(types.ResourceStorage): status.Hard},
		Used:              models.ResourceList{string(types.ResourceStorage): status.Used},
		Free:              status.Free,
		UsedSource:        status.UsedSource,
		Allocated:         status.Allocated,
		UnlimitedProjects: status.UnlimitedProjects,
		Enforce:           status.Enforce,
		UpdateTime:        strfmt.DateTime(status.UpdateTime),
	}
	if status.MeasuredAt != nil {
		res.MeasuredAt = strfmt.DateTime(*status.MeasuredAt)
	}
	return res
}
