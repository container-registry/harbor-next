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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/goharbor/harbor/src/lib/errors"
	operation "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/retention"
	retentiontesting "github.com/goharbor/harbor/src/testing/controller/retention"
)

func TestRetentionMissingPolicyReturnsNotFound(t *testing.T) {
	cases := map[string]func(*retentionAPI, context.Context) middleware.Responder{
		"get": func(api *retentionAPI, ctx context.Context) middleware.Responder {
			return api.GetRetention(ctx, operation.GetRetentionParams{ID: 99})
		},
		"delete": func(api *retentionAPI, ctx context.Context) middleware.Responder {
			return api.DeleteRetention(ctx, operation.DeleteRetentionParams{ID: 99})
		},
		"trigger": func(api *retentionAPI, ctx context.Context) middleware.Responder {
			return api.TriggerRetentionExecution(ctx, operation.TriggerRetentionExecutionParams{ID: 99})
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			ctl := retentiontesting.NewController(t)
			ctl.On("GetRetention", mock.Anything, int64(99)).
				Return(nil, errors.NotFoundError(nil).WithMessage("no such Retention policy with id 99"))

			rr := httptest.NewRecorder()
			call(&retentionAPI{retentionCtl: ctl}, context.Background()).WriteResponse(rr, runtime.JSONProducer())
			assert.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}
