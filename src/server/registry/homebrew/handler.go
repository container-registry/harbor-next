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

package homebrew

import (
	goerrors "errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common/rbac"
	rbacproject "github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	projectcontroller "github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

const (
	routePrefix       = "/homebrew/"
	apiPrefix         = "api/"
	artifactPrefix    = "v2/"
	defaultBottleHost = "https://ghcr.io"
	apiTTL            = 15 * time.Minute
)

type routeKind int

const (
	routeAPI routeKind = iota
	routeArtifact
)

type handler struct{}

func newHandler() http.Handler {
	return &handler{}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	project, kind, upstreamPath, ok := parsePath(r.URL.EscapedPath())
	if !ok {
		writeError(w, http.StatusNotFound, "invalid homebrew proxy path")
		return
	}
	if !authorize(w, r, project) {
		return
	}
	proxy, err := pkgproxy.ForProject(r.Context(), project, model.RegistryTypeHomebrew)
	if goerrors.Is(err, pkgproxy.ErrNotConfigured) {
		writeError(w, http.StatusNotFound, "homebrew proxy is not enabled for project")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	resp, err := fetch(r, proxy, project, kind, upstreamPath)
	if err != nil {
		writeProxyError(w, err)
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	for _, name := range []string{"Accept-Ranges", "Content-Range", "Docker-Content-Digest", "ETag", "Last-Modified"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	contentLength := fmt.Sprintf("%d", len(resp.Body))
	if r.Method == http.MethodHead && resp.Header.Get("Content-Length") != "" {
		contentLength = resp.Header.Get("Content-Length")
	}
	w.Header().Set("Content-Length", contentLength)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func fetch(r *http.Request, proxy *pkgproxy.Proxy, project string, kind routeKind, upstreamPath string) (*pkgproxy.Response, error) {
	headers := make(http.Header)
	for _, name := range []string{"Accept", "If-None-Match", "Range"} {
		if value := r.Header.Get(name); value != "" {
			headers.Set(name, value)
		}
	}
	if kind == routeAPI {
		upstreamPath = apiUpstreamPath(proxy, upstreamPath)
		return proxy.CachedGet(
			r.Context(),
			pkgproxy.CacheKey("homebrew-api", project, upstreamPath),
			upstreamPath,
			apiTTL,
			headers,
		)
	}

	artifactProxy := bottleProxy(proxy)
	if r.Method == http.MethodHead {
		return artifactProxy.HeadOCI(r.Context(), upstreamPath, headers)
	}
	return artifactProxy.GetOCI(r.Context(), upstreamPath, headers)
}

func apiUpstreamPath(proxy *pkgproxy.Proxy, upstreamPath string) string {
	if proxy.Registry.Type == model.RegistryTypeHomebrewRegistry {
		return apiPrefix + upstreamPath
	}
	return upstreamPath
}

func bottleProxy(proxy *pkgproxy.Proxy) *pkgproxy.Proxy {
	if proxy.Registry.Type != model.RegistryTypeHomebrew {
		return proxy
	}
	registry := *proxy.Registry
	registry.URL = defaultBottleHost
	// Credentials configured for the formula metadata origin must never be
	// forwarded to GHCR. Public homebrew/core bottles use anonymous bearer
	// tokens negotiated by GetOCI.
	registry.Credential = nil
	return pkgproxy.New(proxy.Project, &registry)
}

func parsePath(escapedPath string) (string, routeKind, string, bool) {
	raw := strings.TrimPrefix(escapedPath, routePrefix)
	if raw == escapedPath {
		return "", 0, "", false
	}
	raw = strings.Trim(raw, "/")
	project, rest, ok := strings.Cut(raw, "/")
	if !ok || project == "" || rest == "" {
		return "", 0, "", false
	}
	project, err := url.PathUnescape(project)
	if err != nil || project == "" {
		return "", 0, "", false
	}
	if strings.Contains(rest, "..") {
		return "", 0, "", false
	}
	if strings.HasPrefix(rest, apiPrefix) && len(rest) > len(apiPrefix) {
		return project, routeAPI, strings.TrimPrefix(rest, apiPrefix), true
	}
	if strings.HasPrefix(rest, artifactPrefix) && len(rest) > len(artifactPrefix) {
		return project, routeArtifact, rest, true
	}
	return "", 0, "", false
}

func authorize(w http.ResponseWriter, r *http.Request, projectName string) bool {
	securityCtx, ok := security.FromContext(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	project, err := projectcontroller.Ctl.Get(r.Context(), projectName)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return false
	}
	resource := rbacproject.NewNamespace(project.ProjectID).Resource(rbac.ResourceRepository)
	if !securityCtx.Can(r.Context(), rbac.ActionPull, resource) {
		if !securityCtx.IsAuthenticated() {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusForbidden, "forbidden")
		}
		return false
	}
	return true
}

func writeProxyError(w http.ResponseWriter, err error) {
	if errors.IsNotFoundErr(err) {
		writeError(w, http.StatusNotFound, "homebrew metadata not found")
		return
	}
	if errors.IsRateLimitError(err) {
		writeError(w, http.StatusTooManyRequests, "too many requests to upstream registry")
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}
