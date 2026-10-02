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
	"strconv"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
)

var timeType = reflect.TypeOf(time.Time{})

// timestampLayouts are the textual timestamp forms a filter operand may take.
// The API documents a single format ("2020-04-09 02:36:00", see the q parameter
// in api/v2.0/swagger.yaml) and the query parser turns only
// "2006-01-02T15:04:05" into a time.Time; everything else arrives as a string
// and is handed to Postgres as a literal. The list is deliberately wider than
// the documented format so the undocumented forms that reach Postgres today
// keep working; a fractional part and a zone offset are optional in each.
var timestampLayouts = []string{
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// validateFilterValue reports a bad request when a filter operand cannot be
// rendered into SQL for the column it targets, instead of letting the
// Postgres syntax error surface as a 500.
func validateFilterValue(fieldType reflect.Type, key string, value any) error {
	switch v := value.(type) {
	case *q.FuzzyMatchValue, *q.AndList:
		// fuzzy matches render as ILIKE (casts to text); AndList conditions
		// are built by the DAO
		return nil
	case *q.OrList:
		if v == nil {
			return nil
		}
		for _, item := range v.Values {
			if err := validateOperand(fieldType, key, item); err != nil {
				return err
			}
		}
		return nil
	case *q.Range:
		if v == nil {
			return nil
		}
		if err := validateOperand(fieldType, key, v.Min); err != nil {
			return err
		}
		return validateOperand(fieldType, key, v.Max)
	default:
		return validateOperand(fieldType, key, value)
	}
}

func validateOperand(fieldType reflect.Type, key string, value any) error {
	if fieldType == nil || value == nil {
		return nil
	}
	// list operands ("__in") validate element by element
	if rv := reflect.ValueOf(value); rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		if _, isBytes := value.([]byte); !isBytes {
			for i := range rv.Len() {
				if err := validateOperand(fieldType, key, rv.Index(i).Interface()); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if operandAssignable(fieldType, value) {
		return nil
	}
	return errors.New(nil).
		WithCode(errors.BadRequestCode).
		WithMessagef("invalid value for the query parameter %q: %v", key, value)
}

func operandAssignable(fieldType reflect.Type, value any) bool {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch {
	case fieldType == timeType:
		return timestampAssignable(value)
	case isIntegerKind(fieldType.Kind()):
		return integerAssignable(fieldType, value)
	case isFloatKind(fieldType.Kind()):
		if s, ok := value.(string); ok {
			_, err := strconv.ParseFloat(strings.TrimSpace(s), fieldType.Bits())
			return err == nil
		}
		return isNumeric(value)
	case fieldType.Kind() == reflect.Bool:
		return booleanAssignable(value)
	default:
		// text columns take any literal
		return true
	}
}

func timestampAssignable(value any) bool {
	switch v := value.(type) {
	case time.Time, *time.Time:
		return true
	case string:
		v = strings.TrimSpace(v)
		for _, layout := range timestampLayouts {
			if _, err := time.Parse(layout, v); err == nil {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// integerAssignable reports whether value is an integer the column can hold.
// Postgres rejects a non-numeric literal and one that overflows the column
// alike, so the operand is checked against the field's own width rather than
// against int64: "300" is a fine operand for an int32 column and a 22003 for
// an int8 one.
func integerAssignable(fieldType reflect.Type, value any) bool {
	probe := reflect.New(fieldType).Elem()
	signed := probe.CanInt()

	if s, ok := value.(string); ok {
		s = strings.TrimSpace(s)
		if signed {
			_, err := strconv.ParseInt(s, 10, fieldType.Bits())
			return err == nil
		}
		_, err := strconv.ParseUint(s, 10, fieldType.Bits())
		return err == nil
	}

	rv := reflect.ValueOf(value)
	switch {
	case rv.CanInt():
		if signed {
			return !probe.OverflowInt(rv.Int())
		}
		return rv.Int() >= 0 && !probe.OverflowUint(uint64(rv.Int()))
	case rv.CanUint():
		if signed {
			return rv.Uint() <= math.MaxInt64 && !probe.OverflowInt(int64(rv.Uint()))
		}
		return !probe.OverflowUint(rv.Uint())
	case rv.CanFloat():
		// a fractional or non-finite operand is not an integer the column can
		// store, however numeric it looks
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
			return false
		}
		if signed {
			return f >= math.MinInt64 && f <= math.MaxInt64 && !probe.OverflowInt(int64(f))
		}
		return f >= 0 && f <= math.MaxUint64 && !probe.OverflowUint(uint64(f))
	}
	return false
}

// postgresBooleanLiterals is every token Postgres takes for a boolean column,
// which is the documented set plus its unambiguous prefixes. strconv.ParseBool
// accepts a narrower set, so filtering on "yes" or "on" would be rejected here
// while the database would have taken it.
var postgresBooleanLiterals = map[string]struct{}{
	"t": {}, "tr": {}, "tru": {}, "true": {}, "y": {}, "ye": {}, "yes": {}, "on": {}, "1": {},
	"f": {}, "fa": {}, "fal": {}, "fals": {}, "false": {}, "n": {}, "no": {}, "of": {}, "off": {}, "0": {},
}

func booleanAssignable(value any) bool {
	switch v := value.(type) {
	case bool:
		return true
	case string:
		_, ok := postgresBooleanLiterals[strings.ToLower(strings.TrimSpace(v))]
		return ok
	default:
		return false
	}
}

func isNumeric(value any) bool {
	k := reflect.ValueOf(value).Kind()
	return isIntegerKind(k) || isFloatKind(k)
}

func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func isFloatKind(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}
