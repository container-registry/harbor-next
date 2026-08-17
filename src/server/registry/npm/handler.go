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

package npm

import (
	"context"
	"crypto/sha1" // nolint:gosec // sha1 is part of the npm package integrity (shasum) wire format
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/controller/multiformat/semver"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	server "github.com/goharbor/harbor/src/server/registry/multiformat"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

const formatNPM = "npm"

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// After StripPrefix("/npm") the leading segment is the project. Peel it and
	// continue with the multi-format-oci-shaped remainder. Path forms (after peel):
	//   PUT /<name>                      -> publish
	//   GET /<name>                      -> packument
	//   GET /<name>/-/<file>.tgz         -> tarball download
	//   GET    /-/package/<name>/dist-tags        -> list dist-tags
	//   PUT    /-/package/<name>/dist-tags/<tag>  -> add/re-point dist-tag
	//   DELETE /-/package/<name>/dist-tags/<tag>  -> remove dist-tag
	//   GET    /-/whoami                          -> identity of the calling credentials
	// <name> may be scoped "@scope/pkg" (spans path segments).
	project, rest, ok := peelProject(r.URL.Path)
	if !ok {
		writeNPMError(w, http.StatusNotFound, "project required")
		return
	}
	rc := requestCtx{project: project, projectID: projectIDFromContext(r.Context()), rest: rest}

	switch {
	case rest == "/-/ping":
		if r.Method != http.MethodGet {
			writeNPMError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	case rest == "/-/whoami":
		h.whoami(w, r)
		return
	case rest == "/-/v1/login":
		// npm's browser-based web-login flow has no Harbor equivalent; clients
		// authenticate with Basic credentials (.npmrc `_auth`) instead, which
		// multiformatauth already validates upstream of this handler.
		writeNPMError(w, http.StatusNotFound, "web login not supported, use Basic auth (.npmrc _auth)")
		return
	case strings.HasPrefix(rest, "/-/package/"):
		if (r.Method == http.MethodPut || r.Method == http.MethodDelete) && !h.allowPublish(w, r, project) {
			return
		}
		h.distTags(w, r, rc)
		return
	case strings.Contains(rest, "/-rev/"):
		if (r.Method == http.MethodPut || r.Method == http.MethodDelete) && !h.allowPublish(w, r, project) {
			return
		}
		h.revision(w, r, rc)
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !h.allowPublish(w, r, project) {
			return
		}
		h.publish(w, r, rc)
	case http.MethodGet:
		if isTarball(rest) {
			h.tarball(w, r, rc)
		} else {
			h.packument(w, r, rc)
		}
	default:
		writeNPMError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *handler) allowPublish(w http.ResponseWriter, r *http.Request, project string) bool {
	if h.authorizePush == nil {
		return true
	}
	if err := h.authorizePush(r.Context(), project, regmodel.RegistryTypeNPM); err != nil {
		writeNPMError(w, http.StatusForbidden, err.Error())
		return false
	}
	return true
}

// whoami reports the identity multiformatauth already authenticated upstream (it
// requires a valid security.Context to have resolved project access at all,
// so by the time we're here the caller is known).
func (h *handler) whoami(w http.ResponseWriter, r *http.Request) {
	securityCtx, ok := security.FromContext(r.Context())
	if !ok || !securityCtx.IsAuthenticated() {
		w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
		writeNPMError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": securityCtx.GetUsername()})
}

// requestCtx carries the resolved project identity + the project-relative path
// through the per-route handlers.
type requestCtx struct {
	project   string
	projectID int64
	rest      string // path with the leading "/<project>" segment removed
}

// peelProject removes the leading "/<project>" segment from a StripPrefix'd npm
// path, returning the project name and the remaining path (with its leading
// slash). "/proj" alone yields rest "/". Returns ok=false if no project segment
// is present.
func peelProject(p string) (project, rest string, ok bool) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", "", false
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], p[i:], true
	}
	return p, "/", true
}

// distTags handles the npm dist-tag protocol:
//
//	GET    /-/package/<name>/dist-tags         -> {"latest":"1.0.0", ...}
//	PUT    /-/package/<name>/dist-tags/<tag>   body=`"1.0.0"` -> add/re-point
//	DELETE /-/package/<name>/dist-tags/<tag>   -> remove
//
// dist-tags are projection-authoritative; re-point/remove happen WITHOUT
// republishing the artifact. <name> may be scoped (spans path segments).
func (h *handler) distTags(w http.ResponseWriter, r *http.Request, rc requestCtx) {
	rest := strings.TrimPrefix(rc.rest, "/-/package/")
	i := strings.Index(rest, "/dist-tags")
	if i < 0 {
		writeNPMError(w, http.StatusNotFound, "not found")
		return
	}
	name := rest[:i]
	after := strings.TrimPrefix(rest[i+len("/dist-tags"):], "/") // "" or "<tag>"

	switch r.Method {
	case http.MethodGet:
		st, ok, err := h.deps.State.LoadState(r.Context(), rc.project, rc.projectID, formatNPM, name)
		if err != nil {
			h.serverError(w, "load dist-tags "+name, err)
			return
		}
		if !ok {
			writeNPMError(w, http.StatusNotFound, "package not found")
			return
		}
		writeJSON(w, http.StatusOK, st.DistTags)
	case http.MethodPut:
		if after == "" {
			writeNPMError(w, http.StatusBadRequest, "dist-tag name required")
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		var version string
		if err := json.Unmarshal(body, &version); err != nil {
			version = strings.Trim(strings.TrimSpace(string(body)), `"`)
		}
		if _, err := h.deps.DistTags.SetDistTag(r.Context(), rc.project, rc.projectID, formatNPM, name, after, version); err != nil {
			h.distTagError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{after: version})
	case http.MethodDelete:
		if after == "" {
			writeNPMError(w, http.StatusBadRequest, "dist-tag name required")
			return
		}
		if _, err := h.deps.DistTags.SetDistTag(r.Context(), rc.project, rc.projectID, formatNPM, name, after, ""); err != nil {
			h.distTagError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeNPMError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *handler) distTagError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "not found") {
		writeNPMError(w, http.StatusNotFound, err.Error())
		return
	}
	h.serverError(w, "write dist-tag", err)
}

func isTarball(p string) bool {
	return strings.Contains(p, "/-/") && strings.HasSuffix(p, ".tgz")
}

// packageNameFromRest extracts the native package name (scoped or not) from a
// packument path "/<name>".
func packageNameFromRest(p string) string {
	return strings.TrimPrefix(p, "/")
}

// tarballParts splits "/<name>/-/<file>.tgz" into (name, file).
func tarballParts(p string) (name, file string) {
	p = strings.TrimPrefix(p, "/")
	i := strings.Index(p, "/-/")
	if i < 0 {
		return "", ""
	}
	return p[:i], p[i+len("/-/"):]
}

// ---- publish (PUT) ----

// publishBody is the npm publish request shape (empirically captured).
type publishBody struct {
	ID          string                     `json:"_id"`
	Name        string                     `json:"name"`
	DistTags    map[string]string          `json:"dist-tags"`
	Versions    map[string]json.RawMessage `json:"versions"`
	Attachments map[string]attachment      `json:"_attachments"`
}

type attachment struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
	Length      int64  `json:"length"`
}

func (h *handler) publish(w http.ResponseWriter, r *http.Request, rc requestCtx) {
	name := packageNameFromRest(rc.rest)
	body, err := io.ReadAll(io.LimitReader(r.Body, 512<<20))
	if err != nil {
		writeNPMError(w, http.StatusBadRequest, err.Error())
		return
	}
	var pb publishBody
	if err := json.Unmarshal(body, &pb); err != nil {
		writeNPMError(w, http.StatusBadRequest, "invalid publish body: "+err.Error())
		return
	}
	if pb.Name == "" {
		pb.Name = name
	}
	if len(pb.Versions) == 0 || len(pb.Attachments) == 0 {
		if len(pb.Versions) > 0 && len(pb.Attachments) == 0 {
			h.updateMetadata(w, r, rc, pb)
			return
		}
		writeNPMError(w, http.StatusBadRequest, "publish body missing versions or _attachments")
		return
	}

	// Each publish carries exactly one new version (npm CLI behavior).
	for ver, rawMeta := range pb.Versions {
		att, ok := attachmentForVersion(pb)
		if !ok {
			writeNPMError(w, http.StatusBadRequest, "no _attachment for version "+ver)
			return
		}
		// layer0 = the EXACT unmodified tarball bytes the client base64-encoded.
		// We never recompress; integrity is recomputed over these bytes and must
		// match what the client put in dist.
		payload, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			writeNPMError(w, http.StatusBadRequest, "bad base64 attachment: "+err.Error())
			return
		}
		if att.Length != 0 && int64(len(payload)) != att.Length {
			writeNPMError(w, http.StatusBadRequest,
				fmt.Sprintf("attachment length mismatch: header %d decoded %d", att.Length, len(payload)))
			return
		}

		// Strip the client-guessed dist (its tarball URL is wrong for us; we mint
		// dist on render) and embed integrity over the EXACT stored bytes so the
		// packument render is a pure function of projection state.
		shasum, integrity := computeIntegrity(payload)
		meta := stripDistEmbedIntegrity(rawMeta, shasum, integrity)

		ev := server.PublishEvent{
			Format:    formatNPM,
			ProjectID: rc.projectID,
			Name:      pb.Name,
			Version:   ver,
			Payload:   payload,
			Meta:      meta,
			DistTags:  pb.DistTags,
			Immutable: true, // npm versions are immutable once published
		}
		if _, err := h.deps.Publisher.Publish(r.Context(), rc.project, ev); err != nil {
			if strings.Contains(err.Error(), "immutable") {
				writeNPMError(w, http.StatusConflict, err.Error())
				return
			}
			h.serverError(w, "publish "+pb.Name+"@"+ver, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "success": true})
}

func (h *handler) updateMetadata(w http.ResponseWriter, r *http.Request, rc requestCtx, pb publishBody) {
	name := packageNameFromRest(rc.rest)
	st, ok, err := h.deps.State.LoadState(r.Context(), rc.project, rc.projectID, formatNPM, name)
	if err != nil {
		h.serverError(w, "load state "+name, err)
		return
	}
	if !ok {
		writeNPMError(w, http.StatusNotFound, "package not found")
		return
	}
	existing := make(map[string]model.Version, len(st.Versions))
	for _, v := range st.Versions {
		existing[v.Version] = v
	}
	for version, incoming := range pb.Versions {
		current, found := existing[version]
		if !found {
			writeNPMError(w, http.StatusNotFound, "version "+version+" not found")
			return
		}
		meta, err := metadataUpdate(current.Meta, incoming)
		if err != nil {
			writeNPMError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := h.deps.Versions.UpdateVersionMetadata(r.Context(), rc.project, rc.projectID, formatNPM, name, version, meta); err != nil {
			h.serverError(w, "update metadata "+name+"@"+version, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func metadataUpdate(existing, incoming []byte) ([]byte, error) {
	var oldMeta, newMeta map[string]json.RawMessage
	if err := json.Unmarshal(existing, &oldMeta); err != nil {
		return nil, fmt.Errorf("decode existing metadata: %w", err)
	}
	if err := json.Unmarshal(incoming, &newMeta); err != nil {
		return nil, fmt.Errorf("decode metadata update: %w", err)
	}
	delete(newMeta, "dist")
	for _, key := range []string{"_multi-format-ociShasum", "_multi-format-ociIntegrity"} {
		if value, ok := oldMeta[key]; ok {
			newMeta[key] = value
		}
	}
	return canonicalJSON(newMeta)
}

func (h *handler) revision(w http.ResponseWriter, r *http.Request, rc requestCtx) {
	path := strings.SplitN(strings.TrimPrefix(rc.rest, "/"), "/-rev/", 2)[0]
	if strings.Contains(path, "/-/") {
		// libnpmpublish deletes the tarball after removing the version from the
		// packument. The payload is already unreachable, so this is idempotent.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	name := path
	st, ok, err := h.deps.State.LoadState(r.Context(), rc.project, rc.projectID, formatNPM, name)
	if err != nil {
		h.serverError(w, "load state "+name, err)
		return
	}
	if !ok {
		writeNPMError(w, http.StatusNotFound, "package not found")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		for _, v := range st.Versions {
			if _, err := h.deps.Versions.DeleteVersion(r.Context(), rc.project, rc.projectID, formatNPM, name, v.Version); err != nil {
				h.serverError(w, "delete "+name+"@"+v.Version, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			writeNPMError(w, http.StatusBadRequest, err.Error())
			return
		}
		var desired publishBody
		if err := json.Unmarshal(body, &desired); err != nil {
			writeNPMError(w, http.StatusBadRequest, "invalid packument: "+err.Error())
			return
		}
		for _, v := range st.Versions {
			if _, keep := desired.Versions[v.Version]; keep {
				continue
			}
			if _, err := h.deps.Versions.DeleteVersion(r.Context(), rc.project, rc.projectID, formatNPM, name, v.Version); err != nil {
				h.serverError(w, "delete "+name+"@"+v.Version, err)
				return
			}
		}
		for tag := range st.DistTags {
			if _, keep := desired.DistTags[tag]; !keep {
				_, _ = h.deps.DistTags.SetDistTag(r.Context(), rc.project, rc.projectID, formatNPM, name, tag, "")
			}
		}
		for tag, version := range desired.DistTags {
			if _, err := h.deps.DistTags.SetDistTag(r.Context(), rc.project, rc.projectID, formatNPM, name, tag, version); err != nil {
				h.distTagError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeNPMError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// attachmentForVersion returns the single attachment (npm sends one per publish).
func attachmentForVersion(pb publishBody) (attachment, bool) {
	for _, a := range pb.Attachments {
		return a, true
	}
	return attachment{}, false
}

// stripDistEmbedIntegrity removes the client-guessed dist from a version
// manifest and embeds the computed shasum/integrity (over the exact stored
// bytes) under reserved keys so the packument render is a pure function of
// projection state.
func stripDistEmbedIntegrity(raw json.RawMessage, shasum, integrity string) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	delete(m, "dist")
	sb, _ := json.Marshal(shasum)
	ib, _ := json.Marshal(integrity)
	m["_multi-format-ociShasum"] = sb
	m["_multi-format-ociIntegrity"] = ib
	out, err := canonicalJSON(m)
	if err != nil {
		return raw
	}
	return out
}

// baseURL returns the absolute "scheme://host" prefix used to mint tarball URLs.
// A package registry must mint URLs the client can actually reach, so we derive
// it from the incoming request (honoring the proxy's X-Forwarded-Proto/Host)
// rather than a statically configured endpoint, which in some deployments is an
// internal-only address. Falls back to the configured BaseURL if the request
// carries no Host.
func (h *handler) baseURL(r *http.Request) string {
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if host == "" {
		return h.deps.BaseURL
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + host
}

// ---- packument (GET) ----

func (h *handler) packument(w http.ResponseWriter, r *http.Request, rc requestCtx) {
	name := packageNameFromRest(rc.rest)
	st, ok, err := h.deps.State.LoadState(r.Context(), rc.project, rc.projectID, formatNPM, name)
	if err != nil {
		h.serverError(w, "load state "+name, err)
		return
	}
	var local, localOverlay []byte
	if ok && len(st.Versions) > 0 {
		local, err = renderPackument(st, h.baseURL(r), rc.project)
		if err != nil {
			h.serverError(w, "render packument "+name, err)
			return
		}
		localOverlay, err = renderPackumentWithFallbackTags(st, h.baseURL(r), rc.project, false)
		if err != nil {
			h.serverError(w, "render packument overlay "+name, err)
			return
		}
	}

	// Proxy metadata remains authoritative for the complete upstream version
	// set. Overlay local state so hosted versions and dist-tags win without a
	// partially cached package hiding versions that have not been requested yet.
	if upstream, found := h.proxyPackument(r, rc); found {
		rendered := upstream
		if len(localOverlay) > 0 {
			rendered, err = mergePackuments(upstream, localOverlay)
			if err != nil {
				h.serverError(w, "merge packument "+name, err)
				return
			}
		}
		h.writePackument(w, r, name, rendered)
		return
	}

	// An unavailable upstream must not make already cached or locally
	// published versions disappear.
	if len(local) > 0 {
		h.writePackument(w, r, name, local)
		return
	}
	writeNPMError(w, http.StatusNotFound, "package not found")
}

func (h *handler) writePackument(w http.ResponseWriter, r *http.Request, name string, rendered []byte) {
	et := etag(rendered)
	w.Header().Set("ETag", et)
	w.Header().Set("Content-Type", "application/json")
	if match := r.Header.Get("If-None-Match"); match != "" && match == et {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if _, err := w.Write(rendered); err != nil {
		log.Errorf("npm: write packument %s: %v", name, err)
	}
}

// mergePackuments preserves the upstream document while overlaying local
// versions, dist-tags, and timestamps. This keeps uncached upstream releases
// visible and gives explicitly published local data precedence on collisions.
func mergePackuments(upstream, local []byte) ([]byte, error) {
	var merged, overlay map[string]any
	if err := json.Unmarshal(upstream, &merged); err != nil {
		return nil, fmt.Errorf("decode upstream: %w", err)
	}
	if err := json.Unmarshal(local, &overlay); err != nil {
		return nil, fmt.Errorf("decode local: %w", err)
	}
	for _, field := range []string{"versions", "dist-tags"} {
		base, _ := merged[field].(map[string]any)
		if base == nil {
			base = map[string]any{}
		}
		if values, ok := overlay[field].(map[string]any); ok {
			for key, value := range values {
				base[key] = value
			}
		}
		merged[field] = base
	}
	merged["time"] = mergePackumentTimes(merged["time"], overlay["time"])
	for _, field := range []string{"_id", "name"} {
		if value, ok := overlay[field]; ok {
			merged[field] = value
		}
	}
	return canonicalJSON(merged)
}

func mergePackumentTimes(upstream, local any) map[string]any {
	merged, _ := upstream.(map[string]any)
	if merged == nil {
		merged = map[string]any{}
	}
	values, _ := local.(map[string]any)
	for key, value := range values {
		if key != "created" && key != "modified" {
			merged[key] = value
		}
	}
	for _, aggregate := range []struct {
		key     string
		localIs func(time.Time, time.Time) bool
	}{
		{key: "created", localIs: time.Time.Before},
		{key: "modified", localIs: time.Time.After},
	} {
		localValue, localOK := values[aggregate.key].(string)
		upstreamValue, upstreamOK := merged[aggregate.key].(string)
		if !localOK {
			continue
		}
		if !upstreamOK {
			merged[aggregate.key] = localValue
			continue
		}
		localTime, localErr := time.Parse(time.RFC3339, localValue)
		upstreamTime, upstreamErr := time.Parse(time.RFC3339, upstreamValue)
		if localErr == nil && upstreamErr == nil && aggregate.localIs(localTime, upstreamTime) {
			merged[aggregate.key] = localValue
		}
	}
	return merged
}

// renderPackument builds the npm packument deterministically from PackageState.
// Canonical JSON (sorted keys), versions in semver order, RFC3339 UTC times,
// server-minted tarball URLs. Same state -> same bytes -> stable ETag.
func renderPackument(st model.PackageState, baseURL, project string) ([]byte, error) {
	return renderPackumentWithFallbackTags(st, baseURL, project, true)
}

func renderPackumentWithFallbackTags(st model.PackageState, baseURL, project string, fallbackTags bool) ([]byte, error) {
	verStrings := make([]string, 0, len(st.Versions))
	byVer := make(map[string]model.Version, len(st.Versions))
	for _, v := range st.Versions {
		verStrings = append(verStrings, v.Version)
		byVer[v.Version] = v
	}
	sort.Slice(verStrings, func(i, j int) bool {
		return semver.Compare(semver.Parse(verStrings[i]), semver.Parse(verStrings[j])) < 0
	})

	versions := map[string]json.RawMessage{}
	timeMap := map[string]string{}
	var earliest, latest time.Time
	for _, ver := range verStrings {
		v := byVer[ver]
		dist := buildDist(st.Name, ver, baseURL, project, v)
		full, err := mergeMeta(v.Meta, dist)
		if err != nil {
			return nil, err
		}
		versions[ver] = full
		timeMap[ver] = v.Created.UTC().Format(time.RFC3339)
		if earliest.IsZero() || v.Created.Before(earliest) {
			earliest = v.Created
		}
		if v.Created.After(latest) {
			latest = v.Created
		}
	}
	timeMap["created"] = earliest.UTC().Format(time.RFC3339)
	timeMap["modified"] = latest.UTC().Format(time.RFC3339)

	// dist-tags: projection-authoritative. If empty, default latest = semver max.
	distTags := map[string]string{}
	for k, v := range st.DistTags {
		distTags[k] = v
	}
	if _, ok := distTags["latest"]; !ok && fallbackTags {
		if mx := semver.Max(verStrings); mx != "" {
			distTags["latest"] = mx
		}
	}

	packument := map[string]any{
		"_id":       st.Name,
		"_rev":      fmt.Sprintf("%d", st.ProjVersion),
		"name":      st.Name,
		"dist-tags": distTags,
		"versions":  versions,
		"time":      timeMap,
	}
	return canonicalJSON(packument)
}

// buildDist mints the dist object: server tarball URL + the shasum/integrity
// embedded at publish over the exact stored bytes (so the SRI check passes on
// install).
func buildDist(name, version, baseURL, project string, v model.Version) map[string]any {
	tarball := fmt.Sprintf("%s%s/%s/-/%s", strings.TrimRight(baseURL, "/"), Prefix,
		project+"/"+encodePathName(name), tarballFile(name, version))
	dist := map[string]any{"tarball": tarball}
	if v.Meta != nil {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(v.Meta, &m); err == nil {
			if s, ok := m["_multi-format-ociShasum"]; ok {
				var str string
				_ = json.Unmarshal(s, &str)
				dist["shasum"] = str
			}
			if s, ok := m["_multi-format-ociIntegrity"]; ok {
				var str string
				_ = json.Unmarshal(s, &str)
				dist["integrity"] = str
			}
		}
	}
	return dist
}

// mergeMeta splices the minted dist into the stored per-version manifest, drops
// the multi-format-oci-internal keys, and re-canonicalizes.
func mergeMeta(meta []byte, dist map[string]any) (json.RawMessage, error) {
	m := map[string]json.RawMessage{}
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &m); err != nil {
			return nil, err
		}
	}
	delete(m, "_multi-format-ociShasum")
	delete(m, "_multi-format-ociIntegrity")
	distBytes, err := json.Marshal(dist)
	if err != nil {
		return nil, err
	}
	m["dist"] = distBytes
	return canonicalJSON(m)
}

// ---- tarball (GET) ----

func (h *handler) tarball(w http.ResponseWriter, r *http.Request, rc requestCtx) {
	name, file := tarballParts(rc.rest)
	st, ok, err := h.deps.State.LoadState(r.Context(), rc.project, rc.projectID, formatNPM, name)
	if err != nil {
		h.serverError(w, "load state "+name, err)
		return
	}
	var found *model.Version
	if ok {
		ver := versionFromTarballFile(name, file)
		for i := range st.Versions {
			if st.Versions[i].Version == ver {
				found = &st.Versions[i]
				break
			}
		}
	}
	if found == nil {
		// Not stored natively (or version missing) - try the upstream proxy
		// before giving up, same as packument.
		if h.proxyTarball(w, r, rc, name, file) {
			return
		}
		writeNPMError(w, http.StatusNotFound, "version not found")
		return
	}
	data, err := h.deps.Payload.PayloadBlob(r.Context(), rc.project, formatNPM, name, found.PayloadDigest, found.PayloadSize)
	if err != nil {
		h.serverError(w, "fetch tarball "+name+"@"+versionFromTarballFile(name, file), err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := w.Write(data); err != nil {
		log.Errorf("npm: write tarball %s: %v", name, err)
	}
}

// ---- proxy-cache fallback (npm/pypi/maven/... proxy-cache projects) ----
//
// A project can be configured to proxy an upstream npm-type registry (see
// PERMITTED_REGISTRY_TYPES_FOR_PROXY_CACHE). Packuments always consult the
// upstream so a partially populated local cache cannot hide releases. Local
// versions and dist-tags are overlaid on that complete view. Tarballs are
// fetched and cached only when the client asks for a specific version.

func (h *handler) proxyPackument(r *http.Request, rc requestCtx) ([]byte, bool) {
	name := packageNameFromRest(rc.rest)
	proxy, err := h.proxyForProject(r.Context(), rc.project)
	if err != nil || proxy == nil || proxy.Registry == nil {
		return nil, false
	}
	resp, err := proxy.Get(r.Context(), url.PathEscape(name), nil)
	if err != nil {
		return nil, false
	}
	var packument map[string]any
	if err := json.Unmarshal(resp.Body, &packument); err != nil {
		return nil, false
	}
	return rewritePackumentTarballs(resp.Body, h.baseURL(r), rc.project, name), true
}

func (h *handler) proxyTarball(w http.ResponseWriter, r *http.Request, rc requestCtx, name, file string) bool {
	proxy, err := h.proxyForProject(r.Context(), rc.project)
	if err != nil || proxy == nil || proxy.Registry == nil {
		return false
	}
	resp, err := proxy.Get(r.Context(), url.PathEscape(name), nil)
	if err != nil {
		return false
	}
	var packument struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(resp.Body, &packument); err != nil {
		return false
	}
	version := versionFromTarballFile(name, file)
	rawMeta, ok := packument.Versions[version]
	if !ok {
		return false
	}
	var meta struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	}
	if err := json.Unmarshal(rawMeta, &meta); err != nil || meta.Dist.Tarball == "" {
		return false
	}
	tarballResp, err := proxy.Get(r.Context(), meta.Dist.Tarball, nil)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(tarballResp.Body)

	shasum, integrity := computeIntegrity(tarballResp.Body)
	ev := server.PublishEvent{
		Format:    formatNPM,
		ProjectID: rc.projectID,
		Name:      name,
		Version:   version,
		Payload:   tarballResp.Body,
		Meta:      stripDistEmbedIntegrity(rawMeta, shasum, integrity),
		Immutable: true,
	}
	if _, err := h.deps.Publisher.Publish(r.Context(), rc.project, ev); err != nil && !strings.Contains(err.Error(), "immutable") {
		log.Warningf("npm: cache proxied tarball %s@%s: %v", name, version, err)
	}
	return true
}

func (h *handler) proxyForProject(ctx context.Context, project string) (*pkgproxy.Proxy, error) {
	if h.resolveProxy != nil {
		return h.resolveProxy(ctx, project, regmodel.RegistryTypeNPM)
	}
	return pkgproxy.ForProject(ctx, project, regmodel.RegistryTypeNPM)
}

// rewritePackumentTarballs rewrites every version's dist.tarball to point back
// at this handler instead of upstream, so the npm client's next request (the
// actual tarball download) lands on proxyTarball above instead of leaking the
// upstream registry URL to the client.
func rewritePackumentTarballs(raw []byte, baseURL, project, name string) []byte {
	var packument map[string]any
	if err := json.Unmarshal(raw, &packument); err != nil {
		return raw
	}
	versions, ok := packument["versions"].(map[string]any)
	if !ok {
		return raw
	}
	for version, v := range versions {
		meta, ok := v.(map[string]any)
		if !ok {
			continue
		}
		dist, ok := meta["dist"].(map[string]any)
		if !ok {
			dist = map[string]any{}
			meta["dist"] = dist
		}
		dist["tarball"] = fmt.Sprintf("%s%s/%s/-/%s", strings.TrimRight(baseURL, "/"), Prefix,
			project+"/"+encodePathName(name), tarballFile(name, version))
	}
	out, err := json.Marshal(packument)
	if err != nil {
		return raw
	}
	return out
}

// ---- helpers ----

// encodePathName percent-encodes a scoped name's '/' for the tarball URL path
// (npm uses %2f). Unscoped names pass through.
func encodePathName(name string) string {
	return strings.ReplaceAll(name, "/", "%2f")
}

// tarballFile builds the .tgz filename npm expects. For scoped "@scope/pkg" the
// file is "pkg-version.tgz" (scope dropped from filename, kept in path).
func tarballFile(name, version string) string {
	short := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		short = name[i+1:]
	}
	return fmt.Sprintf("%s-%s.tgz", short, version)
}

func versionFromTarballFile(name, file string) string {
	short := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		short = name[i+1:]
	}
	file = strings.TrimSuffix(file, ".tgz")
	return strings.TrimPrefix(file, short+"-")
}

// canonicalJSON marshals with sorted keys (encoding/json sorts map keys),
// producing deterministic bytes.
func canonicalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeNPMError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// serverError logs the underlying cause and returns a generic 500 to the client
// (avoid leaking internal detail to the npm CLI).
func (h *handler) serverError(w http.ResponseWriter, op string, err error) {
	log.Errorf("npm: %s: %v", op, err)
	writeNPMError(w, http.StatusInternalServerError, "internal server error")
}

// computeIntegrity returns (sha1hex, sha512SRI) over the exact payload bytes.
func computeIntegrity(payload []byte) (string, string) {
	s1 := sha1.Sum(payload) // nolint:gosec // sha1 is part of the npm package integrity (shasum) wire format
	s5 := sha512.Sum512(payload)
	return hex.EncodeToString(s1[:]), "sha512-" + base64.StdEncoding.EncodeToString(s5[:])
}

// etag computes the strong ETag for rendered packument bytes.
func etag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}
