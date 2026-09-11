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

// Package dbadmission rejects requests instead of queueing them forever when
// the database connection pool is exhausted. Beego ORM acquires connections
// with context.Background(), so a request arriving while the pool is exhausted
// blocks indefinitely; under sustained load this turns into a system-wide hang
// with no observable progress. This middleware bounds that queue by capping
// in-flight requests at pool size x (1 + WaiterFactor): up to pool size
// requests can hold a connection and up to WaiterFactor x pool size can wait
// for one, and everything beyond that is rejected with 429 so clients can back
// off and retry.
//
// Core applies this middleware only to an explicit allowlist of interactive
// user-facing routes (see core/middlewares.dbAdmissionGated): internal
// machine-to-machine callers — jobservice status callbacks, GC and scan
// hooks, registry token auth, the /v2/ data plane — do not all handle an
// unexpected 429 safely, so they are never gated.
package dbadmission

import (
	"net/http"
	"sync/atomic"

	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/lib/errors"
	lib_http "github.com/goharbor/harbor/src/lib/http"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/server/middleware"
)

// WaiterFactor caps the number of requests allowed to wait for a connection
// while the pool is exhausted, as a multiple of the configured pool size.
const WaiterFactor = 2

// Config defines the config for the dbadmission middleware.
type Config struct {
	// Stats returns the number of acquired connections and the pool's max
	// size. A maxConns of 0 disables admission control (pool not initialized).
	Stats func() (acquired, maxConns int32)
}

// DefaultConfig reads the stats of the active pool registered by dao.
var DefaultConfig = Config{
	Stats: func() (int32, int32) {
		pool := dao.GetPool()
		if pool == nil {
			return 0, 0
		}
		stat := pool.PgxPool().Stat()
		return stat.AcquiredConns(), stat.MaxConns()
	},
}

// Middleware rejects requests with 429 when the DB connection pool is
// exhausted and the waiter queue is full, using the default config.
func Middleware(skippers ...middleware.Skipper) func(http.Handler) http.Handler {
	return MiddlewareWithConfig(DefaultConfig, skippers...)
}

// MiddlewareWithConfig rejects requests with 429 when the DB connection pool
// is exhausted and the waiter queue is full.
func MiddlewareWithConfig(config Config, skippers ...middleware.Skipper) func(http.Handler) http.Handler {
	if config.Stats == nil {
		config.Stats = DefaultConfig.Stats
	}

	// In-flight requests admitted past this middleware. pgxpool exposes no
	// waiter count and gating on pool stats alone is racy (a whole burst can
	// observe a free connection before any of them acquires one and then
	// queue unbounded), so reserve a slot here unconditionally, before
	// looking at anything, and hold it for the request lifetime.
	var inflight atomic.Int64

	return middleware.New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		acquired, maxConns := config.Stats()
		if maxConns <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		limit := int64(maxConns) * (1 + WaiterFactor)
		if inflight.Add(1) > limit {
			n := inflight.Add(-1)
			log.Warningf("DB connections exhausted: rejecting %s %s with 429 (%d/%d connections in use, %d requests in flight, limit %d)",
				r.Method, r.URL.Path, acquired, maxConns, n, limit)
			w.Header().Set("Retry-After", "1")
			lib_http.SendError(w, errors.New(nil).WithCode(errors.RateLimitCode).
				WithMessage("DB connections exhausted, please retry later"))
			return
		}
		defer inflight.Add(-1)

		next.ServeHTTP(w, r)
	}, skippers...)
}
