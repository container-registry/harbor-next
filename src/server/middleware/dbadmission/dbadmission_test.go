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

package dbadmission

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/server/middleware"
)

func staticStats(acquired, maxConns int32) func() (int32, int32) {
	return func() (int32, int32) { return acquired, maxConns }
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestPoolNotInitialized_Admitted(t *testing.T) {
	h := MiddlewareWithConfig(Config{Stats: staticStats(0, 0)})(okHandler())
	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
}

func TestSequentialRequests_AlwaysAdmitted(t *testing.T) {
	// even with the pool reported exhausted, sequential requests never
	// accumulate in-flight slots (limit for pool size 1 is 3)
	h := MiddlewareWithConfig(Config{Stats: staticStats(1, 1)})(okHandler())
	for range 25 {
		assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
	}
}

// blockingHandler blocks every request until release is closed, so tests can
// pin down in-flight slots. After release it answers immediately with 200.
type blockingHandler struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{started: make(chan struct{}, 64), release: make(chan struct{})}
}

func (b *blockingHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	b.started <- struct{}{}
	<-b.release
	w.WriteHeader(http.StatusOK)
}

// fill occupies n in-flight slots of h with concurrent requests and returns a
// func that unblocks them and waits for completion.
func (b *blockingHandler) fill(t *testing.T, h http.Handler, n int) func() {
	t.Helper()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2.0/projects", nil))
		}()
	}
	for range n {
		<-b.started
	}
	return func() {
		close(b.release)
		wg.Wait()
	}
}

func TestInflightLimitFull_Rejected429(t *testing.T) {
	// pool size 1 → in-flight limit 1 + 2x = 3
	b := newBlockingHandler()
	h := MiddlewareWithConfig(Config{Stats: staticStats(1, 1)})(b)
	release := b.fill(t, h, 3)

	rec := doGet(t, h, "/api/v2.0/projects")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "TOO_MANY_REQUEST")
	assert.Contains(t, rec.Body.String(), "DB connections exhausted")
	assert.Equal(t, "1", rec.Header().Get("Retry-After"),
		"429 must carry Retry-After so registry clients back off instead of hammering")

	release()

	// slots released → admitted again
	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
}

// Regression for the burst race: every request can observe spare pool capacity
// (acquired < maxConns) before any of them acquires a connection. The bound
// must come from the atomic in-flight reservation, not from pool stats.
func TestSpareCapacityObserved_BoundStillEnforced(t *testing.T) {
	// stats always claim the pool is empty; limit for pool size 1 is 3
	b := newBlockingHandler()
	h := MiddlewareWithConfig(Config{Stats: staticStats(0, 1)})(b)
	release := b.fill(t, h, 3)
	defer release()

	rec := doGet(t, h, "/api/v2.0/projects")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code,
		"reported spare capacity must not bypass the in-flight bound")
}

func TestSkipper_BypassesAdmission(t *testing.T) {
	skipper := middleware.MethodAndPathSkipper(http.MethodGet, regexp.MustCompile("^/api/v2.0/ping"))
	statsCalled := false
	h := MiddlewareWithConfig(Config{Stats: func() (int32, int32) {
		statsCalled = true
		return 0, 0
	}}, skipper)(okHandler())

	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/ping").Code)
	assert.False(t, statsCalled, "stats must not be consulted for skipped requests")
}

// Hammer the middleware from many goroutines and verify the hard invariant:
// the number of requests inside the next handler never exceeds the cap, and
// every request is answered with either 200 or 429 — nothing hangs or leaks.
func TestConcurrentBurst_NeverExceedsLimit(t *testing.T) {
	const maxConns = 2 // in-flight limit = 6
	limit := int64(maxConns * (1 + WaiterFactor))

	var cur, peak atomic.Int64
	h := MiddlewareWithConfig(Config{Stats: staticStats(maxConns, maxConns)})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			c := cur.Add(1)
			for {
				p := peak.Load()
				if c <= p || peak.CompareAndSwap(p, c) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			cur.Add(-1)
			w.WriteHeader(http.StatusOK)
		}))

	const total = 200
	var ok200, ok429, other atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2.0/projects", nil))
			switch rec.Code {
			case http.StatusOK:
				ok200.Add(1)
			case http.StatusTooManyRequests:
				ok429.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.LessOrEqual(t, peak.Load(), limit, "in-flight requests must never exceed the cap")
	assert.Zero(t, other.Load(), "every request must be answered 200 or 429")
	assert.Equal(t, int64(total), ok200.Load()+ok429.Load())
	assert.Zero(t, cur.Load(), "all slots must be released")

	// counter fully drained → the very next request is admitted
	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
}

// Rejected requests must give their reservation back immediately: after a wave
// of 429s and the release of the held slots, a full limit's worth of sequential
// requests all pass — proof the counter drained to zero with no leaked slots.
func TestRejections_DoNotLeakSlots(t *testing.T) {
	b := newBlockingHandler()
	h := MiddlewareWithConfig(Config{Stats: staticStats(1, 1)})(b) // limit 3
	release := b.fill(t, h, 3)

	for range 5 {
		assert.Equal(t, http.StatusTooManyRequests, doGet(t, h, "/api/v2.0/projects").Code)
	}
	release()

	for range 3 {
		assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
	}
}

// The ping skipper must keep working while the gate is rejecting everything —
// that is what lets the health endpoint answer during an outage.
func TestSkippedRequests_AdmittedWhileLimitFull(t *testing.T) {
	skipper := middleware.MethodAndPathSkipper(http.MethodGet, regexp.MustCompile("^/api/v2.0/ping"))
	b := newBlockingHandler()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2.0/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		b.ServeHTTP(w, r)
	})
	h := MiddlewareWithConfig(Config{Stats: staticStats(1, 1)}, skipper)(inner) // limit 3
	release := b.fill(t, h, 3)
	defer release()

	assert.Equal(t, http.StatusTooManyRequests, doGet(t, h, "/api/v2.0/projects").Code,
		"gate must be full for this test to mean anything")
	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/ping").Code,
		"skipped paths must bypass a full gate")
}

// Middleware() uses DefaultConfig; with no pool registered (unit-test env,
// dao.GetPool() == nil) admission control must stand down, not block or panic.
func TestDefaultMiddleware_NoPool_PassesThrough(t *testing.T) {
	h := Middleware()(okHandler())
	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
}

func TestNilStats_FallsBackToDefault(t *testing.T) {
	h := MiddlewareWithConfig(Config{})(okHandler())
	assert.Equal(t, http.StatusOK, doGet(t, h, "/api/v2.0/projects").Code)
}
