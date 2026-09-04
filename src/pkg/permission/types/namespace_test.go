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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockNamespace for testing
type mockNamespace struct {
	kind     string
	identity any
}

func (m *mockNamespace) Kind() string {
	return m.kind
}

func (m *mockNamespace) Resource(subresources ...Resource) Resource {
	return ""
}

func (m *mockNamespace) Identity() any {
	return m.identity
}

func (m *mockNamespace) GetPolicies() []*Policy {
	return nil
}

func TestResourceAllowedInNamespace_Wildcard(t *testing.T) {
	projectNamespace := &mockNamespace{
		kind:     "project",
		identity: int64(123),
	}

	tests := []struct {
		name     string
		resource Resource
		ns       *mockNamespace
		expected bool
	}{
		{
			name:     "wildcard project repository matches project namespace",
			resource: Resource("/project/*/repository"),
			ns:       projectNamespace,
			expected: true,
		},
		{
			name:     "wildcard artifact matches project namespace",
			resource: Resource("/project/*/artifact"),
			ns:       projectNamespace,
			expected: true,
		},
		{
			name:     "wildcard for different kind does not match",
			resource: Resource("/project/*/repository"),
			ns: &mockNamespace{
				kind:     "system",
				identity: "/",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResourceAllowedInNamespace(tt.resource, tt.ns)
			assert.Equal(t, tt.expected, result)
		})
	}
}
