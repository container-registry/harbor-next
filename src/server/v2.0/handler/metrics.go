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

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	operations "github.com/goharbor/harbor/src/server/v2.0/restapi/operations/metrics"
)

func newMetricsAPI() *metricsAPI {
	return &metricsAPI{
		handler: promhttp.Handler(),
	}
}

type metricsAPI struct {
	BaseAPI
	handler http.Handler
}

func (m *metricsAPI) GetMetrics(ctx context.Context, params operations.GetMetricsParams) middleware.Responder {
	// Without this the endpoint would serve the Go runtime collectors that are
	// always on the default registry, on every deployment that never asked for
	// metrics at all.
	if !config.Metric().Enabled {
		return m.SendError(ctx, errors.New(nil).WithCode(errors.NotFoundCode).
			WithMessage("metrics are not enabled"))
	}
	// Unlike the dedicated metrics port, this path is reachable through the
	// public /api/ proxy route, and the metrics carry data the API withholds
	// from anonymous callers (harbor_system_info labels the Harbor version and
	// git commit, which /systeminfo only returns to authenticated callers).
	if err := m.RequireSystemAccess(ctx, rbac.ActionRead, rbac.ResourceMetric); err != nil {
		return m.SendError(ctx, err)
	}
	// Bypass the generated text/plain producer: promhttp already negotiates
	// gzip, streams the body and sets the versioned Prometheus content type.
	return middleware.ResponderFunc(func(w http.ResponseWriter, _ runtime.Producer) {
		m.handler.ServeHTTP(w, params.HTTPRequest.WithContext(ctx))
	})
}
