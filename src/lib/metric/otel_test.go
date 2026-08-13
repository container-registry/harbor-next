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
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// resetMeterProviderForTest clears package-level state between tests so
// each case gets a fresh provider. Callers hold the lock while mutating.
func resetMeterProviderForTest(t *testing.T) {
	t.Helper()
	meterMu.Lock()
	meterProvider = nil
	meterMu.Unlock()
}

func TestInitMeterProvider_SetsGlobal(t *testing.T) {
	resetMeterProviderForTest(t)
	t.Cleanup(func() { _ = ShutdownMeterProvider(context.Background()) })

	require.NoError(t, InitMeterProvider())

	_, ok := otel.GetMeterProvider().(*sdkmetric.MeterProvider)
	require.True(t, ok, "global meter provider should be an sdkmetric.MeterProvider after Init")
}

func TestInitMeterProvider_Idempotent(t *testing.T) {
	resetMeterProviderForTest(t)
	t.Cleanup(func() { _ = ShutdownMeterProvider(context.Background()) })

	require.NoError(t, InitMeterProvider())
	first := otel.GetMeterProvider()

	require.NoError(t, InitMeterProvider())
	second := otel.GetMeterProvider()

	require.Same(t, first, second, "second InitMeterProvider must not replace the provider")
}

func TestShutdownMeterProvider_NoOpBeforeInit(t *testing.T) {
	resetMeterProviderForTest(t)

	require.NoError(t, ShutdownMeterProvider(context.Background()))
}

func TestShutdownMeterProvider_ResetsState(t *testing.T) {
	resetMeterProviderForTest(t)

	require.NoError(t, InitMeterProvider())
	first := meterProvider
	require.NotNil(t, first)

	require.NoError(t, ShutdownMeterProvider(context.Background()))

	// After shutdown, the package-level pointer must be cleared so that a
	// subsequent Init does not silently early-return and leave the global
	// pointing at the shut-down provider.
	meterMu.Lock()
	require.Nil(t, meterProvider, "shutdown must clear meterProvider")
	meterMu.Unlock()

	require.NoError(t, InitMeterProvider())
	require.NotSame(t, first, meterProvider, "re-init must create a new provider")

	t.Cleanup(func() { _ = ShutdownMeterProvider(context.Background()) })
}
