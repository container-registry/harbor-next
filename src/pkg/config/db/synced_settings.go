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
	"errors"
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
	"github.com/goharbor/harbor/src/pkg/config/db/dao"
	"github.com/goharbor/harbor/src/pkg/config/store"
)

const (
	listenerAppName = "harbor_configuration_listener"
	// fullRefreshInterval bounds staleness when a notification is missed or the
	// properties table is edited outside Harbor.
	fullRefreshInterval = 5 * time.Minute
	reconnectBackoff    = time.Second
	maxReconnectBackoff = 30 * time.Second
	// listenerPingInterval detects a listener connection that died without the
	// socket closing, e.g. after a failover or a network partition.
	listenerPingInterval = 30 * time.Second
	pingTimeout          = 5 * time.Second
	selectUserSettings   = "SELECT k, v FROM properties"
)

// refreshTimeout bounds a refresh, including the wait for a pool connection.
var refreshTimeout = 30 * time.Second

// SyncedSettings serves the user settings from memory and keeps them in sync with
// the properties table through Postgres change notifications, so the request path
// never touches the database or a lock to read configuration.
type SyncedSettings struct {
	driver   store.Driver
	pool     *pgxpool.Pool // set by StartSync; refreshes use it because Beego ignores contexts
	settings atomic.Pointer[map[string]any]
	revision atomic.Uint64

	refreshMu sync.Mutex // orders refreshes so an older read never replaces a newer one
	refreshCh chan struct{}
	running   atomic.Bool
}

var _ store.Revisioned = (*SyncedSettings)(nil)

func newSyncedSettings(driver store.Driver) *SyncedSettings {
	return &SyncedSettings{driver: driver, refreshCh: make(chan struct{}, 1)}
}

// Load returns the synced settings. Until StartSync has read them, it reads the database.
func (s *SyncedSettings) Load(ctx context.Context) (map[string]any, error) {
	if settings := s.settings.Load(); settings != nil {
		return maps.Clone(*settings), nil
	}
	return s.driver.Load(ctx)
}

// Save writes through to the database, which announces the change on commit.
func (s *SyncedSettings) Save(ctx context.Context, cfg map[string]any) error {
	return s.driver.Save(ctx, cfg)
}

// Get - delegate to driver
func (s *SyncedSettings) Get(ctx context.Context, key string) (map[string]any, error) {
	return s.driver.Get(ctx, key)
}

// Revision increases on every refresh; 0 means the settings are not synced.
func (s *SyncedSettings) Revision() uint64 {
	return s.revision.Load()
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *SyncedSettings) refresh(ctx context.Context) error {
	return s.refreshFrom(ctx, s.pool)
}

func (s *SyncedSettings) refreshFrom(ctx context.Context, q querier) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	settings, err := readUserSettings(ctx, q)
	if err != nil {
		return err
	}
	s.settings.Store(&settings)
	s.revision.Add(1)
	return nil
}

func readUserSettings(ctx context.Context, q querier) (map[string]any, error) {
	rows, err := q.Query(ctx, selectUserSettings)
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
	return userSettingsFrom(entries), nil
}

func (s *SyncedSettings) scheduleRefresh() {
	select {
	case s.refreshCh <- struct{}{}:
	default:
	}
}

// StartSync reads the settings and keeps them in sync. It holds one connection of
// pool for LISTEN for the lifetime of the process. The returned function stops the
// sync and must run before the pool is closed, as pgxpool.Close waits for that
// connection.
func (s *SyncedSettings) StartSync(pool *pgxpool.Pool) (stop func()) {
	// Without a listener the settings would only follow the full refresh, and a pod
	// would serve stale values for its own committed writes. Reading the database on
	// every Load is correct and, with no cache lock left, cannot deadlock.
	if pool == nil || pool.Config().MaxConns < 2 {
		log.Warning("database pool too small to hold a connection for the configuration change listener, reading configuration from the database on every request")
		return func() {}
	}
	if !s.running.CompareAndSwap(false, true) {
		log.Warning("configuration sync is already running")
		return func() {}
	}
	ctx, cancel := context.WithCancel(orm.Context())
	var done sync.WaitGroup
	s.pool = pool
	if err := s.refresh(ctx); err != nil {
		log.Errorf("failed to read the user settings, reading configuration from the database until it succeeds: %v", err)
	}
	done.Add(2)
	go func() { defer done.Done(); s.refreshLoop(ctx) }()
	go func() { defer done.Done(); s.watchChanges(ctx, pool) }()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			done.Wait()
			s.running.Store(false)
		})
	}
}

func (s *SyncedSettings) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(fullRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.refreshCh:
		}
		if err := s.refresh(ctx); err != nil && ctx.Err() == nil {
			log.Warningf("failed to refresh the user settings, keeping the previous ones: %v", err)
		}
	}
}

// watchChanges keeps a change subscription open, reconnecting with backoff.
func (s *SyncedSettings) watchChanges(ctx context.Context, pool *pgxpool.Pool) {
	backoff := reconnectBackoff
	for ctx.Err() == nil {
		start := time.Now()
		err := s.subscribe(ctx, pool)
		if ctx.Err() != nil {
			return
		}
		log.Warningf("configuration change listener stopped, falling back to the %s full refresh until it reconnects: %v", fullRefreshInterval, err)
		if time.Since(start) > maxReconnectBackoff {
			backoff = reconnectBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxReconnectBackoff)
	}
}

// subscribe listens for changes on one connection until it fails or ctx ends.
func (s *SyncedSettings) subscribe(ctx context.Context, pool *pgxpool.Pool) error {
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
	// the name shows operators why this session stays open in pg_stat_activity
	if _, err := conn.Exec(ctx, "SET application_name = '"+listenerAppName+"'"); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+dao.ConfigurationChangedChannel); err != nil {
		return err
	}
	// Changes committed while no listener was attached announced themselves to
	// nobody. Reading after LISTEN is active closes that gap; if the read fails the
	// connection is retried rather than trusting settings that may be stale. It
	// reads on the LISTEN connection so it never needs a second one from the pool.
	if err := s.refreshFrom(ctx, conn); err != nil {
		return fmt.Errorf("sync after connect: %w", err)
	}
	for {
		waitCtx, cancel := context.WithTimeout(ctx, listenerPingInterval)
		_, err := conn.Conn().WaitForNotification(waitCtx)
		idle := errors.Is(waitCtx.Err(), context.DeadlineExceeded) // read before cancel overwrites it
		cancel()
		switch {
		case err == nil:
			s.scheduleRefresh()
		case ctx.Err() != nil:
			return ctx.Err()
		case idle:
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
