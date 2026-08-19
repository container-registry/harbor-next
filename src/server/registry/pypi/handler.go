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

package pypi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
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

const maxUploadSize = 512 << 20

const (
	simpleAPIHTMLContentType = "application/vnd.pypi.simple.v1+html; charset=utf-8"
	simpleAPIJSONContentType = "application/vnd.pypi.simple.v1+json"
	coreMetadataContentType  = "text/plain; charset=utf-8"
)

var hrefPattern = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)

type handler struct {
	store         packageStore
	projectID     projectIDResolver
	authenticate  credentialAuthenticator
	authorizePush pushAuthorizer
	proxyProject  proxyResolver
}

type projectIDResolver func(context.Context, string) (int64, error)
type credentialAuthenticator func(context.Context, string, string) (security.Context, error)
type pushAuthorizer func(context.Context, string, string) error
type proxyResolver func(context.Context, string, string) (*pkgproxy.Proxy, error)

func newHandler() http.Handler {
	handler := newHandlerWithDeps(newPackageStore(), resolveProjectID)
	handler.authorizePush = pkgpolicy.AuthorizePush
	return handler
}

func newHandlerWithDeps(store packageStore, projectID projectIDResolver) *handler {
	return newHandlerWithAuthenticator(store, projectID, authenticateCredentials)
}

func newHandlerWithAuthenticator(store packageStore, projectID projectIDResolver, authenticate credentialAuthenticator) *handler {
	return &handler{
		store:         store,
		projectID:     projectID,
		authenticate:  authenticate,
		authorizePush: allowPush,
		proxyProject:  pkgproxy.ForProject,
	}
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
	case requestUpload:
		h.upload(w, r, req, projectID)
	case requestSimpleRoot:
		h.simpleRoot(w, r, req)
	case requestSimple:
		h.simple(w, r, req)
	case requestDistribution:
		h.distribution(w, r, req, projectID)
	case requestMetadata:
		h.metadata(w, r, req)
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
	if req.Type == requestUpload {
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
	if req.Type == requestUpload {
		if err := h.authorizePush(r.Context(), req.Project, model.RegistryTypePyPI); err != nil {
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
	username, password, ok := credentialsFromBasic(r.Header.Get("Authorization"))
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

func credentialsFromBasic(header string) (string, string, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(header, "Basic "))
	if token == header || token == "" {
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

func (h *handler) upload(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload body: "+err.Error())
		return
	}
	upload, err := uploadFromMultipart(r.MultipartForm)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.Publish(r.Context(), projectID, req.Project, upload); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *handler) simpleRoot(w http.ResponseWriter, r *http.Request, req *packageRequest) {
	packages, err := h.store.ListPackages(r.Context(), req.Project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "package list unavailable")
		return
	}
	if wantsJSON(r) {
		writeJSON(w, r, simpleRootJSON(packages))
		return
	}
	w.Header().Set("Content-Type", simpleAPIHTMLContentType)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html><head><meta name=\"pypi:repository-version\" content=\"1.1\"></head><body>\n")
	for _, name := range packages {
		_, _ = fmt.Fprintf(w, `<a href="%s/">%s</a>`+"\n", html.EscapeString(url.PathEscape(name)), html.EscapeString(name))
	}
	_, _ = fmt.Fprintf(w, "</body></html>\n")
}

func (h *handler) simple(w http.ResponseWriter, r *http.Request, req *packageRequest) {
	if h.proxySimple(w, r, req) {
		return
	}
	pkg, err := h.store.Load(r.Context(), req.Project, req.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package not found")
		return
	}
	if wantsJSON(r) {
		writeJSON(w, r, simpleProjectJSON(baseURL(r), req.Project, pkg))
		return
	}
	w.Header().Set("Content-Type", simpleAPIHTMLContentType)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html><head><meta name=\"pypi:repository-version\" content=\"1.1\"></head><body>\n")
	for _, version := range pkg.Versions {
		for _, dist := range version.Distributions {
			href := distributionURL(baseURL(r), req.Project, pkg.Name, version.Version, dist.Filename) + "#sha256=" + dist.SHA256
			attrs := ""
			if version.RequiresPython != "" {
				attrs += fmt.Sprintf(` data-requires-python="%s"`, html.EscapeString(version.RequiresPython))
			}
			if dist.MetadataSHA256 != "" {
				attrs += fmt.Sprintf(` data-core-metadata="sha256=%s"`, html.EscapeString(dist.MetadataSHA256))
			}
			_, _ = fmt.Fprintf(w, `<a href="%s"%s>%s</a>`+"\n", html.EscapeString(href), attrs, html.EscapeString(dist.Filename))
		}
	}
	_, _ = fmt.Fprintf(w, "</body></html>\n")
}

func (h *handler) distribution(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64) {
	content, err := h.store.OpenDistribution(r.Context(), req.Project, req.Package, req.Version, req.Filename)
	if err != nil {
		if harborerrors.IsNotFoundErr(err) && h.proxyDistribution(w, r, req, projectID) {
			return
		}
		writeError(w, http.StatusNotFound, "distribution not found")
		return
	}
	defer content.Body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if content.Size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", content.Size))
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, content.Body)
}

func (h *handler) proxySimple(w http.ResponseWriter, r *http.Request, req *packageRequest) bool {
	proxy, err := h.proxyProject(r.Context(), req.Project, model.RegistryTypePyPI)
	if err != nil || proxy == nil {
		return false
	}
	headers := http.Header{}
	headers.Set("Accept", r.Header.Get("Accept"))
	upstreamPath := upstreamSimplePath(proxy, req.Package)
	resp, err := proxy.CachedGet(r.Context(), pkgproxy.CacheKey("pypi-simple", req.Project, req.Package, r.Header.Get("Accept")), upstreamPath, 5*time.Minute, headers)
	if err != nil {
		return false
	}
	if wantsJSON(r) {
		if !strings.Contains(strings.ToLower(resp.ContentType), "json") && !json.Valid(resp.Body) {
			body := rewriteSimpleHTML(resp.Body, baseURL(r), req.Project, req.Package)
			w.Header().Set("Content-Type", simpleAPIHTMLContentType)
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(body)
			}
			return true
		}
		body := rewriteSimpleJSON(resp.Body, baseURL(r), req.Project, req.Package)
		w.Header().Set("Content-Type", simpleAPIJSONContentType)
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
		return true
	}
	body := rewriteSimpleHTML(resp.Body, baseURL(r), req.Project, req.Package)
	w.Header().Set("Content-Type", simpleAPIHTMLContentType)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
	return true
}

func (h *handler) proxyDistribution(w http.ResponseWriter, r *http.Request, req *packageRequest, projectID int64) bool {
	proxy, err := pkgproxy.ForProject(r.Context(), req.Project, model.RegistryTypePyPI)
	if err != nil || proxy == nil {
		return false
	}
	upstreamURL, ok := h.upstreamDistributionURL(r, proxy, req)
	if !ok {
		return false
	}
	resp, err := proxy.Get(r.Context(), upstreamURL, nil)
	if err != nil {
		return false
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(resp.Body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(resp.Body)
	}
	cacheCtx := orm.Copy(r.Context())
	go func() {
		if err := h.store.Publish(cacheCtx, projectID, req.Project, uploadRequest{
			Name:           req.Package,
			NormalizedName: normalizeName(req.Package),
			Version:        req.Version,
			Filename:       req.Filename,
			ContentType:    contentType,
			Metadata:       map[string][]string{},
			Content:        resp.Body,
		}); err != nil {
			log.Errorf("failed to cache PyPI distribution %s: %v", req.Filename, err)
		}
	}()
	return true
}

func (h *handler) upstreamDistributionURL(r *http.Request, proxy *pkgproxy.Proxy, req *packageRequest) (string, bool) {
	simplePath := upstreamSimplePath(proxy, req.Package)
	headers := http.Header{"Accept": []string{"application/vnd.pypi.simple.v1+html, text/html"}}
	resp, err := proxy.CachedGet(r.Context(), pkgproxy.CacheKey("pypi-simple", req.Project, req.Package, "html"), simplePath, 5*time.Minute, headers)
	if err != nil {
		return "", false
	}
	for _, match := range hrefPattern.FindAllSubmatch(resp.Body, -1) {
		if len(match) != 2 {
			continue
		}
		rawURL := string(match[1])
		if !strings.Contains(rawURL, req.Filename) {
			continue
		}
		cleanURL := strings.Split(rawURL, "#")[0]
		return resolveUpstreamReference(proxy, simplePath, cleanURL)
	}
	return "", false
}

func rewriteSimpleJSON(payload []byte, base, project, name string) []byte {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return payload
	}
	files, ok := body["files"].([]any)
	if !ok {
		return payload
	}
	for _, raw := range files {
		file, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		filename, _ := file["filename"].(string)
		version := pypiVersionFromFilename(name, filename)
		if filename == "" || version == "" {
			continue
		}
		file["url"] = distributionURL(base, project, name, version, filename)
	}
	out, err := json.Marshal(body)
	if err != nil {
		return payload
	}
	return out
}

func rewriteSimpleHTML(payload []byte, base, project, name string) []byte {
	return hrefPattern.ReplaceAllFunc(payload, func(match []byte) []byte {
		parts := hrefPattern.FindSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		rawURL := string(parts[1])
		cleanURL := strings.Split(rawURL, "#")[0]
		fragment := ""
		if _, hash, ok := strings.Cut(rawURL, "#"); ok && hash != "" {
			fragment = "#" + hash
		}
		filename, err := url.PathUnescape(pathBase(cleanURL))
		if err != nil || filename == "" {
			return match
		}
		version := pypiVersionFromFilename(name, filename)
		if version == "" {
			return match
		}
		replacement := `href="` + distributionURL(base, project, name, version, filename) + fragment + `"`
		return []byte(replacement)
	})
}

func pypiVersionFromFilename(name, filename string) string {
	base := filename
	for _, suffix := range []string{".tar.gz", ".zip", ".whl", ".tar.bz2", ".tgz"} {
		base = strings.TrimSuffix(base, suffix)
	}
	name = strings.ReplaceAll(normalizeName(name), "-", "_")
	base = strings.ReplaceAll(base, "-", "_")
	prefix := name + "_"
	if !strings.HasPrefix(strings.ToLower(base), strings.ToLower(prefix)) {
		return ""
	}
	rest := base[len(prefix):]
	version, _, _ := strings.Cut(rest, "_")
	return strings.ReplaceAll(version, "_", "-")
}

func pathBase(value string) string {
	value = strings.TrimRight(value, "/")
	if i := strings.LastIndex(value, "/"); i >= 0 {
		return value[i+1:]
	}
	return value
}

func (h *handler) metadata(w http.ResponseWriter, r *http.Request, req *packageRequest) {
	if h.proxyMetadata(w, r, req) {
		return
	}
	content, err := h.store.OpenMetadata(r.Context(), req.Project, req.Package, req.Version, req.Filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "metadata not found")
		return
	}
	defer content.Body.Close()
	w.Header().Set("Content-Type", coreMetadataContentType)
	if content.Size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", content.Size))
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, content.Body)
}

func (h *handler) proxyMetadata(w http.ResponseWriter, r *http.Request, req *packageRequest) bool {
	proxy, err := pkgproxy.ForProject(r.Context(), req.Project, model.RegistryTypePyPI)
	if err != nil || proxy == nil {
		return false
	}
	upstreamURL, ok := h.upstreamDistributionURL(r, proxy, req)
	if !ok {
		return false
	}
	resp, err := proxy.Get(r.Context(), upstreamURL+".metadata", nil)
	if err != nil {
		return false
	}
	writePayload(w, r, coreMetadataContentType, resp.Body)
	return true
}

func upstreamSimplePath(proxy *pkgproxy.Proxy, name string) string {
	if proxy != nil && proxy.Registry != nil {
		if upstream, err := url.Parse(proxy.Registry.URL); err == nil && strings.HasSuffix(strings.TrimRight(upstream.Path, "/"), "/simple") {
			return url.PathEscape(name) + "/"
		}
	}
	return "simple/" + url.PathEscape(name) + "/"
}

func resolveUpstreamReference(proxy *pkgproxy.Proxy, pagePath, reference string) (string, bool) {
	if proxy == nil || proxy.Registry == nil {
		return "", false
	}
	base, err := url.Parse(strings.TrimRight(proxy.Registry.URL, "/") + "/")
	if err != nil {
		return "", false
	}
	page, err := base.Parse(pagePath)
	if err != nil {
		return "", false
	}
	resolved, err := page.Parse(reference)
	if err != nil || (resolved.Scheme != "http" && resolved.Scheme != "https") {
		return "", false
	}
	return resolved.String(), true
}

func writePayload(w http.ResponseWriter, r *http.Request, contentType string, payload []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(payload)
	}
}

type simpleRootResponse struct {
	Meta     simpleMeta      `json:"meta"`
	Projects []simpleProject `json:"projects"`
}

type simpleMeta struct {
	APIVersion string `json:"api-version"`
}

type simpleProject struct {
	Name string `json:"name"`
}

type simpleProjectResponse struct {
	Meta     simpleMeta   `json:"meta"`
	Name     string       `json:"name"`
	Files    []simpleFile `json:"files"`
	Versions []string     `json:"versions"`
}

type simpleFile struct {
	Filename       string            `json:"filename"`
	URL            string            `json:"url"`
	Hashes         map[string]string `json:"hashes"`
	RequiresPython string            `json:"requires-python,omitempty"`
	CoreMetadata   map[string]string `json:"core-metadata,omitempty"`
	Size           int64             `json:"size"`
	UploadTime     string            `json:"upload-time,omitempty"`
}

func simpleRootJSON(packages []string) simpleRootResponse {
	resp := simpleRootResponse{
		Meta:     simpleMeta{APIVersion: "1.1"},
		Projects: []simpleProject{},
	}
	for _, name := range packages {
		resp.Projects = append(resp.Projects, simpleProject{Name: name})
	}
	return resp
}

func simpleProjectJSON(base, project string, pkg *storedPackage) simpleProjectResponse {
	resp := simpleProjectResponse{
		Meta:     simpleMeta{APIVersion: "1.1"},
		Name:     pkg.Name,
		Files:    []simpleFile{},
		Versions: []string{},
	}
	versionSet := map[string]struct{}{}
	for _, version := range pkg.Versions {
		versionSet[version.Version] = struct{}{}
		for _, dist := range version.Distributions {
			file := simpleFile{
				Filename: dist.Filename,
				URL:      distributionURL(base, project, pkg.Name, version.Version, dist.Filename),
				Hashes: map[string]string{
					"sha256": dist.SHA256,
				},
				RequiresPython: version.RequiresPython,
				Size:           dist.Size,
			}
			if dist.MetadataSHA256 != "" {
				file.CoreMetadata = map[string]string{"sha256": dist.MetadataSHA256}
			}
			if !dist.UploadedAt.IsZero() {
				file.UploadTime = dist.UploadedAt.UTC().Format("2006-01-02T15:04:05.000000Z")
			}
			resp.Files = append(resp.Files, file)
		}
	}
	for version := range versionSet {
		resp.Versions = append(resp.Versions, version)
	}
	sort.Strings(resp.Versions)
	return resp
}

func writeJSON(w http.ResponseWriter, r *http.Request, body any) {
	w.Header().Set("Content-Type", simpleAPIJSONContentType)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func wantsJSON(r *http.Request) bool {
	if strings.EqualFold(r.URL.Query().Get("format"), "application/vnd.pypi.simple.v1+json") ||
		strings.EqualFold(r.URL.Query().Get("format"), "application/vnd.pypi.simple.latest+json") {
		return true
	}
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	jsonQ := acceptQuality(accept, "application/vnd.pypi.simple.v1+json")
	if latestQ := acceptQuality(accept, "application/vnd.pypi.simple.latest+json"); latestQ > jsonQ {
		jsonQ = latestQ
	}
	htmlQ := acceptQuality(accept, "application/vnd.pypi.simple.v1+html")
	if textHTMLQ := acceptQuality(accept, "text/html"); textHTMLQ > htmlQ {
		htmlQ = textHTMLQ
	}
	if wildcardQ := acceptQuality(accept, "*/*"); wildcardQ > htmlQ {
		htmlQ = wildcardQ
	}
	return jsonQ > 0 && jsonQ >= htmlQ
}

func acceptQuality(header, mediaType string) float64 {
	best := 0.0
	for _, part := range strings.Split(header, ",") {
		tokens := strings.Split(strings.TrimSpace(part), ";")
		if len(tokens) == 0 || !strings.EqualFold(strings.TrimSpace(tokens[0]), mediaType) {
			continue
		}
		q := 1.0
		for _, token := range tokens[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(token), "=")
			if !ok || key != "q" {
				continue
			}
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				q = parsed
			}
		}
		if q > best {
			best = q
		}
	}
	return best
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

func distributionURL(base, project, name, version, filename string) string {
	return strings.TrimRight(base, "/") + "/pypi/" + url.PathEscape(project) + "/packages/" + url.PathEscape(name) + "/" + url.PathEscape(version) + "/" + url.PathEscape(filename)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}
