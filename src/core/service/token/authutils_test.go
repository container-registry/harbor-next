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

package token

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	distributiontoken "github.com/distribution/distribution/v3/registry/auth/token"
	"github.com/stretchr/testify/require"
)

// TestGenerateKeyIDMatchesDistribution guards against a regression where
// generateKeyID's output diverged from the key ID algorithm the actual
// docker distribution registry image this project builds (see
// DISTRIBUTION_VERSION in versions.env) uses to compute its trusted key IDs
// from rootcertbundle. If the two don't match byte-for-byte, every token
// this service issues is silently rejected by the registry with
// "token signed by untrusted key", even though the underlying key material
// is identical -- this exact regression shipped and went undetected because
// the only existing tests (TestMakeToken, TestMakeTokenECDSA) verify a
// token round-trips against its own public key, which can't catch a format
// mismatch against the real registry.
//
// distribution v3.1.1 computes trusted key IDs via GetJWKThumbprint (RFC
// 7638 JSON Web Key Thumbprint), having moved off the older libtrust
// key-ID format entirely.
func TestGenerateKeyIDMatchesDistribution(t *testing.T) {
	t.Run("RSA", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		got, err := generateKeyID(key)
		require.NoError(t, err)

		want := distributiontoken.GetJWKThumbprint(&key.PublicKey)
		require.NotEmpty(t, want)
		require.Equal(t, want, got)
	})

	t.Run("ECDSA", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		got, err := generateKeyID(key)
		require.NoError(t, err)

		want := distributiontoken.GetJWKThumbprint(&key.PublicKey)
		require.NotEmpty(t, want)
		require.Equal(t, want, got)
	})

	t.Run("Ed25519", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		got, err := generateKeyID(priv)
		require.NoError(t, err)

		want := distributiontoken.GetJWKThumbprint(pub)
		require.NotEmpty(t, want)
		require.Equal(t, want, got)
	})
}
