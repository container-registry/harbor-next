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
	"os"
	"time"
)

// Environment variables for configuration
const (
	EnvMinFetchInterval = "JWKS_CACHE_MIN_FETCH_INTERVAL" // e.g., "5m"
	EnvDefaultTTL       = "JWKS_CACHE_DEFAULT_TTL"        // e.g., "1h" (for providers without Cache-Control)
	EnvInMemoryTTL      = "JWKS_CACHE_INMEMORY_TTL"       // e.g., "15m"
)

// Defaults (following EKS recommendations - 7 day rotation, respect cache headers)
var (
	DefaultMinFetchInterval = 5 * time.Minute  // Prevent hammering endpoints
	DefaultTTL              = 1 * time.Hour    // For providers without Cache-Control (conservative)
	DefaultInMemoryTTL      = 15 * time.Minute // Per-process optimization, stays reasonably in sync with DB
)

// Config holds cache configuration
type Config struct {
	MinFetchInterval time.Duration // Minimum time between remote fetches (rate limit)
	DefaultTTL       time.Duration // TTL for providers without Cache-Control
	InMemoryTTL      time.Duration // In-memory cache TTL
}

// LoadConfig loads configuration from environment variables with sensible defaults
func LoadConfig() Config {
	cfg := Config{
		MinFetchInterval: DefaultMinFetchInterval,
		DefaultTTL:       DefaultTTL,
		InMemoryTTL:      DefaultInMemoryTTL,
	}

	if v := os.Getenv(EnvMinFetchInterval); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.MinFetchInterval = d
		}
	}
	if v := os.Getenv(EnvDefaultTTL); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.DefaultTTL = d
		}
	}
	if v := os.Getenv(EnvInMemoryTTL); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.InMemoryTTL = d
		}
	}

	return cfg
}
