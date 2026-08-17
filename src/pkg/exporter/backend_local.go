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
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/orm"
	libRedis "github.com/goharbor/harbor/src/lib/redis"
	"github.com/goharbor/harbor/src/pkg/jobmonitor"
)

// jsResolveTimeout bounds the one-off HTTP call that fetches the job service
// redis config. The holder of the resolving lock waits at most this long, so a
// hung job service costs one late scrape round, not a permanently poisoned
// resolution (contenders fail fast and would otherwise never get a retry).
const jsResolveTimeout = 10 * time.Second

// localBackend serves the collectors from inside the core process: health and
// system info come from the controllers core already owns instead of looping
// back through core's own REST API.
type localBackend struct {
	healthCtl health.Controller
	sysCtl    si.Controller
	// newCtx builds the context the system info controller needs. Held as a
	// field so tests can supply one that does not require a live database.
	newCtx func() context.Context
	// jsClient is a dedicated job service client with a request timeout —
	// job.GlobalClient has none and would let JobService block forever.
	jsClient job.Client

	mu        sync.RWMutex
	resolving sync.Mutex
	js        *JobServiceBackend
}

// NewLocalBackend returns the backend used when the collectors run inside core.
func NewLocalBackend() Backend {
	return &localBackend{
		healthCtl: health.Ctl,
		sysCtl:    si.Ctl,
		newCtx:    orm.Context,
		jsClient:  job.NewDefaultClientWithTimeout(config.InternalJobServiceURL(), config.CoreSecret(), jsResolveTimeout),
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

func (b *localBackend) SystemInfo(_ context.Context) (*responseSysInfo, error) {
	// The controller reads config from the database, so it needs an orm context.
	// Built here rather than in the collector so the REST backend, which needs no
	// database at all, does not inherit the dependency.
	ctx := b.newCtx()
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
	if js := b.cached(); js != nil {
		return js, nil
	}

	// GetJobServiceConfig is an HTTP call, bounded by jsResolveTimeout on
	// jsClient. Serialise resolution so a slow job service is asked once, but
	// never queue scrapes behind it: a concurrent scrape gives up here and
	// skips the job service metrics for that round.
	if !b.resolving.TryLock() {
		return nil, fmt.Errorf("job service backend resolution already in progress")
	}
	defer b.resolving.Unlock()

	// The holder of the lock may have finished while we were contending for it.
	if js := b.cached(); js != nil {
		return js, nil
	}

	cfg, err := b.jsClient.GetJobServiceConfig()
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
	resolved := &JobServiceBackend{
		Pool:      pool,
		Client:    work.NewClient(namespace, pool),
		Namespace: namespace,
	}

	b.mu.Lock()
	b.js = resolved
	b.mu.Unlock()
	return resolved, nil
}

func (b *localBackend) cached() *JobServiceBackend {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.js
}
