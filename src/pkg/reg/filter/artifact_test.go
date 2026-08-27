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

func tagsOf(artifacts []*model.Artifact) [][]string {
	var tags [][]string
	for _, artifact := range artifacts {
		tags = append(tags, artifact.Tags)
	}
	return tags
}

func TestDoFilterArtifactsByTagKind(t *testing.T) {
	artifacts := []*model.Artifact{
		{Tags: []string{"v1.0", "v1.0-rc1"}},
		{Tags: []string{"latest"}},
		{Tags: nil},
	}

	cases := []struct {
		name       string
		kind       string
		pattern    string
		decoration string
		want       [][]string
		wantErr    bool
	}{
		{
			name:    "doublestar keeps its meaning",
			pattern: "v1.*",
			want:    [][]string{{"v1.0", "v1.0-rc1"}},
		},
		{
			// a substring match would also keep "v1.0-rc1"
			name:    "regex is anchored",
			kind:    model.FilterKindRegex,
			pattern: `v\d+\.\d+`,
			want:    [][]string{{"v1.0"}},
		},
		{
			name:    "regex keeps every matching tag",
			kind:    model.FilterKindRegex,
			pattern: "v1.*",
			want:    [][]string{{"v1.0", "v1.0-rc1"}},
		},
		{
			name:       "regex excludes",
			kind:       model.FilterKindRegex,
			pattern:    `v\d+\.\d+`,
			decoration: model.Excludes,
			// the untagged artifact doesn't match the regex, so excluding keeps it
			want: [][]string{{"v1.0-rc1"}, {"latest"}, nil},
		},
		{
			// the pattern is never run against the empty tag, an untagged artifact
			// isn't a match even for a regex that would accept an empty string
			name:    "untagged artifact never matches a regex",
			kind:    model.FilterKindRegex,
			pattern: ".*",
			want:    [][]string{{"v1.0", "v1.0-rc1"}, {"latest"}},
		},
		{
			name:    "doublestar still matches the untagged artifact",
			pattern: "**",
			want:    [][]string{{"v1.0", "v1.0-rc1"}, {"latest"}, nil},
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
			filtered, err := DoFilterArtifacts(artifacts, []*model.Filter{
				{
					Type:       model.FilterTypeTag,
					Value:      c.pattern,
					Kind:       c.kind,
					Decoration: c.decoration,
				},
			})
			if c.wantErr {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, c.want, tagsOf(filtered))
		})
	}
}

func TestDoFilterArtifactsByLabelKind(t *testing.T) {
	artifacts := []*model.Artifact{
		{Tags: []string{"a"}, Labels: []string{"prod", "team-a"}},
		{Tags: []string{"b"}, Labels: []string{"staging", "team-b"}},
		{Tags: []string{"c"}, Labels: []string{"production"}},
	}

	cases := []struct {
		name       string
		kind       string
		labels     []string
		decoration string
		want       [][]string
	}{
		{
			name:   "doublestar labels stay an exact match",
			labels: []string{"prod"},
			want:   [][]string{{"a"}},
		},
		{
			// a substring match would also keep the artifact labeled "production"
			name:   "regex labels are anchored",
			kind:   model.FilterKindRegex,
			labels: []string{"prod"},
			want:   [][]string{{"a"}},
		},
		{
			name:   "regex labels match any label of the artifact",
			kind:   model.FilterKindRegex,
			labels: []string{"prod.*"},
			want:   [][]string{{"a"}, {"c"}},
		},
		{
			name:   "every configured label has to match",
			kind:   model.FilterKindRegex,
			labels: []string{"prod.*", "team-[ab]"},
			want:   [][]string{{"a"}},
		},
		{
			name:       "regex labels excludes",
			kind:       model.FilterKindRegex,
			labels:     []string{"prod.*"},
			decoration: model.Excludes,
			want:       [][]string{{"b"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			filtered, err := DoFilterArtifacts(artifacts, []*model.Filter{
				{
					Type:       model.FilterTypeLabel,
					Value:      c.labels,
					Kind:       c.kind,
					Decoration: c.decoration,
				},
			})
			require.Nil(t, err)
			assert.Equal(t, c.want, tagsOf(filtered))
		})
	}
}
