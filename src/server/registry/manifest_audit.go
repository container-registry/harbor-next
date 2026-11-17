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

package registry

import (
	"mime"
	"net/http"
	"time"

	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/controller/event/handler/auditlog"
	"github.com/goharbor/harbor/src/controller/event/operator"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/auditext/collapse"
	securitymiddleware "github.com/goharbor/harbor/src/server/middleware/security"
)

const (
	// pullWindowTTL bounds how long a tag HEAD or an index GET shapes the
	// audit of the manifest requests that follow it in the same pull.
	pullWindowTTL = time.Minute
	// pullWindowMaxEntries caps the memory the window may hold.
	pullWindowMaxEntries = 50000
)

// pulls collapses one client pull into one audit entry.
var pulls = collapse.NewWindow(pullWindowTTL, pullWindowMaxEntries)

// auditPull decides whether a successful manifest request fires a pull event.
// Docker and containerd resolve a tag with HEAD and then GET the digest, so
// the tag from that HEAD is handed back for the GET. A platform manifest
// fetched after its index GET still fires the event (webhooks, pull time),
// but marks the request so the audit handler does not log it twice.
func auditPull(req *http.Request, art *artifact.Artifact, reference string, body []byte) (tag string, audit bool) {
	if req.UserAgent() == common.UserAgent {
		return "", false
	}
	if auditlog.IsScannerPrincipal(req.Context()) {
		collapse.SkipPullAudit(req.Context())
	}
	client := operator.FromContext(req.Context()) + "|" + securitymiddleware.GetClientIP(req) + "|" + art.RepositoryName
	_, byDigest := digest.Parse(reference)
	byTag := byDigest != nil

	if req.Method == http.MethodHead {
		if byTag {
			pulls.Put(client+"|tag|"+art.Digest, reference)
		}
		return "", false
	}
	if !byTag {
		if _, ok := pulls.Get(client + "|child|" + art.Digest); ok {
			collapse.SkipPullAudit(req.Context())
		}
		// the hint belongs to the one GET that follows its HEAD
		tag, _ = pulls.Take(client + "|tag|" + art.Digest)
	}
	if art.IsImageIndex() {
		for _, child := range collapse.IndexChildren(body) {
			pulls.Put(client+"|child|"+child, "")
		}
	}
	return tag, true
}

// readIndexBody buffers the body of an index push, so the platform manifests
// it lists can be audited as part of it; other manifests stream untouched.
func readIndexBody(req *http.Request) ([]byte, error) {
	mediaType, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if mediaType != v1.MediaTypeImageIndex && mediaType != manifestlist.MediaTypeManifestList {
		return nil, nil
	}
	return lib.ReadRequestBody(req, common.MaxManifestBodySize)
}

// markIndexMembers records the index's manifests once the push is committed.
func markIndexMembers(req *http.Request, repo string, index []byte) {
	operatorName := operator.FromContext(req.Context())
	orm.AfterCommit(req.Context(), func() { collapse.MarkIndexMembers(repo, operatorName, index) })
}
