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

package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func TestDoFilterRepositoriesByKind(t *testing.T) {
	repositories := []*model.Repository{
		{Name: "library/hello-world"},
		{Name: "library/busybox"},
		{Name: "library/hello-world-extra"},
		{Name: "test/hello-world"},
	}

	cases := []struct {
		name    string
		kind    string
		pattern string
		want    []string
		wantErr bool
	}{
		{
			name:    "doublestar keeps its meaning",
			kind:    "",
			pattern: "library/**",
			want:    []string{"library/hello-world", "library/busybox", "library/hello-world-extra"},
		},
		{
			name:    "regex matches the whole repository name",
			kind:    model.FilterKindRegex,
			pattern: "library/.*",
			want:    []string{"library/hello-world", "library/busybox", "library/hello-world-extra"},
		},
		{
			// a substring match would also return "library/hello-world-extra"
			name:    "regex is anchored",
			kind:    model.FilterKindRegex,
			pattern: ".*/hello-world",
			want:    []string{"library/hello-world", "test/hello-world"},
		},
		{
			name:    "regex alternation",
			kind:    model.FilterKindRegex,
			pattern: "library/(busybox|hello-world)",
			want:    []string{"library/hello-world", "library/busybox"},
		},
		{
			name:    "invalid regex is reported",
			kind:    model.FilterKindRegex,
			pattern: "foo)|(?:bar",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			filtered, err := DoFilterRepositories(repositories, []*model.Filter{
				{
					Type:  model.FilterTypeName,
					Value: c.pattern,
					Kind:  c.kind,
				},
			})
			if c.wantErr {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			var names []string
			for _, repository := range filtered {
				names = append(names, repository.Name)
			}
			assert.Equal(t, c.want, names)
		})
	}
}
