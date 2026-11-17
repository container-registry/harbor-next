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

package image

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

type configurableRegistry struct {
	fakeRegistry
	got *model.SourceOptions
	err error
}

func (c *configurableRegistry) ConfigureSource(opts *model.SourceOptions) error {
	c.got = opts
	return c.err
}
func (c *configurableRegistry) Info() (*model.RegistryInfo, error)     { return &model.RegistryInfo{}, nil }
func (c *configurableRegistry) PrepareForPush([]*model.Resource) error { return nil }
func (c *configurableRegistry) HealthCheck() (string, error)           { return model.Healthy, nil }

type configurableFactory struct{ reg *configurableRegistry }

func (f configurableFactory) Create(*model.Registry) (adapter.Adapter, error) { return f.reg, nil }
func (f configurableFactory) AdapterPattern() *model.AdapterPattern           { return nil }

func TestInitializeConfiguresTheSource(t *testing.T) {
	reg := &configurableRegistry{}
	require.NoError(t, adapter.RegisterFactory("TEST_TRANSFER_SOURCE_OPTIONS", configurableFactory{reg: reg}))
	resource := func(opts *model.SourceOptions) *model.Resource {
		return &model.Resource{Registry: &model.Registry{Type: "TEST_TRANSFER_SOURCE_OPTIONS"}, SourceOptions: opts}
	}
	tr := &transfer{logger: log.DefaultLogger()}

	opts := &model.SourceOptions{RevisionTag: true, Revisions: map[string]string{"sha-aaaaaaaaaaaa": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	require.NoError(t, tr.initialize(resource(opts), resource(nil)))
	assert.Equal(t, opts, reg.got, "the source adapter gets the options of the source resource")

	reg.err = errors.New("bad options")
	err := tr.initialize(resource(opts), resource(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad options")
}
