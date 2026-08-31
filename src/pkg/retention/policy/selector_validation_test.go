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

package policy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/selector/selectors/doublestar"
	regexpselector "github.com/goharbor/harbor/src/lib/selector/selectors/regexp"
	"github.com/goharbor/harbor/src/pkg/retention/policy/rule"
)

func policyWithSelectors(tagSelector, repoSelector *rule.Selector) *Metadata {
	return &Metadata{
		Algorithm: AlgorithmOR,
		Rules: []rule.Metadata{
			{
				ID:           1,
				Priority:     1,
				Action:       "retain",
				Template:     "latestPushedK",
				Parameters:   rule.Parameters{"latestPushedK": 10},
				TagSelectors: []*rule.Selector{tagSelector},
				ScopeSelectors: map[string][]*rule.Selector{
					"repository": {repoSelector},
				},
			},
		},
		Trigger: &Trigger{
			Kind:     TriggerKindSchedule,
			Settings: map[string]any{TriggerSettingsCron: "0 0 0 * * *"},
		},
		Scope: &Scope{Level: ScopeLevelProject, Reference: 1},
	}
}

func doublestarRepoSelector() *rule.Selector {
	return &rule.Selector{
		Kind:       doublestar.Kind,
		Decoration: doublestar.RepoMatches,
		Pattern:    "**",
	}
}

func TestValidateRetentionPolicySelectors(t *testing.T) {
	cases := []struct {
		name    string
		tag     *rule.Selector
		wantErr string
	}{
		{
			name: "doublestar is untouched",
			tag: &rule.Selector{
				Kind:       doublestar.Kind,
				Decoration: doublestar.Matches,
				Pattern:    "v*.*.*",
			},
		},
		{
			name: "doublestar pattern that is not a valid regex stays valid",
			tag: &rule.Selector{
				Kind:       doublestar.Kind,
				Decoration: doublestar.Matches,
				Pattern:    "v{1,2,3}",
			},
		},
		{
			name: "valid regex is accepted",
			tag: &rule.Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    `v?\d+\.\d+\.\d+`,
				Extras:     `{"untagged": false}`,
			},
		},
		{
			name: "empty regex pattern is accepted",
			tag: &rule.Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    "",
			},
		},
		{
			name: "invalid regex is rejected",
			tag: &rule.Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    "v(",
			},
			wantErr: "invalid regex pattern",
		},
		{
			name: "over long regex is rejected",
			tag: &rule.Selector{
				Kind:       regexpselector.Kind,
				Decoration: regexpselector.Matches,
				Pattern:    strings.Repeat("a", regexpselector.MaxPatternLength+1),
			},
			wantErr: "limited to 512 characters",
		},
		{
			name: "unknown kind is rejected",
			tag: &rule.Selector{
				Kind:       "pcre",
				Decoration: regexpselector.Matches,
				Pattern:    `v\d+`,
			},
			wantErr: "is not registered",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := policyWithSelectors(c.tag, doublestarRepoSelector()).ValidateRetentionPolicy()
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

// TestValidateRetentionPolicyScopeSelectors asserts scope selectors go through
// the same validation as tag selectors
func TestValidateRetentionPolicyScopeSelectors(t *testing.T) {
	validTag := &rule.Selector{
		Kind:       doublestar.Kind,
		Decoration: doublestar.Matches,
		Pattern:    "**",
	}

	p := policyWithSelectors(validTag, &rule.Selector{
		Kind:       regexpselector.Kind,
		Decoration: regexpselector.RepoMatches,
		Pattern:    `library/.*`,
	})
	assert.NoError(t, p.ValidateRetentionPolicy())

	p = policyWithSelectors(validTag, &rule.Selector{
		Kind:       regexpselector.Kind,
		Decoration: regexpselector.RepoMatches,
		Pattern:    "library/[a-",
	})
	err := p.ValidateRetentionPolicy()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex pattern")
}

// TestValidateRetentionPolicyCronStillValidated guards the pre-existing cron
// check against the new selector loop
func TestValidateRetentionPolicyCronStillValidated(t *testing.T) {
	p := policyWithSelectors(&rule.Selector{
		Kind:       doublestar.Kind,
		Decoration: doublestar.Matches,
		Pattern:    "**",
	}, doublestarRepoSelector())

	p.Trigger.Settings = map[string]any{TriggerSettingsCron: "1 0 0 1 1 *"}
	assert.Error(t, p.ValidateRetentionPolicy())
}
