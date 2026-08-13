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

package jwkscache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/federatedidp/dao"
	"github.com/goharbor/harbor/src/pkg/federatedidp/model"
)

// Manager handles two-tier JWKS caching with DB as primary
// Uses sync.Map to avoid mutex contention in high-traffic scenarios
type Manager struct {
	inMemoryCache sync.Map // map[string]*inMemoryEntry - lock-free for reads
	config        Config
	dao           dao.DAO
}

type inMemoryEntry struct {
	jwks      jwk.Set
	expiresAt time.Time
}

// NewManager creates a new cache manager
func NewManager() *Manager {
	return &Manager{
		config: LoadConfig(),
		dao:    dao.New(),
	}
}

// GetJWKS retrieves JWKS using DB-primary two-tier caching
// Flow: In-Memory -> DB (primary) -> Remote (rate-limited)
func (m *Manager) GetJWKS(ctx context.Context, idp *model.FederatedIdp) (jwk.Set, error) {
	// Tier 1: Check in-memory cache (per-process optimization)
	if set := m.getFromInMemory(idp.JWKSURI); set != nil {
		log.Debugf("JWKS cache hit (in-memory) for %s", idp.JWKSURI)
		return set, nil
	}

	// Tier 2: Check DB cache (primary, shared across instances)
	if m.isDBCacheValid(idp) {
		set, err := jwk.Parse([]byte(idp.JWKSKeys))
		if err == nil {
			log.Debugf("JWKS cache hit (DB) for %s", idp.JWKSURI)
			m.saveToInMemory(idp.JWKSURI, set, *idp.JWKSExpiresAt)
			return set, nil
		}
		log.Warningf("DB cache parse failed for %s: %v", idp.JWKSURI, err)
	}

	// Tier 3: DB cache expired or invalid - check rate limit before fetching
	if m.isRateLimited(idp) {
		// Rate limited - return DB cache indefinitely (no stale expiry)
		if idp.JWKSKeys != "" && idp.JWKSKeys != "{}" {
			set, err := jwk.Parse([]byte(idp.JWKSKeys))
			if err == nil {
				log.Warningf("JWKS rate limited, using DB cache for %s", idp.JWKSURI)
				return set, nil
			}
		}
		return nil, fmt.Errorf("JWKS fetch rate limited and no valid cache for %s", idp.JWKSURI)
	}

	// Not rate limited - fetch from remote
	log.Infof("JWKS cache miss, fetching from %s", idp.JWKSURI)
	return m.fetchAndCache(ctx, idp)
}

// ForceRefresh fetches JWKS from remote regardless of cache state
// Called on IdP creation and any IdP edit (bypasses rate limiting)
func (m *Manager) ForceRefresh(ctx context.Context, idpID int64, jwksURI string) error {
	log.Infof("Force refreshing JWKS for IdP %d from %s", idpID, jwksURI)

	// Fetch from remote
	set, ttl, err := m.fetchWithTTL(ctx, jwksURI)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS from %s: %w", jwksURI, err)
	}

	// Update DB cache
	if err := m.updateDBCache(ctx, idpID, set, ttl); err != nil {
		return fmt.Errorf("failed to update DB cache: %w", err)
	}

	// Update in-memory cache
	m.saveToInMemory(jwksURI, set, time.Now().Add(ttl))

	// Update fetch attempt timestamp
	if err := m.markFetchAttempt(ctx, idpID); err != nil {
		log.Warningf("Failed to mark fetch attempt: %v", err)
	}

	return nil
}

// InvalidateInMemory removes entry from in-memory cache (called on IdP delete)
func (m *Manager) InvalidateInMemory(jwksURI string) {
	m.inMemoryCache.Delete(jwksURI)
}

// getFromInMemory checks the in-memory cache (lock-free read)
func (m *Manager) getFromInMemory(jwksURI string) jwk.Set {
	val, ok := m.inMemoryCache.Load(jwksURI)
	if !ok {
		return nil
	}
	entry := val.(*inMemoryEntry)
	if time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.jwks
}

// saveToInMemory stores JWKS in in-memory cache (lock-free write)
func (m *Manager) saveToInMemory(jwksURI string, set jwk.Set, dbExpiresAt time.Time) {
	// In-memory TTL is min(InMemoryTTL, time until DB expiry)
	inMemoryTTL := m.config.InMemoryTTL
	dbTTL := time.Until(dbExpiresAt)
	if dbTTL > 0 && dbTTL < inMemoryTTL {
		inMemoryTTL = dbTTL
	}

	m.inMemoryCache.Store(jwksURI, &inMemoryEntry{
		jwks:      set,
		expiresAt: time.Now().Add(inMemoryTTL),
	})
}

// isDBCacheValid checks if the DB-cached JWKS is still valid (not expired)
func (m *Manager) isDBCacheValid(idp *model.FederatedIdp) bool {
	if idp.JWKSKeys == "" || idp.JWKSKeys == "{}" {
		return false
	}
	if idp.JWKSExpiresAt == nil {
		return false
	}
	return time.Now().Before(*idp.JWKSExpiresAt)
}

// isRateLimited checks if we should wait before fetching again
func (m *Manager) isRateLimited(idp *model.FederatedIdp) bool {
	if idp.JWKSLastFetchAttempt == nil {
		return false // Never fetched, allow
	}
	nextAllowed := idp.JWKSLastFetchAttempt.Add(m.config.MinFetchInterval)
	return time.Now().Before(nextAllowed)
}

// fetchAndCache fetches JWKS from remote and updates both caches
func (m *Manager) fetchAndCache(ctx context.Context, idp *model.FederatedIdp) (jwk.Set, error) {
	// IMPORTANT: Update last fetch attempt BEFORE fetching (prevents dogpile)
	if err := m.markFetchAttempt(ctx, idp.ID); err != nil {
		log.Warningf("Failed to mark fetch attempt: %v", err)
	}

	// Fetch with headers to determine TTL
	set, ttl, err := m.fetchWithTTL(ctx, idp.JWKSURI)
	if err != nil {
		// Fetch failed - return DB cache indefinitely (no stale expiry)
		if idp.JWKSKeys != "" && idp.JWKSKeys != "{}" {
			staleSet, parseErr := jwk.Parse([]byte(idp.JWKSKeys))
			if parseErr == nil {
				log.Warningf("JWKS fetch failed, using DB cache for %s: %v", idp.JWKSURI, err)
				return staleSet, nil
			}
		}
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", idp.JWKSURI, err)
	}

	// Update DB cache
	if err := m.updateDBCache(ctx, idp.ID, set, ttl); err != nil {
		log.Warningf("Failed to update DB cache for %s: %v", idp.JWKSURI, err)
	}

	// Update in-memory cache
	m.saveToInMemory(idp.JWKSURI, set, time.Now().Add(ttl))

	return set, nil
}

// markFetchAttempt updates jwks_last_fetch_attempt in DB
func (m *Manager) markFetchAttempt(ctx context.Context, idpID int64) error {
	return m.dao.UpdateJWKSFetchAttempt(ctx, idpID, time.Now())
}

// fetchWithTTL fetches JWKS and extracts TTL from Cache-Control headers
func (m *Manager) fetchWithTTL(ctx context.Context, jwksURI string) (jwk.Set, time.Duration, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	set, err := jwk.ParseReader(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	ttl := m.extractTTL(resp.Header)
	return set, ttl, nil
}

// extractTTL parses Cache-Control header for max-age
func (m *Manager) extractTTL(headers http.Header) time.Duration {
	cacheControl := headers.Get("Cache-Control")
	if cacheControl == "" {
		return m.config.DefaultTTL
	}

	for _, directive := range strings.Split(cacheControl, ",") {
		directive = strings.TrimSpace(directive)
		if strings.HasPrefix(directive, "max-age=") {
			if seconds, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age=")); err == nil {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	return m.config.DefaultTTL
}

// updateDBCache updates the database cache
func (m *Manager) updateDBCache(ctx context.Context, idpID int64, set jwk.Set, ttl time.Duration) error {
	jwksBytes, err := json.Marshal(set)
	if err != nil {
		return err
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	return m.dao.UpdateJWKSCache(ctx, idpID, string(jwksBytes), now, expiresAt)
}
