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

package dao

import (
	"context"
	stderrors "errors"

	beegoorm "github.com/beego/beego/v2/client/orm"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/systemquota/model"
)

func init() {
	orm.RegisterModel(new(model.SystemQuota))
}

// ProjectAllocation summarizes the hard limits assigned to projects.
type ProjectAllocation struct {
	// Allocated is the sum of finite project storage limits in bytes.
	Allocated int64
	// Unlimited is the number of projects whose storage limit is -1.
	Unlimited int64
}

// DAO is the data access object for the global storage quota.
type DAO interface {
	// Get returns the singleton row, or a NotFound error when no global quota is set.
	Get(ctx context.Context) (*model.SystemQuota, error)
	// Upsert creates or replaces the singleton row.
	Upsert(ctx context.Context, quota *model.SystemQuota) error
	// Delete removes the singleton row; no error when absent.
	Delete(ctx context.Context) error
	// ProjectAllocation aggregates the per-project hard limits from the quota table.
	ProjectAllocation(ctx context.Context) (*ProjectAllocation, error)
}

type dao struct{}

// New creates the DAO.
func New() DAO {
	return &dao{}
}

func (d *dao) Get(ctx context.Context) (*model.SystemQuota, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	quota := &model.SystemQuota{ID: model.SingletonID}
	if err := o.Read(quota); err != nil {
		if stderrors.Is(err, beegoorm.ErrNoRows) {
			return nil, errors.NotFoundError(nil).WithMessage("global storage quota is not set")
		}
		return nil, err
	}
	return quota, nil
}

func (d *dao) Upsert(ctx context.Context, quota *model.SystemQuota) error {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}
	quota.ID = model.SingletonID
	_, err = o.InsertOrUpdate(quota, "id")
	return err
}

func (d *dao) Delete(ctx context.Context) error {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}
	_, err = o.Delete(&model.SystemQuota{ID: model.SingletonID})
	if stderrors.Is(err, beegoorm.ErrNoRows) {
		return nil
	}
	return err
}

func (d *dao) ProjectAllocation(ctx context.Context) (*ProjectAllocation, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	// hard is JSONB; -1 marks an unlimited project and must not enter the sum.
	sql := `SELECT
		COALESCE(SUM(CASE WHEN hard->>'storage' = '-1' THEN NULL ELSE (hard->>'storage')::bigint END), 0) AS allocated,
		COUNT(*) FILTER (WHERE hard->>'storage' = '-1') AS unlimited
		FROM quota WHERE reference = 'project'`
	result := &ProjectAllocation{}
	if err := o.Raw(sql).QueryRow(result); err != nil {
		return nil, err
	}
	return result, nil
}
