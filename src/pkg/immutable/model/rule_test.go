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
	"github.com/goharbor/harbor/src/lib/selector/selectors/doublestar"
	regexpselector "github.com/goharbor/harbor/src/lib/selector/selectors/regexp"
)

func ruleWith(tagSelector, repoSelector *Selector) *Metadata {
	return &Metadata{
		ID:        1,
		ProjectID: 1,
		Action:    "immutable",
		Template:  "immutable_template",
		TagSelectors: []*Selector{
			tagSelector,
		},
		ScopeSelectors: map[string][]*Selector{
			"repository": {repoSelector},
		},
	}
}

func doublestarRepoSelector() *Selector {
	return &Selector{
		Kind:       doublestar.Kind,
		Decoration: doublestar.RepoMatches,
		Pattern:    "**",
	}
}

func TestValidateImmutableRule(t *testing.T) {
	cases := []struct {
		name    string
		tag     *Selector
		wantErr string
	}{
		{
			name: "doublestar is untouched",
			tag: &Selector{
				Kind:       doublestar.Kind,
				Decoration: doublestar.Matches,
				Pattern:    "release-**",
			},
		},
		{
			name: "doublestar brace list stays valid",
			tag: &Selector{
				Kind:       doublestar.Kind,
				Decoration: doublestar.Matches,
				Pattern:    "{v1,v2,v3}",
			},
		},
		{
			name: "valid regex is accepted",
			tag: &Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    `v\d+\.\d+\.\d+`,
			},
		},
		{
			name: "invalid regex is rejected",
			tag: &Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    `v\d+(`,
			},
			wantErr: "invalid regex pattern",
		},
		{
			name: "over long regex is rejected",
			tag: &Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    strings.Repeat("a", regexpselector.MaxPatternLength+1),
			},
			wantErr: "limited to 512 characters",
		},
		{
			name: "unknown kind is rejected",
			tag: &Selector{
				Kind:       "pcre",
				Decoration: regexpselector.Matches,
				Pattern:    `v\d+`,
			},
			wantErr: "is not registered",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ruleWith(c.tag, doublestarRepoSelector()).ValidateImmutableRule()
			if c.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
			assert.True(t, errors.IsErr(err, errors.BadRequestCode), "want a bad request error, got %v", err)
		})
	}
}

func TestValidateImmutableRuleScopeSelectors(t *testing.T) {
	validTag := &Selector{
		Kind:       doublestar.Kind,
		Decoration: doublestar.Matches,
		Pattern:    "**",
	}

	m := ruleWith(validTag, &Selector{
		Kind:       regexpselector.Kind,
		Decoration: regexpselector.RepoExcludes,
		Pattern:    `.*-source`,
	})
	assert.NoError(t, m.ValidateImmutableRule())

	m = ruleWith(validTag, &Selector{
		Kind:       regexpselector.Kind,
		Decoration: regexpselector.RepoExcludes,
		Pattern:    `.*-source(`,
	})
	assert.Error(t, m.ValidateImmutableRule())
}
