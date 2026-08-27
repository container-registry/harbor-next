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

package gitlab

import (
	"net/url"
	"strings"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/pattern"
	adp "github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/native"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/pkg/reg/util"
)

func init() {
	if err := adp.RegisterFactory(model.RegistryTypeGitLab, new(factory)); err != nil {
		log.Errorf("failed to register factory for %s: %v", model.RegistryTypeGitLab, err)
		return
	}
	log.Infof("the factory for adapter %s registered", model.RegistryTypeGitLab)
}

type factory struct {
}

// Create ...
func (f *factory) Create(r *model.Registry) (adp.Adapter, error) {
	return newAdapter(r)
}

// AdapterPattern ...
func (f *factory) AdapterPattern() *model.AdapterPattern {
	return nil
}

var (
	_ adp.Adapter          = (*adapter)(nil)
	_ adp.ArtifactRegistry = (*adapter)(nil)
)

type adapter struct {
	*native.Adapter
	registry        *model.Registry
	url             string
	clientGitlabAPI *Client
}

func newAdapter(registry *model.Registry) (*adapter, error) {
	client, err := NewClient(registry)
	if err != nil {
		return nil, err
	}
	return &adapter{
		registry:        registry,
		url:             registry.URL,
		clientGitlabAPI: client,
		Adapter:         native.NewAdapter(registry),
	}, nil
}

func (a *adapter) Info() (info *model.RegistryInfo, err error) {
	return &model.RegistryInfo{
		Type: model.RegistryTypeGitLab,
		SupportedResourceTypes: []string{
			model.ResourceTypeImage,
		},
		SupportedResourceFilters: []*model.FilterStyle{
			{
				Type:  model.FilterTypeName,
				Style: model.FilterStyleTypeText,
			},
			{
				Type:  model.FilterTypeTag,
				Style: model.FilterStyleTypeText,
			},
		},
		SupportedTriggers: []string{
			model.TriggerTypeManual,
			model.TriggerTypeScheduled,
		},
	}, nil
}

// FetchArtifacts fetches images
func (a *adapter) FetchArtifacts(filters []*model.Filter) ([]*model.Resource, error) {
	var resources []*model.Resource
	var projects []*Project
	var err error
	nameFilter := ""
	nameKind := ""
	tagFilter := ""
	tagKind := ""
	for _, filter := range filters {
		if filter.Type == model.FilterTypeName {
			nameFilter = filter.Value.(string)
			nameKind = filter.Kind
		} else if filter.Type == model.FilterTypeTag {
			tagFilter = filter.Value.(string)
			tagKind = filter.Kind
		}
	}

	projects, err = a.getProjectsByPattern(nameKind, nameFilter)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		projects, err = a.clientGitlabAPI.getProjects()
		if err != nil {
			return nil, err
		}
	}
	var pathPatterns []string

	if paths, ok := util.IsSpecificPathForKind(nameKind, nameFilter); ok {
		pathPatterns = paths
	} else {
		pathPatterns = append(pathPatterns, nameFilter)
	}
	log.Debugf("Patterns: %v", pathPatterns)

	pathMatchers := make([]*caseFoldingMatcher, 0, len(pathPatterns))
	for _, pathPattern := range pathPatterns {
		pathMatchers = append(pathMatchers, newCaseFoldingMatcher(nameKind, pathPattern))
	}
	tagMatcher := newCaseFoldingMatcher(tagKind, tagFilter)

	for _, project := range projects {
		if !project.RegistryEnabled {
			log.Debugf("Skipping project %s: Registry is not enabled", project.Name)
			continue
		}

		repositories, err := a.clientGitlabAPI.getRepositories(project.ID)
		if err != nil {
			return nil, err
		}
		if len(repositories) == 0 {
			continue
		}
		for _, repository := range repositories {
			if !existPatterns(repository.Path, pathMatchers) {
				log.Debugf("Skipping repository path=%s and id=%d", repository.Path, repository.ID)
				continue
			}
			log.Debugf("Search tags repository path=%s and id=%d", repository.Path, repository.ID)
			vTags, err := a.clientGitlabAPI.getTags(project.ID, repository.ID)
			if err != nil {
				return nil, err
			}
			if len(vTags) == 0 {
				continue
			}
			tags := []string{}
			for _, vTag := range vTags {
				if len(tagFilter) > 0 {
					if ok, _ := tagMatcher.Match(vTag.Name); !ok {
						continue
					}
				}
				tags = append(tags, vTag.Name)
			}
			info := make(map[string]any)
			info["location"] = repository.Location
			info["path"] = repository.Path

			resources = append(resources, &model.Resource{
				Type:     model.ResourceTypeImage,
				Registry: a.registry,
				Metadata: &model.ResourceMetadata{
					Repository: &model.Repository{
						Name:     strings.ToLower(repository.Path),
						Metadata: info,
					},
					Vtags: tags,
				},
			})
		}
	}
	return resources, nil
}

func (a *adapter) getProjectsByPattern(kind, expr string) ([]*Project, error) {
	var projects []*Project
	var err error
	// the fallback below derives a project name from the glob syntax of the pattern,
	// which says nothing about a regex: list all projects and let the filters decide
	if kind == pattern.KindRegex {
		return nil, nil
	}
	if len(expr) > 0 {
		names, ok := util.IsSpecificPathForKind(kind, expr)
		if ok {
			for _, name := range names {
				var projectsByName, err = a.clientGitlabAPI.getProjectsByName(url.QueryEscape(name))
				if err != nil {
					return nil, err
				}
				if projectsByName == nil {
					continue
				}
				projects = append(projects, projectsByName...)
			}
		} else {
			projectName := ""
			for i, substring := range strings.Split(expr, "/") {
				if strings.Contains(substring, "*") {
					if i != 0 {
						break
					}
				} else {
					projectName += substring + "/"
				}
			}
			if projectName == "" {
				return projects, nil
			}
			projects, err = a.clientGitlabAPI.getProjectsByName(url.QueryEscape(projectName))
			if err != nil {
				return nil, err
			}
		}
	}
	return projects, nil
}

func existPatterns(path string, matchers []*caseFoldingMatcher) bool {
	correct := false
	if len(matchers) > 0 {
		for _, matcher := range matchers {
			if ok, _ := matcher.Match(path); ok {
				correct = true
				break
			}
		}
	} else {
		correct = true
	}
	return correct
}

// caseFoldingMatcher matches gitlab repository paths and tags, which the adapter
// compares case insensitively. Only doublestar patterns are folded: lowercasing a
// regex would rewrite classes such as \D or [A-Z], the user has (?i) for that.
type caseFoldingMatcher struct {
	matcher *pattern.Matcher
	fold    bool
}

func newCaseFoldingMatcher(kind, expr string) *caseFoldingMatcher {
	fold := kind != pattern.KindRegex
	if fold {
		expr = strings.ToLower(expr)
	}
	return &caseFoldingMatcher{
		matcher: pattern.NewMatcher(kind, expr),
		fold:    fold,
	}
}

func (c *caseFoldingMatcher) Match(value string) (bool, error) {
	if c.fold {
		value = strings.ToLower(value)
	}
	return c.matcher.Match(value)
}
