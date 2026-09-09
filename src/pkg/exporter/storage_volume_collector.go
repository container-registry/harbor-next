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

	si "github.com/goharbor/harbor/src/controller/systeminfo"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
)

// StorageVolumeCollectorName is the name of the registry volume collector.
const StorageVolumeCollectorName = "StorageVolumeCollector"

var (
	storageVolumeBytes = typedDesc{
		desc:      newDescWithLabels("", "storage_volume_bytes", "Size of the registry storage volume in bytes", "kind", "driver"),
		valueType: prometheus.GaugeValue,
	}
	storageVolumeMeasured = typedDesc{
		desc:      newDescWithLabels("", "storage_volume_measured", "Whether the volume numbers were measured on the registry volume through registryctl (1) or only reflect core's local disk (0)"),
		valueType: prometheus.GaugeValue,
	}
)

// StorageVolumeCollector exposes the registry storage volume as measured by
// registryctl (harbor-next #839, theme B). It reads the controller directly and
// therefore only works when the collectors run inside core.
type StorageVolumeCollector struct {
	ctl    si.Controller
	newCtx func() context.Context
}

// NewStorageVolumeCollector ...
func NewStorageVolumeCollector() *StorageVolumeCollector {
	return &StorageVolumeCollector{ctl: si.Ctl, newCtx: orm.Context}
}

// GetName ...
func (c StorageVolumeCollector) GetName() string {
	return StorageVolumeCollectorName
}

// Describe ...
func (c StorageVolumeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- storageVolumeBytes.Desc()
	ch <- storageVolumeMeasured.Desc()
}

// Collect ...
func (c StorageVolumeCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range c.getMetrics() {
		ch <- m
	}
}

func (c StorageVolumeCollector) getMetrics() []prometheus.Metric {
	if CacheEnabled() {
		if value, ok := CacheGet(StorageVolumeCollectorName); ok {
			return value.([]prometheus.Metric)
		}
	}
	capacity, err := c.ctl.GetCapacity(c.newCtx())
	if err != nil {
		log.Errorf("get storage volume capacity error: %v", err)
		return nil
	}
	// Fallback numbers describe the local disk of whatever process runs the
	// collector (core, or the standalone exporter), not the registry volume, so
	// only a measurement is published as volume bytes.
	result := []prometheus.Metric{storageVolumeMeasured.MustNewConstMetric(0)}
	if capacity.Measured {
		result = []prometheus.Metric{
			storageVolumeBytes.MustNewConstMetric(float64(capacity.Total), "total", capacity.Driver),
			storageVolumeBytes.MustNewConstMetric(float64(capacity.Free), "free", capacity.Driver),
			storageVolumeBytes.MustNewConstMetric(float64(capacity.Used), "used", capacity.Driver),
			storageVolumeMeasured.MustNewConstMetric(1),
		}
	}
	if CacheEnabled() {
		CachePut(StorageVolumeCollectorName, result)
	}
	return result
}
