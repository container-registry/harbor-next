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
	"time"

	"github.com/goharbor/harbor/src/lib/orm"
	model "github.com/goharbor/harbor/src/pkg/branding/models"
)

// DefaultBrandingJSON is the default branding configuration seeded on first access.
const DefaultBrandingJSON = `{"headerBgColor":{"darkMode":"","lightMode":""},"loginBgImg":"","loginTitle":"8gears Container Registry","product":{"name":"8gears Container Registry","logo":"https://container-registry.com/img/logo-white.svg","introduction":"8gears Container Registry is a Harbor-based container registry as a service offered and operated by the project's maintainer and contributors. Harbor is a CNCF-graded open-source container registry for securely managing container images and other OCI artifacts with policies, role-based access control, vulnerability scanning, and signing."}}`

// DAO defines the interface to access the branding data model
type DAO interface {
	// Get returns the branding config, seeding defaults on first access
	Get(ctx context.Context) (*model.Branding, error)

	// Update upserts the branding config
	Update(ctx context.Context, branding string) error
}

// New creates a default implementation for DAO
func New() DAO {
	return &dao{}
}

type dao struct{}

func (d *dao) Get(ctx context.Context) (*model.Branding, error) {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Atomic upsert: seed defaults if no row exists, otherwise do nothing.
	_, err = ormer.Raw(
		`INSERT INTO branding (id, config, update_time)
		 VALUES (1, ?, NOW())
		 ON CONFLICT (id) DO NOTHING`,
		DefaultBrandingJSON,
	).Exec()
	if err != nil {
		return nil, err
	}

	b := model.Branding{ID: 1}
	if err = ormer.Read(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (d *dao) Update(ctx context.Context, params string) error {
	ormer, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}

	branding := &model.Branding{
		ID:         1,
		Config:     params,
		UpdateTime: time.Now(),
	}

	existing := &model.Branding{ID: 1}
	err = ormer.Read(existing)

	if err == orm.ErrNoRows {
		_, err = ormer.Insert(branding)
	} else if err != nil {
		return err
	} else {
		_, err = ormer.Update(branding, "Config", "UpdateTime")
	}

	return err
}
