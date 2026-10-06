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

//go:build db

package blob

import (
	"context"
	"time"

	"github.com/goharbor/harbor/src/lib/orm"
)

// backdateBlob rewinds a blob's update_time so tests can cross the GC
// half-window staleness boundary checked by shouldTouchNone.
func backdateBlob(ctx context.Context, id int64, age time.Duration) error {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}
	_, err = o.Raw("UPDATE blob SET update_time = ? WHERE id = ?", time.Now().Add(-age), id).Exec()
	return err
}
