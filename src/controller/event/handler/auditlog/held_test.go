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

package auditlog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/controller/event"
	robotctl "github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/auditext/collapse"
	am "github.com/goharbor/harbor/src/pkg/auditext/model"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	robotModel "github.com/goharbor/harbor/src/pkg/robot/model"
	"github.com/goharbor/harbor/src/pkg/user"
	testingauditext "github.com/goharbor/harbor/src/testing/pkg/auditext"
)

// withMockManager returns a handler with its own mock manager and held set,
// so nothing here touches package state other tests' handlers read.
func withMockManager(t *testing.T) (*Handler, *testingauditext.Manager) {
	t.Helper()
	mgr := &testingauditext.Manager{}
	h := &Handler{held: newHeldPushes(mgr)}
	t.Cleanup(h.held.stop)
	return h, mgr
}

func pushEvent(repo, digest string, tags ...string) *event.PushArtifactEvent {
	return &event.PushArtifactEvent{ArtifactEvent: &event.ArtifactEvent{
		Repository: repo,
		Tags:       tags,
		Operator:   "alice",
		OccurAt:    time.Now(),
		Artifact:   &artifact.Artifact{RepositoryName: repo, Digest: digest, ManifestMediaType: "application/vnd.oci.image.manifest.v1+json"},
	}}
}

func TestHeldPushDroppedWhenAnIndexListsIt(t *testing.T) {
	h, mgr := withMockManager(t)
	h.held.holdFor = time.Hour

	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/held", "sha256:child")))
	collapse.MarkIndexMembers("lib/held", "alice", []byte(`{"manifests":[{"digest":"sha256:child"}]}`))
	h.held.flush()
	mgr.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestHeldIndexDroppedWhenAParentIndexListsIt(t *testing.T) {
	h, mgr := withMockManager(t)
	h.held.holdFor = time.Hour
	child := pushEvent("lib/nested", "sha256:childindex")
	child.Artifact.ManifestMediaType = "application/vnd.oci.image.index.v1+json"

	assert.NoError(t, h.Handle(context.Background(), child))
	collapse.MarkIndexMembers("lib/nested", "alice", []byte(`{"manifests":[{"digest":"sha256:childindex"}]}`))
	h.held.flush()
	mgr.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestHeldPushesAreCapped(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	h.held.holdFor = time.Hour
	for i := range maxHeldPushes + 3 {
		assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/many", fmt.Sprintf("sha256:%d", i))))
	}
	mgr.AssertNumberOfCalls(t, "Create", 3)
}

func TestRepeatedPutOfAHeldDigestIsOnePush(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	h.held.holdFor = time.Hour
	for range 5 {
		assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/retry", "sha256:same")))
	}
	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/retry", "sha256:other")))
	h.held.flush()
	mgr.AssertNumberOfCalls(t, "Create", 2)
}

func TestHeldPushAfterReleaseIsHeldAgain(t *testing.T) {
	h, mgr := withMockManager(t)
	written := make(chan struct{}, 2)
	mgr.On("Create", mock.Anything, mock.Anything).Run(func(mock.Arguments) { written <- struct{}{} }).Return(int64(1), nil)
	h.held.holdFor = 10 * time.Millisecond
	for range 2 {
		assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/again", "sha256:a")))
		select {
		case <-written:
		case <-time.After(2 * time.Second):
			t.Fatal("held push never written")
		}
	}
}

func TestGDPRMaskingFailsClosed(t *testing.T) {
	ctx := context.Background()
	found := func(context.Context, string) error { return nil }
	missing := func(context.Context, string) error { return errors.NotFoundError(nil) }
	broken := func(context.Context, string) error { return errors.New("connection refused") }
	assert.False(t, gdprMasked(ctx, "alice", true, "robot$", found), "existing user")
	assert.True(t, gdprMasked(ctx, "alice", true, "robot$", missing), "deleted user")
	assert.True(t, gdprMasked(ctx, "alice", true, "robot$", broken), "a failed lookup masks")
	assert.False(t, gdprMasked(ctx, "alice", false, "robot$", broken), "GDPR masking off")
	assert.False(t, gdprMasked(ctx, "robot$ci", true, "robot$", broken), "robots are not users")
}

func TestHeldPushOfADeletedUserIsMasked(t *testing.T) {
	h, mgr := withMockManager(t)
	written := make(chan *am.AuditLogExt, 1)
	mgr.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		written <- args.Get(1).(*am.AuditLogExt)
	}).Return(int64(1), nil)
	h.held.holdFor = time.Hour
	h.held.maskedUser = func(_ context.Context, username string) bool { return username == "alice" }

	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/gdpr", "sha256:g")))
	h.held.flush()
	l := <-written
	masked := user.Mgr.GenerateCheckSum("alice")
	assert.Equal(t, masked, l.Username)
	assert.Equal(t, masked+" pushed artifact lib/gdpr@sha256:g", l.OperationDescription)
}

func TestHeldPushWrittenWhenNoIndexComes(t *testing.T) {
	h, mgr := withMockManager(t)
	written := make(chan *am.AuditLogExt, 1)
	mgr.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		written <- args.Get(1).(*am.AuditLogExt)
	}).Return(int64(1), nil)
	h.held.holdFor = 20 * time.Millisecond

	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/lonely", "sha256:solo")))
	select {
	case l := <-written:
		assert.Equal(t, "alice pushed artifact lib/lonely@sha256:solo", l.OperationDescription)
	case <-time.After(2 * time.Second):
		t.Fatal("held push never written")
	}
}

func TestFlushWritesHeldPushes(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	h.held.holdFor = time.Hour

	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/flush", "sha256:f")))
	mgr.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	h.held.flush()
	mgr.AssertNumberOfCalls(t, "Create", 1)
	h.held.flush()
	mgr.AssertNumberOfCalls(t, "Create", 1)
}

func TestTaggedPushIsWrittenAtOnce(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/tagged", "sha256:t", "1.0")))
	mgr.AssertNumberOfCalls(t, "Create", 1)
}

func TestArtifactsOfADeletedRepositoryAreCovered(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	ctx := collapse.WithDeletedRepositories(context.Background(), []string{"lib/gone"})
	del := &event.DeleteArtifactEvent{ArtifactEvent: pushEvent("lib/gone", "sha256:g").ArtifactEvent}
	assert.NoError(t, h.Handle(ctx, del))
	mgr.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)

	assert.NoError(t, h.Handle(ctx, &event.DeleteRepositoryEvent{Repository: "lib/gone", Operator: "admin", OccurAt: time.Now()}))
	mgr.AssertNumberOfCalls(t, "Create", 1)
}

func TestScannerRobotIsNotAudited(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	scanner := &robotModel.Robot{Name: "robot$lib+scanner-Trivy-997cb7a1-e297-11f1-8832-2e120dc2b070"}
	assert.NoError(t, h.Handle(context.Background(), &event.CreateRobotEvent{Robot: scanner, Operator: "admin"}))
	assert.NoError(t, h.Handle(context.Background(), &event.DeleteRobotEvent{Robot: scanner, Operator: "harbor-jobservice"}))
	mgr.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)

	// robots made through the API are visible, whatever their name
	named := &robotModel.Robot{Name: "robot$lib+scanner-ci", Visible: true}
	assert.NoError(t, h.Handle(context.Background(), &event.CreateRobotEvent{Robot: named, Operator: "admin"}))
	mgr.AssertNumberOfCalls(t, "Create", 1)
}

func TestHeldPushNotCoveredByAnotherUsersOrAnOlderIndex(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	h.held.holdFor = time.Hour

	collapse.MarkIndexMembers("lib/shared", "alice", []byte(`{"manifests":[{"digest":"sha256:x"}]}`))
	bob := pushEvent("lib/shared", "sha256:x")
	bob.Operator = "bob"
	assert.NoError(t, h.Handle(context.Background(), bob))
	later := pushEvent("lib/shared", "sha256:x")
	later.OccurAt = time.Now().Add(time.Second)
	assert.NoError(t, h.Handle(context.Background(), later))
	h.held.flush()
	mgr.AssertNumberOfCalls(t, "Create", 2)
}

func TestPushAfterFlushIsWrittenAtOnce(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	h.held.flush()
	assert.NoError(t, h.Handle(context.Background(), pushEvent("lib/late", "sha256:l")))
	mgr.AssertNumberOfCalls(t, "Create", 1)
}

func TestSkippedPullIsNotAudited(t *testing.T) {
	h, mgr := withMockManager(t)
	marks := collapse.NewRequestMarks()
	ctx := marks.Into(context.Background())
	collapse.SkipPullAudit(ctx)
	pull := &event.PullArtifactEvent{ArtifactEvent: pushEvent("lib/app", "sha256:child").ArtifactEvent}
	assert.NoError(t, h.Handle(ctx, pull))
	mgr.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestArtifactsAreAuditedWhenRepositoryDeleteIsNot(t *testing.T) {
	h, mgr := withMockManager(t)
	mgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	h.eventEnabled = func(_ context.Context, event string) bool { return event != "delete_repository" }
	ctx := collapse.WithDeletedRepositories(context.Background(), []string{"lib/gone"})
	del := &event.DeleteArtifactEvent{ArtifactEvent: pushEvent("lib/gone", "sha256:g").ArtifactEvent}
	assert.NoError(t, h.Handle(ctx, del))
	mgr.AssertNumberOfCalls(t, "Create", 1)
}

// tokenPrincipal stands in for a bearer-token security context, which knows
// only the principal's name.
type tokenPrincipal struct {
	security.Context
	name string
}

func (p tokenPrincipal) IsAuthenticated() bool { return true }
func (p tokenPrincipal) GetUsername() string   { return p.name }

func TestIsScannerPrincipalForBearerTokens(t *testing.T) {
	visible := map[string]bool{"lib+scanner-Trivy-0d3c": false, "lib+scanner-ci": true}
	orig := listRobots
	lookups := 0
	listRobots = func(_ context.Context, query *q.Query) ([]*robotctl.Robot, error) {
		lookups++
		name := query.Keywords["name"].(string)
		v, ok := visible[name]
		if !ok {
			return nil, nil
		}
		return []*robotctl.Robot{{Robot: robotModel.Robot{Name: name, Visible: v}}}, nil
	}
	t.Cleanup(func() { listRobots = orig })

	as := func(name string) context.Context {
		return security.NewContext(context.Background(), tokenPrincipal{name: name})
	}
	assert.True(t, IsScannerPrincipal(as("robot$lib+scanner-Trivy-0d3c")))
	assert.False(t, IsScannerPrincipal(as("robot$lib+scanner-ci")), "a visible robot is audited whatever its name")
	assert.False(t, IsScannerPrincipal(as("robot$lib+scanner-gone")))
	before := lookups
	assert.False(t, IsScannerPrincipal(as("alice")))
	assert.Equal(t, before, lookups, "no lookup for names without the scanner prefix")
	assert.False(t, IsScannerPrincipal(context.Background()))
}
