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
	"regexp"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar"

	regexpselector "github.com/goharbor/harbor/src/lib/selector/selectors/regexp"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

// Match returns whether the str matches the doublestar pattern
func Match(pattern, str string) (bool, error) {
	if len(pattern) == 0 {
		return true, nil
	}
	return doublestar.Match(pattern, str)
}

// MatchPattern returns whether the str matches the pattern under the given filter kind.
// An empty kind is the doublestar default. Prefer NewMatcher when the same pattern is
// applied to many values, it keeps a compiled expression instead of compiling per value.
func MatchPattern(kind, pattern, str string) (bool, error) {
	return NewMatcher(kind, pattern).Match(str)
}

// Matcher matches values against one filter pattern
type Matcher struct {
	kind    string
	pattern string

	// a regex pattern is compiled on first use and the outcome kept, so that a
	// filter applied to thousands of artifacts compiles only once
	once       sync.Once
	expression *regexp.Regexp
	compileErr error
}

// NewMatcher returns a matcher for the pattern under the given filter kind
func NewMatcher(kind, pattern string) *Matcher {
	return &Matcher{
		kind:    kind,
		pattern: pattern,
	}
}

// Match returns whether the str matches the pattern of the matcher
func (m *Matcher) Match(str string) (bool, error) {
	if len(m.pattern) == 0 {
		return true, nil
	}

	if m.kind != model.FilterKindRegex {
		return doublestar.Match(m.pattern, str)
	}

	m.once.Do(func() {
		// the anchoring is done by the selector package, so that the replication
		// filters and the retention/immutability selectors share one implementation
		m.expression, m.compileErr = regexpselector.Compile(m.pattern)
	})
	if m.compileErr != nil {
		return false, m.compileErr
	}

	return m.expression.MatchString(str), nil
}

// IsRegex returns whether the kind selects the regular expression engine
func IsRegex(kind string) bool {
	return kind == model.FilterKindRegex
}

// IsSpecificPath checks whether the input path is a specified string
// If it is, the function returns a string array that parsed from the input path
// A specified string means we can get a specific string array after parsing it
// "library/hello-world" is a specified string as it only matches "library/hello-world"
// "library/**" isn't a specified string as it can match all string that starts with "library/"
// "library/{test,busybox}" is a specified string as it only matches "library/hello-world" and "library/busybox"
func IsSpecificPath(path string) ([]string, bool) {
	if len(path) == 0 {
		return nil, false
	}
	components := [][]string{}
	for component := range strings.SplitSeq(path, "/") {
		strs, ok := IsSpecificPathComponent(component)
		if !ok {
			return nil, false
		}
		components = append(components, strs)
	}

	result := []string{}
	for _, component := range components {
		result = combinationPathComponents(result, component)
	}
	return result, true
}

// IsSpecificPathForKind is IsSpecificPath for a filter of the given kind
// A regex pattern is never reversed into a repository list, so the caller falls back to
// listing the catalog, the same as it does for a "**" doublestar pattern
func IsSpecificPathForKind(kind, path string) ([]string, bool) {
	if IsRegex(kind) {
		return nil, false
	}
	return IsSpecificPath(path)
}

func combinationPathComponents(components1, components2 []string) []string {
	if len(components1) == 0 {
		return components2
	}
	if len(components2) == 0 {
		return components1
	}
	components := []string{}
	for _, component1 := range components1 {
		for _, component2 := range components2 {
			components = append(components, fmt.Sprintf("%s/%s", component1, component2))
		}
	}
	return components
}

// IsSpecificPathComponent checks whether the input path component is a specified string
// If it is, the function returns a string array that parsed from the input component
// A specified string means we can get a specific string array after parsing it
// "library" is a specified string as it only matches "library"
// "library*" isn't a specified string as it can match all string that starts with "library"
// "{library, test}" is a specified string as it only matches "library" and "test"
// Note: the function doesn't support the component that contains more than one "{"
// such as "a{b{c,d}e}f"
func IsSpecificPathComponent(component string) ([]string, bool) {
	if len(component) == 0 {
		return nil, false
	}
	// contains any of *?[\\]^
	if strings.ContainsAny(component, "*?[\\]^/") {
		return nil, false
	}
	// doesn't contain {},
	if !strings.ContainsAny(component, "{},") {
		return []string{component}, true
	}
	// support only one pair of {} currently
	n := strings.Count(component, "{")
	if n > 1 {
		return nil, false
	}
	i := strings.Index(component, "{")
	if i == -1 {
		return nil, false
	}
	j := strings.LastIndex(component, "}")
	if j == -1 {
		return nil, false
	}
	if i > j {
		return nil, false
	}
	prefix := component[:i]
	suffix := ""
	if j+1 < len(component) {
		suffix = component[j+1:]
	}
	components := []string{}
	for str := range strings.SplitSeq(component[i+1:j], ",") {
		components = append(components, prefix+str+suffix)
	}
	return components, true
}

// IsSpecificPathComponentForKind is IsSpecificPathComponent for a filter of the given kind
// A regex pattern is never reversed into a namespace list, so the caller falls back to
// listing all namespaces
func IsSpecificPathComponentForKind(kind, component string) ([]string, bool) {
	if IsRegex(kind) {
		return nil, false
	}
	return IsSpecificPathComponent(component)
}
