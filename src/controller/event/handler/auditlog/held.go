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
	"strings"
	"sync"
	"time"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/auditext"
	"github.com/goharbor/harbor/src/pkg/auditext/collapse"
	am "github.com/goharbor/harbor/src/pkg/auditext/model"
	"github.com/goharbor/harbor/src/pkg/auditext/summary"
	"github.com/goharbor/harbor/src/pkg/user"
)

// HoldPushFor is how long a manifest pushed by digest waits for an index that
// lists it. Clients push the platform manifests of a multi-arch image first
// and the index last; the index entry then stands for all of them.
var HoldPushFor = time.Minute

// maxHeldPushes caps the held set; past it a push is written at once, so
// nothing is dropped.
const maxHeldPushes = 10000

var held = newHeldPushes(nil)

type heldPush struct {
	key      string
	pushedAt time.Time
	ctx      context.Context
	art      *artifact.Artifact
	auditLog *am.AuditLogExt
	timer    *time.Timer
}

type heldPushes struct {
	// mgr writes the entries; nil means auditext.Mgr
	mgr     auditext.Manager
	holdFor time.Duration
	mu      sync.Mutex
	entries map[*heldPush]struct{}
	// byKey finds the entry held for a repository, operator and digest
	byKey    map[string]*heldPush
	draining bool
	inFlight sync.WaitGroup
	// maskedUser reports an operator whose rows GDPR masking has rewritten
	// while the push was held; nil means gdprMaskedUser
	maskedUser func(ctx context.Context, username string) bool
}

func newHeldPushes(mgr auditext.Manager) *heldPushes {
	return &heldPushes{mgr: mgr, entries: map[*heldPush]struct{}{}, byKey: map[string]*heldPush{}}
}

func (h *heldPushes) write(ctx context.Context, auditLog *am.AuditLogExt) {
	create(ctx, h.mgr, auditLog)
}

func (h *heldPushes) add(ctx context.Context, art *artifact.Artifact, auditLog *am.AuditLogExt) {
	h.mu.Lock()
	if h.draining || len(h.entries) >= maxHeldPushes {
		h.mu.Unlock()
		h.write(ctx, auditLog)
		return
	}
	// a client retrying the same manifest PUT is one push; the held entry
	// already stands for it
	key := art.RepositoryName + "|" + auditLog.Username + "|" + art.Digest
	if _, dup := h.byKey[key]; dup {
		h.mu.Unlock()
		return
	}
	pushedAt, ok := collapse.RequestStarted(ctx)
	if !ok {
		pushedAt = auditLog.OpTime
	}
	p := &heldPush{key: key, pushedAt: pushedAt, ctx: ctx, art: art, auditLog: auditLog}
	h.entries[p] = struct{}{}
	h.byKey[key] = p
	holdFor := h.holdFor
	if holdFor == 0 {
		holdFor = HoldPushFor
	}
	p.timer = time.AfterFunc(holdFor, func() { h.release(p) })
	h.mu.Unlock()
}

// release writes the entry unless the same operator pushed an index listing
// the manifest after it.
func (h *heldPushes) release(p *heldPush) {
	h.mu.Lock()
	_, ok := h.entries[p]
	delete(h.entries, p)
	if h.byKey[p.key] == p {
		delete(h.byKey, p.key)
	}
	if ok {
		h.inFlight.Add(1)
	}
	h.mu.Unlock()
	if !ok {
		return
	}
	defer h.inFlight.Done()
	if collapse.IsIndexMember(p.art.RepositoryName, p.auditLog.Username, p.art.Digest, p.pushedAt) {
		return
	}
	h.maskDeleted(p.ctx, p.auditLog)
	h.write(p.ctx, p.auditLog)
}

// maskDeleted masks an operator deleted while the push was held, the way the
// GDPR masking job already rewrote their stored rows.
func (h *heldPushes) maskDeleted(ctx context.Context, auditLog *am.AuditLogExt) {
	masked := h.maskedUser
	if masked == nil {
		masked = gdprMaskedUser
	}
	if auditLog.Username == "" || !masked(ctx, auditLog.Username) {
		return
	}
	checksum := user.Mgr.GenerateCheckSum(auditLog.Username)
	auditLog.OperationDescription = summary.ReplaceActor(auditLog.OperationDescription, auditLog.Username, checksum)
	auditLog.Username = checksum
}

func gdprMaskedUser(ctx context.Context, username string) bool {
	return gdprMasked(ctx, username, config.DefaultMgr().Get(ctx, common.GDPRAuditLogs).GetBool(), config.RobotPrefix(ctx),
		func(ctx context.Context, name string) error {
			_, err := user.Mgr.GetByName(ctx, name)
			return err
		})
}

// gdprMasked fails closed: with GDPR masking on, an operator that cannot be
// confirmed to exist is masked.
func gdprMasked(ctx context.Context, username string, gdpr bool, robotPrefix string, lookup func(context.Context, string) error) bool {
	if !gdpr || strings.HasPrefix(username, robotPrefix) {
		return false
	}
	err := lookup(ctx, username)
	if err != nil && !errors.IsNotFoundErr(err) {
		log.Warningf("failed to look up audit log operator, masking it: %v", err)
	}
	return err != nil
}

// Flush releases every held entry and waits for releases already running;
// later pushes are written at once. Core calls it on shutdown, before the
// database pool closes, so no entry is lost.
func Flush() {
	held.flush()
}

func (h *heldPushes) flush() {
	h.mu.Lock()
	h.draining = true
	pending := make([]*heldPush, 0, len(h.entries))
	for p := range h.entries {
		p.timer.Stop()
		pending = append(pending, p)
	}
	h.mu.Unlock()
	for _, p := range pending {
		h.release(p)
	}
	h.inFlight.Wait()
}

// stop cancels every pending timer without writing.
func (h *heldPushes) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for p := range h.entries {
		p.timer.Stop()
	}
}
