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

// Package systemquota implements the instance-wide storage quota decided in
// harbor-next #839 (theme A): one admin-managed limit, usage from Harbor's own
// blob accounting unless a measurer supplies a physical value, and an opt-in
// threshold gate on registry writes.
package systemquota

import (
	"context"
	"fmt"
	"time"

	"github.com/goharbor/harbor/src/controller/blob"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/cache"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/quota/types"
	"github.com/goharbor/harbor/src/pkg/systemartifact"
	"github.com/goharbor/harbor/src/pkg/systemquota"
	"github.com/goharbor/harbor/src/pkg/systemquota/model"
)

const (
	// UsedSourceAccounted marks a usage value derived from Harbor's blob table.
	UsedSourceAccounted = "accounted"
	// UsedSourceMeasured marks a usage value measured on the storage volume.
	UsedSourceMeasured = "measured"

	quotaCacheKey     = "system_quota:quota"
	accountedCacheKey = "system_quota:accounted_used"

	// quotaCacheTTL bounds how long a replica may enforce a limit that another
	// replica has already changed; writes invalidate the key explicitly.
	quotaCacheTTL = time.Minute
	// accountedCacheTTL bounds the cost of SUM(blob.size), which scans the blob
	// table, to one query per interval cluster-wide.
	accountedCacheTTL = 5 * time.Minute
)

// Measurement is a physical usage value supplied by a Measurer.
type Measurement struct {
	Used int64
	At   time.Time
}

// Measurer supplies physically measured usage (harbor-next #839, theme B).
// A nil result means no measurement is available and accounting is used.
type Measurer interface {
	Measure(ctx context.Context) (*Measurement, error)
}

// Status is the effective view of the global storage quota.
type Status struct {
	Hard              int64
	Used              int64
	Free              int64
	UsedSource        string
	MeasuredAt        *time.Time
	Allocated         int64
	UnlimitedProjects int64
	Enforce           bool
	UpdateTime        time.Time
}

// Controller manages the global storage quota.
type Controller interface {
	// Get returns the effective status, or a NotFound error when no quota is set.
	Get(ctx context.Context) (*Status, error)
	// Update sets the hard limit in bytes and the enforcement flag.
	Update(ctx context.Context, hard int64, enforce bool) error
	// Delete unsets the global quota; no error when none is set.
	Delete(ctx context.Context) error
	// EnforceEnabled reports whether registry writes must be checked.
	EnforceEnabled(ctx context.Context) bool
	// CheckCapacity returns a Denied error when used plus extra bytes exceeds the limit.
	CheckCapacity(ctx context.Context, extra int64) error
}

// Ctl is the default controller.
var Ctl = NewController()

// NewController creates a controller wired to the default managers.
func NewController() Controller {
	return &controller{
		mgr:            systemquota.Mgr,
		blobCtl:        blob.Ctl,
		sysArtifactMgr: systemartifact.Mgr,
		cache:          cache.Default,
	}
}

type controller struct {
	mgr            systemquota.Manager
	blobCtl        blob.Controller
	sysArtifactMgr systemartifact.Manager
	// cache is resolved lazily because cache.Default() is nil until core has
	// initialized redis; a nil cache means every call computes directly.
	cache    func() cache.Cache
	measurer Measurer
}

type accountedUsage struct {
	Bytes int64     `json:"bytes"`
	At    time.Time `json:"at"`
}

// SetMeasurer installs the physical usage source. Intended for theme B; a nil
// measurer restores accounting-only behaviour.
func SetMeasurer(m Measurer) {
	if c, ok := Ctl.(*controller); ok {
		c.measurer = m
	}
}

func (c *controller) Get(ctx context.Context) (*Status, error) {
	quota, err := c.quota(ctx)
	if err != nil {
		return nil, err
	}
	status := &Status{
		Hard:       quota.Hard,
		Enforce:    quota.Enforce,
		UpdateTime: quota.UpdateTime,
		UsedSource: UsedSourceAccounted,
	}

	if used, at, ok := c.measured(ctx); ok {
		status.Used = used
		status.UsedSource = UsedSourceMeasured
		status.MeasuredAt = &at
	} else {
		used, err := c.accounted(ctx)
		if err != nil {
			return nil, err
		}
		status.Used = used
	}
	status.Free = max(status.Hard-status.Used, 0)

	allocation, err := c.mgr.ProjectAllocation(ctx)
	if err != nil {
		return nil, err
	}
	status.Allocated = allocation.Allocated
	status.UnlimitedProjects = allocation.Unlimited
	return status, nil
}

func (c *controller) Update(ctx context.Context, hard int64, enforce bool) error {
	if hard == types.UNLIMITED {
		return errors.BadRequestError(nil).WithMessage("use DELETE to unset the global storage quota")
	}
	if err := lib.ValidateQuotaLimit(hard); err != nil {
		return errors.BadRequestError(err)
	}
	if err := c.mgr.Upsert(ctx, &model.SystemQuota{Hard: hard, Enforce: enforce}); err != nil {
		return err
	}
	c.invalidate(ctx)
	return nil
}

func (c *controller) Delete(ctx context.Context) error {
	if err := c.mgr.Delete(ctx); err != nil {
		return err
	}
	c.invalidate(ctx)
	return nil
}

func (c *controller) EnforceEnabled(ctx context.Context) bool {
	quota, err := c.quota(ctx)
	if err != nil {
		if !errors.IsNotFoundErr(err) {
			log.G(ctx).Warningf("failed to load global storage quota, skipping enforcement: %v", err)
		}
		return false
	}
	return quota.Enforce
}

func (c *controller) CheckCapacity(ctx context.Context, extra int64) error {
	if !c.EnforceEnabled(ctx) {
		return nil
	}
	status, err := c.Get(ctx)
	if err != nil {
		if errors.IsNotFoundErr(err) {
			return nil
		}
		// Failing open keeps the registry writable when the accounting query
		// is unavailable; the limit is best-effort by design (#839).
		log.G(ctx).Warningf("failed to evaluate global storage quota, allowing request: %v", err)
		return nil
	}
	if extra < 0 {
		extra = 0
	}
	if status.Used+extra > status.Hard {
		return errors.DeniedError(nil).WithMessagef("global storage quota exceeded: used %s of %s",
			types.ResourceStorage.FormatValue(status.Used), types.ResourceStorage.FormatValue(status.Hard))
	}
	return nil
}

// quota returns the persisted row, cached briefly so the gate does not hit the
// database on every registry write.
func (c *controller) quota(ctx context.Context) (*model.SystemQuota, error) {
	ca := c.cache()
	if ca == nil {
		return c.mgr.Get(ctx)
	}
	quota := &model.SystemQuota{}
	err := cache.FetchOrSave(ctx, ca, quotaCacheKey, quota, func() (any, error) {
		q, err := c.mgr.Get(ctx)
		if err != nil {
			return nil, err
		}
		return q, nil
	}, quotaCacheTTL)
	if err != nil {
		return nil, err
	}
	return quota, nil
}

func (c *controller) accounted(ctx context.Context) (int64, error) {
	compute := func() (*accountedUsage, error) {
		sum, err := c.blobCtl.CalculateTotalSize(ctx, true)
		if err != nil {
			return nil, err
		}
		sysArtifacts, err := c.sysArtifactMgr.GetStorageSize(ctx)
		if err != nil {
			return nil, err
		}
		return &accountedUsage{Bytes: sum + sysArtifacts, At: time.Now()}, nil
	}

	ca := c.cache()
	if ca == nil {
		u, err := compute()
		if err != nil {
			return 0, err
		}
		return u.Bytes, nil
	}
	usage := &accountedUsage{}
	err := cache.FetchOrSave(ctx, ca, accountedCacheKey, usage, func() (any, error) {
		return compute()
	}, accountedCacheTTL)
	if err != nil {
		return 0, err
	}
	return usage.Bytes, nil
}

func (c *controller) measured(ctx context.Context) (int64, time.Time, bool) {
	if c.measurer == nil {
		return 0, time.Time{}, false
	}
	m, err := c.measurer.Measure(ctx)
	if err != nil {
		log.G(ctx).Debugf("no measured storage usage, falling back to accounting: %v", err)
		return 0, time.Time{}, false
	}
	if m == nil {
		return 0, time.Time{}, false
	}
	return m.Used, m.At, true
}

func (c *controller) invalidate(ctx context.Context) {
	ca := c.cache()
	if ca == nil {
		return
	}
	if err := ca.Delete(ctx, quotaCacheKey); err != nil {
		log.G(ctx).Warningf("failed to invalidate cached global storage quota: %v", err)
	}
}

// String renders the status for logs and error messages.
func (s *Status) String() string {
	return fmt.Sprintf("hard=%s used=%s(%s) enforce=%t",
		types.ResourceStorage.FormatValue(s.Hard), types.ResourceStorage.FormatValue(s.Used), s.UsedSource, s.Enforce)
}
