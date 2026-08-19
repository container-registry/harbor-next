// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package event

import (
	"testing"

	"github.com/goharbor/harbor/src/pkg/artifact"
)

func TestPullArtifactAuditEnrichment(t *testing.T) {
	event := &PullArtifactEvent{ArtifactEvent: &ArtifactEvent{
		Artifact: &artifact.Artifact{
			ProjectID:      7,
			RepositoryName: "library/alpine",
			Digest:         "sha256:deadbeef",
		},
		Tags:          []string{"latest", "3.20"},
		ClientAddress: "192.0.2.10",
		UserAgent:     "containerd/2",
	}}
	audit, err := event.ResolveToAuditLog()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Username != "anonymous" {
		t.Errorf("Username = %q, want anonymous", audit.Username)
	}
	if audit.ArtifactRepository != "library/alpine" || audit.ArtifactTag != "latest" || audit.ArtifactDigest != "sha256:deadbeef" {
		t.Errorf("unexpected artifact attributes: %#v", audit)
	}
	if audit.ClientAddress != "192.0.2.10" || audit.UserAgent != "containerd/2" {
		t.Errorf("unexpected client attributes: %#v", audit)
	}
}

func TestDigestPullOmitsArtifactTag(t *testing.T) {
	event := &PullArtifactEvent{ArtifactEvent: &ArtifactEvent{
		Artifact: &artifact.Artifact{RepositoryName: "library/alpine", Digest: "sha256:deadbeef"},
	}}
	audit, err := event.ResolveToAuditLog()
	if err != nil {
		t.Fatal(err)
	}
	if audit.ArtifactTag != "" {
		t.Errorf("ArtifactTag = %q, want empty", audit.ArtifactTag)
	}
}
