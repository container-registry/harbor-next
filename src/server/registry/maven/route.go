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

// Package maven implements the Maven repository HTTP protocol (Wagon) over
// Harbor's multiformat OCI backend. Ported from the multi-format-oci POC
// (internal/format/maven). Wire contract from the Maven repository layout
// (maven.apache.org/repository/layout.html) + the Maven metadata reference.
//
// There is NO publish-body protocol: `mvn deploy` PUTs each file individually,
// and computes+uploads maven-metadata.xml AND .sha1/.md5 sidecars itself. The
// adapter therefore:
//   - on GET of a real file: stream the exact stored layer bytes;
//   - on GET of .sha1/.md5/.sha256/.sha512: derive the checksum from the EXACT
//     bytes we serve (never the client's uploaded sidecar);
//   - on GET of maven-metadata.xml (GA or GAV): ALWAYS synthesize from state;
//   - on PUT of maven-metadata.xml or any checksum sidecar: accept + DISCARD
//     (return 200, never store as truth);
//   - on PUT of a real file: store its exact bytes as one layer of the GAV
//     version manifest (controller PublishFile seam).
//
// Project = the FIRST path segment after /maven (Harbor project name); the
// project id is resolved + authorized upstream in multiformatauth and read from the
// request context.
package maven

import (
	"context"
	"net/http"
	"strings"

	"github.com/goharbor/harbor/src/controller/multiformat"
	regmodel "github.com/goharbor/harbor/src/pkg/reg/model"
	regmultiformat "github.com/goharbor/harbor/src/server/registry/multiformat"
	"github.com/goharbor/harbor/src/server/registry/pkgpolicy"
)

// formatName is the native format identifier.
const formatName = "maven"

// checksum suffixes the client may append to any path; derived on the fly.
var checksumExts = []string{".sha1", ".md5", ".sha256", ".sha512"}

// Adapter is the Maven Format.
type Adapter struct{}

// New returns the Maven adapter.
func New() *Adapter { return &Adapter{} }

// Name implements regmultiformat.Format.
func (a *Adapter) Name() string { return formatName }

// Mount registers the Maven routes under /maven.
func (a *Adapter) Mount(mux *http.ServeMux, deps regmultiformat.Deps) {
	h := &handler{deps: deps, authorizePush: pkgpolicy.AuthorizePush}
	mux.Handle("/maven/", http.StripPrefix("/maven", h))
}

// Register builds the adapter's Deps from the multiformat controller and mounts its
// routes on the given mux. baseURL is the external URL prefix.
func Register(mux *http.ServeMux, ctl multiformat.Controller, baseURL string) {
	New().Mount(mux, regmultiformat.NewDeps(ctl, baseURL))
}

type handler struct {
	deps          regmultiformat.Deps
	authorizePush pushAuthorizer
}

type pushAuthorizer func(context.Context, string, string) error

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	project, rest, ok := splitProject(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		h.get(w, r, project, rest)
	case http.MethodPut:
		if !h.allowPublish(w, r, project) {
			return
		}
		h.put(w, r, project, rest)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handler) allowPublish(w http.ResponseWriter, r *http.Request, project string) bool {
	if h.authorizePush == nil {
		return true
	}
	if err := h.authorizePush(r.Context(), project, regmodel.RegistryTypeMaven); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false
	}
	return true
}

// splitProject peels the leading <project> segment off the post-/maven path and
// returns the project name plus the remaining repo path (no leading slash).
func splitProject(urlPath string) (project, rest string, ok bool) {
	p := strings.TrimPrefix(urlPath, "/") // path after /maven, no leading slash
	i := strings.IndexByte(p, '/')
	if i <= 0 {
		return "", "", false
	}
	return p[:i], p[i+1:], true
}

// chk strips a known checksum suffix, returning the base path and the algorithm
// ("" if none).
func chk(p string) (base, algo string) {
	for _, ext := range checksumExts {
		if strings.HasSuffix(p, ext) {
			return strings.TrimSuffix(p, ext), strings.TrimPrefix(ext, ".")
		}
	}
	return p, ""
}

// parsePath splits a Maven repo path into (groupId, artifactId, version,
// filename). The grammar is .../<g>/<a>/<v>/<filename>; groupId is the
// slash-joined prefix dot-joined. ok==false for metadata-at-GA-level (no version
// dir) which is handled separately.
func parsePath(p string) (groupID, artifactID, version, filename string, ok bool) {
	segs := strings.Split(p, "/")
	if len(segs) < 4 {
		return "", "", "", "", false
	}
	filename = segs[len(segs)-1]
	version = segs[len(segs)-2]
	artifactID = segs[len(segs)-3]
	groupID = strings.Join(segs[:len(segs)-3], ".")
	if groupID == "" || artifactID == "" || version == "" || filename == "" {
		return "", "", "", "", false
	}
	return groupID, artifactID, version, filename, true
}

// parseGAMetadataPath handles the GA-level maven-metadata.xml path:
// .../<g>/<a>/maven-metadata.xml. Returns ok only when the last segment is
// maven-metadata.xml and there are >=2 segments before it.
func parseGAMetadataPath(p string) (groupID, artifactID string, ok bool) {
	if !strings.HasSuffix(p, "/maven-metadata.xml") {
		return "", "", false
	}
	segs := strings.Split(strings.TrimSuffix(p, "/maven-metadata.xml"), "/")
	if len(segs) < 2 {
		return "", "", false
	}
	artifactID = segs[len(segs)-1]
	groupID = strings.Join(segs[:len(segs)-1], ".")
	if groupID == "" || artifactID == "" {
		return "", "", false
	}
	return groupID, artifactID, true
}

func gaName(groupID, artifactID string) string { return groupID + ":" + artifactID }
