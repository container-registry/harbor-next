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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	iselector "github.com/goharbor/harbor/src/lib/selector"
)

// RegexpSelectorTestSuite is a suite for testing the regexp selector
type RegexpSelectorTestSuite struct {
	suite.Suite

	artifacts []*iselector.Candidate
}

// TestRegexpSelector is entrance for RegexpSelectorTestSuite
func TestRegexpSelector(t *testing.T) {
	suite.Run(t, new(RegexpSelectorTestSuite))
}

// SetupSuite to do preparation work
func (suite *RegexpSelectorTestSuite) SetupSuite() {
	suite.artifacts = []*iselector.Candidate{
		{
			NamespaceID:  1,
			Namespace:    "library",
			Repository:   "harbor",
			Tags:         []string{"latest"},
			Kind:         iselector.Image,
			PushedTime:   time.Now().Unix() - 3600,
			PulledTime:   time.Now().Unix(),
			CreationTime: time.Now().Unix() - 7200,
			Labels:       []string{"label1"},
		},
		{
			NamespaceID:  2,
			Namespace:    "retention",
			Repository:   "redis",
			Tags:         []string{"v1"},
			Kind:         iselector.Image,
			PushedTime:   time.Now().Unix() - 3600,
			PulledTime:   time.Now().Unix(),
			CreationTime: time.Now().Unix() - 7200,
			Labels:       []string{"label2"},
		},
		{
			NamespaceID:  2,
			Namespace:    "retention",
			Repository:   "redis",
			Tags:         []string{"v11"},
			Kind:         iselector.Image,
			PushedTime:   time.Now().Unix() - 3600,
			PulledTime:   time.Now().Unix(),
			CreationTime: time.Now().Unix() - 7200,
			Labels:       []string{"label3"},
		},
	}
}

// TestAnchoring verifies the full string semantics: `v\d+` matches `v1` but not
// `v1.0`, the same meaning doublestar.Match carries.
func (suite *RegexpSelectorTestSuite) TestAnchoring() {
	cases := []struct {
		pattern string
		tag     string
		matched bool
	}{
		{`v\d+`, "v1", true},
		{`v\d+`, "v11", true},
		{`v\d+`, "v1.0", false},
		{`v\d+`, "prefix-v1", false},
		{`v\d+`, "v1-suffix", false},
		{`\d+`, "v1", false},
		{`.*`, "anything", true},
		// anchors written by the user stay redundant but harmless
		{`^latest$`, "latest", true},
		{`\Alatest\z`, "latest", true},
	}

	for _, c := range cases {
		s := New(Matches, c.pattern, "").(*selector)
		matched, err := s.match(c.tag)
		suite.Require().NoError(err, "pattern %q", c.pattern)
		suite.Equal(c.matched, matched, "pattern %q against %q", c.pattern, c.tag)
	}
}

// TestEmptyPatternMatchesEverything mirrors the doublestar wrapper behavior
func (suite *RegexpSelectorTestSuite) TestEmptyPatternMatchesEverything() {
	s := New(Matches, "", "")
	selected, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, len(suite.artifacts))
}

// TestInvalidPatternSurfacesAtSelect asserts construction never fails and the
// compile error is reported by Select instead
func (suite *RegexpSelectorTestSuite) TestInvalidPatternSurfacesAtSelect() {
	s := New(Matches, "v(", "")
	suite.Require().NotNil(s)

	selected, err := s.Select(suite.artifacts)
	suite.Require().Error(err)
	suite.Nil(selected)
	suite.Contains(err.Error(), "invalid regex pattern")

	// the cached error is returned on every subsequent call as well
	_, err = s.Select(suite.artifacts)
	suite.Require().Error(err)
}

// TestCompileIsCached asserts the selector keeps one matcher, which is where the
// compiled expression is cached, see TestMatcherCompilesOnce in lib/pattern
func (suite *RegexpSelectorTestSuite) TestCompileIsCached() {
	s := New(Matches, `v\d+`, "").(*selector)

	_, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	first := s.matcher

	_, err = s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Same(first, s.matcher)
}

// TestCaseInsensitiveFlag verifies (?i) is honored
func (suite *RegexpSelectorTestSuite) TestCaseInsensitiveFlag() {
	s := New(Matches, `(?i)latest`, "").(*selector)

	for _, tag := range []string{"latest", "LATEST", "Latest"} {
		matched, err := s.match(tag)
		suite.Require().NoError(err)
		suite.True(matched, "tag %q", tag)
	}

	sensitive := New(Matches, `latest`, "").(*selector)
	matched, err := sensitive.match("LATEST")
	suite.Require().NoError(err)
	suite.False(matched)
}

// TestMaxPatternLength verifies the 512 character cap
func (suite *RegexpSelectorTestSuite) TestMaxPatternLength() {
	atLimit := strings.Repeat("a", MaxPatternLength)
	suite.Require().NoError(Validate(atLimit))

	tooLong := strings.Repeat("a", MaxPatternLength+1)
	err := Validate(tooLong)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "limited to 512 characters")

	// the cap is enforced at match time too, since construction never fails
	s := New(Matches, tooLong, "")
	_, err = s.Select(suite.artifacts)
	suite.Require().Error(err)
}

// TestValidate covers the write time validation helper
func (suite *RegexpSelectorTestSuite) TestValidate() {
	valid := []string{
		"",
		`v?\d+\.\d+\.\d+`,
		`.*-(alpha|beta|rc)\.?\d*`,
		`v\d+\.\d+\.\d+-\d+-g[0-9a-f]+`,
		`(?i)latest`,
		`(?P<major>\d+)\.\d+`,
		`^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$`,
	}
	for _, p := range valid {
		suite.NoError(Validate(p), "pattern %q", p)
	}

	invalid := []string{
		"v(",
		"[a-",
		"*",
		`a{2,1}`,
		// RE2 exclusions
		`(?=foo)`,
		`(foo)\1`,
		// would compile once wrapped and escape the anchoring
		`foo)|(?:bar`,
		`a)\z|\A(?:b`,
	}
	for _, p := range invalid {
		suite.Error(Validate(p), "pattern %q", p)
	}
}

// TestAnchoringCannotBeEscaped covers a pattern that is invalid on its own but
// would compile as an alternation once wrapped, turning the full string match
// into a prefix or suffix match
func (suite *RegexpSelectorTestSuite) TestAnchoringCannotBeEscaped() {
	s := New(Matches, `foo)|(?:bar`, "")

	selected, err := s.Select([]*iselector.Candidate{
		{Repository: "redis", Tags: []string{"fooXXX"}},
		{Repository: "redis", Tags: []string{"XXXbar"}},
	})
	suite.Require().Error(err)
	suite.Nil(selected)
}

// TestTagMatches covers the matches decoration
func (suite *RegexpSelectorTestSuite) TestTagMatches() {
	s := New(Matches, `v\d+`, "")
	selected, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 2)
	suite.ElementsMatch([]string{"v1", "v11"}, tagsOf(selected))
}

// TestTagExcludes covers the excludes decoration
func (suite *RegexpSelectorTestSuite) TestTagExcludes() {
	s := New(Excludes, `v\d+`, "")
	selected, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 1)
	suite.Equal([]string{"latest"}, tagsOf(selected))
}

// TestRepoMatches covers the repoMatches / repoExcludes decorations
func (suite *RegexpSelectorTestSuite) TestRepoMatches() {
	s := New(RepoMatches, `red.*`, "")
	selected, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 2)

	s = New(RepoExcludes, `red.*`, "")
	selected, err = s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 1)
	suite.Equal("harbor", selected[0].Repository)
}

// TestNSMatches covers the nsMatches / nsExcludes decorations
func (suite *RegexpSelectorTestSuite) TestNSMatches() {
	s := New(NSMatches, `library`, "")
	selected, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 1)
	suite.Equal("library", selected[0].Namespace)

	s = New(NSExcludes, `library`, "")
	selected, err = s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 2)
}

// TestUntagged asserts an untagged artifact is governed by the untagged flag
// only and is never evaluated against an empty tag string
func (suite *RegexpSelectorTestSuite) TestUntagged() {
	untaggedArtifacts := []*iselector.Candidate{
		{
			NamespaceID: 1,
			Namespace:   "library",
			Repository:  "harbor",
			Tags:        []string{},
			Kind:        iselector.Image,
		},
	}

	// `.*` would match the empty string, so a selected artifact here would not
	// prove the flag is what governs; `v\d+` cannot match "" either way
	for _, pattern := range []string{`.*`, `v\d+`} {
		// matches: default keeps untagged
		s := New(Matches, pattern, "")
		selected, err := s.Select(untaggedArtifacts)
		suite.Require().NoError(err)
		suite.Len(selected, 1, "pattern %q", pattern)

		// matches: explicitly dropping untagged
		s = New(Matches, pattern, `{"untagged": false}`)
		selected, err = s.Select(untaggedArtifacts)
		suite.Require().NoError(err)
		suite.Empty(selected, "pattern %q", pattern)

		// excludes: the default extras leave the untagged artifact in, since
		// tagSelectExclude returns the negation of the flag
		s = New(Excludes, pattern, "")
		selected, err = s.Select(untaggedArtifacts)
		suite.Require().NoError(err)
		suite.Len(selected, 1, "pattern %q", pattern)

		// excludes: setting the flag takes the untagged artifact out again
		s = New(Excludes, pattern, `{"untagged": true}`)
		selected, err = s.Select(untaggedArtifacts)
		suite.Require().NoError(err)
		suite.Empty(selected, "pattern %q", pattern)
	}
}

// TestUntaggedInvalidPattern asserts an untagged artifact never triggers a
// compile of the pattern, matching the doublestar behavior
func (suite *RegexpSelectorTestSuite) TestUntaggedInvalidPattern() {
	untaggedArtifacts := []*iselector.Candidate{
		{
			NamespaceID: 1,
			Namespace:   "library",
			Repository:  "harbor",
			Tags:        []string{},
			Kind:        iselector.Image,
		},
	}

	s := New(Matches, "v(", "")
	selected, err := s.Select(untaggedArtifacts)
	suite.Require().NoError(err)
	suite.Len(selected, 1)
}

// TestNilPattern asserts a non string pattern degrades to the match everything case
func (suite *RegexpSelectorTestSuite) TestNilPattern() {
	s := New(Matches, nil, "")
	selected, err := s.Select(suite.artifacts)
	suite.Require().NoError(err)
	suite.Len(selected, len(suite.artifacts))
}

// TestReferenceRecipes runs the recipes documented in the proposal
func (suite *RegexpSelectorTestSuite) TestReferenceRecipes() {
	cases := []struct {
		name     string
		pattern  string
		matching []string
		skipping []string
	}{
		{
			name:     "final releases only",
			pattern:  `v?\d+\.\d+\.\d+`,
			matching: []string{"v1.9.0", "v1.10.3", "1.0.0"},
			skipping: []string{"v2.0.0-rc.1", "v1.9.0-12-ge7e744a7", "latest"},
		},
		{
			name:     "ci git describe tags",
			pattern:  `v\d+\.\d+\.\d+-\d+-g[0-9a-f]+`,
			matching: []string{"v1.9.0-12-ge7e744a7"},
			skipping: []string{"v1.9.0", "latest"},
		},
		{
			name:     "pre releases",
			pattern:  `.*-(alpha|beta|rc)\.?\d*`,
			matching: []string{"1.0.0-rc.1", "1.0.0-beta2", "2.0.0-alpha"},
			skipping: []string{"1.0.0", "1.0.0-dev.4"},
		},
		{
			name:     "source variants",
			pattern:  `.*-source`,
			matching: []string{"1.0.0-source"},
			skipping: []string{"1.0.0", "source"},
		},
	}

	for _, c := range cases {
		s := New(Matches, c.pattern, "").(*selector)
		for _, tag := range c.matching {
			matched, err := s.match(tag)
			suite.Require().NoError(err, c.name)
			suite.True(matched, "%s: %q should match %q", c.name, c.pattern, tag)
		}
		for _, tag := range c.skipping {
			matched, err := s.match(tag)
			suite.Require().NoError(err, c.name)
			suite.False(matched, "%s: %q should not match %q", c.name, c.pattern, tag)
		}
	}
}

func tagsOf(candidates []*iselector.Candidate) []string {
	tags := make([]string, 0, len(candidates))
	for _, c := range candidates {
		tags = append(tags, c.Tags...)
	}
	return tags
}
