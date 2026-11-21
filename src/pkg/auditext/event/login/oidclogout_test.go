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

package login

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

func Test_oidcLogout_Resolve(t *testing.T) {
	r := &oidcLogoutResolver{}
	evt := &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "alice", RequestURL: "/c/oidc/logout", RequestMethod: http.MethodGet, ResponseCode: http.StatusFound}, evt))
	data := evt.Data.(*model.CommonEvent)
	assert.True(t, data.IsSuccessful)
	assert.Equal(t, "logout", data.Operation)
	assert.Equal(t, `alice logged out: auth_mode "oidc_auth"`, data.OperationDescription)

	evt = &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "alice", RequestURL: "/c/oidc/logout", RequestMethod: http.MethodGet, ResponseCode: http.StatusInternalServerError,
		ResponseBody: `{"errors":[{"code":"INTERNAL","message":"end-session request failed"}]}`}, evt))
	data = evt.Data.(*model.CommonEvent)
	assert.False(t, data.IsSuccessful)
	assert.Equal(t, `alice logged out: auth_mode "oidc_auth" (failed: 500 end-session request failed)`, data.OperationDescription)
}
