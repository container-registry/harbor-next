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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/controller/quota"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	pkgproject "github.com/goharbor/harbor/src/pkg/project"
	"github.com/goharbor/harbor/src/pkg/project/models"
	"github.com/goharbor/harbor/src/pkg/quota/types"
)

const configOverwriteJSONEnv = "CONFIG_OVERWRITE_JSON"

type defaultProjectInit struct {
	projectCtl   project.Controller
	quotaCtl     quota.Controller
	quotaEnabled func(context.Context) bool
	storageLimit func(context.Context) (int64, error)
	setDefault   func(context.Context, string) error
	getDefault   func(context.Context) string
	// existsLocked share-locks the project row until the surrounding transaction ends
	existsLocked func(context.Context, string) (bool, error)
	inTx         func(context.Context, func(context.Context) error) error
}

func newDefaultProjectInit() *defaultProjectInit {
	return &defaultProjectInit{
		projectCtl:   project.Ctl,
		quotaCtl:     quota.Ctl,
		quotaEnabled: config.QuotaPerProjectEnable,
		storageLimit: func(ctx context.Context) (int64, error) {
			setting, err := config.QuotaSetting(ctx)
			if err != nil {
				return 0, err
			}
			return setting.StoragePerProject, nil
		},
		setDefault: func(ctx context.Context, name string) error {
			return config.GetCfgManager(ctx).UpdateConfig(ctx, map[string]any{common.DefaultProjectName: name})
		},
		getDefault:   config.DefaultProjectName,
		existsLocked: pkgproject.ExistsLocked,
		inTx: func(ctx context.Context, f func(context.Context) error) error {
			return orm.WithTransaction(f)(ctx)
		},
	}
}

// ensureForOverwrite creates the project CONFIG_OVERWRITE_JSON names so its validation passes
func (d *defaultProjectInit) ensureForOverwrite(ctx context.Context) error {
	raw, ok := os.LookupEnv(configOverwriteJSONEnv)
	if !ok {
		return nil
	}
	if _, pinned := os.LookupEnv(common.DefaultProjectNameEnv); pinned {
		// OverwriteConfig drops the key, DEFAULT_PROJECT_NAME wins
		return nil
	}
	// OverwriteConfig reports malformed JSON
	if !json.Valid([]byte(raw)) {
		return nil
	}
	cfgs := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &cfgs); err != nil {
		return err
	}
	name, ok := cfgs[common.DefaultProjectName].(string)
	if !ok || !pkgproject.IsValidName(name) {
		return nil
	}
	return d.ensure(ctx, name)
}

// applyEnv pins DEFAULT_PROJECT_NAME, creating the project if missing
func (d *defaultProjectInit) applyEnv(ctx context.Context) error {
	name, ok := os.LookupEnv(common.DefaultProjectNameEnv)
	if !ok {
		return nil
	}
	if name != "" && !pkgproject.IsValidName(name) {
		return fmt.Errorf("%s=%q is not a valid project name", common.DefaultProjectNameEnv, name)
	}
	// one transaction, so a concurrent delete waits until the project is stored as default
	return d.inTx(ctx, func(ctx context.Context) error {
		if name != "" {
			if err := d.ensure(ctx, name); err != nil {
				return err
			}
		}
		return d.setDefault(ctx, name)
	})
}

func (d *defaultProjectInit) ensure(ctx context.Context, name string) error {
	exists, err := d.existsLocked(ctx, name)
	if err != nil || exists {
		return err
	}
	_, err = d.projectCtl.Create(ctx, &models.Project{
		Name:     name,
		OwnerID:  adminUserID,
		Metadata: map[string]string{"public": "false"},
	})
	if err == nil {
		log.Infof("created default project %q", name)
		return nil
	}
	// another core replica may have created it first
	if exists, existsErr := d.existsLocked(ctx, name); existsErr != nil || !exists {
		return fmt.Errorf("failed to create default project %q: %w", name, err)
	}
	return nil
}

// reconcile runs once the effective config is in place: it warns about a default
// that no longer exists and repairs a missing quota with the effective quota settings
func (d *defaultProjectInit) reconcile(ctx context.Context) error {
	name := d.getDefault(ctx)
	if name == "" {
		return nil
	}
	exists, err := d.projectCtl.Exists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		log.Warningf("default project %q does not exist, pushes and pulls without a project fail until Default Project is changed in System Settings", name)
		return nil
	}
	return d.ensureQuota(ctx, name)
}

func (d *defaultProjectInit) ensureQuota(ctx context.Context, name string) error {
	if !d.quotaEnabled(ctx) {
		return nil
	}
	p, err := d.projectCtl.GetByName(ctx, name)
	if err != nil {
		return err
	}
	ref := quota.ReferenceID(p.ProjectID)
	_, err = d.quotaCtl.GetByRef(ctx, quota.ProjectReference, ref)
	if err == nil || !errors.IsNotFoundErr(err) {
		return err
	}
	limit, err := d.storageLimit(ctx)
	if err != nil {
		return err
	}
	if _, err := d.quotaCtl.Create(ctx, quota.ProjectReference, ref, types.ResourceList{types.ResourceStorage: limit}); err != nil {
		// another core replica may have created it first
		if _, getErr := d.quotaCtl.GetByRef(ctx, quota.ProjectReference, ref); getErr != nil {
			return fmt.Errorf("failed to create quota for default project %q: %w", name, err)
		}
	}
	return nil
}
