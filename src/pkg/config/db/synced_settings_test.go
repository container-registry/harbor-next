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

//go:build db

package db

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/lib/orm"
	cfgdao "github.com/goharbor/harbor/src/pkg/config/db/dao"
	"github.com/goharbor/harbor/src/pkg/config/store"
)

// countingDriver counts Load calls that reach the database.
type countingDriver struct {
	store.Driver
	loads atomic.Int64
}

func (c *countingDriver) Load(ctx context.Context) (map[string]any, error) {
	c.loads.Add(1)
	return c.Driver.Load(ctx)
}

func waitForRevisionAbove(t *testing.T, s *SyncedSettings, after uint64) {
	t.Helper()
	require.Eventually(t, func() bool { return s.Revision() > after }, 10*time.Second, 10*time.Millisecond)
}

func TestCommittedSaveRefreshesSettings(t *testing.T) {
	driver := &countingDriver{Driver: &Database{cfgDAO: cfgdao.New()}}
	s := newSyncedSettings(driver)
	stop := s.StartSync(dao.GetPool().PgxPool())
	defer stop()

	ctx := orm.Context()
	// initial load plus the catch-up refresh once LISTEN is active
	waitForRevisionAbove(t, s, 1)

	// reads are served from memory
	loads := driver.loads.Load()
	for range 100 {
		_, err := s.Load(ctx)
		require.NoError(t, err)
	}
	assert.Equal(t, loads, driver.loads.Load())

	before := s.Revision()
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "ldap_auth"}))
	waitForRevisionAbove(t, s, before)
	v, err := s.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ldap_auth", v[common.AUTHMode])

	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "db_auth"}))
}

func TestRolledBackSavePublishesNoChange(t *testing.T) {
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(dao.GetPool().PgxPool())
	defer stop()

	ctx := orm.Context()
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "db_auth"}))
	waitForRevisionAbove(t, s, 1)
	time.Sleep(500 * time.Millisecond)
	before := s.Revision()

	rollback := errors.New("rollback")
	err := orm.WithTransaction(func(ctx context.Context) error {
		if err := (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "ldap_auth"}); err != nil {
			return err
		}
		return rollback
	})(ctx)
	require.ErrorIs(t, err, rollback)

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, before, s.Revision(), "a rolled back save must not notify")
	v, err := s.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, "db_auth", v[common.AUTHMode])
}

func listenerSessions(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, dao.GetPool().PgxPool().QueryRow(context.Background(),
		"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND application_name = '"+listenerAppName+"'").Scan(&n))
	return n
}

func TestStopClosesListenerSession(t *testing.T) {
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(dao.GetPool().PgxPool())
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)

	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stop did not return")
	}
	// the LISTEN session is destroyed, not returned to the pool for reuse by requests
	require.Eventually(t, func() bool { return listenerSessions(t) == 0 }, 10*time.Second, 10*time.Millisecond)
}

func TestLoadBeforeSyncReadsDatabase(t *testing.T) {
	driver := &countingDriver{Driver: &Database{cfgDAO: cfgdao.New()}}
	s := newSyncedSettings(driver)
	_, err := s.Load(orm.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), driver.loads.Load())
	assert.Equal(t, uint64(0), s.Revision())
}

func TestListenerReconnectsAfterTermination(t *testing.T) {
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(dao.GetPool().PgxPool())
	defer stop()
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)

	_, err := dao.GetPool().PgxPool().Exec(context.Background(),
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND application_name = '"+listenerAppName+"'")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 15*time.Second, 50*time.Millisecond)

	ctx := orm.Context()
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "ldap_auth"}))
	require.Eventually(t, func() bool {
		v, _ := s.Load(ctx)
		return v[common.AUTHMode] == "ldap_auth"
	}, 10*time.Second, 10*time.Millisecond)
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "db_auth"}))
}

// The request path must read configuration while every pool connection is held,
// which is the state of the #92 deadlock.
func TestReadsSucceedWithExhaustedPool(t *testing.T) {
	pool := dao.GetPool().PgxPool()
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(pool)
	defer stop()
	waitForRevisionAbove(t, s, 1)

	mgr := NewDBCfgManager()
	mgr.Store = store.NewConfigStore(s)

	var held []interface{ Release() }
	defer func() {
		for _, c := range held {
			c.Release()
		}
	}()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		c, err := pool.Acquire(ctx)
		cancel()
		if err != nil {
			break
		}
		held = append(held, c)
	}
	require.Equal(t, pool.Stat().MaxConns(), pool.Stat().AcquiredConns())

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(orm.Context(), 5*time.Second)
		defer cancel()
		done <- mgr.Load(ctx)
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("config load blocked on the exhausted pool")
	}
	assert.NotEmpty(t, mgr.Get(context.Background(), common.AUTHMode).GetString())
}

// A change committed while the listener is disconnected sends a notification nobody
// receives; the sync on reconnect must pick it up without waiting for the resync.
func TestReconnectAppliesChangesMissedWhileDisconnected(t *testing.T) {
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(dao.GetPool().PgxPool())
	defer stop()
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)

	ctx := orm.Context()
	_, err := dao.GetPool().PgxPool().Exec(context.Background(),
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND application_name = '"+listenerAppName+"'")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return listenerSessions(t) == 0 }, 5*time.Second, 5*time.Millisecond)
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "ldap_auth"}))

	require.Eventually(t, func() bool {
		v, _ := s.Load(ctx)
		return v[common.AUTHMode] == "ldap_auth"
	}, 15*time.Second, 20*time.Millisecond)
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(ctx, map[string]any{common.AUTHMode: "db_auth"}))
}
