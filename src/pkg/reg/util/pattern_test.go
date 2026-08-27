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

package util

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		str     string
		match   bool
	}{
		{
			pattern: "",
			str:     "library",
			match:   true,
		},
		{
			pattern: "",
			str:     "",
			match:   true,
		},
		{
			pattern: "*",
			str:     "library",
			match:   true,
		},
		{
			pattern: "*",
			str:     "library/hello-world",
			match:   false,
		},
		{
			pattern: "**",
			str:     "library/hello-world",
			match:   true,
		},
		{
			pattern: "{library,harbor}/**",
			str:     "library/hello-world",
			match:   true,
		},
		{
			pattern: "{library,harbor}/**",
			str:     "harbor/hello-world",
			match:   true,
		},
		{
			pattern: "1.?",
			str:     "1.0",
			match:   true,
		},
		{
			pattern: "1.?",
			str:     "1.01",
			match:   false,
		},
		{
			pattern: "v2.[4-6].*", // match v2.4.*~v2.7.* version
			str:     "v2.4.0",
			match:   true,
		},
		{
			pattern: "v2.[4-7].*", // match v2.4.*~v2.7.* version
			str:     "v2.7.0",
			match:   true,
		},
	}
	for _, c := range cases {
		match, err := Match(c.pattern, c.str)
		require.Nil(t, err)
		assert.Equal(t, c.match, match)
	}
}

func TestIsSpecificPathComponent(t *testing.T) {
	cases := []struct {
		component        string
		isSpecific       bool
		resultComponents []string
	}{
		{
			component:        "",
			isSpecific:       false,
			resultComponents: []string{},
		},
		{
			component:        "library/hello-world",
			isSpecific:       false,
			resultComponents: []string{},
		},
		{
			component:        "library",
			isSpecific:       true,
			resultComponents: []string{"library"},
		},
		{
			component:        "lib*",
			isSpecific:       false,
			resultComponents: []string{},
		},
		{
			component:        "{library}",
			isSpecific:       true,
			resultComponents: []string{"library"},
		},
		{
			component:        "{library,test}",
			isSpecific:       true,
			resultComponents: []string{"library", "test"},
		},
		{
			component:        "{library{a}c}",
			isSpecific:       false,
			resultComponents: []string{},
		},
	}
	for i, c := range cases {
		fmt.Printf("running case %d ...\n", i)
		components, ok := IsSpecificPathComponent(c.component)
		require.Equal(t, c.isSpecific, ok)
		require.Equal(t, len(c.resultComponents), len(components))
		for i := range components {
			assert.Equal(t, c.resultComponents[i], components[i])
		}
	}
}

func TestIsSpecificPath(t *testing.T) {
	cases := []struct {
		path        string
		isSpecific  bool
		resultPaths []string
	}{
		{
			path:        "",
			isSpecific:  false,
			resultPaths: []string{},
		},
		{
			path:        "library",
			isSpecific:  true,
			resultPaths: []string{"library"},
		},
		{
			path:        "library/hello-world",
			isSpecific:  true,
			resultPaths: []string{"library/hello-world"},
		},
		{
			path:        "library/**",
			isSpecific:  false,
			resultPaths: []string{},
		},
		{
			path:        "{library}",
			isSpecific:  true,
			resultPaths: []string{"library"},
		},
		{
			path:        "library/{hello-world,busybox}",
			isSpecific:  true,
			resultPaths: []string{"library/hello-world", "library/busybox"},
		},
	}
	for i, c := range cases {
		fmt.Printf("running case %d ...\n", i)
		paths, ok := IsSpecificPath(c.path)
		require.Equal(t, c.isSpecific, ok)
		require.Equal(t, len(c.resultPaths), len(paths))
		for i := range paths {
			assert.Equal(t, c.resultPaths[i], paths[i])
		}
	}
}

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		kind    string
		pattern string
		str     string
		match   bool
		err     bool
	}{
		// an empty pattern matches everything, whatever the kind
		{kind: "", pattern: "", str: "library/hello-world", match: true},
		{kind: "regex", pattern: "", str: "library/hello-world", match: true},
		// an empty kind keeps the doublestar behavior
		{kind: "", pattern: "library/**", str: "library/hello-world", match: true},
		{kind: "doublestar", pattern: "library/**", str: "library/hello-world", match: true},
		// a doublestar pattern isn't interpreted as a regex
		{kind: "doublestar", pattern: "library/.*", str: "library/hello-world", match: false},
		// the regex is anchored on the whole string
		{kind: "regex", pattern: "library/.*", str: "library/hello-world", match: true},
		{kind: "regex", pattern: "library", str: "library/hello-world", match: false},
		{kind: "regex", pattern: "hello-world", str: "library/hello-world", match: false},
		{kind: "regex", pattern: "v[0-9]+", str: "v12", match: true},
		{kind: "regex", pattern: "v[0-9]+", str: "v12-rc1", match: false},
		// an alternation is grouped before it is anchored
		{kind: "regex", pattern: "foo|bar", str: "foo", match: true},
		{kind: "regex", pattern: "foo|bar", str: "foobar", match: false},
		// a pattern that would escape the anchoring is rejected instead of compiled
		{kind: "regex", pattern: "foo)|(?:bar", str: "foo", err: true},
		{kind: "regex", pattern: "[a-", str: "a", err: true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/%s/%s", c.kind, c.pattern, c.str), func(t *testing.T) {
			match, err := MatchPattern(c.kind, c.pattern, c.str)
			if c.err {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, c.match, match)
		})
	}
}

func TestMatchPatternLength(t *testing.T) {
	_, err := MatchPattern("regex", strings.Repeat("a", 513), "aaa")
	require.NotNil(t, err)

	match, err := MatchPattern("regex", strings.Repeat("a", 512), strings.Repeat("a", 512))
	require.Nil(t, err)
	assert.True(t, match)
}

func TestMatcherReuse(t *testing.T) {
	m := NewMatcher("regex", "v[0-9]+")
	for _, c := range []struct {
		str   string
		match bool
	}{
		{str: "v1", match: true},
		{str: "v2", match: true},
		{str: "latest", match: false},
	} {
		match, err := m.Match(c.str)
		require.Nil(t, err)
		assert.Equal(t, c.match, match, c.str)
	}
}

func TestIsSpecificPathForKind(t *testing.T) {
	// a doublestar pattern is still reversed into the repositories it can match
	paths, ok := IsSpecificPathForKind("", "library/{hello-world,busybox}")
	require.True(t, ok)
	assert.Equal(t, []string{"library/hello-world", "library/busybox"}, paths)

	paths, ok = IsSpecificPathForKind("doublestar", "library/hello-world")
	require.True(t, ok)
	assert.Equal(t, []string{"library/hello-world"}, paths)

	// a regex never is, even when it looks like a literal path
	paths, ok = IsSpecificPathForKind("regex", "library/hello-world")
	assert.False(t, ok)
	assert.Nil(t, paths)

	paths, ok = IsSpecificPathForKind("regex", "library/.*")
	assert.False(t, ok)
	assert.Nil(t, paths)
}

func TestIsSpecificPathComponentForKind(t *testing.T) {
	components, ok := IsSpecificPathComponentForKind("doublestar", "library")
	require.True(t, ok)
	assert.Equal(t, []string{"library"}, components)

	// "lib(a|b)" holds no doublestar metacharacter, so it must be the kind that
	// keeps it from being taken for a literal namespace
	components, ok = IsSpecificPathComponentForKind("regex", "lib(a|b)")
	assert.False(t, ok)
	assert.Nil(t, components)
}
