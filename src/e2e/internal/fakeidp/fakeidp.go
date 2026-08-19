//go:build e2e

// Package fakeidp provides an in-process fake OIDC identity provider for e2e tests.
// It generates RSA keys, issues RS256 JWTs, and produces JWKS payloads for
// embedding in offline Harbor FedIDP records.
package fakeidp

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IdP is a fake identity provider backed by an in-process RSA key pair.
type IdP struct {
	PrivateKey *rsa.PrivateKey
	KID        string
	Issuer     string
}

// New creates a new fake IdP with a fresh 2048-bit RSA key pair.
func New(issuer string) (*IdP, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}
	kid := randomHex(8)
	return &IdP{PrivateKey: key, KID: kid, Issuer: issuer}, nil
}

// IssueJWT signs a JWT containing the given extra claims plus standard iss/iat/exp.
func (f *IdP) IssueJWT(extra map[string]any, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": f.Issuer,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = f.KID
	return token.SignedString(f.PrivateKey)
}

// JWKS returns the public key as a JWKS-format map suitable for embedding in
// the Harbor FedIDP create/update payload under the "jwks_keys" field.
func (f *IdP) JWKS() map[string]any {
	pub := &f.PrivateKey.PublicKey
	nBytes := pub.N.Bytes()
	eBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(eBytes, uint32(pub.E))
	// Trim leading zero bytes from the exponent.
	i := 0
	for i < len(eBytes)-1 && eBytes[i] == 0 {
		i++
	}
	eBytes = eBytes[i:]
	return map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": f.KID,
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	}
}

// IssueExpired issues a JWT that expired 10 minutes ago.
func (f *IdP) IssueExpired(extra map[string]any) (string, error) {
	past := time.Now().Add(-10 * time.Minute)
	claims := jwt.MapClaims{
		"iss": f.Issuer,
		"iat": past.Add(-time.Minute).Unix(),
		"exp": past.Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = f.KID
	return token.SignedString(f.PrivateKey)
}

// IssueTampered issues a valid JWT then replaces the last 8 chars of the
// signature with random bytes, producing a syntactically valid but
// cryptographically invalid token.
func (f *IdP) IssueTampered(extra map[string]any, ttl time.Duration) (string, error) {
	raw, err := f.IssueJWT(extra, ttl)
	if err != nil {
		return "", err
	}
	// Flip a few bytes in the last segment (signature).
	// Find last dot.
	last := -1
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == '.' {
			last = i
			break
		}
	}
	if last < 0 || last == len(raw)-1 {
		return raw + "TAMPER", nil
	}
	sig := []byte(raw[last+1:])
	// XOR last 6 bytes to corrupt the signature.
	for i := len(sig) - 6; i < len(sig); i++ {
		sig[i] ^= 0xFF
	}
	return raw[:last+1] + string(sig), nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const hexChars = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, byt := range b {
		out[i*2] = hexChars[byt>>4]
		out[i*2+1] = hexChars[byt&0x0F]
	}
	return string(out)
}
