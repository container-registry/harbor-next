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
	"math"
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
		int8Field    = reflect.TypeOf(int8(0))
		int32Field   = reflect.TypeOf(int32(0))
		uintField    = reflect.TypeOf(uint32(0))
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
		{"time exact match with an RFC3339 string", timeField, "2020-01-02T03:04:05Z", false},
		{"bytes operand against a text column", stringField, []byte("abc"), false},

		// column width: Postgres rejects an overflowing literal the same way
		// it rejects a non-numeric one
		{"int8 takes a value in range", int8Field, "120", false},
		{"int8 rejects a value past its width", int8Field, "300", true},
		{"int8 rejects an overflowing numeric operand", int8Field, 300, true},
		{"int32 takes a value in range", int32Field, "2147483647", false},
		{"int32 rejects a value past its width", int32Field, "2147483648", true},
		{"int64 still takes a large value", intField, "2147483648", false},
		{"unsigned column rejects a negative operand", uintField, "-1", true},
		{"unsigned column rejects a negative numeric operand", uintField, -1, true},
		{"unsigned column takes a positive operand", uintField, "12", false},

		// float operands against an integer column
		{"int rejects a fractional operand", intField, 1.5, true},
		{"int takes an integral float operand", intField, float64(12), false},
		{"int rejects a NaN operand", intField, math.NaN(), true},
		{"int rejects an infinite operand", intField, math.Inf(1), true},

		// surrounding whitespace, which Postgres ignores
		{"int takes a padded numeric string", intField, " 12 ", false},
		{"float takes a padded numeric string", floatField, " 1.5 ", false},
		{"timestamp takes a padded string", timeField, " 2020-01-01 ", false},

		// booleans: Postgres takes more literals than strconv.ParseBool
		{"bool takes yes", boolField, "yes", false},
		{"bool takes on", boolField, "ON", false},
		{"bool takes n", boolField, "n", false},
		{"bool takes off", boolField, "off", false},
		{"bool rejects a non-boolean word", boolField, "maybe", true},
		{"bool rejects an ambiguous prefix", boolField, "o", true},

		// timestamp forms Postgres accepts beyond the documented one. These are
		// validator-level: a "+" offset reaches here intact only from a caller
		// that builds a q.Query in code, since the query parser decodes "+" in
		// a query string as a space.
		{"timestamp with a zone offset", timeField, "2020-01-01 15:04:05+05:30", false},
		{"timestamp with a negative zone offset", timeField, "2020-01-01 15:04:05-05:30", false},
		{"timestamp with T and a zone offset", timeField, "2020-01-01T15:04:05+05:30", false},
		{"timestamp with a fractional part", timeField, "2020-01-01 15:04:05.123456", false},
		{"timestamp to the minute", timeField, "2020-01-01 15:04", false},
		{"timestamp documented format", timeField, "2020-04-09 02:36:00", false},
		{"year alone is still rejected", timeField, "2020", true},
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
