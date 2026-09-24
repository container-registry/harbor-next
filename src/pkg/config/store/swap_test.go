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

package store

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config/metadata"
)

type versionedDriver struct {
	mu      sync.Mutex
	values  map[string]any
	version uint64
	loads   int
	saved   map[string]any
}

func (d *versionedDriver) Load(context.Context) (map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loads++
	out := map[string]any{}
	for k, v := range d.values {
		out[k] = v
	}
	return out, nil
}

func (d *versionedDriver) Save(_ context.Context, cfg map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.saved = cfg
	return nil
}

func (d *versionedDriver) Get(context.Context, string) (map[string]any, error) { return nil, nil }

func (d *versionedDriver) Version() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version
}

func (d *versionedDriver) publish(values map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.values = values
	d.version++
}

func TestLoadSkipsUnchangedVersion(t *testing.T) {
	d := &versionedDriver{}
	d.publish(map[string]any{common.AUTHMode: "db_auth"})
	s := NewConfigStore(d)

	require.NoError(t, s.Load(context.Background()))
	require.NoError(t, s.Load(context.Background()))
	assert.Equal(t, 1, d.loads)

	d.publish(map[string]any{common.AUTHMode: "oidc_auth"})
	require.NoError(t, s.Load(context.Background()))
	assert.Equal(t, 2, d.loads)
	v, err := s.Get(common.AUTHMode)
	require.NoError(t, err)
	assert.Equal(t, "oidc_auth", v.GetString())
}

func TestLoadAfterUpdateFollowsDriver(t *testing.T) {
	d := &versionedDriver{}
	d.publish(map[string]any{common.AUTHMode: "db_auth"})
	s := NewConfigStore(d)
	require.NoError(t, s.Load(context.Background()))

	require.NoError(t, s.Update(context.Background(), map[string]any{common.AUTHMode: "ldap_auth"}))
	assert.Equal(t, "ldap_auth", d.saved[common.AUTHMode])
	v, _ := s.Get(common.AUTHMode)
	assert.Equal(t, "ldap_auth", v.GetString(), "Get without Load sees the local write")

	// not committed yet, or rolled back: the driver still has the old value
	require.NoError(t, s.Load(context.Background()))
	v, _ = s.Get(common.AUTHMode)
	assert.Equal(t, "db_auth", v.GetString())

	d.publish(map[string]any{common.AUTHMode: "ldap_auth"})
	require.NoError(t, s.Load(context.Background()))
	v, _ = s.Get(common.AUTHMode)
	assert.Equal(t, "ldap_auth", v.GetString())
}

func TestZeroVersionAlwaysLoads(t *testing.T) {
	d := &versionedDriver{values: map[string]any{common.AUTHMode: "db_auth"}}
	s := NewConfigStore(d)
	require.NoError(t, s.Load(context.Background()))
	require.NoError(t, s.Load(context.Background()))
	assert.Equal(t, 2, d.loads)
}

// Readers must see either the old or the new set of values, never a mix.
func TestLoadSwapsValuesAtomically(t *testing.T) {
	gen := func(i int) map[string]any {
		n := strconv.Itoa(i)
		return map[string]any{common.OIDCName: "name-" + n, common.OIDCEndpoint: "https://idp-" + n}
	}
	d := &versionedDriver{}
	d.publish(gen(0))
	s := NewConfigStore(d)
	require.NoError(t, s.Load(context.Background()))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i < 2000; i++ {
			d.publish(gen(i))
			_ = s.Load(context.Background())
		}
		close(stop)
	}()
	for {
		select {
		case <-stop:
			wg.Wait()
			return
		default:
		}
		m := s.values()
		name, endpoint := m[common.OIDCName], m[common.OIDCEndpoint]
		require.Equal(t, name.Value[len("name-"):], endpoint.Value[len("https://idp-"):])
	}
}

func TestSetOnStoreWithoutValues(t *testing.T) {
	s := &ConfigStore{}
	_, err := s.Get(common.AUTHMode)
	assert.Error(t, err)
	require.NoError(t, s.Set(common.AUTHMode, mustValue(t, common.AUTHMode, "db_auth")))
	v, err := s.Get(common.AUTHMode)
	require.NoError(t, err)
	assert.Equal(t, "db_auth", v.GetString())
}

func mustValue(t *testing.T, key, value string) metadata.ConfigureValue {
	t.Helper()
	v, err := metadata.NewCfgValue(key, value)
	require.NoError(t, err)
	return *v
}
