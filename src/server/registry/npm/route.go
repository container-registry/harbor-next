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

// Package npm implements the npm registry native protocol over Harbor's OCI
// backend (multiformat), with a fallback to an upstream npm registry for projects
// configured as an npm proxy-cache (see the pkgproxy fallback in handler.go).
// Wire contract verified empirically against npm 10.9.2: publish is
// PUT /<project>/<pkg> with a CouchDB-style body (_id, name, dist-tags,
// versions, _attachments base64 tarball); the packument GET returns name,
// dist-tags, versions{}, time{}; the per-version dist carries shasum (sha1
// hex) + integrity (sha512 SRI) + a server-minted tarball URL.
//
// Version ordering = node-semver. npm `latest` is the MUTABLE dist-tag the
// publisher set, never computed as max.
//
// Harbor URL scheme prefixes every npm route with /npm/<project>/. The project
// is the first path segment after the prefix; multiformatauth resolves + authorizes it
// upstream and stashes the project id on the request context. Scoped names
// "@scope/name" arrive percent-decoded by net/http into a literal '/', so the
// handler is a catch-all that reconstructs the package name from the remaining
// segments rather than relying on segment count.
package npm

import (
	"context"
	"net/http"

	multiformat "github.com/goharbor/harbor/src/controller/multiformat"
	server "github.com/goharbor/harbor/src/server/registry/multiformat"
	"github.com/goharbor/harbor/src/server/registry/pkgpolicy"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

// Prefix is the URL prefix npm clients point their registry at:
// http://harbor/npm/<project>/.
const Prefix = "/npm"

// WithProjectID returns a context carrying the resolved project id. multiformatauth
// calls this after RBAC succeeds. It delegates to the shared multiformat context key
// so npm, maven, and multiformatauth all agree on a single key.
func WithProjectID(ctx context.Context, id int64) context.Context {
	return server.WithProjectID(ctx, id)
}

// projectIDFromContext reads the project id multiformatauth stashed. Absent (0) means
// the middleware did not run, which only happens in direct unit tests.
func projectIDFromContext(ctx context.Context) int64 {
	return server.ProjectIDFromContext(ctx)
}

// Register builds the npm Deps from the multiformat controller and mounts the npm
// catch-all handler on the given mux. baseURL is the external URL prefix
// (scheme + host, no trailing slash) used to render absolute tarball URLs;
// the per-project segment is appended at render time.
func Register(mux *http.ServeMux, ctl multiformat.Controller, baseURL string) {
	deps := server.NewDeps(ctl, baseURL)
	h := &handler{deps: deps, authorizePush: pkgpolicy.AuthorizePush, resolveProxy: pkgproxy.ForProject}
	// StripPrefix removes "/npm"; the handler then peels the leading
	// "/<project>" segment itself. The trailing slash makes this a subtree
	// (catch-all) match so scoped names and tarball paths all land here.
	mux.Handle(Prefix+"/", http.StripPrefix(Prefix, h))
}

// handler dispatches every npm route by method + path shape, falling back to
// an upstream npm registry (pkgproxy) for proxy-cache projects on a native miss.
type handler struct {
	deps          server.Deps
	authorizePush pushAuthorizer
	resolveProxy  proxyResolver
}

type pushAuthorizer func(context.Context, string, string) error
type proxyResolver func(context.Context, string, string) (*pkgproxy.Proxy, error)
