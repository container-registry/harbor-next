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

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/server/v2.0/restapi/operations/permissions"
	testsecurity "github.com/goharbor/harbor/src/testing/common/security"
)

func TestGetPermissionsAccess(t *testing.T) {
	cases := []struct {
		name          string
		authenticated bool
		sysAdmin      bool
		status        int
		wantSystem    bool
	}{
		{"anonymous is rejected", false, false, 401, false},
		{"plain user gets the project catalog", true, false, 200, false},
		{"system admin also gets the system catalog", true, true, 200, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secCtx := &testsecurity.Context{}
			secCtx.On("IsAuthenticated").Return(tc.authenticated)
			secCtx.On("IsSysAdmin").Return(tc.sysAdmin)
			secCtx.On("GetUsername").Return("")
			ctx := security.NewContext(context.Background(), secCtx)

			rsp := newPermissionsAPIAPI().GetPermissions(ctx, permissions.GetPermissionsParams{})
			if tc.status == 200 {
				ok, isOK := rsp.(*permissions.GetPermissionsOK)
				if !assert.True(t, isOK) {
					return
				}
				assert.NotEmpty(t, ok.Payload.Project)
				assert.Equal(t, tc.wantSystem, len(ok.Payload.System) > 0)
				return
			}
			_, isOK := rsp.(*permissions.GetPermissionsOK)
			assert.False(t, isOK)
		})
	}
}
