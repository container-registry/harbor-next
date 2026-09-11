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

package blob

import (
	"net/http"
	"time"

	"github.com/goharbor/harbor/src/controller/blob"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/blob/models"
	"github.com/goharbor/harbor/src/server/middleware/requestid"
)

// touchSkipMaxAge bounds how stale a blob's update_time may be before a Touch
// is skipped. It is deliberately far below any usable GC window. The row-lock
// contention the skip exists to remove comes from re-pushes of one digest
// arriving within seconds of each other, so a few minutes captures nearly all
// of it, while the liveness margin a skipped Touch leaves stays close to a
// full GC window instead of collapsing to half of it.
const touchSkipMaxAge = 5 * time.Minute

// shouldTouchNone decides whether a blob that is already StatusNone still needs
// a Touch. In that state Touch only bumps version/update_time to coordinate
// with GC - a redundant single-row UPDATE that, under concurrent re-pushes of
// the same digest, becomes a row-lock contention hot spot (the lock is held for
// the whole request transaction on PUT manifest).
//
// Touch never makes a blob ineligible for GC; it only moves update_time
// forward. GC collects an unassociated blob once
// "update_time <= now() - window" (pkg/blob/dao.GetBlobsNotRefedByProjectBlob),
// so skipping the Touch leaves the blob protected for window minus the row's
// current age rather than for a full window. Bounding the skip to
// touchSkipMaxAge keeps that margin at window - touchSkipMaxAge, which is the
// same order as the unconditional-Touch behaviour it replaces. A window that is
// not comfortably wider than touchSkipMaxAge leaves no margin worth having -
// including the non-positive values GC_TIME_WINDOW_HOURS accepts without
// validation - so always touch then.
func shouldTouchNone(bb *models.Blob) bool {
	window := time.Duration(config.GetGCTimeWindow()) * time.Hour
	if window <= 2*touchSkipMaxAge {
		return true
	}
	return time.Since(bb.UpdateTime) > touchSkipMaxAge
}

// probeBlob handles config/layer and manifest status in the PUT Blob & Manifest middleware, and update the status before it passed into proxy(distribution).
func probeBlob(r *http.Request, digest string) error {
	logger := log.G(r.Context())

	// digest empty is handled by the blob controller GET method
	bb, err := blobController.Get(r.Context(), digest)
	if err != nil {
		if errors.IsNotFoundErr(err) {
			return nil
		}
		return err
	}

	// touch resets the blob to StatusNone, bumping version/update_time. Its only
	// purpose is to coordinate with GC, so it is called only when it actually
	// matters (see the switch below).
	touch := func() error {
		if err := blobController.Touch(r.Context(), bb); err != nil {
			logger.Errorf("failed to update blob: %s status to StatusNone, error:%v", bb.Digest, err)
			return errors.Wrapf(err, "the request id is: %s", r.Header.Get(requestid.HeaderXRequestID))
		}
		return nil
	}

	switch bb.Status {
	case models.StatusNone:
		if shouldTouchNone(bb) {
			return touch()
		}
	case models.StatusDelete, models.StatusDeleteFailed:
		// GC marked this blob for deletion but an incoming push needs it: pull it
		// back to StatusNone (the version bump defeats GC's compare-and-swap).
		return touch()
	case models.StatusDeleting:
		now := time.Now().UTC()
		// if the deleting exceed 2 hours, marks the blob as StatusDeleteFailed
		if now.Sub(bb.UpdateTime) > time.Duration(config.GetGCTimeWindow())*time.Hour {
			if err := blob.Ctl.Fail(r.Context(), bb); err != nil {
				log.Errorf("failed to update blob: %s status to StatusDeleteFailed, error:%v", bb.Digest, err)
				return errors.Wrapf(err, "the request id is: %s", r.Header.Get(requestid.HeaderXRequestID))
			}
			// StatusDeleteFailed => StatusNone, and then let the proxy to handle manifest upload
			return probeBlob(r, digest)
		}
		return errors.New(nil).WithMessagef("the asking blob is in GC, mark it as non existing, request id: %s", r.Header.Get(requestid.HeaderXRequestID)).WithCode(errors.NotFoundCode)
	default:
		return nil
	}
	return nil
}
