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

// Package mirror routes pulls that arrive at the root of the registry API onto
// the proxy cache project that mirrors the upstream they came from, so a client
// configured with Harbor as a registry mirror can pull "library/nginx" without
// naming a Harbor project.
package mirror

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"unicode"

	"github.com/goharbor/harbor/src/controller/repository"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/project"
)

// NamespaceQuery is the query parameter containerd appends to a request when
// the host it talks to is configured as a mirror of another registry, see
// https://github.com/containerd/containerd/blob/main/docs/hosts.md
const NamespaceQuery = "ns"

// catchAllNamespace is the key in the namespace mapping that serves clients
// which send no namespace hint at all, such as the docker daemon's
// registry-mirrors.
const catchAllNamespace = "*"

// repoExists reports whether a repository is already known to Harbor. It is a
// variable so tests can drive the routing without a database.
var repoExists = func(ctx context.Context, name string) bool {
	_, err := repository.Ctl.GetByName(ctx, name)
	if err == nil {
		return true
	}
	if !errors.IsNotFoundErr(err) {
		// Unknown is treated as present so a lookup failure never swaps a
		// local repository for upstream content.
		log.G(ctx).Errorf("mirror: failed to look up repository %s: %v", name, err)
		return true
	}
	return false
}

// Enabled reports whether mirror routing is turned on.
func Enabled(ctx context.Context) bool {
	return config.RegistryMirrorEnabled(ctx)
}

// Route returns the repository that serves a mirror pull of repo, and whether
// routing applies at all.
//
// A client that names the upstream it is mirroring (containerd sends ?ns=) is
// taken at its word. A client that does not (the docker daemon's
// registry-mirrors) is served locally whenever the repository already exists in
// Harbor, and routed to the mirror otherwise: a local project and an upstream
// namespace can share a name, "library" being the one every Harbor has.
func Route(ctx context.Context, repo, ns string) (string, bool) {
	if !Enabled(ctx) {
		return repo, false
	}
	target := targetFor(ctx, ns)
	if target == "" {
		return repo, false
	}
	// The client already addresses the proxy project, e.g. containerd
	// configured with a path prefixed mirror endpoint.
	if repo == target || strings.HasPrefix(repo, target+"/") {
		return repo, false
	}
	if ns == "" && repoExists(ctx, repo) {
		return repo, false
	}
	return target + "/" + repo, true
}

// targetFor returns the proxy cache project that serves ns, honouring the
// catch-all entry when ns has no mapping of its own.
func targetFor(ctx context.Context, ns string) string {
	mapping := ParseNamespaces(config.RegistryMirrorNamespaces(ctx))
	if p, ok := mapping[ns]; ok && ns != "" {
		return p
	}
	return mapping[catchAllNamespace]
}

// ParseNamespaces reads the "<namespace>=<project>,..." mirror mapping. Entries
// that name no project are dropped, so a half-written setting degrades to "no
// mirror route" instead of routing pulls somewhere unintended.
func ParseNamespaces(raw string) map[string]string {
	mapping := make(map[string]string)
	for e := range entries(raw) {
		if e.ok {
			mapping[e.ns] = e.project
		}
	}
	return mapping
}

// ValidateNamespaces is the strict reading of the mapping used when it is
// saved: every entry must be <namespace>=<project>, a namespace may appear
// once and carry no whitespace, and the project name must be legal. It
// returns the mapping that ParseNamespaces will route with.
func ValidateNamespaces(raw string) (map[string]string, error) {
	mapping := make(map[string]string)
	for e := range entries(raw) {
		if !e.ok {
			return nil, fmt.Errorf("entry %q is not in the <namespace>=<project> form", e.raw)
		}
		if strings.ContainsFunc(e.ns, unicode.IsSpace) {
			return nil, fmt.Errorf("entry %q has whitespace in its namespace", e.raw)
		}
		if _, dup := mapping[e.ns]; dup {
			return nil, fmt.Errorf("namespace %q is mapped more than once", e.ns)
		}
		if !project.IsValidName(e.project) {
			return nil, fmt.Errorf("entry %q does not name a valid project", e.raw)
		}
		mapping[e.ns] = e.project
	}
	return mapping, nil
}

type entry struct {
	raw, ns, project string
	ok               bool
}

// entries yields the non-empty comma separated entries of a mapping.
func entries(raw string) iter.Seq[entry] {
	return func(yield func(entry) bool) {
		for item := range strings.SplitSeq(raw, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			ns, name, found := strings.Cut(item, "=")
			ns, name = strings.TrimSpace(ns), strings.TrimSpace(name)
			if !yield(entry{raw: item, ns: ns, project: name, ok: found && ns != "" && name != ""}) {
				return
			}
		}
	}
}
