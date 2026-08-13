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

import (
	"time"

	"github.com/beego/beego/v2/client/orm"

	"github.com/goharbor/harbor/src/lib/errors"
)

func init() {
	orm.RegisterModel(&Branding{})
}

// Branding holds the branding configuration.
type Branding struct {
	ID         int64     `orm:"pk;column(id)" json:"id"`
	Config     string    `orm:"type(text);column(config)" json:"config"`
	UpdateTime time.Time `orm:"column(update_time);auto_now" json:"update_time"`
}

// TableName ...
func (r *Branding) TableName() string {
	return "branding"
}

// FromJSON parses branding from json data
func (r *Branding) FromJSON(jsonData string) error {
	if jsonData == "" {
		return errors.New("empty json data to parse")
	}
	r.Config = jsonData // ← fixed
	return nil
}

// Convert branding config to JSON string
func (r *Branding) ToJSON() (string, error) {
	if len(r.Config) == 0 {
		return "", errors.New("no config to marshal")
	}
	return string(r.Config), nil
}
