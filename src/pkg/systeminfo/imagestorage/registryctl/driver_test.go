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

package registryctl

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/cache"
	_ "github.com/goharbor/harbor/src/lib/cache/memory"
	storage "github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
)

type fakeClient struct {
	info  *storage.VolumeInfo
	err   error
	calls int
}

func (f *fakeClient) Health() error                       { return nil }
func (f *fakeClient) DeleteBlob(string) error             { return nil }
func (f *fakeClient) DeleteManifest(string, string) error { return nil }
func (f *fakeClient) Storage() (*storage.VolumeInfo, error) {
	f.calls++
	return f.info, f.err
}

type fakeFallback struct{ capacity *storage.Capacity }

func (fakeFallback) Name() string { return "filesystem" }
func (f fakeFallback) Cap() (*storage.Capacity, error) {
	return f.capacity, nil
}

func newDriver(c *fakeClient, ca cache.Cache) *driver {
	return &driver{
		client:   c,
		fallback: fakeFallback{capacity: &storage.Capacity{Total: 10, Free: 4}},
		cache:    func() cache.Cache { return ca },
	}
}

func TestMeasured(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Second)
	c := &fakeClient{info: &storage.VolumeInfo{Driver: "filesystem", Supported: true, Total: 100, Free: 40, Used: 60, MeasuredAt: at}}
	got, err := newDriver(c, nil).Cap()
	require.NoError(t, err)
	assert.True(t, got.Measured)
	assert.Equal(t, uint64(60), got.Used)
	assert.Equal(t, uint64(100), got.Total)
	assert.Equal(t, "filesystem", got.Driver)
	assert.Equal(t, at, got.MeasuredAt)
}

func TestUnreachableFallsBack(t *testing.T) {
	c := &fakeClient{err: errors.New("connection refused")}
	got, err := newDriver(c, nil).Cap()
	require.NoError(t, err)
	assert.False(t, got.Measured)
	assert.Equal(t, uint64(10), got.Total)
	assert.Equal(t, "filesystem", got.Driver)
}

func TestUnsupportedDriverFallsBack(t *testing.T) {
	c := &fakeClient{info: &storage.VolumeInfo{Driver: "s3", Supported: false}}
	got, err := newDriver(c, nil).Cap()
	require.NoError(t, err)
	assert.False(t, got.Measured)
}

func TestNoClient(t *testing.T) {
	d := &driver{cache: func() cache.Cache { return nil }}
	got, err := d.Cap()
	require.NoError(t, err)
	assert.False(t, got.Measured)
	assert.Zero(t, got.Total)
}

func TestCached(t *testing.T) {
	ca, err := cache.New("memory")
	require.NoError(t, err)
	c := &fakeClient{info: &storage.VolumeInfo{Driver: "filesystem", Supported: true, Total: 100, Free: 1, Used: 99}}
	d := newDriver(c, ca)
	for range 5 {
		_, err := d.Cap()
		require.NoError(t, err)
	}
	assert.Equal(t, 1, c.calls)
}
