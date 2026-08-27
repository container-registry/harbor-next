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

package pattern

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileRegex(t *testing.T) {
	cases := []struct {
		expr    string
		value   string
		match   bool
		wantErr bool
	}{
		// the expression has to match the whole value
		{expr: "library/.*", value: "library/hello-world", match: true},
		{expr: "library", value: "library/hello-world", match: false},
		{expr: "hello-world", value: "library/hello-world", match: false},
		{expr: `v\d+\.\d+`, value: "v1.0", match: true},
		{expr: `v\d+\.\d+`, value: "v1.0-rc1", match: false},
		// an alternation is grouped before it is anchored
		{expr: "foo|bar", value: "foo", match: true},
		{expr: "foo|bar", value: "foobar", match: false},
		// an inline flag stays scoped to the wrapped group, it cannot reach the
		// anchors and let a value match on one of its lines
		{expr: "(?m)v1", value: "junk\nv1", match: false},
		{expr: "(?m)v1", value: "v1\njunk", match: false},
		{expr: "(?m)v1", value: "v1", match: true},
		// (?i) stays available to the user
		{expr: "(?i)latest", value: "LATEST", match: true},
		// a pattern that would escape the anchoring is rejected, wrapping it
		// would produce a valid but unanchored alternation
		{expr: "foo)|(?:bar", wantErr: true},
		{expr: "[a-", wantErr: true},
		{expr: "v(", wantErr: true},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/%s", c.expr, c.value), func(t *testing.T) {
			expression, err := CompileRegex(c.expr)
			if c.wantErr {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), "invalid regex pattern")
				return
			}
			require.Nil(t, err)
			assert.Equal(t, c.match, expression.MatchString(c.value))
		})
	}
}

func TestCompileRegexLength(t *testing.T) {
	_, err := CompileRegex(strings.Repeat("a", MaxRegexLength))
	assert.Nil(t, err)

	_, err = CompileRegex(strings.Repeat("a", MaxRegexLength+1))
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "limited to")

	// the cap counts characters, not bytes
	_, err = CompileRegex(strings.Repeat("ä", MaxRegexLength))
	assert.Nil(t, err)
}

func TestValidateRegex(t *testing.T) {
	// an empty pattern matches everything and is never compiled
	assert.Nil(t, ValidateRegex(""))
	assert.Nil(t, ValidateRegex("library/.*"))
	assert.NotNil(t, ValidateRegex("foo)|(?:bar"))
}

func TestMatcher(t *testing.T) {
	cases := []struct {
		kind    string
		expr    string
		value   string
		match   bool
		wantErr bool
	}{
		// an empty pattern matches everything, whatever the kind
		{kind: "", expr: "", value: "library/hello-world", match: true},
		{kind: KindRegex, expr: "", value: "library/hello-world", match: true},
		// an empty kind is the doublestar default
		{kind: "", expr: "library/**", value: "library/hello-world", match: true},
		{kind: KindDoublestar, expr: "library/**", value: "library/hello-world", match: true},
		// a doublestar pattern isn't interpreted as a regex, and the other way round
		{kind: KindDoublestar, expr: "library/.*", value: "library/hello-world", match: false},
		{kind: KindRegex, expr: "library/.*", value: "library/hello-world", match: true},
		{kind: KindRegex, expr: "library/**", value: "library/hello-world", wantErr: true},
		{kind: KindRegex, expr: "foo)|(?:bar", value: "foo", wantErr: true},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/%s/%s", c.kind, c.expr, c.value), func(t *testing.T) {
			match, err := NewMatcher(c.kind, c.expr).Match(c.value)
			if c.wantErr {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, c.match, match)
		})
	}
}

func TestMatcherCompilesOnce(t *testing.T) {
	m := NewMatcher(KindRegex, `v\d+`)

	match, err := m.Match("v1")
	require.Nil(t, err)
	assert.True(t, match)
	first := m.expression

	match, err = m.Match("latest")
	require.Nil(t, err)
	assert.False(t, match)
	assert.Same(t, first, m.expression)
}

func TestMatcherKeepsCompileError(t *testing.T) {
	m := NewMatcher(KindRegex, "v(")

	_, err := m.Match("v1")
	require.NotNil(t, err)

	// the cached error is returned on every subsequent call as well
	_, err = m.Match("v2")
	assert.NotNil(t, err)
}
