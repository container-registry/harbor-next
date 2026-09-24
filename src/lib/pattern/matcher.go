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
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/goharbor/harbor/src/lib/errors"
)

const (
	// KindRegex indicates regular expression pattern matching
	KindRegex = "regex"
	// KindDoublestar indicates doublestar (glob) pattern matching
	KindDoublestar = "doublestar"
)

// ValidateRepositoryFilter validates the repository filter kind and pattern.
// Empty pattern is valid and means all repositories are allowed.
func ValidateRepositoryFilter(filterPattern, kind string) error {
	_, err := Match("", filterPattern, kind)
	return err
}

// ValidateKind validates the repository filter kind. Empty kind is valid and
// means the doublestar default.
func ValidateKind(kind string) error {
	switch kind {
	case "", KindRegex, KindDoublestar:
		return nil
	default:
		return errors.Errorf("unsupported repository filter kind %q, must be %q, %q, or empty (defaults to %q)", kind, KindDoublestar, KindRegex, KindDoublestar)
	}
}

// MatchDoublestar reports whether value matches the doublestar pattern, keeping
// the semantics Harbor's stored filters were written against.
//
// doublestar v4 made a trailing /** match the bare prefix too, so "library/**"
// began selecting the "library" repository itself on top of everything under
// it. Replication, retention, proxy-cache and scan-export filters already in
// databases were written when it did not, so that one case is filtered back
// out rather than silently widening them on upgrade.
func MatchDoublestar(filterPattern, value string) (bool, error) {
	matched, err := doublestar.Match(filterPattern, value)
	if err != nil || !matched {
		return false, err
	}

	prefix, ok := strings.CutSuffix(filterPattern, "/**")
	if !ok {
		return true, nil
	}

	// Matching the prefix on its own is the case v4 added.
	bare, err := doublestar.Match(prefix, value)
	if err != nil {
		return false, err
	}

	return !bare, nil
}

// Match returns true if the value matches the pattern according to the kind.
// Empty pattern matches all (returns true).
func Match(value, filterPattern, kind string) (bool, error) {
	filterPattern = strings.TrimSpace(filterPattern)
	if filterPattern == "" {
		return true, nil
	}
	switch kind {
	case KindRegex:
		// Compile the bare pattern first: a group-imbalanced pattern such as
		// `foo)|(bar` fails alone but compiles inside the ^(?:...)$ wrapper,
		// where the top-level alternation silently destroys the anchoring.
		if _, err := regexp.Compile(filterPattern); err != nil {
			return false, err
		}
		re, err := regexp.Compile("^(?:" + filterPattern + ")$")
		if err != nil {
			return false, err
		}
		return re.MatchString(value), nil
	case KindDoublestar, "":
		if !doublestar.ValidatePattern(filterPattern) {
			return false, doublestar.ErrBadPattern
		}
		return MatchDoublestar(filterPattern, value)
	default:
		return false, errors.Errorf("unsupported repository filter kind %q", kind)
	}
}
