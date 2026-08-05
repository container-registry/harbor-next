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

package exporter

import (
	"sync"
	"sync/atomic"
	"time"
)

// c is replaced wholesale by CacheInit while the cleaner goroutine and
// in-flight scrapes read it, so the pointer itself is atomic. Every operation
// takes one stable snapshot up front and works on that; a concurrent CacheInit
// then races only on which generation an entry lands in, never on the map.
var c atomic.Pointer[cache]

const defaultCacheCleanInterval = 10

type cachedValue struct {
	Value      any
	Expiration int64
}

type cache struct {
	CacheDuration int64
	store         map[string]cachedValue
	*sync.RWMutex
}

// CacheGet get a value from cache
func CacheGet(key string) (value any, ok bool) {
	cc := c.Load()
	if cc == nil {
		return nil, false
	}
	cc.RLock()
	v, ok := cc.store[key]
	cc.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().Unix() > v.Expiration {
		cc.Lock()
		delete(cc.store, key)
		cc.Unlock()
		return nil, false
	}
	return v.Value, true
}

// CachePut put a value to cache with key
func CachePut(key, value any) {
	cc := c.Load()
	if cc == nil {
		return
	}
	cc.Lock()
	defer cc.Unlock()
	cc.store[key.(string)] = cachedValue{
		Value:      value,
		Expiration: time.Now().Unix() + cc.CacheDuration,
	}
}

// CacheDelete delete a key from cache
func CacheDelete(key string) {
	cc := c.Load()
	if cc == nil {
		return
	}
	cc.Lock()
	defer cc.Unlock()
	delete(cc.store, key)
}

// StartCacheCleaner start a cache clean job
func StartCacheCleaner() {
	cc := c.Load()
	if cc == nil {
		return
	}
	// Expiration is stored in Unix seconds by CachePut. Comparing it against
	// UnixNano made every entry look expired, so each tick emptied the whole
	// cache and the configured cache duration was never reached.
	now := time.Now().Unix()
	cc.Lock()
	defer cc.Unlock()
	for k, v := range cc.store {
		if v.Expiration < now {
			delete(cc.store, k)
		}
	}
}

// CacheEnabled returns if the cache in exporter enabled
func CacheEnabled() bool {
	return c.Load() != nil
}

// cleanerOnce keeps CacheInit from leaking a new cleaner goroutine on every
// call; the ticker lives for the whole process either way. The interval is held
// separately so a later CacheInit still takes effect on the running cleaner.
var (
	cleanerOnce      sync.Once
	cleanIntervalSec atomic.Int64
)

// CacheInit add cache to exporter
func CacheInit(opt *Opt) {
	c.Store(&cache{
		CacheDuration: opt.CacheDuration,
		store:         make(map[string]cachedValue),
		RWMutex:       &sync.RWMutex{},
	})
	interval := opt.CacheCleanInterval
	if interval <= 0 {
		interval = defaultCacheCleanInterval
	}
	cleanIntervalSec.Store(interval)

	cleanerOnce.Do(func() {
		go func() {
			for {
				time.Sleep(time.Duration(cleanIntervalSec.Load()) * time.Second)
				StartCacheCleaner()
			}
		}()
	})
}
