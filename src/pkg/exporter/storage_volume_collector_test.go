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
	"io"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	si "github.com/goharbor/harbor/src/controller/systeminfo"
	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
)

type fakeVolumeSysInfoCtl struct{ capacity *imagestorage.Capacity }

func (f fakeVolumeSysInfoCtl) GetInfo(context.Context, si.Options) (*si.Data, error) { return nil, nil }
func (f fakeVolumeSysInfoCtl) GetCA(context.Context) (io.ReadCloser, error)          { return nil, nil }
func (f fakeVolumeSysInfoCtl) GetCapacity(context.Context) (*imagestorage.Capacity, error) {
	return f.capacity, nil
}

func TestStorageVolumeCollector(t *testing.T) {
	c := StorageVolumeCollector{
		ctl:    fakeVolumeSysInfoCtl{capacity: &imagestorage.Capacity{Total: 100, Free: 30, Used: 70, Driver: "filesystem", Measured: true}},
		newCtx: context.Background,
	}
	metrics := c.getMetrics()
	require.Len(t, metrics, 4)
	values := map[string]float64{}
	for _, m := range metrics[:3] {
		d := &dto.Metric{}
		require.NoError(t, m.Write(d))
		// the DTO sorts labels by name, so look them up rather than by position
		labels := map[string]string{}
		for _, l := range d.Label {
			labels[l.GetName()] = l.GetValue()
		}
		values[labels["kind"]] = d.Gauge.GetValue()
		assert.Equal(t, "filesystem", labels["driver"])
	}
	assert.Equal(t, map[string]float64{"total": 100, "free": 30, "used": 70}, values)
	d := &dto.Metric{}
	require.NoError(t, metrics[3].Write(d))
	assert.Equal(t, float64(1), d.Gauge.GetValue())
}
