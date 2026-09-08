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

package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// System robots may read the global storage quota but never change it; the
// write actions stay with system administrators only.
func TestSystemQuotaRobotPolicies(t *testing.T) {
	actions := map[Action]bool{}
	for _, p := range PoliciesMap[ScopeSystem] {
		if p.Resource == ResourceSystemQuota {
			actions[p.Action] = true
		}
	}
	assert.True(t, actions[ActionRead])
	assert.False(t, actions[ActionUpdate])
	assert.False(t, actions[ActionDelete])
}
