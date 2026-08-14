//go:build e2e

// Package harborclient wraps the Harbor v2.0 REST API over HTTP. It is a direct
// port of src/e2e/harbor_client_test.go with exported methods and typed
// configuration so it can be used across the godog step packages.
package harborclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultURL      = "http://localhost:8080"
	defaultUser     = "admin"
	defaultPassword = "Harbor12345"
)

// Client is a thin authenticated wrapper around http.Client talking to Harbor.
type Client struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client
}

// Response holds the essentials of an HTTP response after the body has been drained.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// FromEnv constructs a Client using HARBOR_URL / HARBOR_USERNAME / HARBOR_PASSWORD.
func FromEnv() *Client {
	return &Client{
		BaseURL:    envDefault("HARBOR_URL", defaultURL),
		Username:   envDefault("HARBOR_USERNAME", defaultUser),
		Password:   envDefault("HARBOR_PASSWORD", defaultPassword),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// WithCredentials returns a shallow copy authenticating as a different user.
func (c *Client) WithCredentials(username, password string) *Client {
	cp := *c
	cp.Username = username
	cp.Password = password
	return &cp
}

// Host extracts the host:port for building image references.
func (c *Client) Host() string {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(c.BaseURL, "http://"), "https://")
	}
	return u.Host
}

// Insecure reports whether the registry endpoint is HTTP (for crane).
func (c *Client) Insecure() bool {
	return strings.HasPrefix(c.BaseURL, "http://")
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (c *Client) do(method, path string, body any, auth bool) (*Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	if auth {
		req.SetBasicAuth(c.Username, c.Password)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: respBody}, nil
}

// DoWithBearer issues a request authenticated with a Bearer token instead of Basic auth.
func (c *Client) DoWithBearer(method, path, token string, body any) (*Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: respBody}, nil
}

// Get issues a GET with Basic auth.
func (c *Client) Get(path string) (*Response, error) { return c.do(http.MethodGet, path, nil, true) }

// Post issues a POST with Basic auth.
func (c *Client) Post(path string, body any) (*Response, error) {
	return c.do(http.MethodPost, path, body, true)
}

// Put issues a PUT with Basic auth.
func (c *Client) Put(path string, body any) (*Response, error) {
	return c.do(http.MethodPut, path, body, true)
}

// Delete issues a DELETE with Basic auth.
func (c *Client) Delete(path string) (*Response, error) {
	return c.do(http.MethodDelete, path, nil, true)
}

// DeleteWithBody issues a DELETE with a JSON body and Basic auth.
func (c *Client) DeleteWithBody(path string, body any) (*Response, error) {
	return c.do(http.MethodDelete, path, body, true)
}

// GetAnonymous issues a GET without credentials.
func (c *Client) GetAnonymous(path string) (*Response, error) {
	return c.do(http.MethodGet, path, nil, false)
}

// GetWithCookies issues a GET authenticated by session cookies rather than
// Basic auth. Used after OIDC login to make requests as an OIDC user.
func (c *Client) GetWithCookies(path string, cookies []*http.Cookie) (*Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// Decode parses a JSON body into out. Empty bodies return without error.
func Decode[T any](resp *Response, out *T) error {
	if len(bytes.TrimSpace(resp.Body)) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Body, out)
}

// Expect returns an error if the response status does not match want.
func Expect(resp *Response, want int) error {
	if resp.StatusCode != want {
		return fmt.Errorf("status %d (want %d): %s", resp.StatusCode, want, truncate(resp.Body))
	}
	return nil
}

func truncate(b []byte) string {
	const max = 400
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
