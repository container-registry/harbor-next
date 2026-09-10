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

package exporter

import (
	"context"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/controller/systemquota"
	"github.com/goharbor/harbor/src/lib/errors"
	systemquotatesting "github.com/goharbor/harbor/src/testing/controller/systemquota"
	"github.com/goharbor/harbor/src/testing/mock"
)

func TestSystemQuotaCollectorNoQuota(t *testing.T) {
	ctl := &systemquotatesting.Controller{}
	ctl.On("Get", mock.Anything).Return(nil, errors.NotFoundError(nil))
	c := SystemQuotaCollector{ctl: ctl, newCtx: context.Background}
	assert.Empty(t, c.getMetrics())
}

func TestSystemQuotaCollector(t *testing.T) {
	ctl := &systemquotatesting.Controller{}
	ctl.On("Get", mock.Anything).Return(&systemquota.Status{Hard: 1000, Used: 250, UsedSource: "accounted", Enforce: true}, nil)
	c := SystemQuotaCollector{ctl: ctl, newCtx: context.Background}

	metrics := c.getMetrics()
	if !assert.Len(t, metrics, 3) {
		return
	}
	values := make([]float64, 0, 3)
	for _, m := range metrics {
		d := &dto.Metric{}
		assert.NoError(t, m.Write(d))
		values = append(values, d.Gauge.GetValue())
		if len(d.Label) > 0 {
			assert.Equal(t, "source", d.Label[0].GetName())
			assert.Equal(t, "accounted", d.Label[0].GetValue())
		}
	}
	assert.Equal(t, []float64{1000, 250, 1}, values)
}
