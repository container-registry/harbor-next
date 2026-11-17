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

// Package collapse remembers, for a short time, which manifest requests belong
// to one client operation, so a multi-arch push or pull is audited once.
// State is per core instance: on several replicas an operation may still log a
// platform manifest, but no entry is ever lost.
package collapse

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goharbor/harbor/src/lib/config"
)

const (
	// IndexMemberTTL bounds how long an index push hides the platform
	// manifests it lists.
	IndexMemberTTL = 10 * time.Minute
	maxEntries     = 50000
)

// IndexMembers records the manifests listed by indexes pushed recently.
var IndexMembers = NewWindow(IndexMemberTTL, maxEntries)

// MarkIndexMembers records that operator pushed an index to repo listing the
// manifests in its body.
func MarkIndexMembers(repo, operator string, index []byte) {
	marked := strconv.FormatInt(IndexMembers.now().UnixNano(), 10)
	for _, d := range IndexChildren(index) {
		IndexMembers.Put(repo+"@"+d, operator+"\x00"+marked)
	}
}

// IsIndexMember reports whether operator pushed, at or after pushedAt, an
// index to repo that lists digest. An older index, or one pushed by someone
// else, never covers a push.
func IsIndexMember(repo, operator, digest string, pushedAt time.Time) bool {
	v, ok := IndexMembers.Get(repo + "@" + digest)
	if !ok {
		return false
	}
	by, marked, _ := strings.Cut(v, "\x00")
	nanos, err := strconv.ParseInt(marked, 10, 64)
	return err == nil && by == operator && !time.Unix(0, nanos).Before(pushedAt)
}

// IndexChildren returns the manifest digests an OCI index or Docker manifest list names.
func IndexChildren(body []byte) []string {
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(body, &index); err != nil {
		return nil
	}
	children := make([]string, 0, len(index.Manifests))
	for _, m := range index.Manifests {
		if m.Digest != "" {
			children = append(children, m.Digest)
		}
	}
	return children
}

type entry struct {
	value   string
	expires time.Time
}

// Window is a bounded map whose entries expire after a fixed TTL.
type Window struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	now     func() time.Time
	entries map[string]entry
}

// NewWindow returns a Window holding at most maxEntries keys for ttl each.
func NewWindow(ttl time.Duration, maxEntries int) *Window {
	return &Window{ttl: ttl, max: maxEntries, now: time.Now, entries: map[string]entry{}}
}

// Put stores value under key for the window's TTL.
func (w *Window) Put(key, value string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if _, exists := w.entries[key]; !exists && len(w.entries) >= w.max {
		for k, e := range w.entries {
			if now.After(e.expires) {
				delete(w.entries, k)
			}
		}
		if len(w.entries) >= w.max {
			// a flood of distinct keys costs a few extra audit rows, never memory
			clear(w.entries)
		}
	}
	w.entries[key] = entry{value: value, expires: now.Add(w.ttl)}
}

// Get returns the live value under key.
func (w *Window) Get(key string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.entries[key]
	if !ok {
		return "", false
	}
	if w.now().After(e.expires) {
		delete(w.entries, key)
		return "", false
	}
	return e.value, true
}

// Take returns the live value under key and removes it, so it is used once.
func (w *Window) Take(key string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.entries[key]
	if !ok {
		return "", false
	}
	delete(w.entries, key)
	if w.now().After(e.expires) {
		return "", false
	}
	return e.value, true
}

type deletedRepositoriesKey struct{}

// WithDeletedRepositories marks the repositories one request deletes; the
// artifacts removed with them are covered by the repository's own entry.
func WithDeletedRepositories(ctx context.Context, repositories []string) context.Context {
	if len(repositories) == 0 {
		return ctx
	}
	set := make(map[string]struct{}, len(repositories))
	for _, r := range repositories {
		set[r] = struct{}{}
	}
	return context.WithValue(ctx, deletedRepositoriesKey{}, set)
}

// RepositoryDeleted reports whether the request behind ctx deletes repository.
func RepositoryDeleted(ctx context.Context, repository string) bool {
	set, _ := ctx.Value(deletedRepositoriesKey{}).(map[string]struct{})
	_, ok := set[repository]
	return ok
}

type requestMarksKey struct{}

// RequestMarks carries audit decisions made while serving a request to the
// audit handler, which sees the events after the response is written.
type RequestMarks struct {
	started  time.Time
	skipPull atomic.Bool
}

// NewRequestMarks returns empty marks for a request starting now.
func NewRequestMarks() *RequestMarks {
	return &RequestMarks{started: time.Now()}
}

// RequestStarted returns when the request behind ctx started. Event times are
// stamped later, when the events are published, so they cannot order a push
// against the index that lists it.
func RequestStarted(ctx context.Context) (time.Time, bool) {
	m, ok := ctx.Value(requestMarksKey{}).(*RequestMarks)
	if !ok {
		return time.Time{}, false
	}
	return m.started, true
}

// Into returns ctx carrying the marks.
func (m *RequestMarks) Into(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestMarksKey{}, m)
}

// SkipPullAudit records that the pull served by this request is part of an
// operation already audited. The pull event itself is still published.
func SkipPullAudit(ctx context.Context) {
	if m, ok := ctx.Value(requestMarksKey{}).(*RequestMarks); ok {
		m.skipPull.Store(true)
	}
}

// PullAuditSkipped reports whether the request behind ctx asked to skip its pull audit.
func PullAuditSkipped(ctx context.Context) bool {
	m, ok := ctx.Value(requestMarksKey{}).(*RequestMarks)
	return ok && m.skipPull.Load()
}

// IsScannerRobot reports the hidden robot a scan creates to pull the artifact.
// Robots created through the API are always visible, so a user robot named
// like a scanner robot is still audited.
func IsScannerRobot(ctx context.Context, name string, visible bool) bool {
	prefix := config.ScannerRobotPrefix(ctx)
	if visible || prefix == "" || name == "" {
		return false
	}
	name = strings.TrimPrefix(name, config.RobotPrefix(ctx))
	if _, short, ok := strings.Cut(name, "+"); ok {
		name = short
	}
	return strings.HasPrefix(name, prefix+"-")
}
