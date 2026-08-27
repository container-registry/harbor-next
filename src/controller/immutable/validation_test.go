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

package immutable

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/lib/selector/selectors/doublestar"
	regexpselector "github.com/goharbor/harbor/src/lib/selector/selectors/regexp"
	"github.com/goharbor/harbor/src/pkg/immutable/model"
)

// fakeManager records whether the controller reached the persistence layer
type fakeManager struct {
	created bool
	updated bool
	stored  *model.Metadata
}

func (f *fakeManager) CreateImmutableRule(_ context.Context, _ *model.Metadata) (int64, error) {
	f.created = true
	return 1, nil
}

func (f *fakeManager) UpdateImmutableRule(_ context.Context, _ int64, _ *model.Metadata) error {
	f.updated = true
	return nil
}

func (f *fakeManager) EnableImmutableRule(_ context.Context, _ int64, _ bool) error {
	f.updated = true
	return nil
}

func (f *fakeManager) GetImmutableRule(_ context.Context, _ int64) (*model.Metadata, error) {
	return f.stored, nil
}

func (f *fakeManager) Count(_ context.Context, _ *q.Query) (int64, error) {
	return 0, nil
}

func (f *fakeManager) ListImmutableRules(_ context.Context, _ *q.Query) ([]*model.Metadata, error) {
	return nil, nil
}

func (f *fakeManager) DeleteImmutableRule(_ context.Context, _ int64) error {
	return nil
}

func ruleWithTagPattern(kind, pattern string) *model.Metadata {
	return &model.Metadata{
		ID:        1,
		ProjectID: 1,
		Action:    "immutable",
		Template:  "immutable_template",
		TagSelectors: []*model.Selector{
			{
				Kind:       kind,
				Decoration: "matches",
				Pattern:    pattern,
			},
		},
		ScopeSelectors: map[string][]*model.Selector{
			"repository": {
				{
					Kind:       doublestar.Kind,
					Decoration: doublestar.RepoMatches,
					Pattern:    "**",
				},
			},
		},
	}
}

func TestCreateImmutableRuleValidatesSelectors(t *testing.T) {
	mgr := &fakeManager{}
	ctr := NewAPIController(mgr)

	id, err := ctr.CreateImmutableRule(context.TODO(), ruleWithTagPattern(regexpselector.Kind, `v\d+\.\d+\.\d+`))
	require.NoError(t, err)
	assert.Equal(t, int64(1), id)
	assert.True(t, mgr.created)

	mgr = &fakeManager{}
	ctr = NewAPIController(mgr)
	_, err = ctr.CreateImmutableRule(context.TODO(), ruleWithTagPattern(regexpselector.Kind, "v("))
	require.Error(t, err)
	assert.True(t, errors.IsErr(err, errors.BadRequestCode))
	assert.False(t, mgr.created, "a rejected rule must not reach the manager")
}

func TestUpdateImmutableRuleValidatesSelectors(t *testing.T) {
	stored := ruleWithTagPattern(doublestar.Kind, "**")
	mgr := &fakeManager{stored: stored}
	ctr := NewAPIController(mgr)

	err := ctr.UpdateImmutableRule(context.TODO(), 1, ruleWithTagPattern(regexpselector.Kind, `v\d+`))
	require.NoError(t, err)
	assert.True(t, mgr.updated)

	mgr = &fakeManager{stored: stored}
	ctr = NewAPIController(mgr)
	err = ctr.UpdateImmutableRule(context.TODO(), 1, ruleWithTagPattern(regexpselector.Kind, "[a-"))
	require.Error(t, err)
	assert.True(t, errors.IsErr(err, errors.BadRequestCode))
	assert.False(t, mgr.updated, "a rejected rule must not reach the manager")
}

// TestDoublestarRulesUnaffected guards the existing behavior
func TestDoublestarRulesUnaffected(t *testing.T) {
	mgr := &fakeManager{}
	ctr := NewAPIController(mgr)

	_, err := ctr.CreateImmutableRule(context.TODO(), ruleWithTagPattern(doublestar.Kind, "{v1,v2}"))
	require.NoError(t, err)
	assert.True(t, mgr.created)
}
