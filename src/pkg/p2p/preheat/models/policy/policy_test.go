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

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/errors"
)

// PolicyTestSuite is a test suite for policy schema.
type PolicyTestSuite struct {
	suite.Suite

	schema *Schema
}

// TestPolicy is the entry method of running PolicyTestSuite.
func TestPolicy(t *testing.T) {
	suite.Run(t, &PolicyTestSuite{})
}

// SetupSuite prepares the env for PolicyTestSuite.
func (p *PolicyTestSuite) SetupSuite() {
	p.schema = &Schema{}
	p.schema.Trigger = &Trigger{}
}

// TearDownSuite clears the env for PolicyTestSuite.
func (p *PolicyTestSuite) TearDownSuite() {
	p.schema = nil
}

// TestValidatePreheatPolicy tests the ValidatePreheatPolicy method
func (p *PolicyTestSuite) TestValidatePreheatPolicy() {
	// manual trigger
	p.schema.Trigger.Type = TriggerTypeManual
	p.NoError(p.schema.ValidatePreheatPolicy())

	// event trigger
	p.schema.Trigger.Type = TriggerTypeEventBased
	p.NoError(p.schema.ValidatePreheatPolicy())

	// scheduled trigger
	p.schema.Trigger.Type = TriggerTypeScheduled
	// cron string is empty
	p.schema.Trigger.Settings.Cron = ""
	p.NoError(p.schema.ValidatePreheatPolicy())
	// the 1st field of cron string is not 0
	p.schema.Trigger.Settings.Cron = "1 0 0 1 1 *"
	p.Error(p.schema.ValidatePreheatPolicy())
	// valid cron string
	p.schema.Trigger.Settings.Cron = "0 0 0 1 1 *"
	p.NoError(p.schema.ValidatePreheatPolicy())
}

// TestValidateFilterKind tests the pattern engine validation of the policy filters
func (p *PolicyTestSuite) TestValidateFilterKind() {
	cases := []struct {
		name    string
		filter  *Filter
		wantErr bool
	}{
		{
			name:   "absent kind",
			filter: &Filter{Type: FilterTypeRepository, Value: "**"},
		},
		{
			name:   "explicit doublestar kind",
			filter: &Filter{Type: FilterTypeTag, Value: "prod*", Kind: FilterKindDoublestar},
		},
		{
			name:   "regex repository",
			filter: &Filter{Type: FilterTypeRepository, Value: "library/.*", Kind: FilterKindRegex},
		},
		{
			name:   "regex tag",
			filter: &Filter{Type: FilterTypeTag, Value: `v\d+\.\d+`, Kind: FilterKindRegex},
		},
		{
			name:   "empty regex pattern",
			filter: &Filter{Type: FilterTypeTag, Value: "", Kind: FilterKindRegex},
		},
		{
			name:    "unknown kind",
			filter:  &Filter{Type: FilterTypeTag, Value: "**", Kind: "glob"},
			wantErr: true,
		},
		{
			name:    "kind on a label filter",
			filter:  &Filter{Type: FilterTypeLabel, Value: "prod", Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "kind on a signature filter",
			filter:  &Filter{Type: FilterTypeSignature, Value: true, Kind: FilterKindDoublestar},
			wantErr: true,
		},
		{
			name:    "kind on a vulnerability filter",
			filter:  &Filter{Type: FilterTypeVulnerability, Value: 3, Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "invalid regex",
			filter:  &Filter{Type: FilterTypeTag, Value: "[", Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "regex escaping the anchoring",
			filter:  &Filter{Type: FilterTypeTag, Value: "foo)|(?:bar", Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:    "regex longer than the pattern limit",
			filter:  &Filter{Type: FilterTypeTag, Value: strings.Repeat("a", 513), Kind: FilterKindRegex},
			wantErr: true,
		},
		{
			name:   "regex at the pattern limit",
			filter: &Filter{Type: FilterTypeTag, Value: strings.Repeat("a", 512), Kind: FilterKindRegex},
		},
		{
			name:    "regex value that isn't a string",
			filter:  &Filter{Type: FilterTypeTag, Value: 100, Kind: FilterKindRegex},
			wantErr: true,
		},
	}

	for _, c := range cases {
		p.Run(c.name, func() {
			s := &Schema{Filters: []*Filter{c.filter}, Trigger: &Trigger{Type: TriggerTypeManual}}
			err := s.ValidatePreheatPolicy()
			if !c.wantErr {
				p.NoError(err)
				return
			}
			p.Error(err)
			p.True(errors.IsErr(err, errors.BadRequestCode), "error is a bad request: %v", err)
		})
	}
}

// TestFilterKindRoundTrip tests that the kind survives an encode/decode cycle and that a
// policy stored without a kind keeps decoding
func (p *PolicyTestSuite) TestFilterKindRoundTrip() {
	s := &Schema{
		Filters: []*Filter{
			{Type: FilterTypeRepository, Value: "library/.*", Kind: FilterKindRegex},
			{Type: FilterTypeTag, Value: "**"},
		},
		Trigger: &Trigger{Type: TriggerTypeManual},
	}
	p.NoError(s.Encode())
	p.Equal(`[{"type":"repository","value":"library/.*","kind":"regex"},{"type":"tag","value":"**"}]`, s.FiltersStr)

	// a policy stored before the kind existed decodes into the doublestar default
	stored := &Schema{
		FiltersStr: `[{"type":"repository","value":"**"},{"type":"tag","value":"**"},{"type":"label","value":"test"}]`,
		TriggerStr: `{"type":"manual","trigger_setting":{"cron":""}}`,
	}
	p.NoError(stored.Decode())
	p.Len(stored.Filters, 3)
	for _, f := range stored.Filters {
		p.Empty(f.Kind)
	}
	p.NoError(stored.ValidatePreheatPolicy())

	decoded := &Schema{FiltersStr: s.FiltersStr, TriggerStr: `{"type":"manual","trigger_setting":{"cron":""}}`}
	p.NoError(decoded.Decode())
	p.Equal(FilterKindRegex, decoded.Filters[0].Kind)
	p.Empty(decoded.Filters[1].Kind)
}

// TestDecode tests decode.
func (p *PolicyTestSuite) TestDecode() {
	s := &Schema{
		ID:            100,
		Name:          "test-for-decode",
		Description:   "",
		ProjectID:     1,
		ProviderID:    1,
		Filters:       nil,
		FiltersStr:    "[{\"type\":\"repository\",\"value\":\"**\"},{\"type\":\"tag\",\"value\":\"**\"},{\"type\":\"label\",\"value\":\"test\"}]",
		Trigger:       nil,
		TriggerStr:    "{\"type\":\"event_based\",\"trigger_setting\":{\"cron\":\"\"}}",
		Enabled:       false,
		ExtraAttrsStr: "{\"key\":\"value\"}",
	}
	p.NoError(s.Decode())
	p.Len(s.Filters, 3)
	p.NotNil(s.Trigger)

	p.Equal(map[string]any{"key": "value"}, s.ExtraAttrs)

	// invalid filter or trigger
	s.FiltersStr = ""
	s.TriggerStr = "invalid"
	p.Error(s.Decode())

	s.FiltersStr = "invalid"
	s.TriggerStr = ""
	p.Error(s.Decode())
}

// TestEncode tests encode.
func (p *PolicyTestSuite) TestEncode() {
	s := &Schema{
		ID:          101,
		Name:        "test-for-encode",
		Description: "",
		ProjectID:   2,
		ProviderID:  2,
		Filters: []*Filter{
			{
				Type:  FilterTypeRepository,
				Value: "**",
			},
			{
				Type:  FilterTypeTag,
				Value: "**",
			},
			{
				Type:  FilterTypeLabel,
				Value: "test",
			},
		},
		FiltersStr: "",
		Trigger: &Trigger{
			Type: "event_based",
		},
		TriggerStr: "",
		Enabled:    false,
		ExtraAttrs: map[string]any{
			"key": "value",
		},
	}
	p.NoError(s.Encode())
	p.Equal(`[{"type":"repository","value":"**"},{"type":"tag","value":"**"},{"type":"label","value":"test"}]`, s.FiltersStr)
	p.Equal(`{"type":"event_based","trigger_setting":{}}`, s.TriggerStr)
	p.Equal(`{"key":"value"}`, s.ExtraAttrsStr)
}
