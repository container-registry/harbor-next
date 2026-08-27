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
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/version"
)

// SystemInfoCollectorName ...
const (
	systemInfoCollectorName = "SystemInfoCollector"
	sysInfoURL              = "/api/v2.0/systeminfo"
)

var (
	harborSysInfo = typedDesc{
		desc: newDescWithLabels("", "system_info", "Information of Harbor system",
			"auth_mode",
			"harbor_version",
			"self_registration"),
		valueType: prometheus.GaugeValue,
	}
)

// NewSystemInfoCollector ...
func NewSystemInfoCollector(backend Backend) *SystemInfoCollector {
	return &SystemInfoCollector{
		backend: backend,
	}
}

// SystemInfoCollector ...
type SystemInfoCollector struct {
	backend Backend
}

// Describe implements prometheus.Collector
func (hc *SystemInfoCollector) Describe(c chan<- *prometheus.Desc) {
	c <- harborSysInfo.Desc()
}

// Collect implements prometheus.Collector
func (hc *SystemInfoCollector) Collect(c chan<- prometheus.Metric) {
	for _, m := range hc.getSysInfo() {
		c <- m
	}
}

// GetName returns the name of the system info collector
func (hc *SystemInfoCollector) GetName() string {
	return systemInfoCollectorName
}

func (hc *SystemInfoCollector) getSysInfo() []prometheus.Metric {
	if CacheEnabled() {
		value, ok := CacheGet(systemInfoCollectorName)
		if ok {
			return value.([]prometheus.Metric)
		}
	}
	result := []prometheus.Metric{}

	// Get version directly from package (set at build time via ldflags)
	harborVersion := version.ReleaseVersion
	if version.GitCommit != "" {
		harborVersion = fmt.Sprintf("%s-%s", version.ReleaseVersion, version.GitCommit)
	}

	// Still look up auth_mode and self_registration (dynamic config)
	sysInfoResponse, err := hc.backend.SystemInfo(context.Background())
	if err != nil {
		log.Errorf("request system info failed with err: %v", err)
		return result
	}
	result = append(result, harborSysInfo.MustNewConstMetric(1,
		sysInfoResponse.AuthMode,
		harborVersion,
		strconv.FormatBool(sysInfoResponse.SelfRegistration)))
	if CacheEnabled() {
		CachePut(systemInfoCollectorName, result)
	}
	return result
}

type responseSysInfo struct {
	AuthMode         string `json:"auth_mode"`
	HarborVersion    string `json:"harbor_version"`
	SelfRegistration bool   `json:"self_registration"`
}
