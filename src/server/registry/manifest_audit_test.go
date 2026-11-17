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
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/security"
	robotsec "github.com/goharbor/harbor/src/common/security/robot"
	robotctl "github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/auditext/collapse"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	robotModel "github.com/goharbor/harbor/src/pkg/robot/model"
)

const (
	indexDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	amd64Digest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	otherDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

func manifestRequest(method, repo, reference, remote string) *http.Request {
	req := httptest.NewRequest(method, "/v2/"+repo+"/manifests/"+reference, nil)
	req.RemoteAddr = remote
	return req
}

func withMarks(req *http.Request) *http.Request {
	return req.WithContext(collapse.NewRequestMarks().Into(req.Context()))
}

func TestAuditPullDockerFlow(t *testing.T) {
	index := &artifact.Artifact{RepositoryName: "lib/app", Digest: indexDigest, ManifestMediaType: "application/vnd.oci.image.index.v1+json"}
	child := &artifact.Artifact{RepositoryName: "lib/app", Digest: amd64Digest, ManifestMediaType: "application/vnd.oci.image.manifest.v1+json"}
	body := []byte(`{"manifests":[{"digest":"` + amd64Digest + `"}]}`)

	// HEAD by tag is never audited but remembers the tag
	_, audit := auditPull(manifestRequest(http.MethodHead, "lib/app", "1.0", "10.0.0.1:4000"), index, "1.0", nil)
	assert.False(t, audit)

	// GET of the index by digest is audited under the tag, from another connection
	tag, audit := auditPull(manifestRequest(http.MethodGet, "lib/app", indexDigest, "10.0.0.1:4001"), index, indexDigest, body)
	assert.True(t, audit)
	assert.Equal(t, "1.0", tag)

	// the platform manifest still fires its event, but is marked as part of that pull
	req := withMarks(manifestRequest(http.MethodGet, "lib/app", amd64Digest, "10.0.0.1:4002"))
	_, audit = auditPull(req, child, amd64Digest, nil)
	assert.True(t, audit)
	assert.True(t, collapse.PullAuditSkipped(req.Context()))

	// another client pulling the same platform manifest is audited
	req = withMarks(manifestRequest(http.MethodGet, "lib/app", amd64Digest, "10.0.0.2:4000"))
	_, audit = auditPull(req, child, amd64Digest, nil)
	assert.True(t, audit)
	assert.False(t, collapse.PullAuditSkipped(req.Context()))
}

func TestAuditPullUsesATagHintOnce(t *testing.T) {
	art := &artifact.Artifact{RepositoryName: "lib/once", Digest: otherDigest, ManifestMediaType: "application/vnd.oci.image.manifest.v1+json"}
	_, _ = auditPull(manifestRequest(http.MethodHead, "lib/once", "2.0", "10.0.0.3:4000"), art, "2.0", nil)
	tag, _ := auditPull(manifestRequest(http.MethodGet, "lib/once", otherDigest, "10.0.0.3:4001"), art, otherDigest, nil)
	assert.Equal(t, "2.0", tag)
	// a later pull by digest alone is not the tag pull
	tag, audit := auditPull(manifestRequest(http.MethodGet, "lib/once", otherDigest, "10.0.0.3:4002"), art, otherDigest, nil)
	assert.True(t, audit)
	assert.Empty(t, tag)
}

func TestAuditPullByTagAndReplication(t *testing.T) {
	art := &artifact.Artifact{RepositoryName: "lib/single", Digest: otherDigest, ManifestMediaType: "application/vnd.oci.image.manifest.v1+json"}
	tag, audit := auditPull(manifestRequest(http.MethodGet, "lib/single", "2.0", "10.0.0.3:1"), art, "2.0", nil)
	assert.True(t, audit)
	assert.Empty(t, tag, "a pull by tag carries its tag already")

	req := manifestRequest(http.MethodGet, "lib/single", "2.0", "10.0.0.3:1")
	req.Header.Set("User-Agent", common.UserAgent)
	_, audit = auditPull(req, art, "2.0", nil)
	assert.False(t, audit)
}

func TestReadIndexBody(t *testing.T) {
	body := `{"manifests":[{"digest":"` + amd64Digest + `"}]}`
	req := httptest.NewRequest(http.MethodPut, "/v2/lib/app/manifests/1.0", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/vnd.oci.image.index.v1+json")
	got, err := readIndexBody(req)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	rest, _ := io.ReadAll(req.Body)
	assert.Equal(t, body, string(rest), "the body is still there for the registry")

	req = httptest.NewRequest(http.MethodPut, "/v2/lib/app/manifests/1.0", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	got, err = readIndexBody(req)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuditPullSkipsTheScanRobotOnly(t *testing.T) {
	config.InitWithSettings(map[string]any{common.RobotScannerNamePrefix: "scanner", common.RobotNamePrefix: "robot$"})
	art := &artifact.Artifact{RepositoryName: "lib/scan", Digest: otherDigest, ManifestMediaType: "application/vnd.oci.image.manifest.v1+json"}
	pull := func(r *robotctl.Robot) bool {
		req := withMarks(manifestRequest(http.MethodGet, "lib/scan", otherDigest, "10.0.0.9:1"))
		req = req.WithContext(security.NewContext(req.Context(), robotsec.NewSecurityContext(r)))
		_, audit := auditPull(req, art, otherDigest, nil)
		assert.True(t, audit, "the pull event is always fired")
		return collapse.PullAuditSkipped(req.Context())
	}
	assert.True(t, pull(&robotctl.Robot{Robot: robotModel.Robot{Name: "lib+scanner-Trivy-0d3c"}}))
	assert.False(t, pull(&robotctl.Robot{Robot: robotModel.Robot{Name: "lib+scanner-ci", Visible: true}}))
}
