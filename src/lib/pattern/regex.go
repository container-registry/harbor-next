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
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar"

	"github.com/goharbor/harbor/src/lib/errors"
)

// MaxRegexLength bounds the compile cost of a user supplied pattern
const MaxRegexLength = 512

// CompileRegex validates and compiles a regex pattern with the full string
// semantics of doublestar.Match: the whole candidate value has to be matched,
// not a substring.
//
// The anchors are \A and \z rather than the ^ and $ of Match. Go scopes an
// inline flag to the group it appears in, so the two forms agree here, but \A
// and \z say "text" rather than "text or line, depending on the flags in force".
func CompileRegex(expr string) (*regexp.Regexp, error) {
	if l := utf8.RuneCountInString(expr); l > MaxRegexLength {
		return nil, errors.Errorf("regex pattern is limited to %d characters, got %d", MaxRegexLength, l)
	}

	// The pattern has to hold up on its own before it is wrapped: an unbalanced
	// group such as `foo)|(?:bar` is rejected here but would compile once
	// wrapped, as an alternation of `\A(?:foo)` and `(?:bar)\z`, and would then
	// match on a prefix or a suffix instead of the full string.
	if _, err := regexp.Compile(expr); err != nil {
		return nil, errors.Wrapf(err, "invalid regex pattern %q", expr)
	}

	expression, err := regexp.Compile(`\A(?:` + expr + `)\z`)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid regex pattern %q", expr)
	}

	return expression, nil
}

// ValidateRegex checks whether the expression is usable as a regex pattern.
// An empty pattern matches everything and is never compiled.
func ValidateRegex(expr string) error {
	if len(expr) == 0 {
		return nil
	}

	_, err := CompileRegex(expr)

	return err
}

// Matcher matches values against one pattern of one kind. Build it once per
// filter or selector: a regex is compiled on first use and the outcome kept, so
// that a policy evaluated over thousands of candidates compiles once instead of
// once per candidate. A Matcher is safe for concurrent use.
//
// Its doublestar branch is the plain doublestar.Match that the selectors and the
// replication filters have always used, not the trimmed and pre-validated
// doublestar of Match. Moving every caller onto one doublestar behavior, and the
// proxy cache filter onto this cached type, is left to a follow-up.
type Matcher struct {
	kind    string
	pattern string

	once       sync.Once
	expression *regexp.Regexp
	compileErr error
}

// NewMatcher returns a matcher for the pattern under the given kind, where an
// empty kind is the doublestar default
func NewMatcher(kind, expr string) *Matcher {
	return &Matcher{
		kind:    kind,
		pattern: expr,
	}
}

// Match returns whether the value matches the pattern of the matcher. An empty
// pattern matches everything, and a kind that is neither of the two known ones
// is an error rather than a silent fallback to doublestar: a pattern written for
// some other engine would otherwise select the wrong repositories or artifacts.
func (m *Matcher) Match(value string) (bool, error) {
	if len(m.pattern) == 0 {
		return true, nil
	}

	switch m.kind {
	case "", KindDoublestar:
		return doublestar.Match(m.pattern, value)
	case KindRegex:
		m.once.Do(func() {
			m.expression, m.compileErr = CompileRegex(m.pattern)
		})
		if m.compileErr != nil {
			return false, m.compileErr
		}
		return m.expression.MatchString(value), nil
	default:
		return false, errors.Errorf("unsupported pattern kind %q", m.kind)
	}
}
