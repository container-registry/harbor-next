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

package gomod

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/mod/module"

	"github.com/goharbor/harbor/src/common/rbac"
	rbacproject "github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	projectcontroller "github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/gosum"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
	"github.com/goharbor/harbor/src/server/registry/pkgstore"
)

const metadataTTL = 15 * time.Minute

type handler struct {
	store           packageStore
	authorize       projectAuthorizer
	proxyForProject proxyResolver
	cacheContext    func(context.Context) context.Context
	checksumURL     func(string) (string, bool)
	checksumMirror  *gosum.Mirror
}

type projectAuthorizer func(http.ResponseWriter, *http.Request, string) (int64, bool)
type proxyResolver func(context.Context, string, string) (*pkgproxy.Proxy, error)

func newHandler() http.Handler {
	return newHandlerWithCacheContext(newPackageStore(), authorize, pkgproxy.ForProject, orm.Copy)
}

func newHandlerWithDeps(store packageStore, authorize projectAuthorizer, proxyForProject proxyResolver) http.Handler {
	return newHandlerWithCacheContext(store, authorize, proxyForProject, context.WithoutCancel)
}

func newHandlerWithCacheContext(store packageStore, authorize projectAuthorizer, proxyForProject proxyResolver, cacheContext func(context.Context) context.Context) http.Handler {
	return &handler{
		store:           store,
		authorize:       authorize,
		proxyForProject: proxyForProject,
		cacheContext:    cacheContext,
		checksumURL:     checksumDatabaseURL,
		checksumMirror:  gosum.NewMirror(gosum.NewStore()),
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := parseRequest(r.Method, r.URL.EscapedPath())
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	projectID, ok := h.authorize(w, r, req.Project)
	if !ok {
		return
	}
	proxy, err := h.proxyForProject(r.Context(), req.Project, model.RegistryTypeGo)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if proxy == nil {
		writeError(w, http.StatusNotFound, "go module proxy is not enabled for project")
		return
	}
	if req.Type == requestSumDB {
		h.sumdb(w, r, req, projectID, proxy)
		return
	}
	if req.Type == requestList || req.Type == requestLatest {
		h.mutable(w, r, req, proxy)
		return
	}
	h.version(w, r, req, projectID, proxy)
}

func authorize(w http.ResponseWriter, r *http.Request, projectName string) (int64, bool) {
	securityCtx, ok := security.FromContext(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return 0, false
	}
	project, err := projectcontroller.Ctl.Get(r.Context(), projectName)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return 0, false
	}
	resource := rbacproject.NewNamespace(project.ProjectID).Resource(rbac.ResourceRepository)
	if !securityCtx.Can(r.Context(), rbac.ActionPull, resource) {
		if !securityCtx.IsAuthenticated() {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusForbidden, "forbidden")
		}
		return 0, false
	}
	return project.ProjectID, true
}

func (h *handler) mutable(w http.ResponseWriter, r *http.Request, req *moduleRequest, proxy *pkgproxy.Proxy) {
	upstreamPath, err := moduleUpstreamPath(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cacheKey := pkgproxy.CacheKey("go", req.Project, req.TypeString(), req.Module)
	resp, err := proxy.CachedGet(r.Context(), cacheKey, upstreamPath, metadataTTL, nil)
	if err != nil {
		writeProxyError(w, err)
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
		if req.Type == requestLatest {
			contentType = "application/json"
		}
	}
	writePayload(w, http.StatusOK, contentType, resp.Body, r.Method != http.MethodHead)
}

func (h *handler) version(w http.ResponseWriter, r *http.Request, req *moduleRequest, projectID int64, proxy *pkgproxy.Proxy) {
	escapedModule, escapedVersion, err := escaped(req.Module, req.Version)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	title := titleFor(req.Type)
	content, err := h.store.Open(r.Context(), req.Project, escapedModule, req.Version, title)
	if err == nil {
		defer content.Body.Close()
		writeContent(w, contentTypeFor(req.Type), content, r.Method != http.MethodHead)
		return
	}
	if !errors.IsNotFoundErr(err) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	upstreamPath := path.Join(escapedModule, "@v", escapedVersion+extensionFor(req.Type))
	resp, err := proxy.Get(r.Context(), upstreamPath, nil)
	if err != nil {
		writeProxyError(w, err)
		return
	}
	cacheCtx := h.cacheContext(r.Context())
	go func() {
		if err := h.store.Publish(cacheCtx, projectID, req.Project, req.Module, escapedModule, req.Version, req.Type, resp.Body); err != nil {
			log.Errorf("failed to cache Go module %s@%s %s: %v", req.Module, req.Version, req.Type, err)
		}
	}()
	writePayload(w, http.StatusOK, contentTypeFor(req.Type), resp.Body, r.Method != http.MethodHead)
}

func (h *handler) sumdb(w http.ResponseWriter, r *http.Request, req *moduleRequest, projectID int64, proxy *pkgproxy.Proxy) {
	base, ok := h.checksumURL(req.SumDB)
	if !ok {
		writeError(w, http.StatusNotFound, "checksum database is not supported")
		return
	}
	if req.Path == "supported" {
		w.WriteHeader(http.StatusOK)
		return
	}
	database := gosum.Database{Name: req.SumDB, URL: strings.TrimRight(base, "/")}
	resp, err := h.checksumMirror.Resolve(r.Context(), projectID, req.Project, database, req.Path, func(ctx context.Context) (*pkgproxy.Response, error) {
		return proxy.Get(ctx, strings.TrimRight(base, "/")+"/"+req.Path, nil)
	})
	if err != nil {
		writeProxyError(w, err)
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	writePayload(w, http.StatusOK, contentType, resp.Body, r.Method != http.MethodHead)
}

func escaped(modulePath, version string) (string, string, error) {
	escapedModule, err := module.EscapePath(modulePath)
	if err != nil {
		return "", "", fmt.Errorf("invalid module path: %w", err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		return "", "", fmt.Errorf("invalid module version: %w", err)
	}
	return escapedModule, escapedVersion, nil
}

func moduleUpstreamPath(req *moduleRequest) (string, error) {
	escapedModule, err := module.EscapePath(req.Module)
	if err != nil {
		return "", fmt.Errorf("invalid module path: %w", err)
	}
	base := path.Join(escapedModule, "@v")
	if req.Type == requestList {
		return path.Join(base, "list"), nil
	}
	return path.Join(escapedModule, "@latest"), nil
}

func titleFor(typ requestType) string {
	switch typ {
	case requestInfo:
		return infoLayerTitle
	case requestMod:
		return modLayerTitle
	default:
		return zipLayerTitle
	}
}

func contentTypeFor(typ requestType) string {
	switch typ {
	case requestInfo:
		return "application/json"
	case requestZip:
		return "application/zip"
	default:
		return "text/plain; charset=utf-8"
	}
}

func extensionFor(typ requestType) string {
	switch typ {
	case requestInfo:
		return ".info"
	case requestMod:
		return ".mod"
	default:
		return ".zip"
	}
}

func checksumDatabaseURL(name string) (string, bool) {
	if name != "sum.golang.org" {
		return "", false
	}
	return "https://sum.golang.org", true
}

func writeContent(w http.ResponseWriter, contentType string, content *pkgstore.Content, includeBody bool) {
	w.Header().Set("Content-Type", contentType)
	if content.Size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", content.Size))
	}
	w.WriteHeader(http.StatusOK)
	if includeBody {
		_, _ = io.Copy(w, content.Body)
	}
}

func writePayload(w http.ResponseWriter, status int, contentType string, payload []byte, includeBody bool) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
	w.WriteHeader(status)
	if includeBody {
		_, _ = w.Write(payload)
	}
}

func writeProxyError(w http.ResponseWriter, err error) {
	if errors.IsNotFoundErr(err) {
		writeError(w, http.StatusNotFound, "module not found")
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

func (r *moduleRequest) TypeString() string {
	return strings.TrimSpace(string(r.Type))
}
