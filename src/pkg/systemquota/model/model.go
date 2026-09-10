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

package model

import "time"

// Table is the name of the table holding the global storage quota.
const Table = "system_quota"

// SingletonID is the only row id the table accepts (enforced by a CHECK constraint).
const SingletonID int64 = 1

// SystemQuota is the instance-wide storage limit.
type SystemQuota struct {
	ID         int64     `orm:"pk;column(id)" json:"id"`
	Hard       int64     `orm:"column(hard)" json:"hard"`
	Enforce    bool      `orm:"column(enforce)" json:"enforce"`
	UpdateTime time.Time `orm:"column(update_time);auto_now" json:"update_time"`
}

// TableName ...
func (s *SystemQuota) TableName() string {
	return Table
}

// ProjectAllocation summarizes the storage limits assigned to active projects.
type ProjectAllocation struct {
	// Allocated is the sum of finite project storage limits in bytes.
	Allocated int64
	// Unlimited is the number of projects whose storage limit is -1.
	Unlimited int64
}
