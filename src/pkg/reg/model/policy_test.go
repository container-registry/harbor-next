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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/errors"
)

func TestFilterValidate(t *testing.T) {
	cases := []struct {
		name    string
		filter  *Filter
		wantErr bool
	}{
		{
			name:   "name filter without kind",
			filter: &Filter{Type: FilterTypeName, Value: "library/**"},
		},
		{
			name:   "name filter with explicit doublestar kind",
			filter: &Filter{Type: FilterTypeName, Value: "library/**", Kind: FilterKindDoublestar},
		},
		{
			name:   "name filter with regex kind",
			filter: &Filter{Type: FilterTypeName, Value: "library/.*", Kind: FilterKindRegex},
		},
		{
			name:   "tag filter with regex kind",
			filter: &Filter{Type: FilterTypeTag, Value: `v\d+\.\d+`, Kind: FilterKindRegex, Decoration: Excludes},
		},
		{
			name:   "label filter with regex kind",
			filter: &Filter{Type: FilterTypeLabel, Value: []any{"prod-.*", "team-[ab]"}, Kind: FilterKindRegex},
		},
		{
			name:   "empty regex matches everything",
			filter: &Filter{Type: FilterTypeTag, Value: "", Kind: FilterKindRegex},
		},
		{
			name:    "unknown kind",
			filter:  &Filter{Type: FilterTypeName, Value: "library/**", Kind: "glob"},
			wantErr: true,
		},
		{
			name:    "resource filter rejects kind",
			filter:  &Filter{Type: FilterTypeResource, Value: ResourceTypeImage, Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "resource filter rejects the doublestar kind as well",
			filter:  &Filter{Type: FilterTypeResource, Value: ResourceTypeImage, Kind: FilterKindDoublestar},
			wantErr: true,
		},
		{
			name:    "invalid regex",
			filter:  &Filter{Type: FilterTypeTag, Value: "[a-", Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			// wrapping this one for anchoring turns it into a valid unanchored
			// expression, it has to be rejected on its own
			name:    "regex escaping the anchoring",
			filter:  &Filter{Type: FilterTypeName, Value: "foo)|(?:bar", Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "regex over the length limit",
			filter:  &Filter{Type: FilterTypeTag, Value: strings.Repeat("a", 513), Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "invalid regex in a label filter",
			filter:  &Filter{Type: FilterTypeLabel, Value: []any{"prod-.*", "foo)|(?:bar"}, Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			// the value is a doublestar pattern, it isn't compiled
			name:   "invalid regex is a valid doublestar value",
			filter: &Filter{Type: FilterTypeTag, Value: "[a-", Kind: FilterKindDoublestar},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.filter.Validate()
			if !c.wantErr {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.True(t, errors.IsErr(err, errors.BadRequestCode), err.Error())
		})
	}
}
