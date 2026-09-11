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

package orm

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
)

func TestValidateFilterValue(t *testing.T) {
	var (
		timeField    = reflect.TypeOf(time.Time{})
		timePtrField = reflect.TypeOf(&time.Time{})
		intField     = reflect.TypeOf(int64(0))
		floatField   = reflect.TypeOf(float32(0))
		boolField    = reflect.TypeOf(false)
		stringField  = reflect.TypeOf("")
	)
	ts, err := time.Parse(time.RFC3339, "2026-01-02T03:04:05Z")
	assert.NoError(t, err)

	cases := []struct {
		name      string
		fieldType reflect.Type
		value     any
		wantErr   bool
	}{
		// timestamp columns
		{"time range with time operands", timeField, q.NewRange(ts, ts), false},
		{"time range with parseable string operands", timeField, q.NewRange("2020-01-01T00:00:00", "2021-01-01T00:00:00"), false},
		{"time range with date-only operands", timeField, q.NewRange("2020-01-01", "2021-01-01"), false},
		{"time range with open lower bound", timeField, q.NewRange(nil, ts), false},
		{"time range with garbage operands", timeField, q.NewRange("a", "b"), true},
		{"time range with a bad upper bound", timeField, q.NewRange(ts, "abc"), true},
		{"time range with an integer operand", timeField, q.NewRange(int64(2020), "abc"), true},
		{"time exact match with garbage", timeField, "abc", true},
		{"time exact match with a time", timeField, ts, false},
		{"time pointer field", timePtrField, "abc", true},
		{"time or list with garbage", timeField, &q.OrList{Values: []any{"a", "b"}}, true},
		{"time or list with times", timeField, &q.OrList{Values: []any{ts, ts}}, false},
		{"time fuzzy match is cast to text", timeField, &q.FuzzyMatchValue{Value: "abc"}, false},
		{"time and list is left to the DAO", timeField, &q.AndList{Values: []any{"a"}}, false},

		// integer columns
		{"int range with garbage", intField, q.NewRange("a", "b"), true},
		{"int exact match with garbage", intField, "abc", true},
		{"int exact match with a numeric string", intField, "12", false},
		{"int exact match with an int", intField, 12, false},
		{"int exact match with an int64", intField, int64(12), false},
		{"int in list", intField, []int64{1, 2, 3}, false},
		{"int in list with garbage", intField, []any{1, "a"}, true},
		{"int exact match with a time", intField, ts, true},

		// other columns
		{"float with garbage", floatField, "abc", true},
		{"float with a numeric string", floatField, "1.5", false},
		{"bool with garbage", boolField, "abc", true},
		{"bool with a bool string", boolField, "true", false},
		{"bool with a bool", boolField, true, false},
		{"text column takes any literal", stringField, "abc", false},
		{"text column takes a range of any literal", stringField, q.NewRange("a", "b"), false},
		{"unknown field type", nil, "abc", false},
		{"nil value", intField, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilterValue(tc.fieldType, "field", tc.value)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
			assert.True(t, errors.IsErr(err, errors.BadRequestCode), "want a bad request error, got %v", err)
		})
	}
}
