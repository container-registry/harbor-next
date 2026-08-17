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

package cargo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/rbac"
	rbacproject "github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	localsecurity "github.com/goharbor/harbor/src/common/security/local"
	projectcontroller "github.com/goharbor/harbor/src/controller/project"
	coreauth "github.com/goharbor/harbor/src/core/auth"
	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/server/registry/pkgpolicy"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

const maxPublishSize = 512 << 20

type handler struct {
	store         packageStore
	projectID     projectIDResolver
	authenticate  credentialAuthenticator
	authorizePush pushAuthorizer
}

type projectIDResolver func(context.Context, string) (int64, error)
type credentialAuthenticator func(context.Context, string, string) (security.Context, error)
type pushAuthorizer func(context.Context, string, string) error

func newHandler() http.Handler {
	handler := newHandlerWithDeps(newPackageStore(), resolveProjectID)
	handler.authorizePush = pkgpolicy.AuthorizePush
	return handler
}

func newHandlerWithDeps(store packageStore, projectID projectIDResolver) *handler {
	return newHandlerWithAuthenticator(store, projectID, authenticateCredentials)
}

func newHandlerWithAuthenticator(store packageStore, projectID projectIDResolver, authenticate credentialAuthenticator) *handler {
	return &handler{store: store, projectID: projectID, authenticate: authenticate, authorizePush: allowPush}
}

func allowPush(context.Context, string, string) error { return nil }

func resolveProjectID(ctx context.Context, name string) (int64, error) {
	project, err := projectcontroller.Ctl.Get(ctx, name)
	if err != nil {
		return 0, err
	}
	return project.ProjectID, nil
}

func authenticateCredentials(ctx context.Context, username, password string) (security.Context, error) {
	user, err := coreauth.Login(ctx, models.AuthModel{Principal: username, Password: password})
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("unauthorized")
	}
	return localsecurity.NewSecurityContext(user), nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := parseRequest(r.Method, r.URL.EscapedPath())
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, errUnsupportedMethod) {
			status = http.StatusMethodNotAllowed
		}
		writeError(w, status, err.Error())
		return
	}
	projectID, ok := h.authorize(w, r, req)
	if !ok {
		return
	}
	switch req.Type {
	case requestConfig:
		h.config(w, r, req)
	case requestIndex:
		h.index(w, r, req)
	case requestDownload:
		h.download(w, r, req, projectID)
	case requestPublish:
		h.publish(w, r, req, projectID)
	case requestYank:
		h.setYanked(w, r, req, projectID, true)
	case requestUnyank:
		h.setYanked(w, r, req, projectID, false)
	default:
		writeError(w, http.StatusNotFound, errInvalidPath.Error())
	}
}

func (h *handler) authorize(w http.ResponseWriter, r *http.Request, req *packageRequest) (int64, bool) {
	securityCtx, ok := h.securityContext(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return 0, false
	}
	projectID, err := h.projectID(r.Context(), req.Project)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return 0, false
	}
	action := rbac.ActionPull
	if req.Type == requestPublish || req.Type == requestYank || req.Type == requestUnyank {
		action = rbac.ActionPush
	}
	resource := rbacproject.NewNamespace(projectID).Resource(rbac.ResourceRepository)
	if !securityCtx.Can(r.Context(), action, resource) {
		if !securityCtx.IsAuthenticated() {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusForbidden, "forbidden")
		}
		return 0, false
	}
	if req.Type == requestPublish || req.Type == requestYank || req.Type == requestUnyank {
		if err := h.authorizePush(r.Context(), req.Project, model.RegistryTypeCargo); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return 0, false
		}
	}
	*r = *r.WithContext(security.NewContext(r.Context(), securityCtx))
	return projectID, true
}

func (h *handler) securityContext(r *http.Request) (security.Context, bool) {
	securityCtx, hasContext := security.FromContext(r.Context())
	if hasContext && securityCtx.IsAuthenticated() {
		return securityCtx, true
	}
	username, password, ok := credentialsFromAuth(r.Header.Get("Authorization"))
	if !ok || h.authenticate == nil {
		return securityCtx, hasContext
	}
	securityCtx, err := h.authenticate(r.Context(), username, password)
	if err != nil || securityCtx == nil || !securityCtx.IsAuthenticated() {
		if hasContext {
			return security.FromContext(r.Context())
		}
		return nil, false
	}
	return securityCtx, true
}

func credentialsFromAuth(header string) (string, string, bool) {
	if username, password, ok := credentialsFromToken(strings.TrimSpace(strings.TrimPrefix(header, "Basic ")), header); ok {
		return username, password, true
	}
	if username, password, ok := credentialsFromToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), header); ok {
		return username, password, true
	}
	return credentialsFromToken(strings.TrimSpace(header), "")
}

func credentialsFromToken(token, original string) (string, string, bool) {
	if token == "" || token == original {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", "", false
	}
	username, password, ok := strings.Cut(string(raw), ":")
	if !ok || username == "" {
		return "", "", false
	}
	return username, password, true
}

func (h *handler) config(w http.ResponseWriter, r *http.Request, req *packageRequest) {
	base := strings.TrimRight(baseURL(r), "/") + "/cargo/" + url.PathEscape(req.Project)
	authRequired := true
	if securityCtx, ok := security.FromContext(r.Context()); ok && !securityCtx.IsAuthenticated() {
		authRequired = false
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dl":            base + "/api/v1/crates/{crate}/{version}/download",
		"api":           base,
		"auth-required": authRequired,
	})
}

func (h *handler) index(w http.ResponseWriter, r *http.Request, req *packageRequest) {
	if h.proxyIndex(w, r, req) {
		return
	}
	entries, err := h.store.Index(r.Context(), req.Project, req.Crate)
	if err != nil {
		writeError(w, http.StatusNotFound, "crate not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n"))
	}
}

func (h *handler) publish(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPublishSize))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	publish, err := parsePublish(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.Publish(r.Context(), projectID, req.Project, publish); err != nil {
		writeCargoError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *handler) setYanked(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64, yanked bool) {
	if err := h.store.SetYanked(r.Context(), projectID, req.Project, req.Crate, req.Version, yanked); err != nil {
		status := http.StatusInternalServerError
		if harborerrors.IsNotFoundErr(err) {
			status = http.StatusNotFound
		}
		writeCargoError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *handler) download(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64) {
	content, err := h.store.OpenCrate(r.Context(), req.Project, req.Crate, req.Version)
	if err != nil {
		if harborerrors.IsNotFoundErr(err) && h.proxyDownload(w, r, req, projectID) {
			return
		}
		writeError(w, http.StatusNotFound, "crate not found")
		return
	}
	defer content.Body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if content.Size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", content.Size))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Body)
}

func (h *handler) proxyIndex(w http.ResponseWriter, r *http.Request, req *packageRequest) bool {
	proxy, err := pkgproxy.ForProject(r.Context(), req.Project, model.RegistryTypeCargo)
	if err != nil || proxy == nil {
		return false
	}
	resp, err := proxy.CachedGet(
		r.Context(),
		pkgproxy.CacheKey("cargo-index", req.Project, req.Crate),
		indexPath(req.Crate),
		5*time.Minute,
		nil,
	)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(resp.Body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Body)
	return true
}

func (h *handler) proxyDownload(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64) bool {
	proxy, err := pkgproxy.ForProject(r.Context(), req.Project, model.RegistryTypeCargo)
	if err != nil || proxy == nil {
		return false
	}
	entry, ok := h.upstreamIndexEntry(r, proxy, req)
	if !ok {
		return false
	}
	downloadPath, ok := h.upstreamDownloadPath(r, proxy, req, entry)
	if !ok {
		downloadPath = "api/v1/crates/" + url.PathEscape(req.Crate) + "/" + url.PathEscape(req.Version) + "/download"
	}
	resp, err := proxy.Get(r.Context(), downloadPath, nil)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(resp.Body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Body)
	cacheCtx := orm.Copy(r.Context())
	go func() {
		if err := h.store.Publish(cacheCtx, projectID, req.Project, publishRequest{
			Metadata: crateMetadata{
				Name:        entry.Name,
				Version:     entry.Version,
				Deps:        entry.Deps,
				Features:    entry.Features,
				Links:       entry.Links,
				RustVersion: entry.RustVersion,
			},
			Content: resp.Body,
		}); err != nil {
			log.Errorf("failed to cache Cargo crate %s@%s: %v", req.Crate, req.Version, err)
		}
	}()
	return true
}

func (h *handler) upstreamIndexEntry(r *http.Request, proxy *pkgproxy.Proxy, req *packageRequest) (indexEntry, bool) {
	resp, err := proxy.CachedGet(r.Context(), pkgproxy.CacheKey("cargo-index", req.Project, req.Crate), indexPath(req.Crate), 5*time.Minute, nil)
	if err != nil {
		return indexEntry{}, false
	}
	for _, line := range strings.Split(string(resp.Body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry indexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Version == req.Version {
			return entry, true
		}
	}
	return indexEntry{}, false
}

func (h *handler) upstreamDownloadPath(r *http.Request, proxy *pkgproxy.Proxy, req *packageRequest, entry indexEntry) (string, bool) {
	resp, err := proxy.CachedGet(r.Context(), pkgproxy.CacheKey("cargo-config", req.Project), "config.json", 15*time.Minute, nil)
	if err != nil {
		return "", false
	}
	var config struct {
		DL string `json:"dl"`
	}
	if err := json.Unmarshal(resp.Body, &config); err != nil || config.DL == "" {
		return "", false
	}
	return cargoDownloadURL(config.DL, req.Crate, req.Version, entry.Checksum)
}

func cargoDownloadURL(template, crate, version, checksum string) (string, bool) {
	template = strings.TrimRight(strings.TrimSpace(template), "/")
	if template == "" {
		return "", false
	}
	if !strings.Contains(template, "{") {
		return template + "/" + url.PathEscape(crate) + "/" + url.PathEscape(version) + "/download", true
	}
	prefix := cratePrefix(crate)
	replacements := map[string]string{
		"{crate}":           crate,
		"{version}":         version,
		"{prefix}":          prefix,
		"{lowerprefix}":     strings.ToLower(prefix),
		"{sha256-checksum}": checksum,
	}
	download := template
	for marker, value := range replacements {
		download = strings.ReplaceAll(download, marker, value)
	}
	if strings.Contains(download, "{") {
		return "", false
	}
	return download, true
}

func cratePrefix(crate string) string {
	switch len(crate) {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3/" + crate[:1]
	default:
		return crate[:2] + "/" + crate[2:4]
	}
}

func baseURL(r *http.Request) string {
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	scheme := "http"
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + host
}

func writeCargoError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"errors": []map[string]string{{"detail": message}}})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
