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

// Package registryctl is the image storage driver that asks registryctl for
// the registry volume's usage (harbor-next #839, theme B: core pulls, because
// the core-to-registryctl edge and its secret already exist). registryctl
// shares the registry's mount, so its statfs is truthful where core's own disk
// is not. Results are cached briefly so the global quota gate on the push path
// does not turn into one HTTP call per push.
package registryctl

import (
	"context"
	"time"

	"github.com/goharbor/harbor/src/lib/cache"
	"github.com/goharbor/harbor/src/lib/log"
	storage "github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
	"github.com/goharbor/harbor/src/registryctl/client"
)

const (
	driverName = "registryctl"
	cacheKey   = "systeminfo:registryctl_volume"
	cacheTTL   = time.Minute
	// callTimeout bounds one registryctl round trip; Cap runs on the push path
	// through the global quota gate, so a hung registryctl must fall back fast.
	callTimeout = 5 * time.Second
)

type driver struct {
	client   client.Client
	fallback storage.Driver
	cache    func() cache.Cache
}

// NewDriver returns a driver that measures through registryctl and degrades to
// the fallback driver (with Measured=false) when registryctl is unreachable or
// the registry storage driver cannot be measured.
func NewDriver(c client.Client, fallback storage.Driver) storage.Driver {
	return &driver{client: c, fallback: fallback, cache: cache.Default}
}

// Name ...
func (d *driver) Name() string {
	return driverName
}

// Cap ...
func (d *driver) Cap() (*storage.Capacity, error) {
	info, err := d.volume()
	if err != nil {
		log.Warningf("registryctl storage measurement unavailable, falling back to local disk: %v", err)
		return d.unmeasured()
	}
	if !info.Supported {
		// Object stores expose no capacity; report the real driver with zero
		// sizes rather than core's local disk, which would describe the wrong volume.
		return &storage.Capacity{Driver: info.Driver, Measured: false}, nil
	}
	return &storage.Capacity{
		Total:      info.Total,
		Free:       info.Free,
		Used:       info.Used,
		Driver:     info.Driver,
		Measured:   true,
		MeasuredAt: info.MeasuredAt,
	}, nil
}

func (d *driver) unmeasured() (*storage.Capacity, error) {
	if d.fallback == nil {
		return &storage.Capacity{}, nil
	}
	c, err := d.fallback.Cap()
	if err != nil {
		return nil, err
	}
	c.Measured = false
	if c.Driver == "" {
		c.Driver = d.fallback.Name()
	}
	if c.Used == 0 && c.Total >= c.Free {
		c.Used = c.Total - c.Free
	}
	return c, nil
}

func (d *driver) volume() (*storage.VolumeInfo, error) {
	if d.client == nil {
		return nil, errNoClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	ca := d.cache()
	if ca == nil {
		return d.client.Storage(ctx)
	}
	info := &storage.VolumeInfo{}
	err := cache.FetchOrSave(ctx, ca, cacheKey, info, func() (any, error) {
		return d.client.Storage(ctx)
	}, cacheTTL)
	if err != nil {
		return nil, err
	}
	return info, nil
}
