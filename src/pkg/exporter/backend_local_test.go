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
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/controller/health"
	si "github.com/goharbor/harbor/src/controller/systeminfo"
	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
)

type fakeHealthCtl struct {
	status *health.OverallHealthStatus
}

func (f *fakeHealthCtl) GetHealth(context.Context) *health.OverallHealthStatus {
	return f.status
}

type fakeSysInfoCtl struct {
	data   *si.Data
	err    error
	gotOpt si.Options
}

func (f *fakeSysInfoCtl) GetInfo(_ context.Context, opt si.Options) (*si.Data, error) {
	f.gotOpt = opt
	return f.data, f.err
}

func (f *fakeSysInfoCtl) GetCapacity(context.Context) (*imagestorage.Capacity, error) {
	return nil, nil
}

func (f *fakeSysInfoCtl) GetCA(context.Context) (io.ReadCloser, error) {
	return nil, nil
}

func TestLocalBackendHealth(t *testing.T) {
	b := &localBackend{
		healthCtl: &fakeHealthCtl{status: &health.OverallHealthStatus{
			Status: "unhealthy",
			Components: []*health.ComponentHealthStatus{
				{Name: "core", Status: "healthy"},
				{Name: "jobservice", Status: "unhealthy", Error: "boom"},
			},
		}},
	}

	got, err := b.Health(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", got.Status)
	assert.Equal(t, []responseComponent{
		{Name: "core", Status: "healthy"},
		{Name: "jobservice", Status: "unhealthy"},
	}, got.Components)
}

// The exporter must not ask for protected system info: harbor_version comes
// from the version package, and requesting protected data inside core would be
// a needless privilege escalation.
func TestLocalBackendSystemInfoDoesNotRequestProtectedInfo(t *testing.T) {
	ctl := &fakeSysInfoCtl{data: &si.Data{
		AuthMode:         "db_auth",
		SelfRegistration: true,
	}}
	b := &localBackend{sysCtl: ctl, newCtx: context.Background}

	got, err := b.SystemInfo(context.Background())
	require.NoError(t, err)
	assert.False(t, ctl.gotOpt.WithProtectedInfo)
	assert.Equal(t, "db_auth", got.AuthMode)
	assert.True(t, got.SelfRegistration)
}

func TestLocalBackendSystemInfoError(t *testing.T) {
	b := &localBackend{sysCtl: &fakeSysInfoCtl{err: errors.New("nope")}, newCtx: context.Background}

	got, err := b.SystemInfo(context.Background())
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestNewCollectorRegistersAllCollectors(t *testing.T) {
	e, ok := NewCollector(&Opt{}, NewLocalBackend()).(*Exporter)
	require.True(t, ok)

	for _, name := range []string{
		healthCollectorName,
		systemInfoCollectorName,
		ProjectCollectorName,
		JobServiceCollectorName,
		StatisticsCollectorName,
	} {
		_, found := e.collectors[name]
		assert.Truef(t, found, "collector %s not registered", name)
	}
}

// Repeated CacheInit must stay usable, and a later call must be able to change
// the cleaner cadence even though the goroutine is only started once.
func TestCacheInitIsRepeatable(t *testing.T) {
	CacheInit(&Opt{CacheDuration: 60, CacheCleanInterval: 7})
	assert.Equal(t, int64(7), cleanIntervalSec.Load())

	CacheInit(&Opt{CacheDuration: 60, CacheCleanInterval: 11})
	assert.Equal(t, int64(11), cleanIntervalSec.Load(), "a later CacheInit must retune the running cleaner")

	CacheInit(&Opt{CacheDuration: 60})
	assert.Equal(t, int64(defaultCacheCleanInterval), cleanIntervalSec.Load())

	assert.True(t, CacheEnabled())
	CachePut("k", "v")
	v, ok := CacheGet("k")
	assert.True(t, ok)
	assert.Equal(t, "v", v)
}

// StartCacheCleaner used to compare a seconds-based Expiration against
// UnixNano, so every tick emptied the whole cache and the configured cache
// duration was never actually reached.
func TestStartCacheCleanerKeepsUnexpiredEntries(t *testing.T) {
	CacheInit(&Opt{CacheDuration: 3600})

	CachePut("fresh", "value")
	c.Lock()
	c.store["stale"] = cachedValue{Value: "old", Expiration: time.Now().Unix() - 1}
	c.Unlock()

	StartCacheCleaner()

	v, ok := CacheGet("fresh")
	assert.True(t, ok, "unexpired entry must survive the cleaner")
	assert.Equal(t, "value", v)

	_, ok = CacheGet("stale")
	assert.False(t, ok, "expired entry must be evicted")
}
