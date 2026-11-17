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

	"github.com/goharbor/harbor/src/controller/event"
	evtModel "github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/auditext"
	"github.com/goharbor/harbor/src/pkg/auditext/collapse"
	am "github.com/goharbor/harbor/src/pkg/auditext/model"
	"github.com/goharbor/harbor/src/pkg/auditext/summary"
)

// Handler - audit log handler
type Handler struct {
	// held defaults to the shared held set, which writes through auditext.Mgr
	held *heldPushes
	// eventEnabled defaults to the audit log event settings
	eventEnabled func(ctx context.Context, event string) bool
}

// AuditResolver - interface to resolve to AuditLog
type AuditResolver interface {
	ResolveToAuditLog() (*am.AuditLogExt, error)
}

// Name ...
func (h *Handler) Name() string {
	return "AuditLog"
}

// Handle ...
func (h *Handler) Handle(ctx context.Context, value any) error {
	var addAuditLog, hold bool
	switch v := value.(type) {
	case *event.PushArtifactEvent:
		addAuditLog = true
		// an index pushed by digest may itself be listed by a parent index
		hold = len(v.Tags) == 0 && v.Artifact != nil
	case *event.DeleteArtifactEvent:
		addAuditLog = v.Artifact == nil || !collapse.RepositoryDeleted(ctx, v.Artifact.RepositoryName) ||
			!h.enabled(ctx, "delete_repository")
	case *event.DeleteRepositoryEvent, *event.CreateProjectEvent, *event.DeleteProjectEvent,
		*event.DeleteTagEvent, *event.CreateTagEvent, *evtModel.CommonEvent:
		addAuditLog = true
	case *event.CreateRobotEvent:
		addAuditLog = v.Robot == nil || !collapse.IsScannerRobot(ctx, v.Robot.Name, v.Robot.Visible)
	case *event.DeleteRobotEvent:
		addAuditLog = v.Robot == nil || !collapse.IsScannerRobot(ctx, v.Robot.Name, v.Robot.Visible)
	case *event.PullArtifactEvent:
		// the proxy-cache path publishes with the request context, which still
		// carries the security context; the registry path marks the request
		addAuditLog = !config.PullAuditLogDisable(ctx) && !collapse.PullAuditSkipped(ctx) && !IsScannerPrincipal(ctx)
	default:
		log.Errorf("Can not handler this event type! %#v", v)
	}

	if addAuditLog {
		resolver := value.(AuditResolver)
		auditLog, err := resolver.ResolveToAuditLog()
		if err != nil {
			log.Errorf("failed to handler event %v", err)
			return err
		}
		if auditLog != nil && h.enabled(ctx, fmt.Sprintf("%v_%v", auditLog.Operation, auditLog.ResourceType)) {
			auditLog.OperationDescription = summary.Describe(auditLog)
			if hold {
				h.heldSet().add(ctx, value.(*event.PushArtifactEvent).Artifact, auditLog)
				return nil
			}
			h.heldSet().write(ctx, auditLog)
		}
	}
	return nil
}

func (h *Handler) enabled(ctx context.Context, event string) bool {
	if h.eventEnabled != nil {
		return h.eventEnabled(ctx, event)
	}
	return config.AuditLogEventEnabled(ctx, event)
}

func (h *Handler) heldSet() *heldPushes {
	if h.held != nil {
		return h.held
	}
	return held
}

func create(ctx context.Context, mgr auditext.Manager, auditLog *am.AuditLogExt) {
	if mgr == nil {
		mgr = auditext.Mgr
	}
	if _, err := mgr.Create(ctx, auditLog); err != nil {
		log.Infof("add audit log err: %v", err)
	}
}

// IsStateful ...
func (h *Handler) IsStateful() bool {
	return false
}
