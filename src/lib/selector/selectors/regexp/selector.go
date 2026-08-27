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

package regexp

import (
	"encoding/json"
	"regexp"

	"github.com/goharbor/harbor/src/lib/pattern"
	iselector "github.com/goharbor/harbor/src/lib/selector"
)

const (
	// Kind ...
	Kind = pattern.KindRegex
	// Matches [pattern] for tag (default)
	Matches = "matches"
	// Excludes [pattern] for tag (default)
	Excludes = "excludes"
	// RepoMatches represents repository matches [pattern]
	RepoMatches = "repoMatches"
	// RepoExcludes represents repository excludes [pattern]
	RepoExcludes = "repoExcludes"
	// NSMatches represents namespace matches [pattern]
	NSMatches = "nsMatches"
	// NSExcludes represents namespace excludes [pattern]
	NSExcludes = "nsExcludes"

	// MaxPatternLength bounds the compile cost of a user supplied pattern
	MaxPatternLength = pattern.MaxRegexLength
)

// selector for regular expression
type selector struct {
	// Pre defined pattern declarator
	// "matches", "excludes", "repoMatches" or "repoExcludes"
	decoration string
	// whether match untagged
	untagged bool

	// The pattern is compiled on first use and the outcome kept, so that a
	// policy evaluated over thousands of artifacts compiles only once.
	matcher *pattern.Matcher
}

// Compile validates and compiles the pattern with the full string semantics of
// doublestar.Match: the whole candidate value has to be matched, not a substring.
func Compile(expr string) (*regexp.Regexp, error) {
	return pattern.CompileRegex(expr)
}

// Validate checks whether the pattern is usable as a regex selector pattern
func Validate(expr string) error {
	return pattern.ValidateRegex(expr)
}

// Select candidates by regular expressions
func (s *selector) Select(artifacts []*iselector.Candidate) (selected []*iselector.Candidate, err error) {
	for _, art := range artifacts {
		value := ""
		excludes := false

		switch s.decoration {
		case Matches:
			matched, err := s.tagSelectMatch(art)
			if err != nil {
				return nil, err
			}
			if matched {
				selected = append(selected, art)
			}
		case Excludes:
			matched, err := s.tagSelectExclude(art)
			if err != nil {
				return nil, err
			}
			if matched {
				selected = append(selected, art)
			}
		case RepoMatches:
			value = art.Repository
		case RepoExcludes:
			value = art.Repository
			excludes = true
		case NSMatches:
			value = art.Namespace
		case NSExcludes:
			value = art.Namespace
			excludes = true
		}

		if len(value) > 0 {
			matched, err := s.match(value)
			if err != nil {
				// if error occurred, directly throw it out
				return nil, err
			}

			if (matched && !excludes) || (!matched && excludes) {
				selected = append(selected, art)
			}
		}
	}

	return selected, nil
}

func (s *selector) tagSelectMatch(artifact *iselector.Candidate) (selected bool, err error) {
	if len(artifact.Tags) > 0 {
		for _, t := range artifact.Tags {
			matched, err := s.match(t)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}
	return s.untagged, nil
}

func (s *selector) tagSelectExclude(artifact *iselector.Candidate) (selected bool, err error) {
	if len(artifact.Tags) > 0 {
		for _, t := range artifact.Tags {
			matched, err := s.match(t)
			if err != nil {
				return false, err
			}
			if !matched {
				return true, nil
			}
		}
		return false, nil
	}
	return !s.untagged, nil
}

// match returns whether the str matches the pattern of the selector
func (s *selector) match(str string) (bool, error) {
	return s.matcher.Match(str)
}

// New is factory method for regex selector
func New(decoration string, expr any, extras string) iselector.Selector {
	untagged := true // default behavior for upgrade, active keep the untagged images
	if decoration == Excludes {
		untagged = false
	}
	if extras != "" {
		var extraObj struct {
			Untagged bool `json:"untagged"`
		}
		if err := json.Unmarshal([]byte(extras), &extraObj); err == nil {
			untagged = extraObj.Untagged
		}
	}

	var p string
	if expr != nil {
		p, _ = expr.(string)
	}

	return &selector{
		decoration: decoration,
		untagged:   untagged,
		matcher:    pattern.NewMatcher(Kind, p),
	}
}
