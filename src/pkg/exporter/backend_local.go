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
	"fmt"
	"sync"
	"time"

	"github.com/gocraft/work"

	"github.com/goharbor/harbor/src/common/job"
	"github.com/goharbor/harbor/src/controller/health"
	si "github.com/goharbor/harbor/src/controller/systeminfo"
	libRedis "github.com/goharbor/harbor/src/lib/redis"
	"github.com/goharbor/harbor/src/pkg/jobmonitor"
)

// localBackend serves the collectors from inside the core process: health and
// system info come from the controllers core already owns instead of looping
// back through core's own REST API.
type localBackend struct {
	healthCtl health.Controller
	sysCtl    si.Controller

	mu sync.Mutex
	js *JobServiceBackend
}

// NewLocalBackend returns the backend used when the collectors run inside core.
func NewLocalBackend() Backend {
	return &localBackend{
		healthCtl: health.Ctl,
		sysCtl:    si.Ctl,
	}
}

func (b *localBackend) Health(ctx context.Context) (*responseHealth, error) {
	status := b.healthCtl.GetHealth(ctx)
	out := &responseHealth{Status: status.Status}
	for _, c := range status.Components {
		out.Components = append(out.Components, responseComponent{
			Name:   c.Name,
			Status: c.Status,
		})
	}
	return out, nil
}

func (b *localBackend) SystemInfo(ctx context.Context) (*responseSysInfo, error) {
	// Deliberately without WithProtectedInfo: the exporter only needs auth_mode
	// and self_registration, both unprotected. The harbor_version label is
	// filled from the version package at build time, not from here.
	data, err := b.sysCtl.GetInfo(ctx, si.Options{})
	if err != nil {
		return nil, err
	}
	return &responseSysInfo{
		AuthMode:         data.AuthMode,
		SelfRegistration: data.SelfRegistration,
	}, nil
}

// JobService resolves the job service redis handles lazily. It cannot be done at
// core startup: the redis URL is fetched over HTTP from job service, which in
// turn waits for core to become healthy before it starts.
func (b *localBackend) JobService(_ context.Context) (*JobServiceBackend, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.js != nil {
		return b.js, nil
	}

	cfg, err := job.GlobalClient.GetJobServiceConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.RedisPoolConfig == nil {
		return nil, fmt.Errorf("job service returned no redis pool config")
	}
	poolCfg := cfg.RedisPoolConfig

	// Same pool name core's job monitor uses, so both share one pool.
	pool, err := libRedis.GetRedisPool(jobmonitor.JobServicePool, poolCfg.RedisURL, &libRedis.PoolParam{
		PoolMaxIdle:     0,
		PoolIdleTimeout: time.Duration(poolCfg.IdleTimeoutSecond) * time.Second,
	})
	if err != nil {
		return nil, err
	}

	namespace := fmt.Sprintf("{%s}", poolCfg.Namespace)
	b.js = &JobServiceBackend{
		Pool:      pool,
		Client:    work.NewClient(namespace, pool),
		Namespace: namespace,
	}
	return b.js, nil
}
