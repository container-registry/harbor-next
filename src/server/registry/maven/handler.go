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

package maven

import (
	"context"
	"crypto/md5"  // nolint:gosec // md5 is mandated by the Maven repository checksum protocol
	"crypto/sha1" // nolint:gosec // sha1 is mandated by the Maven repository checksum protocol
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/controller/multiformat/mavenver"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	regmultiformat "github.com/goharbor/harbor/src/server/registry/multiformat"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

// ---- filename codec ----

// fileCoordFromFilename derives classifier/extension/timestamp/buildNumber from a
// Maven filename, given the artifactId and version directory. Forms:
//
//	<a>-<v>.<ext>                                       release main
//	<a>-<v>-<classifier>.<ext>                          release classified
//	<a>-<base>-<TS>-<BN>.<ext>          (v=base-SNAPSHOT) snapshot main
//	<a>-<base>-<TS>-<BN>-<classifier>.<ext>            snapshot classified
//
// where <base> is the version with "-SNAPSHOT" stripped, <TS> is
// yyyyMMdd.HHmmss, and <BN> a build number. A client may also deploy the literal
// "<a>-<v>-SNAPSHOT.<ext>" (non-unique snapshot); that is handled by the release
// branch (stem == version).
func fileCoordFromFilename(artifactID, version, filename string) (regmultiformat.FileCoord, bool) {
	prefix := artifactID + "-"
	if !strings.HasPrefix(filename, prefix) {
		return regmultiformat.FileCoord{}, false
	}
	rest := strings.TrimPrefix(filename, prefix)
	// Split extension (last dot; the version's own '.' separators stay in stem).
	dot := strings.LastIndex(rest, ".")
	if dot < 0 {
		return regmultiformat.FileCoord{}, false
	}
	ext := rest[dot+1:]
	stem := rest[:dot] // version[-classifier] or base-TS-BN[-classifier]

	if mavenver.IsSnapshot(version) {
		// Timestamped form: stem == "<base>-<TS>-<BN>[-<classifier>]" where
		// base = version with "-SNAPSHOT" stripped.
		base := version[:len(version)-len("-SNAPSHOT")]
		if strings.HasPrefix(stem, base+"-") {
			tail := strings.TrimPrefix(stem, base+"-") // "<TS>-<BN>[-<classifier>]"
			if ts, bn, after, ok := parseSnapshotStem(tail); ok {
				cls := strings.TrimPrefix(after, "-")
				return regmultiformat.FileCoord{Classifier: cls, Extension: ext, Timestamp: ts, BuildNumber: bn}, true
			}
		}
		// Fall through to the release branch for the literal "-SNAPSHOT" form.
	}
	// Release form: stem == version or version-classifier.
	if stem == version {
		return regmultiformat.FileCoord{Extension: ext}, true
	}
	if strings.HasPrefix(stem, version+"-") {
		return regmultiformat.FileCoord{Classifier: strings.TrimPrefix(stem, version+"-"), Extension: ext}, true
	}
	// Could not bind to the version dir; reject so we don't mis-store.
	return regmultiformat.FileCoord{}, false
}

// parseSnapshotStem parses "<yyyyMMdd>.<HHmmss>-<BN>[-<rest>]".
func parseSnapshotStem(stem string) (ts string, bn int, rest string, ok bool) {
	// TS = 8 digits, '.', 6 digits.
	if len(stem) < 16 || stem[8] != '.' {
		return "", 0, "", false
	}
	if !allDigits(stem[:8]) || !allDigits(stem[9:15]) {
		return "", 0, "", false
	}
	if stem[15] != '-' {
		return "", 0, "", false
	}
	ts = stem[:15]
	bnRest := stem[16:]
	// BN = leading digits.
	i := 0
	for i < len(bnRest) && bnRest[i] >= '0' && bnRest[i] <= '9' {
		i++
	}
	if i == 0 {
		return "", 0, "", false
	}
	bn, _ = strconv.Atoi(bnRest[:i])
	rest = bnRest[i:] // "" or "-<classifier>"
	return ts, bn, rest, true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ---- GET ----

func (h *handler) get(w http.ResponseWriter, r *http.Request, project, p string) {
	base, algo := chk(p)

	// maven-metadata.xml is ambiguous: GA-level (<g>/<a>/maven-metadata.xml) vs
	// GAV-level (<g>/<a>/<v>/maven-metadata.xml). Disambiguate by whether the
	// segment before the filename is a known version of the package.
	if strings.HasSuffix(base, "/maven-metadata.xml") {
		if g, a, v := h.classifyMetadataPath(r, project, base); v != "" {
			if h.getGAVMetadata(w, r, project, g, a, v, algo) {
				return
			}
		} else if g != "" {
			if h.getGAMetadata(w, r, project, g, a, algo) {
				return
			}
		}
		// Not (yet) resolvable natively - a proxy-cache project may still have it
		// upstream. Metadata is always fetched fresh (never cached as truth), so
		// this also covers "package known upstream but not cached locally yet".
		if h.proxyRaw(w, r, project, p, false) {
			return
		}
		http.NotFound(w, r)
		return
	}

	groupID, artifactID, version, filename, ok := parsePath(base)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.getFile(w, r, project, groupID, artifactID, version, filename, algo) {
		return
	}
	if h.proxyRaw(w, r, project, p, true) {
		return
	}
	http.NotFound(w, r)
}

// classifyMetadataPath resolves a .../maven-metadata.xml path to either a GA
// (returns g,a, v="") or a GAV (returns g,a,v). It first tries the GAV reading
// (segment before the filename is the version); if that version exists in the
// projection, it's GAV. Otherwise it falls back to the GA reading.
func (h *handler) classifyMetadataPath(r *http.Request, project, base string) (groupID, artifactID, version string) {
	if g, a, v, f, ok := parsePath(base); ok && f == "maven-metadata.xml" {
		name := gaName(g, a)
		if st, found, err := h.deps.State.LoadState(r.Context(), project, h.projectID(r), formatName, name); err == nil && found {
			for i := range st.Versions {
				if st.Versions[i].Version == v {
					return g, a, v
				}
			}
		}
	}
	if g, a, ok := parseGAMetadataPath(base); ok {
		return g, a, ""
	}
	return "", "", ""
}

// getFile serves a real file's exact bytes, or a checksum derived from them.
// Returns false (and writes nothing) when the file isn't found natively, so
// the caller can fall back to the upstream proxy before giving up.
func (h *handler) getFile(w http.ResponseWriter, r *http.Request, project, groupID, artifactID, version, filename, algo string) bool {
	name := gaName(groupID, artifactID)
	st, ok, err := h.deps.State.LoadState(r.Context(), project, h.projectID(r), formatName, name)
	if err != nil {
		h.serverError(w, r, err)
		return true
	}
	if !ok {
		return false
	}
	var fr *model.FileRef
	for vi := range st.Versions {
		if st.Versions[vi].Version != version {
			continue
		}
		for fi := range st.Versions[vi].Files {
			if st.Versions[vi].Files[fi].Filename == filename {
				fr = &st.Versions[vi].Files[fi]
				break
			}
		}
	}
	if fr == nil {
		return false
	}
	data, err := h.deps.MavenFile.MavenFileBlob(r.Context(), project, formatName, name, fr.Digest, fr.Size)
	if err != nil {
		h.serverError(w, r, err)
		return true
	}
	if algo != "" {
		writeChecksum(w, r, data, algo)
		return true
	}
	w.Header().Set("Content-Type", contentTypeFor(filename))
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		return true
	}
	_, _ = w.Write(data) // nolint:gosec // G705: response body is the requested artifact blob served to Maven clients, not an HTML sink
	return true
}

// ---- maven-metadata.xml model ----

type metadata struct {
	XMLName    xml.Name    `xml:"metadata"`
	GroupID    string      `xml:"groupId"`
	ArtifactID string      `xml:"artifactId"`
	Version    string      `xml:"version,omitempty"`
	Versioning *versioning `xml:"versioning"`
}

type versioning struct {
	Latest           string            `xml:"latest,omitempty"`
	Release          string            `xml:"release,omitempty"`
	Versions         *versionsList     `xml:"versions,omitempty"`
	Snapshot         *snapshot         `xml:"snapshot,omitempty"`
	SnapshotVersions *snapshotVersions `xml:"snapshotVersions,omitempty"`
	LastUpdated      string            `xml:"lastUpdated,omitempty"`
}

type versionsList struct {
	Version []string `xml:"version"`
}

type snapshot struct {
	Timestamp   string `xml:"timestamp,omitempty"`
	BuildNumber int    `xml:"buildNumber,omitempty"`
}

type snapshotVersions struct {
	SnapshotVersion []snapshotVersion `xml:"snapshotVersion"`
}

type snapshotVersion struct {
	Classifier string `xml:"classifier,omitempty"`
	Extension  string `xml:"extension"`
	Value      string `xml:"value"`
	Updated    string `xml:"updated"`
}

// ---- GA maven-metadata.xml synthesis ----

func (h *handler) getGAMetadata(w http.ResponseWriter, r *http.Request, project, groupID, artifactID, algo string) bool {
	name := gaName(groupID, artifactID)
	st, ok, err := h.deps.State.LoadState(r.Context(), project, h.projectID(r), formatName, name)
	if err != nil {
		h.serverError(w, r, err)
		return true
	}
	if !ok || len(st.Versions) == 0 {
		return false
	}
	xmlBytes, err := renderGAMetadata(groupID, artifactID, st)
	if err != nil {
		h.serverError(w, r, err)
		return true
	}
	if algo != "" {
		writeChecksum(w, r, xmlBytes, algo)
		return true
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write(xmlBytes) // nolint:gosec // G705: response body is synthesized maven-metadata.xml served to Maven clients, not an HTML sink
	return true
}

// renderGAMetadata synthesizes the GA maven-metadata.xml deterministically:
// versions in Aether order, <latest>=max, <release>=max non-SNAPSHOT,
// <lastUpdated> from the max version's Created (NOT time.Now()).
func renderGAMetadata(groupID, artifactID string, st model.PackageState) ([]byte, error) {
	vers := make([]string, 0, len(st.Versions))
	for _, v := range st.Versions {
		vers = append(vers, v.Version)
	}
	// Sort by Aether ordering, then DEDUP by comparator equality. Nexus stores
	// base versions in a TreeSet<Version> (GenericVersionScheme), which collapses
	// any versions that compareTo()==0 — so "1", "1.0", "1.0.0" become ONE
	// <version> entry. Sorting first makes equal-comparing versions adjacent; we
	// keep the lexicographically smallest raw string in each equal group so the
	// result is deterministic regardless of publish order.
	sort.Slice(vers, func(i, j int) bool {
		if c := mavenver.Compare(mavenver.Parse(vers[i]), mavenver.Parse(vers[j])); c != 0 {
			return c < 0
		}
		return vers[i] < vers[j]
	})
	vers = dedupByComparator(vers)
	latest := mavenver.Latest(vers)
	release := mavenver.Release(vers)
	// lastUpdated = max created across versions, deterministic.
	var maxCreated time.Time
	for _, v := range st.Versions {
		if v.Created.After(maxCreated) {
			maxCreated = v.Created
		}
	}
	md := metadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Versioning: &versioning{
			Latest:      latest,
			Release:     release,
			Versions:    &versionsList{Version: vers},
			LastUpdated: fmtTimestamp(maxCreated),
		},
	}
	return marshalMetadata(md)
}

// dedupByComparator collapses runs of versions that compare equal under Aether
// ordering (trailing-zero/qualifier equivalence: 1 == 1.0 == 1.0.0), keeping the
// first of each run. Input MUST already be sorted so equal-comparing versions are
// adjacent and the kept element is deterministic.
func dedupByComparator(sorted []string) []string {
	if len(sorted) == 0 {
		return sorted
	}
	out := sorted[:1]
	for _, v := range sorted[1:] {
		if mavenver.Compare(mavenver.Parse(out[len(out)-1]), mavenver.Parse(v)) != 0 {
			out = append(out, v)
		}
	}
	return out
}

// ---- GAV maven-metadata.xml (SNAPSHOT) synthesis ----

func (h *handler) getGAVMetadata(w http.ResponseWriter, r *http.Request, project, groupID, artifactID, version, algo string) bool {
	name := gaName(groupID, artifactID)
	st, ok, err := h.deps.State.LoadState(r.Context(), project, h.projectID(r), formatName, name)
	if err != nil {
		h.serverError(w, r, err)
		return true
	}
	if !ok {
		return false
	}
	var ver *model.Version
	for vi := range st.Versions {
		if st.Versions[vi].Version == version {
			ver = &st.Versions[vi]
			break
		}
	}
	if ver == nil || !mavenver.IsSnapshot(version) {
		// GAV metadata is only meaningful for SNAPSHOT versions.
		return false
	}
	xmlBytes, err := renderGAVMetadata(groupID, artifactID, *ver)
	if err != nil {
		h.serverError(w, r, err)
		return true
	}
	if algo != "" {
		writeChecksum(w, r, xmlBytes, algo)
		return true
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write(xmlBytes) // nolint:gosec // G705: response body is synthesized maven-metadata.xml served to Maven clients, not an HTML sink
	return true
}

// renderGAVMetadata synthesizes the SNAPSHOT GAV maven-metadata.xml: the
// <snapshot> (latest timestamp/buildNumber) and <snapshotVersions> entries the
// client reads to resolve the timestamped filename. Derived entirely from the
// version's file set.
func renderGAVMetadata(groupID, artifactID string, v model.Version) ([]byte, error) {
	// Find the highest buildNumber and its timestamp.
	maxBN := 0
	maxTS := ""
	for _, f := range v.Files {
		if f.BuildNumber > maxBN {
			maxBN = f.BuildNumber
			maxTS = f.Timestamp
		}
	}
	// snapshotVersions: one entry per (classifier,extension) at the LATEST build.
	type key struct{ cls, ext string }
	latest := map[key]model.FileRef{}
	for _, f := range v.Files {
		if f.BuildNumber != maxBN {
			continue
		}
		latest[key{f.Classifier, f.Extension}] = f
	}
	keys := make([]key, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].cls != keys[j].cls {
			return keys[i].cls < keys[j].cls
		}
		return keys[i].ext < keys[j].ext
	})
	updated := strings.ReplaceAll(maxTS, ".", "") // yyyyMMddHHmmss
	var svs []snapshotVersion
	for _, k := range keys {
		f := latest[k]
		// value = "<baseVersion stripped of -SNAPSHOT>-<TS>-<BN>"
		baseVer := strings.TrimSuffix(v.Version, "-SNAPSHOT")
		val := fmt.Sprintf("%s-%s-%d", baseVer, f.Timestamp, f.BuildNumber)
		svs = append(svs, snapshotVersion{
			Classifier: f.Classifier,
			Extension:  f.Extension,
			Value:      val,
			Updated:    updated,
		})
	}
	md := metadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    v.Version,
		Versioning: &versioning{
			Snapshot:         &snapshot{Timestamp: maxTS, BuildNumber: maxBN},
			SnapshotVersions: &snapshotVersions{SnapshotVersion: svs},
			LastUpdated:      updated,
		},
	}
	return marshalMetadata(md)
}

// ---- proxy-cache fallback (maven proxy-cache projects) ----
//
// Native lookups above always run first; proxyRaw only runs on a native miss,
// so a file published directly also shadows an upstream file of the same
// coordinate. Real GAV files (cacheable=true) are best-effort mirrored into
// native storage via a synthetic PublishFile so the next request is served
// locally; maven-metadata.xml (cacheable=false) is never cached as truth,
// matching the native synthesis invariant above - it is always re-fetched.

func (h *handler) proxyRaw(w http.ResponseWriter, r *http.Request, project, p string, cacheable bool) bool {
	proxy, err := pkgproxy.ForProject(r.Context(), project, regmodel.RegistryTypeMaven)
	if err != nil || proxy == nil || proxy.Registry == nil {
		return false
	}
	resp, err := proxy.Get(r.Context(), p, nil)
	if err != nil {
		return false
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = contentTypeForProxy(p)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(resp.Body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return true
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Body)

	if cacheable {
		go h.cacheProxiedFile(orm.Copy(context.Background()), h.projectID(r), project, p, resp.Body)
	}
	return true
}

// cacheProxiedFile mirrors one proxied GAV file into native storage. Best
// effort: a cache miss here only costs an extra upstream round trip on the
// next request, never surfaced to the client that already got its response.
func (h *handler) cacheProxiedFile(ctx context.Context, projectID int64, project, p string, payload []byte) {
	base, algo := chk(p)
	if algo != "" || strings.HasSuffix(base, "/maven-metadata.xml") {
		return // checksums are derived, not stored; metadata is never cached as truth
	}
	groupID, artifactID, version, filename, ok := parsePath(base)
	if !ok {
		return
	}
	coord, ok := fileCoordFromFilename(artifactID, version, filename)
	if !ok {
		return
	}
	ev := regmultiformat.FilePublishEvent{
		Format:          formatName,
		ProjectID:       projectID,
		Name:            gaName(groupID, artifactID),
		Version:         version,
		File:            coord,
		Filename:        filename,
		Payload:         payload,
		SnapshotMutable: mavenver.IsSnapshot(version),
	}
	if _, err := h.deps.FilePublisher.PublishFile(ctx, project, ev); err != nil && !errors.IsErr(err, errors.ConflictCode) {
		log.Warningf("maven: cache proxied file %s/%s: %v", project, p, err)
	}
}

// ---- PUT ----

func (h *handler) put(w http.ResponseWriter, r *http.Request, project, p string) {
	base, algo := chk(p)

	// PUT of a checksum sidecar: accept + discard (client computed it; we derive
	// our own on GET).
	if algo != "" {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		return
	}

	// PUT of GA or GAV maven-metadata.xml: accept + discard (we always synthesize).
	if _, _, ok := parseGAMetadataPath(base); ok {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		return
	}
	groupID, artifactID, version, filename, ok := parsePath(base)
	if !ok {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	if filename == "maven-metadata.xml" {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Real file PUT: store the exact bytes as one layer of the GAV manifest.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<30))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	coord, ok := fileCoordFromFilename(artifactID, version, filename)
	if !ok {
		http.Error(w, "filename does not match GAV", http.StatusBadRequest)
		return
	}
	name := gaName(groupID, artifactID)
	ev := regmultiformat.FilePublishEvent{
		Format:          formatName,
		ProjectID:       h.projectID(r),
		Name:            name,
		Version:         version,
		File:            coord,
		Filename:        filename,
		Payload:         body,
		SnapshotMutable: mavenver.IsSnapshot(version),
	}
	if _, err := h.deps.FilePublisher.PublishFile(r.Context(), project, ev); err != nil {
		if errors.IsErr(err, errors.ConflictCode) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		h.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// ---- checksums + helpers ----

// projectID reads the resolved project id stashed by multiformatauth on the request
// context.
func (h *handler) projectID(r *http.Request) int64 {
	return regmultiformat.ProjectIDFromContext(r.Context())
}

// serverError logs the underlying cause and returns 500 without leaking
// internals to the client beyond the error message.
func (h *handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	log.Errorf("maven adapter: %s %s: %v", r.Method, r.URL.Path, err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// writeChecksum derives the requested checksum over the EXACT served bytes and
// writes it as the hex digest (Maven sidecar format: bare hex, no filename).
func writeChecksum(w http.ResponseWriter, r *http.Request, data []byte, algo string) {
	var hh hash.Hash
	switch algo {
	case "sha1":
		hh = sha1.New() // nolint:gosec // sha1 is mandated by the Maven repository checksum protocol
	case "md5":
		hh = md5.New() // nolint:gosec // md5 is mandated by the Maven repository checksum protocol
	case "sha256":
		hh = sha256.New()
	case "sha512":
		hh = sha512.New()
	default:
		http.NotFound(w, r)
		return
	}
	hh.Write(data)
	sum := hex.EncodeToString(hh.Sum(nil))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(sum)))
		return
	}
	_, _ = io.WriteString(w, sum)
}

func marshalMetadata(md metadata) ([]byte, error) {
	body, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(body, '\n')...), nil
}

func fmtTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("20060102150405")
}

func contentTypeFor(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".pom"), strings.HasSuffix(filename, ".xml"):
		return "text/xml; charset=utf-8"
	case strings.HasSuffix(filename, ".jar"), strings.HasSuffix(filename, ".war"):
		return "application/java-archive"
	default:
		return "application/octet-stream"
	}
}

func contentTypeForProxy(path string) string {
	return contentTypeFor(path)
}
