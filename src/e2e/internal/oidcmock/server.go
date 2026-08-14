//go:build e2e

// Package oidcmock provides a minimal OIDC identity provider for E2E tests.
// It issues RS256-signed JWTs for a single pre-configured test user and
// implements the OAuth2 authorization-code flow without a browser.
//
// Usage in a step:
//
//	mock, err := oidcmock.Start()
//	// configure Harbor with mock.URL as oidc_endpoint
//	// ...run OIDC login dance...
//	mock.Stop()
//
// Docker networking: the mock binds to 0.0.0.0:<random-port>. Set OIDC_HOST_IP
// to the Docker bridge gateway (e.g. 172.17.0.1) so Harbor containers can reach
// the host process. Defaults to 127.0.0.1 for native (task dev:up) runs.
package oidcmock

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Server is a running OIDC mock provider.
type Server struct {
	// URL is the externally-reachable issuer base URL (http://HOST:PORT).
	// Use this as the oidc_endpoint value when configuring Harbor.
	URL      string
	TestUser User

	privateKey *rsa.PrivateKey
	srv        *http.Server
}

// User is the identity the mock issues tokens for.
type User struct {
	Sub   string // OIDC subject identifier
	Email string
	Name  string
}

const (
	mockClientID = "harbor-e2e"
	mockCode     = "e2e-mock-auth-code"
	keyID        = "e2e-key"
)

// DefaultTestUser is the pre-configured OIDC identity issued by every mock.
var DefaultTestUser = User{
	Sub:   "e2e-oidc-subject-001",
	Email: "oidc-user@e2e.test",
	Name:  "OIDC Test User",
}

// Start generates an RSA key pair, binds to a random port on all interfaces,
// and serves OIDC discovery, JWKS, authorize, and token endpoints.
func Start() (*Server, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("oidcmock: generate RSA key: %w", err)
	}

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("oidcmock: listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	hostIP := os.Getenv("OIDC_HOST_IP")
	if hostIP == "" {
		hostIP = "127.0.0.1"
	}

	s := &Server{
		URL:        fmt.Sprintf("http://%s:%d", hostIP, port),
		TestUser:   DefaultTestUser,
		privateKey: key,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("/keys", s.handleJWKS)
	mux.HandleFunc("/auth", s.handleAuth)
	mux.HandleFunc("/token", s.handleToken)

	s.srv = &http.Server{Handler: mux}
	go func() { _ = s.srv.Serve(ln) }()

	return s, nil
}

// Stop shuts down the HTTP server.
func (s *Server) Stop() {
	if s.srv != nil {
		_ = s.srv.Close()
	}
}

// ClientID returns the OAuth2 client_id that mock tokens are issued for.
// Configure Harbor's oidc_client_id with this value.
func (s *Server) ClientID() string { return mockClientID }

// handleDiscovery serves the OIDC discovery document.
func (s *Server) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"issuer":                                s.URL,
		"authorization_endpoint":                s.URL + "/auth",
		"token_endpoint":                        s.URL + "/token",
		"jwks_uri":                              s.URL + "/keys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "email", "name"},
		"grant_types_supported":                 []string{"authorization_code"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// handleJWKS serves the RSA public key in JWK format.
func (s *Server) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := &s.privateKey.PublicKey
	nBytes := pub.N.Bytes()
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	doc := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": keyID,
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// handleAuth implements the OAuth2 authorization endpoint.
// It immediately redirects back to redirect_uri with the pre-set code,
// bypassing any browser interaction.
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")

	if redirectURI == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}

	callback := redirectURI + "?code=" + url.QueryEscape(mockCode) + "&state=" + url.QueryEscape(state)
	http.Redirect(w, r, callback, http.StatusFound)
}

// handleToken implements the OAuth2 token endpoint.
// It accepts any authorization code and returns a signed JWT for TestUser.
func (s *Server) handleToken(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   s.URL,
		"sub":   s.TestUser.Sub,
		"aud":   jwt.ClaimStrings{mockClientID},
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"email": s.TestUser.Email,
		"name":  s.TestUser.Name,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID

	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		http.Error(w, "token signing error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"access_token": "mock-access-token-" + fmt.Sprint(now.UnixNano()),
		"token_type":   "bearer",
		"id_token":     signed,
		"expires_in":   3600,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
