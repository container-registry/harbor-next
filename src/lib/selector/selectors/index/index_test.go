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

package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/selector"
	"github.com/goharbor/harbor/src/lib/selector/selectors/doublestar"
	regexpselector "github.com/goharbor/harbor/src/lib/selector/selectors/regexp"
)

func TestIndexAdvertisesBothKinds(t *testing.T) {
	kinds := make(map[string][]string)
	for _, m := range Index() {
		kinds[m.Kind] = m.Decorations
	}

	require.Contains(t, kinds, doublestar.Kind)
	require.Contains(t, kinds, regexpselector.Kind)
	assert.ElementsMatch(t, kinds[doublestar.Kind], kinds[regexpselector.Kind])
}

func TestGetRegexpSelector(t *testing.T) {
	s, err := Get(regexpselector.Kind, regexpselector.Matches, `v\d+`, "")
	require.NoError(t, err)
	require.NotNil(t, s)

	selected, err := s.Select([]*selector.Candidate{
		{Repository: "redis", Tags: []string{"v1"}},
		{Repository: "redis", Tags: []string{"v1.0"}},
	})
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, []string{"v1"}, selected[0].Tags)
}

// TestGetRegexpSelectorInvalidPattern asserts construction still succeeds and
// the compile error is deferred to Select
func TestGetRegexpSelectorInvalidPattern(t *testing.T) {
	s, err := Get(regexpselector.Kind, regexpselector.Matches, "v(", "")
	require.NoError(t, err)

	_, err = s.Select([]*selector.Candidate{{Repository: "redis", Tags: []string{"v1"}}})
	assert.Error(t, err)
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name       string
		kind       string
		decoration string
		pattern    string
		wantErr    string
	}{
		{"doublestar untouched", doublestar.Kind, doublestar.Matches, "v*.*.*", ""},
		{"doublestar keeps regexp meaningless patterns", doublestar.Kind, doublestar.Matches, "v(", ""},
		{"regexp valid", regexpselector.Kind, regexpselector.Matches, `v\d+`, ""},
		{"regexp empty pattern", regexpselector.Kind, regexpselector.RepoMatches, "", ""},
		{"regexp invalid", regexpselector.Kind, regexpselector.Matches, "v(", "invalid regexp pattern"},
		{"regexp too long", regexpselector.Kind, regexpselector.Matches, longPattern(513), "limited to 512 characters"},
		{"unknown kind", "pcre", regexpselector.Matches, `v\d+`, "is not registered"},
		{"unknown decoration", regexpselector.Kind, "labelMatches", `v\d+`, "is not supported"},
		{"empty kind", "", regexpselector.Matches, `v\d+`, "empty selector kind or decoration"},
		{"empty decoration", regexpselector.Kind, "", `v\d+`, "empty selector kind or decoration"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.kind, c.decoration, c.pattern)
			if c.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

func longPattern(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
