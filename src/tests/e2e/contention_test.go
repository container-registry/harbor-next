//go:build e2e

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

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestConcurrentSharedLayerPushBurst reproduces the incident shape: many
// concurrent pushes into ONE unlimited-quota project, all images sharing
// the same base layers. Pre-fix this serializes on the project's
// quota_usage row (reserve CAS, no backoff) and on the shared blob rows
// (Touch per probe); the assertion is simply that every push succeeds and
// the burst completes in bounded time.
func TestConcurrentSharedLayerPushBurst(t *testing.T) {
	e := newEnv()
	project := fmt.Sprintf("e2e-burst-%d", time.Now().UnixMilli())
	if err := e.createProject(project, -1); err != nil {
		t.Fatalf("create project: %v", err)
	}

	shared := []blob{randomBlob(256 << 10), randomBlob(256 << 10), randomBlob(256 << 10)}

	const workers = 16
	const perWorker = 4
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				layers := append([]blob{}, shared...)
				layers = append(layers, randomBlob(64<<10)) // unique layer per image
				repo := fmt.Sprintf("%s/burst-%d", project, w%4)
				if err := e.pushImage(repo, fmt.Sprintf("w%d-i%d", w, i), layers); err != nil {
					errs <- fmt.Errorf("worker %d push %d: %w", w, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
	t.Logf("burst: %d pushes across %d workers in %s", workers*perWorker, workers, time.Since(start))
}

// TestQuotaEnforcementDeniesOverLimit guards the unlimited-skip change:
// projects with a REAL storage limit must still be denied synchronously.
func TestQuotaEnforcementDeniesOverLimit(t *testing.T) {
	e := newEnv()
	project := fmt.Sprintf("e2e-limited-%d", time.Now().UnixMilli())
	if err := e.createProject(project, 1<<20); err != nil { // 1 MiB limit
		t.Fatalf("create project: %v", err)
	}

	over := randomBlob(2 << 20) // 2 MiB blob
	err := e.pushBlob(project+"/limited", over)
	if err == nil {
		t.Fatal("push over quota succeeded; enforcement is broken")
	}
	t.Logf("over-quota push denied as expected: %v", err)

	// and a small push inside the limit still works
	small := randomBlob(64 << 10)
	if err := e.pushImage(project+"/limited", "ok", []blob{small}); err != nil {
		t.Fatalf("within-quota push failed: %v", err)
	}
}

// TestUsageConvergesOnUnlimitedProject: with the reservation skipped on
// unlimited projects, the usage figure is maintained by the refresh path
// (sync per request, or coalesced when QUOTA_ASYNC_REFRESH_DURATION is
// set). Either way it must converge to the true stored bytes.
func TestUsageConvergesOnUnlimitedProject(t *testing.T) {
	e := newEnv()
	project := fmt.Sprintf("e2e-usage-%d", time.Now().UnixMilli())
	if err := e.createProject(project, -1); err != nil {
		t.Fatalf("create project: %v", err)
	}

	layers := []blob{randomBlob(512 << 10), randomBlob(256 << 10)}
	if err := e.pushImage(project+"/usage", "v1", layers); err != nil {
		t.Fatalf("push: %v", err)
	}

	// used must cover at least the layer bytes (config+manifest add a bit);
	// poll until that bound, not merely non-zero, so a partially refreshed
	// intermediate value cannot end the wait early
	var minExpected int64
	for _, l := range layers {
		minExpected += int64(len(l.data))
	}
	deadline := time.Now().Add(90 * time.Second)
	var used int64
	var err error
	for time.Now().Before(deadline) {
		used, err = e.projectUsedStorage(project)
		if err == nil && used >= minExpected {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if used < minExpected {
		t.Fatalf("usage did not converge: used=%d < expected>=%d", used, minExpected)
	}
	t.Logf("usage converged: %d bytes (expected >= %d)", used, minExpected)
}

// TestTouchDebounce verifies the blob fix at the database level: repeated
// HEAD probes of a healthy blob must NOT rewrite its row while its
// update_time is fresh (pre-fix: every HEAD bumps version and update_time).
func TestTouchDebounce(t *testing.T) {
	e := newEnv()
	db, err := sql.Open("pgx", e.dbDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("database not reachable (%v); set E2E_DB_DSN", err)
	}

	project := fmt.Sprintf("e2e-touch-%d", time.Now().UnixMilli())
	if err := e.createProject(project, -1); err != nil {
		t.Fatalf("create project: %v", err)
	}
	layer := randomBlob(128 << 10)
	repo := project + "/touch"
	if err := e.pushImage(repo, "v1", []blob{layer}); err != nil {
		t.Fatalf("push: %v", err)
	}

	row := func() (int64, time.Time) {
		// per-query context: the outer 10s ctx only guards the initial
		// ping, and the second row read happens after a push plus 25
		// HEAD probes, which may outlive it
		qctx, qcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer qcancel()
		var v int64
		var ut time.Time
		err := db.QueryRowContext(qctx,
			"SELECT version, update_time FROM blob WHERE digest = $1", layer.digest).Scan(&v, &ut)
		if err != nil {
			t.Fatalf("query blob row: %v", err)
		}
		return v, ut
	}

	v0, t0 := row()
	const probes = 25
	for i := 0; i < probes; i++ {
		code, err := e.headBlob(repo, layer)
		if err != nil || code != http.StatusOK {
			t.Fatalf("head %d: code=%d err=%v", i, code, err)
		}
	}
	v1, t1 := row()

	if v1 != v0 || !t1.Equal(t0) {
		t.Fatalf("debounce failed: %d HEAD probes rewrote the fresh blob row (version %d->%d, update_time %s->%s)",
			probes, v0, v1, t0, t1)
	}
	t.Logf("debounce ok: %d HEAD probes, blob row untouched (version=%d)", probes, v0)
}

// TestAsyncRefreshConvergence: tighter bound when the target core runs with
// QUOTA_ASYNC_REFRESH_DURATION - usage must appear within ~3 intervals.
func TestAsyncRefreshConvergence(t *testing.T) {
	if os.Getenv("E2E_ASYNC_REFRESH") != "1" {
		t.Skip("set E2E_ASYNC_REFRESH=1 when core runs with QUOTA_ASYNC_REFRESH_DURATION")
	}
	e := newEnv()
	project := fmt.Sprintf("e2e-async-%d", time.Now().UnixMilli())
	if err := e.createProject(project, -1); err != nil {
		t.Fatalf("create project: %v", err)
	}
	layer := randomBlob(256 << 10)
	if err := e.pushImage(project+"/async", "v1", []blob{layer}); err != nil {
		t.Fatalf("push: %v", err)
	}

	// assume interval <= 10s; allow 3 intervals + slack
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		used, err := e.projectUsedStorage(project)
		if err == nil && used >= int64(len(layer.data)) {
			t.Logf("async refresh converged: used=%d", used)
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("async refresh did not converge within 35s")
}
