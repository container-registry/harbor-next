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
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/goharbor/harbor/src/lib/log"
)

// Opt is the config of the Harbor exporter collectors
type Opt struct {
	CacheDuration      int64
	CacheCleanInterval int64
}

// NewCollector builds the Harbor collectors against the given backend. Core
// runs them in-process and serves them from its own metrics endpoint.
func NewCollector(opt *Opt, backend Backend) prometheus.Collector {
	return newCollectorSet(opt, backend)
}

func newCollectorSet(opt *Opt, backend Backend) *Exporter {
	exporter := &Exporter{
		Opt:        opt,
		collectors: make(map[string]prometheus.Collector),
	}
	if opt.CacheDuration > 0 {
		CacheInit(opt)
	}
	err := exporter.RegisterCollector(NewHealthCollect(backend),
		NewSystemInfoCollector(backend),
		NewProjectCollector(),
		NewJobServiceCollector(backend),
		NewStatisticsCollector(),
	)
	if err != nil {
		log.Warningf("calling RegisterCollector() errored out, error: %v", err)
	}
	return exporter
}

// Exporter bundles the Harbor collectors into one prometheus.Collector
type Exporter struct {
	Opt        *Opt
	collectors map[string]prometheus.Collector
}

// RegisterCollector register a collector to exporter
func (e *Exporter) RegisterCollector(collectors ...collector) error {
	for _, c := range collectors {
		name := c.GetName()
		if _, ok := e.collectors[name]; ok {
			return errors.New("collector name is already registered")
		}
		e.collectors[name] = c
		log.Infof("collector %s registered ...", name)
	}
	return nil
}

// Describe implements prometheus.Collector
func (e *Exporter) Describe(c chan<- *prometheus.Desc) {
	for _, v := range e.collectors {
		v.Describe(c)
	}
}

// Collect implements prometheus.Collector
func (e *Exporter) Collect(c chan<- prometheus.Metric) {
	for _, v := range e.collectors {
		v.Collect(c)
	}
}

func checkErr(err error, arg string) {
	if err == nil {
		return
	}

	log.Errorf("%s: %v", arg, err)
}
