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

package metric

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

var (
	meterMu       sync.Mutex
	meterProvider *sdkmetric.MeterProvider
)

// InitMeterProvider sets up the OTel-to-Prometheus metrics bridge.
// The Prometheus exporter auto-registers with prometheus.DefaultRegisterer,
// so metrics appear on the existing /metrics endpoint served by
// promhttp.Handler(). Must be called before any code that creates OTel
// instruments against otel.GetMeterProvider() (dbpool.NewTracer and
// dbpool.RegisterPoolMetrics), otherwise those instruments attach to the
// no-op provider and emit nothing. Safe to call multiple times.
func InitMeterProvider() error {
	meterMu.Lock()
	defer meterMu.Unlock()
	if meterProvider != nil {
		return nil
	}
	exporter, err := promexporter.New(
		promexporter.WithoutTargetInfo(),
		promexporter.WithoutScopeInfo(),
	)
	if err != nil {
		return fmt.Errorf("metric: init prometheus exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meterProvider = mp
	otel.SetMeterProvider(mp)
	return nil
}

// ShutdownMeterProvider flushes pending metrics, releases resources, and
// restores the global OTel MeterProvider to the no-op implementation so
// subsequent instrument reads do not write into the shut-down provider.
// Safe to call before InitMeterProvider (no-op in that case) and safe to
// call followed by another InitMeterProvider to re-init.
func ShutdownMeterProvider(ctx context.Context) error {
	meterMu.Lock()
	mp := meterProvider
	meterProvider = nil
	meterMu.Unlock()
	if mp == nil {
		return nil
	}
	otel.SetMeterProvider(noopmetric.NewMeterProvider())
	return mp.Shutdown(ctx)
}
