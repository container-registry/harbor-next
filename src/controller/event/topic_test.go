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
	"github.com/goharbor/harbor/src/pkg/auditext/model"
	robotModel "github.com/goharbor/harbor/src/pkg/robot/model"
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

// Every notifier-path row keeps its columns and gains the template line.
func TestNotifierEvents_Descriptions(t *testing.T) {
	at := time.Unix(1700000000, 0)
	art := &artifact.Artifact{ID: 7, ProjectID: 1, RepositoryName: "lib/nginx", Digest: "sha256:7f3a", Type: "IMAGE", Size: 44040192}
	robot := &robotModel.Robot{ID: 31, Name: "robot$ci", ProjectID: 0, Secret: "s3cr3t"}
	projRobot := &robotModel.Robot{ID: 32, Name: "robot$myproj+ci", ProjectID: 5, Secret: "s3cr3t"}
	tests := []struct {
		name  string
		event interface {
			ResolveToAuditLog() (*model.AuditLogExt, error)
		}
		operation string
		resource  string
		username  string
		projectID int64
		desc      string
	}{
		{"push by tag", &PushArtifactEvent{ArtifactEvent: &ArtifactEvent{Artifact: art, Tags: []string{"1.25", "latest"}, Operator: "robot$ci+build", OccurAt: at}},
			"create", "lib/nginx:1.25", "robot$ci+build", 1, `robot$ci+build pushed artifact "lib/nginx:1.25": digest "sha256:7f3a", type "IMAGE", size 44040192`},
		{"push by digest", &PushArtifactEvent{ArtifactEvent: &ArtifactEvent{Artifact: art, Operator: "alice", OccurAt: at}},
			"create", "lib/nginx@sha256:7f3a", "alice", 1, `alice pushed artifact "lib/nginx@sha256:7f3a": digest "sha256:7f3a", type "IMAGE", size 44040192`},
		{"pull", &PullArtifactEvent{ArtifactEvent: &ArtifactEvent{Artifact: art, Tags: []string{"1.25"}, Operator: "robot$proj+ci", OccurAt: at}},
			"pull", "lib/nginx:1.25", "robot$proj+ci", 1, `robot$proj+ci pulled artifact "lib/nginx:1.25": digest "sha256:7f3a"`},
		{"anonymous pull", &PullArtifactEvent{ArtifactEvent: &ArtifactEvent{Artifact: art, Operator: "", OccurAt: at}},
			"pull", "lib/nginx@sha256:7f3a", "anonymous", 1, `anonymous pulled artifact "lib/nginx@sha256:7f3a": digest "sha256:7f3a"`},
		{"delete artifact", &DeleteArtifactEvent{ArtifactEvent: &ArtifactEvent{Artifact: art, Tags: []string{"1.25", "latest"}, Labels: []string{"prod"}, Operator: "alice", OccurAt: at}},
			"delete", "lib/nginx@sha256:7f3a", "alice", 1, `alice deleted artifact "lib/nginx@sha256:7f3a": tags ["1.25","latest"], labels ["prod"], size 44040192`},
		{"delete untagged artifact", &DeleteArtifactEvent{ArtifactEvent: &ArtifactEvent{Artifact: art, Operator: "harbor-jobservice", OccurAt: at}},
			"delete", "lib/nginx@sha256:7f3a", "harbor-jobservice", 1, `harbor-jobservice deleted artifact "lib/nginx@sha256:7f3a": size 44040192`},
		{"delete repository", &DeleteRepositoryEvent{ProjectID: 1, Repository: "lib/nginx", Operator: "alice", OccurAt: at},
			"delete", "lib/nginx", "alice", 1, `alice deleted repository "lib/nginx"`},
		{"create project", &CreateProjectEvent{ProjectID: 5, Project: "myproj", Operator: "admin", OccurAt: at},
			"create", "myproj", "admin", 5, `admin created project "myproj": id 5`},
		{"delete project", &DeleteProjectEvent{ProjectID: 5, Project: "myproj", Operator: "admin", OccurAt: at},
			"delete", "myproj", "admin", 5, `admin deleted project "myproj": id 5`},
		{"create tag", &CreateTagEvent{Repository: "lib/nginx", Tag: "v1.2", AttachedArtifact: art, Operator: "alice", OccurAt: at},
			"create", "lib/nginx:v1.2", "alice", 1, `alice created tag "lib/nginx:v1.2": digest "sha256:7f3a"`},
		{"delete tag", &DeleteTagEvent{Repository: "lib/nginx", Tag: "v1.2", AttachedArtifact: art, Operator: "alice", OccurAt: at},
			"delete", "lib/nginx:v1.2", "alice", 1, `alice deleted tag "lib/nginx:v1.2": digest "sha256:7f3a"`},
		{"create system robot", &CreateRobotEvent{Robot: robot, Operator: "admin", OccurAt: at},
			"create", "robot$ci", "admin", 0, `admin created robot "robot$ci": id 31, level "system"`},
		{"create project robot", &CreateRobotEvent{Robot: projRobot, Operator: "admin", OccurAt: at},
			"create", "robot$myproj+ci", "admin", 5, `admin created robot "robot$myproj+ci": id 32, level "project"`},
		{"delete robot", &DeleteRobotEvent{Robot: robot, Operator: "harbor-jobservice", OccurAt: at},
			"delete", "robot$ci", "harbor-jobservice", 0, `harbor-jobservice deleted robot "robot$ci": id 31, level "system"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditLog, err := tt.event.ResolveToAuditLog()
			require.NoError(t, err)
			assert.Equal(t, tt.desc, auditLog.OperationDescription)
			assert.Equal(t, tt.operation, auditLog.Operation)
			assert.Equal(t, tt.resource, auditLog.Resource)
			assert.Equal(t, tt.username, auditLog.Username)
			assert.Equal(t, tt.projectID, auditLog.ProjectID)
			assert.Equal(t, at, auditLog.OpTime)
			assert.True(t, auditLog.IsSuccessful, "notifier events stay success-only")
			assert.NotContains(t, auditLog.OperationDescription, "s3cr3t")
		})
	}
}
