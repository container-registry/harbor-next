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
	"strconv"
	"time"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
)

var timeType = reflect.TypeOf(time.Time{})

// timestampLayouts are the textual operand forms accepted for timestamp
// columns; anything else would reach Postgres as an unparseable literal.
var timestampLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	time.RFC3339,
	time.RFC3339Nano,
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
		if s, ok := value.(string); ok {
			_, err := strconv.ParseInt(s, 10, 64)
			return err == nil
		}
		return isNumeric(value)
	case isFloatKind(fieldType.Kind()):
		if s, ok := value.(string); ok {
			_, err := strconv.ParseFloat(s, 64)
			return err == nil
		}
		return isNumeric(value)
	case fieldType.Kind() == reflect.Bool:
		if s, ok := value.(string); ok {
			_, err := strconv.ParseBool(s)
			return err == nil
		}
		_, ok := value.(bool)
		return ok
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
