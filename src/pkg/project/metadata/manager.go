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

package metadata

import (
	"context"
	"maps"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/project/metadata/dao"
	"github.com/goharbor/harbor/src/pkg/project/metadata/models"
)

// Validator validates the effective project metadata before a write.
type Validator func(map[string]string) error

// Manager defines the operations that a project metadata manager should implement
type Manager interface {
	// Add metadatas for project specified by projectID
	Add(ctx context.Context, projectID int64, meta map[string]string) error

	// Delete metadatas whose keys are specified in parameter meta, if it is absent, delete all
	Delete(ctx context.Context, projectID int64, meta ...string) error

	// Update metadatas
	Update(ctx context.Context, projectID int64, meta map[string]string) error

	// AddWithValidation atomically validates and adds metadatas.
	AddWithValidation(ctx context.Context, projectID int64, meta map[string]string, validate Validator) error

	// DeleteWithValidation atomically validates the remaining metadatas and deletes meta.
	DeleteWithValidation(ctx context.Context, projectID int64, validate Validator, meta ...string) error

	// UpdateWithValidation atomically validates and updates metadatas, creating the missing ones.
	UpdateWithValidation(ctx context.Context, projectID int64, meta map[string]string, validate Validator) error

	// Get metadatas whose keys are specified in parameter meta, if it is absent, get all
	Get(ctx context.Context, projectID int64, meta ...string) (map[string]string, error)

	// List metadata according to the name and value
	List(ctx context.Context, name, value string) ([]*models.ProjectMetadata, error)
}

// New returns a default implementation of Manager
func New() Manager {
	return &manager{dao: dao.New()}
}

type manager struct {
	dao dao.DAO
}

// Add metadatas for project specified by projectID
func (m *manager) Add(ctx context.Context, projectID int64, meta map[string]string) error {
	h := func(ctx context.Context) error {
		for name, value := range meta {
			if _, err := m.dao.Create(ctx, projectID, name, value); err != nil {
				return err
			}
		}
		return nil
	}
	return orm.WithTransaction(h)(orm.SetTransactionOpNameToContext(ctx, "tx-add-project"))
}

// Delete metadatas whose keys are specified in parameter meta, if it is absent, delete all
func (m *manager) Delete(ctx context.Context, projectID int64, meta ...string) error {
	return m.dao.Delete(ctx, makeQuery(projectID, meta...))
}

// Update metadatas
func (m *manager) Update(ctx context.Context, projectID int64, meta map[string]string) error {
	if len(meta) == 0 {
		return nil
	}

	h := func(ctx context.Context) error {
		for name, value := range meta {
			if err := m.dao.Update(ctx, projectID, name, value); err != nil {
				return err
			}
		}

		return nil
	}

	return orm.WithTransaction(h)(orm.SetTransactionOpNameToContext(ctx, "tx-delete-project"))
}

// AddWithValidation atomically validates and adds metadatas.
func (m *manager) AddWithValidation(ctx context.Context, projectID int64, meta map[string]string, validate Validator) error {
	return m.writeWithValidation(ctx, projectID, validate, func(effective map[string]string) { maps.Copy(effective, meta) }, func(ctx context.Context, _ map[string]string) error {
		for name, value := range meta {
			if _, err := m.dao.Create(ctx, projectID, name, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteWithValidation atomically validates the metadata left after deleting meta and deletes it.
func (m *manager) DeleteWithValidation(ctx context.Context, projectID int64, validate Validator, meta ...string) error {
	return m.writeWithValidation(ctx, projectID, validate, func(effective map[string]string) {
		if len(meta) == 0 {
			clear(effective)
		}
		for _, name := range meta {
			delete(effective, name)
		}
	}, func(ctx context.Context, _ map[string]string) error {
		return m.Delete(ctx, projectID, meta...)
	})
}

// UpdateWithValidation atomically validates and updates metadatas, creating the missing ones.
func (m *manager) UpdateWithValidation(ctx context.Context, projectID int64, meta map[string]string, validate Validator) error {
	return m.writeWithValidation(ctx, projectID, validate, func(effective map[string]string) { maps.Copy(effective, meta) }, func(ctx context.Context, current map[string]string) error {
		for name, value := range meta {
			var err error
			if _, ok := current[name]; ok {
				err = m.dao.Update(ctx, projectID, name, value)
			} else {
				_, err = m.dao.Create(ctx, projectID, name, value)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// writeWithValidation validates the stored metadata after change and runs write
// while holding the project row lock.
func (m *manager) writeWithValidation(ctx context.Context, projectID int64, validate Validator,
	change func(effective map[string]string), write func(ctx context.Context, current map[string]string) error) error {
	h := func(ctx context.Context) error {
		o, err := orm.FromContext(ctx)
		if err != nil {
			return err
		}

		// Lock the parent row because metadata rows may not exist yet. All
		// validated metadata writes for a project serialize on this row. NO KEY
		// UPDATE still admits the KEY SHARE locks taken by metadata inserts
		// earlier in the same transactions, which FOR UPDATE would deadlock on.
		var ids []int64
		if _, err := o.Raw("SELECT project_id FROM project WHERE project_id = ? FOR NO KEY UPDATE", projectID).QueryRows(&ids); err != nil {
			return err
		}
		if len(ids) == 0 {
			return errors.NotFoundError(nil).WithMessagef("project %d not found", projectID)
		}

		current, err := m.Get(ctx, projectID)
		if err != nil {
			return err
		}
		effective := maps.Clone(current)
		if effective == nil {
			effective = map[string]string{}
		}
		change(effective)
		if err := validate(effective); err != nil {
			return err
		}
		return write(ctx, current)
	}

	return orm.WithTransaction(h)(orm.SetTransactionOpNameToContext(ctx, "tx-update-project-metadata"))
}

// Get metadatas whose keys are specified in parameter meta, if it is absent, get all
func (m *manager) Get(ctx context.Context, projectID int64, meta ...string) (map[string]string, error) {
	mds, err := m.dao.List(ctx, makeQuery(projectID, meta...))
	if err != nil {
		return nil, err
	}

	data := map[string]string{}
	for _, md := range mds {
		data[md.Name] = md.Value
	}

	return data, nil
}

// List metadata according to the name and value
func (m *manager) List(ctx context.Context, name string, value string) ([]*models.ProjectMetadata, error) {
	return m.dao.List(ctx, q.New(q.KeyWords{"name": name, "value": value}))
}

func makeQuery(projectID int64, meta ...string) *q.Query {
	kw := q.KeyWords{
		"project_id": projectID,
	}
	if len(meta) > 0 {
		var names []string
		names = append(names, meta...)
		kw["name__in"] = names
	}

	return q.New(kw)
}
