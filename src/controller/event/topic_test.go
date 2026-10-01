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

package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/artifact"
)

func testArtifactEvent() *ArtifactEvent {
	return &ArtifactEvent{
		Repository:    "library/hello-world",
		Artifact:      &artifact.Artifact{ProjectID: 1, RepositoryName: "library/hello-world", Digest: "sha256:abc"},
		Tags:          []string{"latest"},
		Operator:      "admin",
		OccurAt:       time.Now(),
		ClientAddress: "203.0.113.10",
		UserAgent:     "docker/24.0.0",
	}
}

func TestPushArtifactEvent_ResolveToAuditLog_CarriesClientInfo(t *testing.T) {
	e := &PushArtifactEvent{ArtifactEvent: testArtifactEvent()}
	auditLog, err := e.ResolveToAuditLog()
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.10", auditLog.ClientAddress)
	assert.Equal(t, "docker/24.0.0", auditLog.UserAgent)
}

func TestPullArtifactEvent_ResolveToAuditLog_CarriesClientInfo(t *testing.T) {
	e := &PullArtifactEvent{ArtifactEvent: testArtifactEvent()}
	auditLog, err := e.ResolveToAuditLog()
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.10", auditLog.ClientAddress)
	assert.Equal(t, "docker/24.0.0", auditLog.UserAgent)
}

func TestDeleteArtifactEvent_ResolveToAuditLog_CarriesClientInfo(t *testing.T) {
	e := &DeleteArtifactEvent{ArtifactEvent: testArtifactEvent()}
	auditLog, err := e.ResolveToAuditLog()
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.10", auditLog.ClientAddress)
	assert.Equal(t, "docker/24.0.0", auditLog.UserAgent)
}
