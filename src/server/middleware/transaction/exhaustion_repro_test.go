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

// The connection-exhaustion repro for harbor-next #850 / #856 / #92. It is
// behind its own build tag because, by design, it spends most of its runtime
// watching requests that never finish. It needs no database:
//
//	cd src && go test -tags dbpool_repro -count=1 -v -run TestConnectionExhaustion ./server/middleware/transaction/
package transaction_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/user"
	ormmw "github.com/goharbor/harbor/src/server/middleware/orm"
	txmw "github.com/goharbor/harbor/src/server/middleware/transaction"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

// maxConns is the pool size under test. The wedge threshold is exactly this
// many concurrent two-connection requests, and one fewer always completes.
const maxConns = 4

// observeFor is how long a wedged phase is watched before it is called a
// deadlock. The stub pool answers instantly, so a healthy request needs
// microseconds and nothing in the stack has a timeout that could expire later.
const observeFor = 3 * time.Second

// gate makes the overlap deterministic. In production, N requests happen to be
// inside their transactions at the same moment because of load; here every
// request announces that it is holding its first connection and waits until
// the phase's full set has done the same before reaching for a second.
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
// GET path is the innocent bystander: no transaction, one pooled read.
func handler(g *gate) http.Handler {
	skipGET := func(r *http.Request) bool { return r.Method == http.MethodGet }

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Method != http.MethodGet {
			// Holding connection #1 (the request transaction) at this point.
			g.arrive()
			// The shape user.go:61 had before #888: a fresh orm context, so a
			// second connection from the same pool.
			ctx = orm.Context()
		}
		// The stub pool returns no rows, so this is a not-found; the request
		// completing at all is what the phases count.
		_, _ = user.Mgr.Get(ctx, 1)
		w.WriteHeader(http.StatusOK)
	})

	return ormmw.Middleware()(txmw.Middleware(skipGET)(inner))
}

// result counts how many of a phase's requests finished, so a phase can
// distinguish "slow" from "never".
type result struct {
	done    atomic.Int64
	wg      sync.WaitGroup
	cancels []context.CancelFunc
}

func fire(url, method string, n int) *result {
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
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
	}
	return r.done.Load()
}

func (r *result) cancelAll() {
	for _, c := range r.cancels {
		c()
	}
}

func TestConnectionExhaustion(t *testing.T) {
	pool := ormtesting.RegisterLimitedPool(t, maxConns)

	g := newGate()
	srv := httptest.NewServer(handler(g))
	// Deliberately not closed: httptest.Server.Close waits for the handlers this
	// test strands, the same reason a wedged core only comes back by being killed.

	t.Logf("pool MaxConns=%d, observation window=%s", maxConns, observeFor)

	// Phase 1: one below the pool size. Every request still needs two
	// connections, but one is always free, so they serialise and all finish.
	g.reset(maxConns - 1)
	below := fire(srv.URL, http.MethodPost, maxConns-1)
	if got := below.waitFor(observeFor); got != int64(maxConns-1) {
		t.Fatalf("phase 1: %d/%d requests completed at concurrency %d; expected all of them",
			got, maxConns-1, maxConns-1)
	}
	t.Logf("phase 1 PASS: %d concurrent two-connection requests all completed", maxConns-1)

	// Phase 2: at the pool size. Every connection is held by a request that
	// needs one more, and nothing in the stack bounds the wait.
	g.reset(maxConns)
	at := fire(srv.URL, http.MethodPost, maxConns)
	got := at.waitFor(observeFor)
	stats := pool.Stats()
	if got != 0 {
		t.Fatalf("phase 2: %d/%d requests completed at concurrency %d; expected a deadlock",
			got, maxConns, maxConns)
	}
	if stats.InUse != maxConns {
		t.Fatalf("phase 2: expected all %d connections held, found %d in use", maxConns, stats.InUse)
	}
	t.Logf("phase 2 WEDGED: 0/%d requests completed in %s; %d/%d connections held, every request parked on a second",
		maxConns, observeFor, stats.InUse, maxConns)

	// Phase 3: an unrelated read. GET skips the transaction middleware, but it
	// still needs one pooled connection, so the wedge is total.
	unrelated := fire(srv.URL, http.MethodGet, 1)
	if n := unrelated.waitFor(observeFor); n != 0 {
		t.Errorf("phase 3: the unrelated GET completed (%d); expected it to hang with the rest", n)
	} else {
		t.Logf("phase 3 CONFIRMED: an unrelated GET that writes nothing also hangs")
	}

	// Phase 4: no self-recovery. beego's Ormer.Begin passes
	// context.Background() to BeginTx and the acquire has no deadline, so
	// hanging up on every client releases nothing.
	at.cancelAll()
	unrelated.cancelAll()
	time.Sleep(time.Second)
	if inUse := pool.Stats().InUse; inUse != maxConns {
		t.Errorf("phase 4: %d connections held after every client hung up; expected %d", inUse, maxConns)
	} else {
		t.Logf("phase 4 CONFIRMED: all %d connections still held after every client hung up; "+
			"only a process restart clears them", inUse)
	}
}
