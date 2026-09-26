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

//go:build dbpool_repro

// Package dbpool_test holds the connection-exhaustion repro for harbor-next
// #850 / #856 / #92. It is behind its own build tag because it needs a
// dedicated PostgreSQL and, by design, spends most of its runtime watching
// requests that never finish. Run it through tests/dbpool-exhaustion/run.sh.
package dbpool_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/dbpool"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/migration"
	"github.com/goharbor/harbor/src/pkg/user"
	ormmw "github.com/goharbor/harbor/src/server/middleware/orm"
	txmw "github.com/goharbor/harbor/src/server/middleware/transaction"
)

// maxConns is the pool size under test. Everything the repro claims is stated
// relative to it: the wedge threshold is exactly this many concurrent
// two-connection requests, and one fewer always completes.
var maxConns = envInt("REPRO_MAX_CONNS", 4)

// observeFor is how long a wedged phase is watched before it is called a
// deadlock. Any value comfortably above a healthy request's latency works;
// nothing in the stack has a timeout that could expire later.
var observeFor = envDuration("REPRO_OBSERVE", 20*time.Second)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func pgConfig(tb testing.TB) *models.PostGreSQL {
	tb.Helper()
	port := envInt("REPRO_PG_PORT", 55432)
	return &models.PostGreSQL{
		Host:         envString("REPRO_PG_HOST", "127.0.0.1"),
		Port:         port,
		Username:     envString("REPRO_PG_USER", "postgres"),
		Password:     envString("REPRO_PG_PASSWORD", "root123"),
		Database:     envString("REPRO_PG_DATABASE", "registry"),
		SSLMode:      "disable",
		MaxOpenConns: maxConns,
		MinConns:     ptr(int32(0)),
	}
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func ptr[T any](v T) *T { return &v }

// observer opens one connection outside the pool under test so the repro can
// read pg_stat_activity while that pool is wedged.
func observer(tb testing.TB, cfg *models.PostGreSQL) *sql.DB {
	tb.Helper()
	// Same builder the pool uses, so a password with spaces, quotes or
	// backslashes is escaped identically here and in the pool under test.
	db, err := sql.Open("pgx", dbpool.BuildDSN(cfg))
	if err != nil {
		tb.Fatalf("observer connection: %v", err)
	}
	db.SetMaxOpenConns(2)
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

// idleInTransaction counts the sessions Harbor is holding open with a
// transaction it is not using — the signature every incident report in #856
// leads with.
func idleInTransaction(tb testing.TB, db *sql.DB) int {
	tb.Helper()
	var n int
	err := db.QueryRow(`
		SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database()
		  AND state = 'idle in transaction'
		  AND pid <> pg_backend_pid()`).Scan(&n)
	if err != nil {
		tb.Fatalf("read pg_stat_activity: %v", err)
	}
	return n
}

// gate makes the overlap deterministic. In production, N requests happen to be
// inside their transactions at the same moment because of load; here every
// request announces that it is holding its first connection and waits until
// the phase's full set has done the same before reaching for a second. The
// gate adds no connection use of its own — it only removes the race that would
// otherwise decide whether a run reproduces the bug.
type gate struct {
	mu      sync.Mutex
	want    int
	arrived int
	ch      chan struct{}
}

func newGate() *gate { return &gate{ch: make(chan struct{})} }

// reset arms the gate for a phase of n concurrent requests.
func (g *gate) reset(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.want = n
	g.arrived = 0
	g.ch = make(chan struct{})
}

// arrive blocks until the whole set has arrived, or until the fallback expires
// so that a miscounted phase degrades to "slow" rather than "hung in the gate".
func (g *gate) arrive() {
	g.mu.Lock()
	g.arrived++
	ch := g.ch
	if g.arrived >= g.want {
		close(g.ch)
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
	}
}

// handler builds the middleware chain core actually runs, trimmed to the two
// links that own connections: orm.Middleware puts a pooled Ormer in the
// context, transaction.Middleware opens a transaction (and so takes a
// connection) for every non-GET request.
//
// The POST path reproduces the #850 shape: work inside the request transaction
// that builds its own orm context instead of using the request's, which means
// a second connection from the same pool while the first is still held. The
// GET path is the innocent bystander — no transaction, no second connection,
// one pooled read.
func handler(g *gate) http.Handler {
	skipGET := func(r *http.Request) bool { return r.Method == http.MethodGet }

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Method != http.MethodGet {
			// Holding connection #1 (the request transaction) at this point.
			g.arrive()
			// src/pkg/auditext/event/user/user.go:61 does exactly this: a
			// fresh orm context, so a second connection from the same pool.
			ctx = orm.Context()
		}
		if _, err := user.Mgr.Get(ctx, 1); err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return ormmw.Middleware()(txmw.Middleware(skipGET)(inner))
}

// fire sends n concurrent requests and reports how many finished, so a phase
// can distinguish "slow" from "never".
type result struct {
	done    atomic.Int64
	wg      sync.WaitGroup
	cancels []context.CancelFunc
}

func fire(tb testing.TB, url, method string, n int) *result {
	tb.Helper()
	res := &result{}
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		res.cancels = append(res.cancels, cancel)
		res.wg.Add(1)
		go func() {
			defer res.wg.Done()
			req, err := http.NewRequestWithContext(ctx, method, url, nil)
			if err != nil {
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			res.done.Add(1)
		}()
	}
	return res
}

func (r *result) waitFor(d time.Duration) int64 {
	deadline := time.After(d)
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-deadline:
	}
	return r.done.Load()
}

func (r *result) cancelAll() {
	for _, c := range r.cancels {
		c()
	}
}

func TestConnectionExhaustion(t *testing.T) {
	cfg := pgConfig(t)

	db := &models.Database{Type: "postgresql", PostGreSQL: cfg}
	// Same order as src/core/main.go: register the pool first, because the
	// authoritative-schema phase of Migrate runs through it.
	if err := dao.InitDatabase(db); err != nil {
		t.Fatalf("init database: %v", err)
	}
	if err := migration.Migrate(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}

	watch := observer(t, cfg)

	g := newGate()
	srv := httptest.NewServer(handler(g))
	// Deliberately not closed, and the pool is deliberately not drained.
	// httptest.Server.Close and pgxpool.Close both wait for the connections
	// this test strands, so a clean teardown is impossible by construction —
	// the same reason a wedged core only comes back by being killed.

	t.Logf("pool MaxConns=%d, observation window=%s", maxConns, observeFor)

	// Phase 1 — one below the pool size. Every request still needs two
	// connections, but one connection is always free, so they serialise and
	// all of them finish.
	g.reset(maxConns - 1)
	below := fire(t, srv.URL, http.MethodPost, maxConns-1)
	if got := below.waitFor(observeFor); got != int64(maxConns-1) {
		t.Fatalf("phase 1: %d/%d requests completed at concurrency %d; expected all of them",
			got, maxConns-1, maxConns-1)
	}
	t.Logf("phase 1 PASS: %d concurrent two-connection requests all completed", maxConns-1)

	// Phase 2 — at the pool size. Every connection is held by a request that
	// needs one more, and nothing in the stack bounds the wait.
	g.reset(maxConns)
	at := fire(t, srv.URL, http.MethodPost, maxConns)
	got := at.waitFor(observeFor)
	held := idleInTransaction(t, watch)
	if got != 0 {
		t.Fatalf("phase 2: %d/%d requests completed at concurrency %d; expected a deadlock",
			got, maxConns, maxConns)
	}
	t.Logf("phase 2 WEDGED: 0/%d requests completed in %s; %d sessions idle in transaction",
		maxConns, observeFor, held)
	if held != maxConns {
		// Phases 3 and 4 assume the exact maxConns-session wedge, so a partial
		// one has to stop the run rather than cascade into their assertions.
		t.Fatalf("phase 2: expected %d sessions idle in transaction, found %d", maxConns, held)
	}

	// Phase 3 — an unrelated read. GET skips the transaction middleware and
	// touches none of the rows above, but it still needs one pooled
	// connection, so the wedge is total rather than per-endpoint.
	unrelated := fire(t, srv.URL, http.MethodGet, 1)
	if n := unrelated.waitFor(observeFor); n != 0 {
		t.Errorf("phase 3: the unrelated GET completed (%d); expected it to hang with the rest", n)
	} else {
		t.Logf("phase 3 CONFIRMED: an unrelated GET that writes nothing also hangs")
	}

	// Phase 4 — no self-recovery. beego's Ormer.Begin passes
	// context.Background() to BeginTx, and pgxpool has no acquire timeout, so
	// hanging up on every client releases nothing: the handler goroutines stay
	// parked in Acquire and the transactions stay open.
	at.cancelAll()
	unrelated.cancelAll()
	time.Sleep(2 * time.Second)
	if held := idleInTransaction(t, watch); held != maxConns {
		t.Errorf("phase 4: %d sessions still idle in transaction after every client hung up; expected %d",
			held, maxConns)
	} else {
		t.Logf("phase 4 CONFIRMED: all %d transactions still open after every client hung up; "+
			"only a process restart clears them", held)
	}
}
