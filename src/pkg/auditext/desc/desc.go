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

// Package desc builds the audit log description stored in
// audit_log_ext.op_desc. Every row follows one template:
//
//	<actor> <verb> <resource> "<name>"[ in project "<project>"]: <details> [(failed: <error>)]
//
// where details are comma separated, in insertion order, and each one is a
// change (field "old" -> "new"), an added or removed field, a secret that
// only says "changed", or a plain fact (field value). Login and logout drop
// the resource and name since the actor is the resource. The result is cut
// to MaxLen so it always fits the op_desc column.
package desc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// KeyError is reserved for the failure reason, rendered last.
	KeyError = "error"
	// Changed is all a sensitive field ever says: never its content.
	Changed = "changed"
	// MaxLen is the width of audit_log_ext.op_desc (varchar(1024)).
	MaxLen = 1024
	// ellipsis ends a description that was cut to MaxLen.
	ellipsis = "..."
)

// Description is the ordered set of parts the template is rendered from.
// The sentence head (actor, action, resource, name, project) is kept apart
// from the details, so a detail key such as "name" never rewrites the head.
type Description struct {
	actor, action, resource, name, project string
	keys                                   []string
	vals                                   map[string]any
	err                                    string
}

// secret is the value stored for a sensitive field; it only renders as Changed.
type secret struct{}

// maxReasonLen bounds the failure reason so it never crowds out the details.
const maxReasonLen = 256

// verbs maps the operation column to the verb used in the sentence
var verbs = map[string]string{
	"create": "created",
	"update": "updated",
	"delete": "deleted",
	"login":  "logged in",
	"logout": "logged out",
	"pull":   "pulled",
	"push":   "pushed",
}

// Change is a before/after pair for one field. A missing From means the
// field did not exist before; a missing To means it was removed.
type Change struct {
	From    any
	To      any
	HasFrom bool
	HasTo   bool
}

func (c Change) render(key string) string {
	switch {
	case c.HasFrom && c.HasTo:
		return key + " " + value(c.From) + " -> " + value(c.To)
	case c.HasTo:
		return key + " set " + value(c.To)
	default:
		return key + " removed (was " + value(c.From) + ")"
	}
}

// New starts a description with the three parts every row carries.
func New(actor, action, resource string) *Description {
	return &Description{actor: actor, action: action, resource: resource, vals: map[string]any{}}
}

// Name sets the resource name. Empty names are left out.
func (d *Description) Name(name string) *Description {
	d.name = name
	return d
}

// Project sets the project the resource lives in. Empty names are left out.
func (d *Description) Project(project string) *Description {
	d.project = project
	return d
}

// Set records a plain fact about the resource. Nil, empty strings, empty
// slices and empty maps are dropped so the line never carries padding.
func (d *Description) Set(key string, v any) *Description {
	if key == KeyError || isEmpty(v) {
		return d
	}
	if _, ok := d.vals[key]; !ok {
		d.keys = append(d.keys, key)
	}
	d.vals[key] = v
	return d
}

// Changed records a from/to pair. Equal values are not a change and are
// dropped; a nil side reads as the field being added or removed.
func (d *Description) Changed(key string, from, to any) *Description {
	switch {
	case Equal(from, to):
		return d
	case from == nil:
		return d.Added(key, to)
	case to == nil:
		return d.Removed(key, from)
	}
	return d.Set(key, Change{From: from, To: to, HasFrom: true, HasTo: true})
}

// Added records a field that did not exist before.
func (d *Description) Added(key string, to any) *Description {
	return d.Set(key, Change{To: to, HasTo: true})
}

// Removed records a field that no longer exists.
func (d *Description) Removed(key string, from any) *Description {
	return d.Set(key, Change{From: from, HasFrom: true})
}

// Secret records that a sensitive field changed without its value.
func (d *Description) Secret(key string) *Description {
	return d.Set(key, secret{})
}

// Error marks the operation failed. It is always rendered last.
func (d *Description) Error(status int, reason string) *Description {
	d.err = truncate(Reason(status, reason), maxReasonLen)
	return d
}

// Has reports whether key is set.
func (d *Description) Has(key string) bool {
	_, ok := d.vals[key]
	return ok
}

// String renders the template, cut to MaxLen characters. The failure suffix
// is kept whole; only the sentence before it is cut.
func (d *Description) String() string {
	suffix := ""
	if d.err != "" {
		suffix = " (failed: " + d.err + ")"
	}
	return truncate(d.render(), MaxLen-utf8.RuneCountInString(suffix)) + suffix
}

func (d *Description) render() string {
	var b strings.Builder
	verb, ok := verbs[d.action]
	if !ok {
		verb = d.action
	}
	b.WriteString(d.actor)
	b.WriteByte(' ')
	b.WriteString(verb)
	if d.action != "login" && d.action != "logout" {
		if d.resource != "" {
			b.WriteByte(' ')
			b.WriteString(d.resource)
		}
		if d.name != "" {
			b.WriteByte(' ')
			b.WriteString(strconv.Quote(d.name))
		}
		if d.project != "" {
			b.WriteString(" in project ")
			b.WriteString(strconv.Quote(d.project))
		}
	}
	details := make([]string, 0, len(d.keys))
	for _, k := range d.keys {
		details = append(details, detail(k, d.vals[k]))
	}
	if len(details) > 0 {
		b.WriteString(": ")
		b.WriteString(strings.Join(details, ", "))
	}
	return b.String()
}

// truncate cuts s to max characters, ending it with an ellipsis when it was
// longer. It counts runes, as the column does.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-len(ellipsis)]) + ellipsis
}

func detail(key string, v any) string {
	switch t := v.(type) {
	case Change:
		return t.render(key)
	case secret:
		return key + " " + Changed
	}
	return key + " " + value(v)
}

// value quotes strings and writes everything else (numbers, booleans,
// lists, maps) in JSON form.
func value(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	var b bytes.Buffer
	if err := writeValue(&b, v); err != nil {
		return fmt.Sprint(v)
	}
	return b.String()
}

// Reason turns an HTTP status and a response body into "<status> <reason>".
// Harbor API errors ({"errors":[{"code":..,"message":..}]}) yield their
// message; anything else falls back to the status text. Control characters
// in the message are escaped so a rejected value can neither break the line
// nor, as NUL would, make PostgreSQL refuse the row.
func Reason(status int, body string) string {
	msg := ""
	var apiErr struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &apiErr); err == nil && len(apiErr.Errors) > 0 {
		msg = apiErr.Errors[0].Message
	}
	msg = escapeControl(strings.TrimSpace(msg))
	if msg == "" {
		msg = strings.ToLower(http.StatusText(status))
	}
	if msg == "" {
		return fmt.Sprintf("%d", status)
	}
	return fmt.Sprintf("%d %s", status, msg)
}

// escapeControl writes control characters the way strconv.Quote does, leaving
// everything else, quotes and backslashes included, untouched.
func escapeControl(s string) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func writeValue(b *bytes.Buffer, v any) error {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encode appends a newline
	b.Truncate(b.Len() - 1)
	return nil
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// Equal compares two values the way Changed does: by Go equality first, then by
// printed form, so 30 equals float64(30) and true equals "true".
func Equal(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	// values read back from the store and values parsed from JSON differ in
	// Go type (int vs float64, bool vs "true"); compare their printed form
	return fmt.Sprint(a) == fmt.Sprint(b)
}
