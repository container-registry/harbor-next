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

package packageadapter

import (
	"net/http"
	"strings"
	"time"

	commonhttp "github.com/goharbor/harbor/src/common/http"
	"github.com/goharbor/harbor/src/lib/log"
	adp "github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func init() {
	for _, provider := range packageProviders {
		if err := adp.RegisterFactory(provider.registryType, &factory{provider: provider}); err != nil {
			log.Errorf("failed to register factory for %s: %v", provider.registryType, err)
			continue
		}
		log.Infof("the factory for adapter %s registered", provider.registryType)
	}
}

type provider struct {
	registryType string
	protocolType string
	endpoint     *model.Endpoint
}

var packageProviders = []provider{
	{registryType: model.RegistryTypeNPMJS, protocolType: model.RegistryTypeNPM, endpoint: &model.Endpoint{Key: "registry.npmjs.org", Value: "https://registry.npmjs.org"}},
	{registryType: model.RegistryTypeNPM, protocolType: model.RegistryTypeNPM},
	{registryType: model.RegistryTypeMavenCentral, protocolType: model.RegistryTypeMaven, endpoint: &model.Endpoint{Key: "Maven Central", Value: "https://repo.maven.apache.org/maven2"}},
	{registryType: model.RegistryTypeMaven, protocolType: model.RegistryTypeMaven},
	{registryType: model.RegistryTypePyPI, protocolType: model.RegistryTypePyPI, endpoint: &model.Endpoint{Key: "pypi.org", Value: "https://pypi.org/simple"}},
	{registryType: model.RegistryTypePyPIRegistry, protocolType: model.RegistryTypePyPI},
	{registryType: model.RegistryTypeCratesIO, protocolType: model.RegistryTypeCargo, endpoint: &model.Endpoint{Key: "index.crates.io", Value: "https://index.crates.io"}},
	{registryType: model.RegistryTypeCargo, protocolType: model.RegistryTypeCargo},
	{registryType: model.RegistryTypeGo, protocolType: model.RegistryTypeGo, endpoint: &model.Endpoint{Key: "proxy.golang.org", Value: "https://proxy.golang.org"}},
	{registryType: model.RegistryTypeGoRegistry, protocolType: model.RegistryTypeGo},
	{registryType: model.RegistryTypeGoSumDB, protocolType: model.RegistryTypeGoSumDB},
	{registryType: model.RegistryTypeHomebrew, protocolType: model.RegistryTypeHomebrew, endpoint: &model.Endpoint{Key: "formulae.brew.sh", Value: "https://formulae.brew.sh/api"}},
	{registryType: model.RegistryTypeHomebrewRegistry, protocolType: model.RegistryTypeHomebrew},
}

type factory struct {
	provider provider
}

func (f *factory) Create(registry *model.Registry) (adp.Adapter, error) {
	return &adapter{registry: registry, provider: f.provider}, nil
}

func (f *factory) AdapterPattern() *model.AdapterPattern {
	pattern := &model.AdapterPattern{EndpointPattern: model.NewDefaultEndpointPattern()}
	if f.provider.endpoint != nil {
		pattern.EndpointPattern.Endpoints = []*model.Endpoint{f.provider.endpoint}
	}
	return pattern
}

type adapter struct {
	registry *model.Registry
	provider provider
}

func (a *adapter) Info() (*model.RegistryInfo, error) {
	return &model.RegistryInfo{
		Type:                   a.provider.registryType,
		SupportedResourceTypes: []string{a.provider.protocolType},
		SupportedTriggers:      []string{model.TriggerTypeManual, model.TriggerTypeScheduled},
		SupportedResourceFilters: []*model.FilterStyle{{
			Type:  model.FilterTypeName,
			Style: model.FilterStyleTypeText,
		}},
	}, nil
}

func (a *adapter) PrepareForPush([]*model.Resource) error {
	return nil
}

func (a *adapter) HealthCheck() (string, error) {
	if a.registry == nil || strings.TrimSpace(a.registry.URL) == "" {
		return model.Unhealthy, nil
	}
	method := http.MethodHead
	checkPath := "/"
	if a.provider.protocolType == model.RegistryTypeGoSumDB {
		method = http.MethodGet
		checkPath = "/latest"
	}
	req, err := http.NewRequest(method, strings.TrimRight(a.registry.URL, "/")+checkPath, nil)
	if err != nil {
		return model.Unhealthy, nil
	}
	if a.registry.Credential != nil {
		req.SetBasicAuth(a.registry.Credential.AccessKey, a.registry.Credential.AccessSecret)
	}
	client := &http.Client{
		Transport: commonhttp.GetHTTPTransport(
			commonhttp.WithInsecure(a.registry.Insecure),
			commonhttp.WithCACert(a.registry.CACertificate),
		),
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		if err != nil {
			log.Warningf("failed to check package registry %s: %v", a.registry.URL, err)
		}
		return model.Unhealthy, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return model.Unhealthy, nil
	}
	return model.Healthy, nil
}
