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

package filter

import (
	"fmt"

	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/pkg/reg/util"
)

// DoFilterArtifacts filter the artifacts according to the filters
func DoFilterArtifacts(artifacts []*model.Artifact, filters []*model.Filter) ([]*model.Artifact, error) {
	fl, err := BuildArtifactFilters(filters)
	if err != nil {
		return nil, err
	}
	return fl.Filter(artifacts)
}

// BuildArtifactFilters from the defined filters
func BuildArtifactFilters(filters []*model.Filter) (ArtifactFilters, error) {
	var fs ArtifactFilters
	for _, filter := range filters {
		var f ArtifactFilter
		switch filter.Type {
		case model.FilterTypeLabel:
			if labels, ok := filter.Value.([]string); ok {
				lf := &artifactLabelFilter{
					labels:     labels,
					kind:       filter.Kind,
					decoration: filter.Decoration,
				}
				if util.IsRegex(filter.Kind) {
					for _, label := range labels {
						lf.matchers = append(lf.matchers, util.NewMatcher(filter.Kind, label))
					}
				}
				f = lf
			} else {
				return nil, fmt.Errorf("invalid filter value type for label filter, expecting []string")
			}
		case model.FilterTypeTag:
			if pattern, ok := filter.Value.(string); ok {
				f = &artifactTagFilter{
					pattern:    pattern,
					kind:       filter.Kind,
					matcher:    util.NewMatcher(filter.Kind, pattern),
					decoration: filter.Decoration,
				}
			} else {
				return nil, fmt.Errorf("invalid filter value type for tag filter, expecting string")
			}
		}
		if f != nil {
			fs = append(fs, f)
		}
	}
	return fs, nil
}

// ArtifactFilter filter the artifacts
type ArtifactFilter interface {
	Filter([]*model.Artifact) ([]*model.Artifact, error)
}

// ArtifactFilters is an array of artifact filter
type ArtifactFilters []ArtifactFilter

// Filter artifacts
func (a ArtifactFilters) Filter(artifacts []*model.Artifact) ([]*model.Artifact, error) {
	var err error
	for _, filter := range a {
		artifacts, err = filter.Filter(artifacts)
		if err != nil {
			return nil, err
		}
	}
	return artifacts, nil
}

// filter the artifacts according to the labels. Only the artifact contains all labels defined
// in the filter is the valid one
type artifactLabelFilter struct {
	labels []string
	// "", "doublestar" or "regex"
	kind string
	// only populated for the regex kind, one matcher per configured label
	matchers []*util.Matcher
	// "matches", "excludes"
	decoration string
}

func (a *artifactLabelFilter) Filter(artifacts []*model.Artifact) ([]*model.Artifact, error) {
	if len(a.labels) == 0 {
		return artifacts, nil
	}
	var result []*model.Artifact
	for _, artifact := range artifacts {
		match, err := a.matchAll(artifact.Labels)
		if err != nil {
			return nil, err
		}
		// add the artifact to the result list if it contains all labels defined for the filter
		if a.decoration == model.Excludes {
			if !match {
				result = append(result, artifact)
			}
		} else {
			if match {
				result = append(result, artifact)
			}
		}
	}
	return result, nil
}

// matchAll returns whether the labels of the artifact satisfy every label of the filter:
// an exact match for a doublestar filter, a match against any of the artifact labels for
// a regex one
func (a *artifactLabelFilter) matchAll(artifactLabels []string) (bool, error) {
	if !util.IsRegex(a.kind) {
		labels := make(map[string]struct{}, len(artifactLabels))
		for _, label := range artifactLabels {
			labels[label] = struct{}{}
		}
		for _, label := range a.labels {
			if _, exist := labels[label]; !exist {
				return false, nil
			}
		}
		return true, nil
	}

	for _, matcher := range a.matchers {
		matched := false
		for _, label := range artifactLabels {
			ok, err := matcher.Match(label)
			if err != nil {
				return false, err
			}
			if ok {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

type artifactTagFilter struct {
	pattern string
	// "", "doublestar" or "regex"
	kind    string
	matcher *util.Matcher
	// "matches", "excludes"
	decoration string
}

// matchUntagged returns whether an artifact without tags matches the pattern.
// A doublestar pattern is matched against the empty tag, keeping the behavior of "**".
// A regex is never run against the empty string, an untagged artifact just doesn't match.
func (a *artifactTagFilter) matchUntagged() (bool, error) {
	if util.IsRegex(a.kind) {
		return false, nil
	}
	return a.matcher.Match("")
}

func (a *artifactTagFilter) Filter(artifacts []*model.Artifact) ([]*model.Artifact, error) {
	if len(a.pattern) == 0 {
		return artifacts, nil
	}
	var result []*model.Artifact
	for _, artifact := range artifacts {
		// for individual artifact, use its own tags to match, reserve the matched tags.
		// for accessory artifact, use the parent tags to match,
		var tagsForMatching []string
		if artifact.IsAcc {
			tagsForMatching = append(tagsForMatching, artifact.ParentTags...)
		} else {
			tagsForMatching = append(tagsForMatching, artifact.Tags...)
		}

		// untagged artifact
		if len(tagsForMatching) == 0 {
			match, err := a.matchUntagged()
			if err != nil {
				return nil, err
			}
			if a.decoration == model.Excludes {
				if !match {
					result = append(result, artifact)
				}
			} else {
				if match {
					result = append(result, artifact)
				}
			}
			continue
		}

		// tagged artifact
		var tags []string
		for _, tag := range tagsForMatching {
			match, err := a.matcher.Match(tag)
			if err != nil {
				return nil, err
			}
			if a.decoration == model.Excludes {
				if !match {
					tags = append(tags, tag)
				}
			} else {
				if match {
					tags = append(tags, tag)
				}
			}
		}
		if len(tags) == 0 {
			continue
		}
		// copy a new artifact here to avoid changing the original one
		if artifact.IsAcc {
			result = append(result, &model.Artifact{
				Type:   artifact.Type,
				Digest: artifact.Digest,
				Labels: artifact.Labels,
				Tags:   artifact.Tags, // use its own tags to replicate
			})
		} else {
			result = append(result, &model.Artifact{
				Type:   artifact.Type,
				Digest: artifact.Digest,
				Labels: artifact.Labels,
				Tags:   tags, // only replicate the matched tags
			})
		}
	}
	return result, nil
}
