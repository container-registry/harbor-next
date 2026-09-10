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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/goharbor/harbor/src/controller/systemquota"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
)

// SystemQuotaCollectorName is the name of the global storage quota collector.
const SystemQuotaCollectorName = "SystemQuotaCollector"

var (
	systemQuotaHard = typedDesc{
		desc:      newDescWithLabels("", "system_quota_hard_bytes", "Global storage quota limit in bytes"),
		valueType: prometheus.GaugeValue,
	}
	systemQuotaUsed = typedDesc{
		desc:      newDescWithLabels("", "system_quota_used_bytes", "Global storage usage in bytes", "source"),
		valueType: prometheus.GaugeValue,
	}
	systemQuotaEnforce = typedDesc{
		desc:      newDescWithLabels("", "system_quota_enforce", "Whether the global storage quota is enforced on registry writes (1) or advisory (0)"),
		valueType: prometheus.GaugeValue,
	}
)

// SystemQuotaCollector exposes the global storage quota. It reads the
// controller directly and therefore only works when the collectors run inside
// core, like the statistics collector.
type SystemQuotaCollector struct {
	ctl    systemquota.Controller
	newCtx func() context.Context
}

// NewSystemQuotaCollector ...
func NewSystemQuotaCollector() *SystemQuotaCollector {
	return &SystemQuotaCollector{ctl: systemquota.Ctl, newCtx: orm.Context}
}

// GetName ...
func (c SystemQuotaCollector) GetName() string {
	return SystemQuotaCollectorName
}

// Describe ...
func (c SystemQuotaCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- systemQuotaHard.Desc()
	ch <- systemQuotaUsed.Desc()
	ch <- systemQuotaEnforce.Desc()
}

// Collect ...
func (c SystemQuotaCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range c.getMetrics() {
		ch <- m
	}
}

func (c SystemQuotaCollector) getMetrics() []prometheus.Metric {
	if CacheEnabled() {
		if value, ok := CacheGet(SystemQuotaCollectorName); ok {
			return value.([]prometheus.Metric)
		}
	}
	status, err := c.ctl.Get(c.newCtx())
	if err != nil {
		// No quota configured is the normal state; the gauges are simply absent.
		if !errors.IsNotFoundErr(err) {
			log.Errorf("get global storage quota error: %v", err)
		}
		return nil
	}
	enforce := 0.0
	if status.Enforce {
		enforce = 1
	}
	result := []prometheus.Metric{
		systemQuotaHard.MustNewConstMetric(float64(status.Hard)),
		systemQuotaUsed.MustNewConstMetric(float64(status.Used), status.UsedSource),
		systemQuotaEnforce.MustNewConstMetric(enforce),
	}
	if CacheEnabled() {
		CachePut(SystemQuotaCollectorName, result)
	}
	return result
}
