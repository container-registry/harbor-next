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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/lib/orm"
	cfgdao "github.com/goharbor/harbor/src/pkg/config/db/dao"
	"github.com/goharbor/harbor/src/pkg/config/store"
)

func holdAllPoolConnections(t *testing.T, pool *pgxpool.Pool) (release func()) {
	t.Helper()
	var held []*pgxpool.Conn
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
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, c := range held {
				c.Release()
			}
		})
	}
}

func saveAuthMode(t *testing.T, mode string) {
	t.Helper()
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(orm.Context(), map[string]any{common.AUTHMode: mode}))
}

func stopWithin(t *testing.T, stop func(), d time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("stop did not return within %s", d)
	}
	return time.Since(start)
}

func TestListenerReconnectsOncePoolFreesUp(t *testing.T) {
	pool := dao.GetPool().PgxPool()
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(pool)
	defer stop()
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)
	saveAuthMode(t, "db_auth")
	require.Eventually(t, authModeIs(s, "db_auth"), 5*time.Second, 10*time.Millisecond)

	// drop the listener, then take every connection so it cannot reconnect
	_, err := pool.Exec(context.Background(),
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND application_name = '"+listenerAppName+"'")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return listenerSessions(t) == 0 }, 5*time.Second, 5*time.Millisecond)
	release := holdAllPoolConnections(t, pool)
	defer release()

	// a change committed by another instance while this one is starved
	other, err := pgxpool.New(context.Background(), pool.Config().ConnString())
	require.NoError(t, err)
	defer other.Close()
	_, err = other.Exec(context.Background(), "UPDATE properties SET v = 'ldap_auth' WHERE k = 'auth_mode'; SELECT pg_notify('harbor_configuration_changed', '')")
	require.NoError(t, err)

	// reads keep working from memory, with the old value
	for range 100 {
		v, err := s.Load(orm.Context())
		require.NoError(t, err)
		require.Equal(t, "db_auth", v[common.AUTHMode])
	}
	time.Sleep(2 * time.Second)
	release()
	start := time.Now()
	require.Eventually(t, authModeIs(s, "ldap_auth"), 30*time.Second, 20*time.Millisecond)
	t.Logf("missed change applied %s after the pool freed up", time.Since(start))
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 30*time.Second, 20*time.Millisecond)
	saveAuthMode(t, "db_auth")
}

func TestRefreshWaitsForPoolAndKeepsPreviousSettings(t *testing.T) {
	pool := dao.GetPool().PgxPool()
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(pool)
	defer stop()
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)
	saveAuthMode(t, "db_auth")
	require.Eventually(t, authModeIs(s, "db_auth"), 5*time.Second, 10*time.Millisecond)

	release := holdAllPoolConnections(t, pool)
	defer release()
	other, err := pgxpool.New(context.Background(), pool.Config().ConnString())
	require.NoError(t, err)
	defer other.Close()
	// the notification reaches the listener, but the refresh cannot get a connection
	_, err = other.Exec(context.Background(), "UPDATE properties SET v = 'ldap_auth' WHERE k = 'auth_mode'; SELECT pg_notify('harbor_configuration_changed', '')")
	require.NoError(t, err)
	time.Sleep(time.Second)
	v, err := s.Load(orm.Context())
	require.NoError(t, err)
	assert.Equal(t, "db_auth", v[common.AUTHMode], "previous settings are kept while the refresh waits")

	release()
	start := time.Now()
	require.Eventually(t, authModeIs(s, "ldap_auth"), 35*time.Second, 20*time.Millisecond)
	t.Logf("refresh completed %s after the pool freed up", time.Since(start))
	saveAuthMode(t, "db_auth")
}

func TestStopReturnsWhilePoolExhausted(t *testing.T) {
	pool := dao.GetPool().PgxPool()
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(pool)
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)

	release := holdAllPoolConnections(t, pool)
	defer release()
	// a refresh queued behind the exhausted pool, and a listener that must reconnect
	s.scheduleRefresh()
	time.Sleep(500 * time.Millisecond)
	t.Logf("stop returned after %s", stopWithin(t, stop, 10*time.Second))
}

func TestStartGivesUpOnExhaustedPoolThenCatchesUp(t *testing.T) {
	defer func(d time.Duration) { refreshTimeout = d }(refreshTimeout)
	refreshTimeout = 2 * time.Second
	pool := dao.GetPool().PgxPool()
	release := holdAllPoolConnections(t, pool)
	defer release()

	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	start := time.Now()
	started := make(chan func(), 1)
	go func() { started <- s.StartSync(pool) }()
	var stop func()
	select {
	case stop = <-started:
		t.Logf("StartSync returned after %s with revision %d", time.Since(start), s.Revision())
	case <-time.After(10 * time.Second):
		t.Fatal("Start blocked on an exhausted pool")
	}
	release()
	require.Eventually(t, func() bool { return s.Revision() > 0 }, 35*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 35*time.Second, 20*time.Millisecond)
	stopWithin(t, stop, 10*time.Second)
}

func TestConcurrentSavesConvergeOnLastValue(t *testing.T) {
	driver := &countingDriver{Driver: &Database{cfgDAO: cfgdao.New()}}
	s := newSyncedSettings(driver)
	stop := s.StartSync(dao.GetPool().PgxPool())
	defer stop()
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)
	base := s.Revision()

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				if err := (&Database{cfgDAO: cfgdao.New()}).Save(orm.Context(), map[string]any{common.LDAPURL: fmt.Sprintf("ldap://%d-%d", i, j)}); err != nil {
					t.Errorf("save failed: %v", err)
				}
			}
		}()
	}
	// concurrent readers during the storm
	stopReaders := make(chan struct{})
	var readers sync.WaitGroup
	for range 16 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
					_, err := s.Load(orm.Context())
					if err != nil {
						t.Error(err)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	final := "ldap://final"
	require.NoError(t, (&Database{cfgDAO: cfgdao.New()}).Save(orm.Context(), map[string]any{common.LDAPURL: final}))
	require.Eventually(t, func() bool {
		v, _ := s.Load(orm.Context())
		return v[common.LDAPURL] == final
	}, 10*time.Second, 10*time.Millisecond)
	close(stopReaders)
	readers.Wait()
	t.Logf("401 committed saves caused %d refreshes", s.Revision()-base)
	assert.Equal(t, int64(0), driver.loads.Load(), "no read went through the Beego driver")
}

func TestManagersShareOneListener(t *testing.T) {
	stop := StartSettingsSync(dao.GetPool().PgxPool())
	defer stop()
	for range 20 {
		_ = NewDBCfgManager()
	}
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 1, listenerSessions(t))
}
func authModeIs(s *SyncedSettings, want string) func() bool {
	return func() bool { v, _ := s.Load(orm.Context()); return v[common.AUTHMode] == want }
}

func TestSecondStartIsIgnoredAndRestartWorks(t *testing.T) {
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(dao.GetPool().PgxPool())
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)
	s.StartSync(dao.GetPool().PgxPool())()
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 1, listenerSessions(t))

	stopWithin(t, stop, 10*time.Second)
	require.Eventually(t, func() bool { return listenerSessions(t) == 0 }, 10*time.Second, 10*time.Millisecond)
	stop = s.StartSync(dao.GetPool().PgxPool())
	require.Eventually(t, func() bool { return listenerSessions(t) == 1 }, 10*time.Second, 10*time.Millisecond)
	stopWithin(t, stop, 10*time.Second)
}

// PUT /configurations runs in the request transaction; when it rolls back the
// writing instance must not keep serving the rejected values.
func TestRolledBackUpdateIsNotServed(t *testing.T) {
	s := newSyncedSettings(&Database{cfgDAO: cfgdao.New()})
	stop := s.StartSync(dao.GetPool().PgxPool())
	defer stop()
	saveAuthMode(t, "db_auth")
	require.Eventually(t, authModeIs(s, "db_auth"), 10*time.Second, 10*time.Millisecond)

	mgr := NewDBCfgManager()
	mgr.Store = store.NewConfigStore(s)
	ctx := orm.Context()
	require.NoError(t, mgr.Load(ctx))

	rollback := errors.New("rollback")
	err := orm.WithTransaction(func(ctx context.Context) error {
		if err := mgr.UpdateConfig(ctx, map[string]any{common.AUTHMode: "ldap_auth"}); err != nil {
			return err
		}
		return rollback
	})(ctx)
	require.ErrorIs(t, err, rollback)

	require.NoError(t, mgr.Load(ctx))
	assert.Equal(t, "db_auth", mgr.Get(ctx, common.AUTHMode).GetString())
}

func TestSingleConnectionPoolReadsDatabase(t *testing.T) {
	cfg := dao.GetPool().PgxPool().Config().Copy()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()

	driver := &countingDriver{Driver: &Database{cfgDAO: cfgdao.New()}}
	s := newSyncedSettings(driver)
	stop := s.StartSync(pool)
	defer stop()
	assert.Equal(t, uint64(0), s.Revision(), "settings are not synced without a listener")
	assert.Equal(t, int32(0), pool.Stat().AcquiredConns(), "no connection is held")

	// a committed write is visible on the next Load
	saveAuthMode(t, "ldap_auth")
	v, err := s.Load(orm.Context())
	require.NoError(t, err)
	assert.Equal(t, "ldap_auth", v[common.AUTHMode])
	assert.Equal(t, int64(1), driver.loads.Load())
	saveAuthMode(t, "db_auth")
}
