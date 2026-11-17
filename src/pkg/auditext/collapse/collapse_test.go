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

package collapse

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestWindowExpires(t *testing.T) {
	now := time.Unix(1000, 0)
	w := NewWindow(time.Minute, 10)
	w.now = func() time.Time { return now }
	w.Put("k", "v")
	v, ok := w.Get("k")
	assert.True(t, ok)
	assert.Equal(t, "v", v)
	now = now.Add(2 * time.Minute)
	_, ok = w.Get("k")
	assert.False(t, ok)
}

func TestWindowTakeIsOnce(t *testing.T) {
	w := NewWindow(time.Minute, 10)
	w.Put("k", "v")
	v, ok := w.Take("k")
	assert.True(t, ok)
	assert.Equal(t, "v", v)
	_, ok = w.Take("k")
	assert.False(t, ok)
	_, ok = w.Get("k")
	assert.False(t, ok)
}

func TestWindowIsBounded(t *testing.T) {
	w := NewWindow(time.Hour, 5)
	for i := range 100 {
		w.Put(fmt.Sprint(i), "")
		assert.LessOrEqual(t, len(w.entries), 5)
	}
	_, ok := w.Get("99")
	assert.True(t, ok)
}

func TestIndexMembers(t *testing.T) {
	index := []byte(`{"schemaVersion":2,"manifests":[{"digest":"sha256:aa"},{"digest":"sha256:bb"},{"mediaType":"x"}]}`)
	assert.Equal(t, []string{"sha256:aa", "sha256:bb"}, IndexChildren(index))
	assert.Nil(t, IndexChildren([]byte("not json")))

	pushed := time.Now().Add(-time.Second)
	MarkIndexMembers("lib/app", "alice", index)
	assert.True(t, IsIndexMember("lib/app", "alice", "sha256:aa", pushed))
	assert.False(t, IsIndexMember("lib/other", "alice", "sha256:aa", pushed))
	assert.False(t, IsIndexMember("lib/app", "alice", "sha256:cc", pushed))
	assert.False(t, IsIndexMember("lib/app", "bob", "sha256:aa", pushed), "another user's index covers nothing")
	assert.False(t, IsIndexMember("lib/app", "alice", "sha256:aa", time.Now().Add(time.Second)), "an older index never covers a later push")
}

func TestRequestMarks(t *testing.T) {
	ctx := context.Background()
	SkipPullAudit(ctx)
	assert.False(t, PullAuditSkipped(ctx))
	marks := NewRequestMarks()
	SkipPullAudit(marks.Into(ctx))
	assert.True(t, PullAuditSkipped(marks.Into(context.Background())), "marks set while serving reach the publish context")
}

func TestDeletedRepositories(t *testing.T) {
	ctx := context.Background()
	assert.False(t, RepositoryDeleted(ctx, "lib/app"))
	assert.Equal(t, ctx, WithDeletedRepositories(ctx, nil))
	ctx = WithDeletedRepositories(ctx, []string{"lib/app"})
	assert.True(t, RepositoryDeleted(ctx, "lib/app"))
	assert.False(t, RepositoryDeleted(ctx, "lib/other"))
}

func TestIsScannerRobot(t *testing.T) {
	config.InitWithSettings(map[string]any{common.RobotScannerNamePrefix: "scanner", common.RobotNamePrefix: "robot$"})
	ctx := context.Background()
	assert.True(t, IsScannerRobot(ctx, "robot$lib+scanner-Trivy-0d3c", false))
	assert.True(t, IsScannerRobot(ctx, "lib+scanner-Trivy-0d3c", false))
	assert.False(t, IsScannerRobot(ctx, "robot$lib+scanner-ci", true), "API robots are visible")
	assert.False(t, IsScannerRobot(ctx, "robot$lib+ci", false))
}

func TestWindowRefreshAtCapacityKeepsOtherKeys(t *testing.T) {
	w := NewWindow(time.Hour, 3)
	w.Put("a", "")
	w.Put("b", "")
	w.Put("c", "")
	w.Put("a", "again")
	for _, k := range []string{"a", "b", "c"} {
		_, ok := w.Get(k)
		assert.True(t, ok, k)
	}
}
