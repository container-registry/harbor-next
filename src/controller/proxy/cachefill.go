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

package proxy

import (
	"os"
	"strconv"

	"github.com/goharbor/harbor/src/lib/log"
)

const (
	// maxConcurrentCacheFillEnvKey overrides the maximum number of concurrent
	// background proxy-cache tasks. Empty, non-numeric or non-positive values
	// fall back to defaultMaxConcurrentCacheFill.
	maxConcurrentCacheFillEnvKey = "PROXY_CACHE_MAX_CONCURRENT_FILL"

	// defaultMaxConcurrentCacheFill bounds the number of background proxy-cache
	// goroutines that may run at once. Proxy-cache fills run detached from the
	// inbound request (they outlive it on purpose, to populate the local
	// cache), so without a cap a slow or unresponsive backend lets them
	// accumulate one goroutine plus one held socket each, without bound. The
	// default is well above healthy steady-state concurrency while keeping
	// worst-case resource use bounded.
	defaultMaxConcurrentCacheFill = 100
)

// cacheFillSem bounds concurrent background proxy-cache tasks. It is sized once
// at process start from PROXY_CACHE_MAX_CONCURRENT_FILL.
var cacheFillSem = make(chan struct{}, maxConcurrentCacheFill())

func maxConcurrentCacheFill() int {
	if env := os.Getenv(maxConcurrentCacheFillEnvKey); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			return n
		}
		log.Warningf("invalid %s=%q, using default %d", maxConcurrentCacheFillEnvKey, env, defaultMaxConcurrentCacheFill)
	}
	return defaultMaxConcurrentCacheFill
}

// GoCacheFill runs fn in a background goroutine bounded by a global concurrency
// limit (PROXY_CACHE_MAX_CONCURRENT_FILL). Proxy-cache background work is
// best-effort and intentionally outlives the inbound request, so when the limit
// is already reached the task is skipped (and logged) rather than queued -- a
// slow or unresponsive backend therefore cannot make these goroutines, and the
// sockets they hold, accumulate without bound. It returns true if fn was
// scheduled, false if it was skipped.
func GoCacheFill(label string, fn func()) bool {
	sem := cacheFillSem
	select {
	case sem <- struct{}{}:
	default:
		log.Warningf("proxy-cache background task %q skipped: concurrency limit (%d) reached", label, cap(sem))
		return false
	}
	go func() {
		defer func() { <-sem }()
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("proxy-cache background task %q panicked: %v", label, r)
			}
		}()
		fn()
	}()
	return true
}
