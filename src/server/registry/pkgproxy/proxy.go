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

package pkgproxy

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	commonhttp "github.com/goharbor/harbor/src/common/http"
	projectctl "github.com/goharbor/harbor/src/controller/project"
	regctl "github.com/goharbor/harbor/src/controller/registry"
	"github.com/goharbor/harbor/src/lib/cache"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/redis"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	"github.com/goharbor/harbor/src/pkg/proxy/connection"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	registryauth "github.com/goharbor/harbor/src/pkg/registry/auth"
)

const (
	defaultTimeout   = 60 * time.Second
	defaultUserAgent = "Harbor-Package-Proxy"
)

// ErrNotConfigured is returned by ForProject when projectName has no upstream
// registry configured for the requested registryType. It is not a failure --
// callers use it (via errors.Is) to distinguish "nothing to proxy" from a
// genuine lookup error.
var ErrNotConfigured = goerrors.New("pkgproxy: project is not configured for this registry type")

// Proxy contains the resolved upstream registry for a package proxy project.
type Proxy struct {
	Project  *proModels.Project
	Registry *regmodel.Registry
	client   *http.Client
}

// Response contains an upstream response payload.
type Response struct {
	Body        []byte
	ContentType string
	StatusCode  int
	Header      http.Header
}

// ForProject returns a package proxy for projectName when it is enabled for registryType.
func ForProject(ctx context.Context, projectName, registryType string) (*Proxy, error) {
	project, err := projectctl.Ctl.GetByName(ctx, projectName, projectctl.Metadata(true))
	if err != nil {
		return nil, err
	}
	if project.RegistryID <= 0 {
		return nil, ErrNotConfigured
	}
	registry, err := regctl.Ctl.Get(ctx, project.RegistryID)
	if err != nil {
		return nil, err
	}
	if !regmodel.RegistryTypesCompatible(registry.Type, registryType) {
		return nil, ErrNotConfigured
	}
	if registry.Status != regmodel.Healthy {
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("upstream registry %s is %s", registry.Name, registry.Status)
	}
	return New(project, registry), nil
}

// New returns a proxy for an already resolved project and registry.
func New(project *proModels.Project, registry *regmodel.Registry) *Proxy {
	return &Proxy{
		Project:  project,
		Registry: registry,
		client: &http.Client{
			Transport: commonhttp.GetHTTPTransport(
				commonhttp.WithInsecure(registry.Insecure),
				commonhttp.WithCACert(registry.CACertificate),
			),
			Timeout: defaultTimeout,
		},
	}
}

// Get fetches an upstream path relative to the registry URL.
func (p *Proxy) Get(ctx context.Context, upstreamPath string, headers http.Header) (*Response, error) {
	return p.Do(ctx, http.MethodGet, upstreamPath, headers, nil)
}

// GetOCI fetches an OCI Distribution path and negotiates the upstream
// registry's authentication challenge. Package ecosystems such as Homebrew
// use OCI registries for their downloadable artifacts even though their
// metadata is served by a separate package API.
func (p *Proxy) GetOCI(ctx context.Context, upstreamPath string, headers http.Header) (*Response, error) {
	return p.do(ctx, http.MethodGet, upstreamPath, headers, nil, true)
}

// HeadOCI fetches headers for an OCI Distribution path and negotiates the
// upstream registry's authentication challenge.
func (p *Proxy) HeadOCI(ctx context.Context, upstreamPath string, headers http.Header) (*Response, error) {
	return p.do(ctx, http.MethodHead, upstreamPath, headers, nil, true)
}

// Head fetches upstream headers for a path relative to the registry URL.
func (p *Proxy) Head(ctx context.Context, upstreamPath string, headers http.Header) (*Response, error) {
	return p.Do(ctx, http.MethodHead, upstreamPath, headers, nil)
}

// Do sends an upstream request to a path relative to the registry URL.
func (p *Proxy) Do(ctx context.Context, method, upstreamPath string, headers http.Header, body io.Reader) (*Response, error) {
	return p.do(ctx, method, upstreamPath, headers, body, false)
}

func (p *Proxy) do(ctx context.Context, method, upstreamPath string, headers http.Header, body io.Reader, authorizeOCI bool) (*Response, error) {
	if p == nil || p.Registry == nil {
		return nil, errors.New("package proxy is not configured")
	}
	release, err := p.acquire(ctx, upstreamPath)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}
	req, err := http.NewRequestWithContext(ctx, method, p.upstreamURL(upstreamPath), body)
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	if p.Registry.Credential != nil && sameOrigin(p.Registry.URL, req.URL) {
		req.SetBasicAuth(p.Registry.Credential.AccessKey, p.Registry.Credential.AccessSecret)
	}
	if authorizeOCI {
		username, password := "", ""
		if p.Registry.Credential != nil {
			username = p.Registry.Credential.AccessKey
			password = p.Registry.Credential.AccessSecret
		}
		authorizer := registryauth.NewAuthorizer(username, password, p.Registry.Insecure, p.Registry.CACertificate)
		if err := authorizer.Modify(req); err != nil {
			return nil, err
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, errors.NotFoundError(nil).WithMessage("upstream package not found")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("upstream returned status %d", resp.StatusCode)
	}
	return &Response{
		Body:        payload,
		ContentType: resp.Header.Get("Content-Type"),
		StatusCode:  resp.StatusCode,
		Header:      resp.Header.Clone(),
	}, nil
}

// CachedGet fetches a mutable upstream path using Harbor's shared cache.
func (p *Proxy) CachedGet(ctx context.Context, key, upstreamPath string, ttl time.Duration, headers http.Header) (*Response, error) {
	c := cache.Default()
	if c == nil || ttl <= 0 {
		return p.Get(ctx, upstreamPath, headers)
	}
	var cached Response
	if err := c.Fetch(ctx, key, &cached); err == nil {
		return &cached, nil
	}
	resp, err := p.Get(ctx, upstreamPath, headers)
	if err != nil {
		return nil, err
	}
	saveCtx := context.Background()
	if err := c.Save(saveCtx, key, resp, ttl); err != nil {
		log.Warningf("failed to cache upstream package metadata %s: %v", key, err)
	}
	return resp, nil
}

// CacheKey builds a package proxy cache key.
func CacheKey(parts ...string) string {
	var buf bytes.Buffer
	buf.WriteString("pkgproxy")
	for _, part := range parts {
		buf.WriteString(":")
		buf.WriteString(strings.ReplaceAll(part, ":", "_"))
	}
	return buf.String()
}

func (p *Proxy) acquire(ctx context.Context, upstreamPath string) (func(), error) {
	if p.Project == nil || p.Project.MaxUpstreamConnection() <= 0 {
		return func() {}, nil
	}
	client, err := redis.GetHarborClient()
	if err != nil {
		return nil, errors.NewErrs(err)
	}
	key := fmt.Sprintf("{pkgproxy_upstream}:%s:%s", p.Project.Name, upstreamPath)
	if !connection.Limiter.Acquire(ctx, client, key, p.Project.MaxUpstreamConnection()) {
		return nil, errors.New("too many requests to upstream registry").WithCode(errors.RateLimitCode)
	}
	return func() {
		connection.Limiter.Release(context.Background(), client, key)
	}, nil
}

func (p *Proxy) upstreamURL(upstreamPath string) string {
	base, err := url.Parse(strings.TrimRight(p.Registry.URL, "/") + "/")
	if err != nil {
		return p.Registry.URL
	}
	if upstreamPath == "" {
		return base.String()
	}
	if strings.HasPrefix(upstreamPath, "http://") || strings.HasPrefix(upstreamPath, "https://") {
		return upstreamPath
	}
	rawPath := strings.TrimLeft(upstreamPath, "/")
	decodedPath, err := url.PathUnescape(rawPath)
	if err == nil && decodedPath != rawPath {
		escapedBasePath := base.EscapedPath()
		base.Path = singleJoiningSlash(base.Path, decodedPath)
		base.RawPath = singleJoiningSlash(escapedBasePath, rawPath)
		return base.String()
	}
	base.Path = singleJoiningSlash(base.Path, rawPath)
	return base.String()
}

func singleJoiningSlash(left, right string) string {
	if left == "" || left == "/" {
		return "/" + right
	}
	joined := path.Join(left, right)
	if strings.HasSuffix(right, "/") && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}

func sameOrigin(registryURL string, target *url.URL) bool {
	base, err := url.Parse(registryURL)
	if err != nil || target == nil {
		return false
	}
	return strings.EqualFold(base.Scheme, target.Scheme) && strings.EqualFold(base.Host, target.Host)
}
