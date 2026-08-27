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
	"github.com/beego/beego/v2/core/validation"

	"github.com/goharbor/harbor/src/lib/errors"
	selectorindex "github.com/goharbor/harbor/src/lib/selector/selectors/index"
)

// Metadata of the immutable rule
type Metadata struct {
	// UUID of rule
	ID int64 `json:"id"`

	// ProjectID of project
	ProjectID int64 `json:"project_id"`

	// Disabled rule
	Disabled bool `json:"disabled"`

	// Priority of rule when doing calculating
	Priority int `json:"priority"`

	// Action of the rule performs
	// "immutable"
	Action string `json:"action" valid:"Required"`

	// Template ID
	Template string `json:"template" valid:"Required"`

	// The parameters of this rule
	Parameters Parameters `json:"params" valid:"Required"`

	// TagSelectors attached to the rule for filtering tags
	TagSelectors []*Selector `json:"tag_selectors" valid:"Required"`

	// Selector attached to the rule for filtering scope (e.g: repositories or namespaces)
	ScopeSelectors map[string][]*Selector `json:"scope_selectors" valid:"Required"`
}

// ValidateImmutableRule rejects a rule carrying a selector that could not be
// built when the rule is evaluated
func (m *Metadata) ValidateImmutableRule() error {
	for _, ts := range m.TagSelectors {
		if err := validateSelector(ts); err != nil {
			return err
		}
	}
	for _, ss := range m.ScopeSelectors {
		for _, s := range ss {
			if err := validateSelector(s); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateSelector(s *Selector) error {
	if s == nil {
		return nil
	}

	if err := selectorindex.Validate(s.Kind, s.Decoration, s.Pattern); err != nil {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("invalid immutable rule selector: %v", err)
	}

	return nil
}

// Valid Valid
func (m *Metadata) Valid(v *validation.Validation) {
	for _, ts := range m.TagSelectors {
		if pass, _ := v.Valid(ts); !pass {
			return
		}
	}
	for _, ss := range m.ScopeSelectors {
		for _, s := range ss {
			if pass, _ := v.Valid(s); !pass {
				return
			}
		}
	}
}

// Selector to narrow down the list
type Selector struct {
	// Kind of the selector
	// "doublestar" or "regexp"
	Kind string `json:"kind" valid:"Required;Match(/^(doublestar|regexp)$/)"`

	// Decorated the selector
	// for "doublestar" and "regexp" : "matching" and "excluding"
	Decoration string `json:"decoration" valid:"Required"`

	// Param for the selector
	Pattern string `json:"pattern" valid:"Required"`
}

// Parameters of rule, indexed by the key
type Parameters map[string]Parameter

// Parameter of rule
type Parameter interface{}
