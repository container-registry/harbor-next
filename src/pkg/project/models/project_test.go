// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import "testing"

func TestProxyCacheAllowPush(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{name: "missing metadata"},
		{name: "disabled", metadata: map[string]string{ProMetaProxyCacheAllowPush: "false"}},
		{name: "invalid", metadata: map[string]string{ProMetaProxyCacheAllowPush: "invalid"}},
		{name: "enabled", metadata: map[string]string{ProMetaProxyCacheAllowPush: "true"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := &Project{Metadata: tt.metadata}
			if got := project.ProxyCacheAllowPush(); got != tt.want {
				t.Fatalf("ProxyCacheAllowPush() = %v, want %v", got, tt.want)
			}
		})
	}
}
