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

package systeminfo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
)

type fakeCapacityCtl struct {
	Controller
	capacity *imagestorage.Capacity
	err      error
}

func (f fakeCapacityCtl) GetCapacity(context.Context) (*imagestorage.Capacity, error) {
	return f.capacity, f.err
}

func TestMeasurer(t *testing.T) {
	at := time.Now()
	m := &measurer{ctl: fakeCapacityCtl{capacity: &imagestorage.Capacity{Used: 42, Measured: true, MeasuredAt: at}}}
	got, err := m.Measure(context.Background())
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, int64(42), got.Used)
		assert.Equal(t, at, got.At)
	}

	m = &measurer{ctl: fakeCapacityCtl{capacity: &imagestorage.Capacity{Used: 42, Measured: false}}}
	got, err = m.Measure(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, got)

	m = &measurer{ctl: fakeCapacityCtl{err: errors.New("boom")}}
	_, err = m.Measure(context.Background())
	assert.Error(t, err)
}
