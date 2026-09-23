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
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/pattern"
)

// const definition
const (
	FilterTypeResource = "resource"
	FilterTypeName     = "name"
	FilterTypeTag      = "tag"
	FilterTypeLabel    = "label"

	TriggerTypeManual     = "manual"
	TriggerTypeScheduled  = "scheduled"
	TriggerTypeEventBased = "event_based"

	// Matches [pattern] for tag (default)
	Matches = "matches"
	// Excludes [pattern] for tag
	Excludes = "excludes"

	// FilterKindDoublestar interprets the filter value as a doublestar pattern, the default
	FilterKindDoublestar = pattern.KindDoublestar
	// FilterKindRegex interprets the filter value as a regular expression matching the whole
	// value, the same engine the retention, immutability and proxy cache filters use
	FilterKindRegex = pattern.KindRegex
)

// Filter holds the info of the filter
type Filter struct {
	Type       string `json:"type"`
	Value      any    `json:"value"`
	Decoration string `json:"decoration,omitempty"`
	// Kind selects the pattern engine used for Value, empty means FilterKindDoublestar
	Kind string `json:"kind,omitempty"`
}

func (f *Filter) Validate() error {
	if f.Kind != "" && f.Kind != FilterKindDoublestar && f.Kind != FilterKindRegex {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("invalid filter kind: %s", f.Kind)
	}

	switch f.Type {
	case FilterTypeResource, FilterTypeName, FilterTypeTag:
		value, ok := f.Value.(string)
		if !ok {
			return errors.New(nil).WithCode(errors.BadRequestCode).
				WithMessage("the type of filter value isn't string")
		}
		if f.Type == FilterTypeResource {
			rt := value
			if !(rt == ResourceTypeArtifact || rt == ResourceTypeImage) {
				return errors.New(nil).WithCode(errors.BadRequestCode).
					WithMessagef("invalid resource filter: %s", value)
			}
			if f.Kind != "" {
				return errors.New(nil).WithCode(errors.BadRequestCode).
					WithMessage("only name, tag and label filter support kind")
			}
		}
		if f.Type == FilterTypeName || f.Type == FilterTypeResource {
			if f.Decoration != "" {
				return errors.New(nil).WithCode(errors.BadRequestCode).
					WithMessage("only tag and label filter support decoration")
			}
		}
		if err := f.validatePattern(value); err != nil {
			return err
		}
	case FilterTypeLabel:
		labels, ok := f.Value.([]any)
		if !ok {
			return errors.New(nil).WithCode(errors.BadRequestCode).
				WithMessage("the type of label filter value isn't string slice")
		}
		for _, label := range labels {
			value, ok := label.(string)
			if !ok {
				return errors.New(nil).WithCode(errors.BadRequestCode).
					WithMessage("the type of label filter value isn't string slice")
			}
			if err := f.validatePattern(value); err != nil {
				return err
			}
		}
	default:
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessage("invalid filter type")
	}

	if f.Decoration != "" && f.Decoration != Matches && f.Decoration != Excludes {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("invalid filter decoration, :%s", f.Decoration)
	}

	return nil
}

// validatePattern checks a single filter value against the pattern engine selected by the kind
func (f *Filter) validatePattern(value string) error {
	if f.Kind != FilterKindRegex {
		return nil
	}
	if err := pattern.ValidateRegex(value); err != nil {
		return errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("invalid regex filter value %q: %v", value, err)
	}
	return nil
}

// Trigger holds info for a trigger
type Trigger struct {
	Type     string           `json:"type"`
	Settings *TriggerSettings `json:"trigger_settings"`
}

// TriggerSettings is the setting about the trigger
type TriggerSettings struct {
	Cron string `json:"cron"`
}
