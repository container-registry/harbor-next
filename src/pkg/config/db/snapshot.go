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

package db

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goharbor/harbor/src/lib/config/models"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/config/store"
)

const (
	// notifyChannel is signalled by Database.Save; Postgres delivers it only on commit.
	notifyChannel = "harbor_config"
	// resyncInterval bounds staleness when a notification is missed or the properties
	// table is edited outside Harbor.
	resyncInterval   = 5 * time.Minute
	listenBackoff    = time.Second
	maxListenBackoff = 30 * time.Second
	// idlePing detects a listener connection that died without the socket closing,
	// e.g. after a failover or a network partition.
	idlePing    = 30 * time.Second
	pingTimeout = 5 * time.Second
)

// reloadTimeout bounds a reload, including the wait for a pool connection.
var reloadTimeout = 30 * time.Second

// Snapshot serves user settings from an in-memory copy of the properties table so
// the request path never touches the database or a lock for configuration.
// The copy is refreshed on Postgres NOTIFY and periodically.
type Snapshot struct {
	driver  store.Driver
	pool    *pgxpool.Pool // set by Start; reloads use it because Beego ignores contexts
	values  atomic.Pointer[map[string]any]
	version atomic.Uint64

	reloadMu sync.Mutex // orders reloads so an older read never replaces a newer one
	reloadCh chan struct{}
	running  atomic.Bool
}

var _ store.Versioned = (*Snapshot)(nil)

func newSnapshot(driver store.Driver) *Snapshot {
	return &Snapshot{driver: driver, reloadCh: make(chan struct{}, 1)}
}

// Load returns the snapshot. Before Start has loaded it, it reads the database directly.
func (s *Snapshot) Load(ctx context.Context) (map[string]any, error) {
	if v := s.values.Load(); v != nil {
		return maps.Clone(*v), nil
	}
	return s.driver.Load(ctx)
}

// Save writes through to the database, which notifies all listeners on commit.
func (s *Snapshot) Save(ctx context.Context, cfg map[string]any) error {
	return s.driver.Save(ctx, cfg)
}

// Get - delegate to driver
func (s *Snapshot) Get(ctx context.Context, key string) (map[string]any, error) {
	return s.driver.Get(ctx, key)
}

// Version increases on every reload; 0 means no snapshot is loaded yet.
func (s *Snapshot) Version() uint64 {
	return s.version.Load()
}

func (s *Snapshot) reload(ctx context.Context) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, reloadTimeout)
	defer cancel()
	cfgs, err := s.load(ctx)
	if err != nil {
		return err
	}
	s.values.Store(&cfgs)
	s.version.Add(1)
	return nil
}

func (s *Snapshot) load(ctx context.Context) (map[string]any, error) {
	if s.pool == nil {
		return s.driver.Load(ctx)
	}
	rows, err := s.pool.Query(ctx, "SELECT k, v FROM properties")
	if err != nil {
		return nil, err
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*models.ConfigEntry, error) {
		e := &models.ConfigEntry{}
		return e, row.Scan(&e.Key, &e.Value)
	})
	if err != nil {
		return nil, err
	}
	return decodeEntries(entries), nil
}

func (s *Snapshot) requestReload() {
	select {
	case s.reloadCh <- struct{}{}:
	default:
	}
}

// Start loads the snapshot and keeps it current. It holds one connection of pool
// for LISTEN for the lifetime of the process. The returned function stops the
// background work and must run before the pool is closed, as pgxpool.Close waits
// for that connection.
func (s *Snapshot) Start(pool *pgxpool.Pool) (stop func()) {
	if !s.running.CompareAndSwap(false, true) {
		log.Warning("configuration snapshot is already running")
		return func() {}
	}
	ctx, cancel := context.WithCancel(orm.Context())
	var done sync.WaitGroup
	s.pool = pool
	if err := s.reload(ctx); err != nil {
		log.Errorf("failed to load the configuration snapshot, reading configuration from the database until it succeeds: %v", err)
	}
	done.Add(1)
	go func() { defer done.Done(); s.reloadLoop(ctx) }()
	if pool != nil {
		done.Add(1)
		go func() { defer done.Done(); s.listenLoop(ctx, pool) }()
	} else {
		log.Warningf("no database pool for the configuration change listener, changes from other instances apply within %s", resyncInterval)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			done.Wait()
			s.running.Store(false)
		})
	}
}

func (s *Snapshot) reloadLoop(ctx context.Context) {
	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.reloadCh:
		}
		if err := s.reload(ctx); err != nil && ctx.Err() == nil {
			log.Warningf("failed to reload the configuration snapshot, keeping the previous one: %v", err)
		}
	}
}

func (s *Snapshot) listenLoop(ctx context.Context, pool *pgxpool.Pool) {
	backoff := listenBackoff
	for ctx.Err() == nil {
		start := time.Now()
		err := s.listen(ctx, pool)
		if ctx.Err() != nil {
			return
		}
		log.Warningf("configuration change listener stopped, falling back to the %s resync until it reconnects: %v", resyncInterval, err)
		if time.Since(start) > maxListenBackoff {
			backoff = listenBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxListenBackoff)
	}
}

func (s *Snapshot) listen(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// A LISTEN session must never go back to the pool: closing it makes Release destroy it.
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Conn().Close(closeCtx)
		conn.Release()
	}()
	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return err
	}
	// Changes committed while no listener was attached sent notifications nobody
	// received. Reading after LISTEN is active closes that gap; if the read fails the
	// connection is retried rather than trusting a snapshot that may be stale.
	if err := s.reload(ctx); err != nil {
		return fmt.Errorf("sync after connect: %w", err)
	}
	for {
		waitCtx, cancel := context.WithTimeout(ctx, idlePing)
		_, err := conn.Conn().WaitForNotification(waitCtx)
		cancel()
		switch {
		case err == nil:
			s.requestReload()
		case ctx.Err() != nil:
			return ctx.Err()
		case waitCtx.Err() != nil:
			// pgconn keeps the connection open on a timeout, so it can be probed
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			err = conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return fmt.Errorf("listener connection lost: %w", err)
			}
		default:
			return err
		}
	}
}
