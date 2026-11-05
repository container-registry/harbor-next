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

package project

import (
	"context"

	"github.com/goharbor/harbor/src/lib/orm"
)

// WithDeleteLock runs f in a transaction holding a row lock on the project, so a
// concurrent change of the default project to this project waits for the delete
func WithDeleteLock(ctx context.Context, projectID int64, f func(context.Context) error) error {
	return orm.WithTransaction(func(ctx context.Context) error {
		o, err := orm.FromContext(ctx)
		if err != nil {
			return err
		}
		var ids []int64
		if _, err := o.Raw("SELECT project_id FROM project WHERE project_id = ? FOR UPDATE", projectID).QueryRows(&ids); err != nil {
			return err
		}
		return f(ctx)
	})(ctx)
}

// ExistsLocked reports whether a live project with the name exists and, inside a
// transaction, share-locks its row until commit so a concurrent delete waits
func ExistsLocked(ctx context.Context, name string) (bool, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return false, err
	}
	var ids []int64
	if _, err := o.Raw("SELECT project_id FROM project WHERE name = ? AND NOT deleted FOR SHARE", name).QueryRows(&ids); err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}
