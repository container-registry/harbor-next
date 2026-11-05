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

package gcclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	common_http "github.com/goharbor/harbor/src/common/http"
	"github.com/goharbor/harbor/src/common/http/modifier/auth"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/registry/interceptor"
)

// const definition
const (
	UserAgent = "harbor-gc-client"
)

// Vars rather than consts so tests can shorten the wait. The client has no
// timeout of its own, so each attempt is bounded to keep a hung registry from
// blocking GC startup indefinitely.
var (
	healthAttempts       = 60
	healthInterval       = 2 * time.Second
	healthAttemptTimeout = 5 * time.Second
)

// Client calls the GC delete API that the patched registry serves under /api/registry
type Client interface {
	// Health tests the connection with registry server
	Health() error
	// DeleteBlob deletes the specified blob. The "reference" should be "digest"
	DeleteBlob(reference string) (err error)
	// DeleteManifest deletes the specified manifest. The "reference" can be "tag" or "digest"
	DeleteManifest(repository, reference string) (err error)
}

type client struct {
	baseURL      string
	client       *common_http.Client
	interceptors []interceptor.Interceptor
}

// Config contains configurations needed for client
type Config struct {
	Secret string
}

// NewClient return an instance of Registry client
func NewClient(baseURL string, cfg *Config, interceptors ...interceptor.Interceptor) Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	client := &client{
		baseURL:      baseURL,
		interceptors: interceptors,
	}
	if cfg != nil {
		client.client = common_http.NewClient(nil, auth.NewSecretAuthorizer(cfg.Secret))
	} else {
		client.client = common_http.NewClient(nil)
	}
	return client
}

// Health fails unless the registry serves the GC API. An unpatched registry
// answers 404 to every GC call, and GC treats 404 as already deleted, so it
// would drop the database records and leave the data in storage.
func (c *client) Health() error {
	var lastErr error
	for range healthAttempts {
		code, err := c.healthStatus()
		if err != nil {
			lastErr = err
			time.Sleep(healthInterval)
			continue
		}
		if code != http.StatusOK {
			return errors.Errorf("registry at %s does not serve the GC API: GET /api/health returned %d; it needs the patched registry with JOBSERVICE_SECRET set", c.baseURL, code)
		}
		return nil
	}
	return errors.Wrapf(lastErr, "registry at %s is unreachable", c.baseURL)
}

// DeleteBlob ...
func (c *client) DeleteBlob(reference string) (err error) {
	req, err := http.NewRequest(http.MethodDelete, buildBlobURL(c.baseURL, reference), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// DeleteManifest ...
func (c *client) DeleteManifest(repository, reference string) (err error) {
	req, err := http.NewRequest(http.MethodDelete, buildManifestURL(c.baseURL, repository, reference), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *client) do(req *http.Request) (*http.Response, error) {
	for _, interceptor := range c.interceptors {
		if err := interceptor.Intercept(req); err != nil {
			return nil, err
		}
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		code := errors.GeneralCode
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			code = errors.UnAuthorizedCode
		case http.StatusForbidden:
			code = errors.ForbiddenCode
		case http.StatusNotFound:
			code = errors.NotFoundCode
		}
		return nil, errors.New(nil).WithCode(code).
			WithMessagef("http status code: %d, body: %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

func buildManifestURL(endpoint, repository, reference string) string {
	return fmt.Sprintf("%s/api/registry/%s/manifests/%s", endpoint, repository, reference)
}

func buildBlobURL(endpoint, reference string) string {
	return fmt.Sprintf("%s/api/registry/blob/%s", endpoint, reference)
}

func (c *client) healthStatus() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), healthAttemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/health", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}
