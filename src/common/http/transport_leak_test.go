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

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewDefaultTransportLeakGuards verifies the default transport carries the
// two settings that prevent harbor-core from leaking goroutines/FDs when a
// backend (such as the registry) becomes unresponsive on a pooled connection.
func TestNewDefaultTransportLeakGuards(t *testing.T) {
	tr := newDefaultTransport()
	assert.Positive(t, tr.MaxIdleConnsPerHost,
		"MaxIdleConnsPerHost must be set (>0); Go's default of 2 forces connection churn under load")
	assert.Equal(t, defaultMaxIdleConnsPerHost, tr.MaxIdleConnsPerHost)
	assert.Positive(t, tr.ResponseHeaderTimeout,
		"ResponseHeaderTimeout must be set (>0) so requests to an unresponsive backend cannot hang forever")
	assert.Equal(t, defaultResponseHeaderTimeout, tr.ResponseHeaderTimeout)
}

// TestParseResponseHeaderTimeout covers the env parsing. It targets the
// uncached parser directly because responseHeaderTimeout() caches its result
// for the lifetime of the process (sync.Once), so it cannot be toggled here.
func TestParseResponseHeaderTimeout(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv(responseHeaderTimeoutEnvKey, "")
		assert.Equal(t, defaultResponseHeaderTimeout, parseResponseHeaderTimeout())
	})
	t.Run("override in seconds", func(t *testing.T) {
		t.Setenv(responseHeaderTimeoutEnvKey, "15")
		assert.Equal(t, 15*time.Second, parseResponseHeaderTimeout())
	})
	t.Run("zero disables the timeout", func(t *testing.T) {
		t.Setenv(responseHeaderTimeoutEnvKey, "0")
		assert.Equal(t, time.Duration(0), parseResponseHeaderTimeout())
	})
	t.Run("invalid value falls back to default", func(t *testing.T) {
		t.Setenv(responseHeaderTimeoutEnvKey, "not-a-number")
		assert.Equal(t, defaultResponseHeaderTimeout, parseResponseHeaderTimeout())
	})
	t.Run("negative value falls back to default", func(t *testing.T) {
		t.Setenv(responseHeaderTimeoutEnvKey, "-5")
		assert.Equal(t, defaultResponseHeaderTimeout, parseResponseHeaderTimeout())
	})

	// responseHeaderTimeout() returns a positive, cached value (the env is
	// unset at process start, so it resolves to the default).
	assert.Equal(t, defaultResponseHeaderTimeout, responseHeaderTimeout())
}
