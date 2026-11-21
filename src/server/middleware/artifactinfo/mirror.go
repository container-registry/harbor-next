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

package artifactinfo

import (
	"net/http"
	"strings"

	"github.com/goharbor/harbor/src/controller/mirror"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/log"
)

// routableRequest reports whether a request is one that only a pull makes.
//
// A push opens with a HEAD of every blob it is about to upload, which is
// indistinguishable from a pull's blob HEAD, so blob HEADs are left alone: they
// are answered from the project the client named, and a pull that finds nothing
// there still fetches the blob with the GET that follows.
func routableRequest(req *http.Request) bool {
	path := req.URL.Path
	switch {
	case lib.V2ManifestURLRe.MatchString(path):
		return req.Method == http.MethodGet || req.Method == http.MethodHead
	case lib.V2BlobURLRe.MatchString(path):
		return req.Method == http.MethodGet
	case lib.V2TagListURLRe.MatchString(path):
		return req.Method == http.MethodGet
	}
	return false
}

// rewriteMirrorRepository maps a pull that arrives at the root of the registry
// API onto the proxy cache project that mirrors the upstream it came from, and
// rewrites the request URL in place so routing, authorization and the proxy
// cache all see the repository the pull is actually served from.
func rewriteMirrorRepository(req *http.Request, repo string) (string, bool) {
	ctx := req.Context()
	if !mirror.Enabled(ctx) || !routableRequest(req) {
		return repo, false
	}
	ns := req.URL.Query().Get(mirror.NamespaceQuery)
	rewritten, ok := mirror.Route(ctx, repo, ns)
	if !ok {
		return repo, false
	}
	rest, ok := strings.CutPrefix(req.URL.Path, "/v2/"+repo+"/")
	if !ok {
		return repo, false
	}
	req.URL.Path = "/v2/" + rewritten + "/" + rest
	log.G(ctx).Debugf("mirror routing: %q (ns %q) served from %q", repo, ns, rewritten)
	return rewritten, true
}
