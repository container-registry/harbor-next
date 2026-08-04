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
	b := &localBackend{sysCtl: ctl}

	got, err := b.SystemInfo(context.Background())
	require.NoError(t, err)
	assert.False(t, ctl.gotOpt.WithProtectedInfo)
	assert.Equal(t, "db_auth", got.AuthMode)
	assert.True(t, got.SelfRegistration)
}

func TestLocalBackendSystemInfoError(t *testing.T) {
	b := &localBackend{sysCtl: &fakeSysInfoCtl{err: errors.New("nope")}}

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

// A second CacheInit must not leak another cleaner goroutine.
func TestCacheInitStartsSingleCleaner(t *testing.T) {
	CacheInit(&Opt{CacheDuration: 60})
	CacheInit(&Opt{CacheDuration: 60})
	CacheInit(&Opt{CacheDuration: 60})

	assert.Equal(t, int32(1), cleanerStarts.Load())
	assert.True(t, CacheEnabled())

	CachePut("k", "v")
	v, ok := CacheGet("k")
	assert.True(t, ok)
	assert.Equal(t, "v", v)
}
