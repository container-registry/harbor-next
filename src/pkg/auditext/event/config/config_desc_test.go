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

package config

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

func TestIsSecret(t *testing.T) {
	for _, k := range []string{"ldap_search_password", "oidc_client_secret", "uaa_client_secret", "email_password", "robot_token_duration"} {
		assert.True(t, isSecret(k), k)
	}
	for _, k := range []string{"auth_mode", "ldap_url", "oidc_endpoint", "project_creation_restriction"} {
		assert.False(t, isSecret(k), k)
	}
}

func TestDiffConfig(t *testing.T) {
	before := map[string]any{"auth_mode": "db_auth", "ldap_url": "", "ldap_search_password": "old", "token_expiration": 30, "self_registration": true}
	req := map[string]any{"auth_mode": "ldap_auth", "ldap_url": "ldaps://ldap.corp", "ldap_search_password": "new", "token_expiration": float64(30), "ldap_timeout": float64(5), "oidc_client_secret": "same"}
	t.Run("stored values on success, secrets only when changed", func(t *testing.T) {
		after := map[string]any{"auth_mode": "ldap_auth", "ldap_url": "ldaps://ldap.corp", "ldap_search_password": "new", "token_expiration": 30, "ldap_timeout": 5, "oidc_client_secret": "same"}
		before["oidc_client_secret"] = "same"
		d := desc.New("admin", "update", "configuration")
		diffConfig(d, req, before, after)
		assert.Equal(t, `admin updated configuration: auth_mode "db_auth" -> "ldap_auth", ldap_search_password changed, ldap_timeout set 5, ldap_url "" -> "ldaps://ldap.corp"`, d.String())
	})
	t.Run("requested values on failure, secrets omitted", func(t *testing.T) {
		d := desc.New("admin", "update", "configuration")
		diffConfig(d, req, before, nil)
		assert.Equal(t, `admin updated configuration: auth_mode "db_auth" -> "ldap_auth", ldap_timeout set 5, ldap_url "" -> "ldaps://ldap.corp"`, d.String())
	})
}

func TestResolveWithSnapshot(t *testing.T) {
	orig := userCfgsFn
	defer func() { userCfgsFn = orig }()
	userCfgsFn = func(_ context.Context) map[string]any {
		return map[string]any{"auth_mode": "ldap_auth", "project_creation_restriction": "adminonly"}
	}
	before := map[string]any{"auth_mode": "db_auth", "project_creation_restriction": "everyone"}
	r := &resolver{}

	evt := &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "admin", RequestURL: "/api/v2.0/configurations", RequestMethod: http.MethodPut,
		RequestPayload: `{"auth_mode":"ldap_auth","project_creation_restriction":"adminonly"}`, ResponseCode: http.StatusOK, Before: before}, evt))
	assert.Equal(t, `admin updated configuration: auth_mode "db_auth" -> "ldap_auth", project_creation_restriction "everyone" -> "adminonly"`,
		evt.Data.(*model.CommonEvent).OperationDescription)

	evt = &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "admin", RequestURL: "/api/v2.0/configurations", RequestMethod: http.MethodPut,
		RequestPayload: `{"auth_mode":"ldap_auth"}`, ResponseCode: http.StatusBadRequest, ResponseBody: `{"errors":[{"code":"BAD_REQUEST","message":"ldap_url is required"}]}`, Before: before}, evt))
	data := evt.Data.(*model.CommonEvent)
	assert.False(t, data.IsSuccessful)
	assert.Equal(t, `admin updated configuration: auth_mode "db_auth" -> "ldap_auth" (failed: 400 ldap_url is required)`, data.OperationDescription)

	// without a snapshot the row still stands on its own
	evt = &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "admin", RequestURL: "/api/v2.0/configurations", RequestMethod: http.MethodPut, ResponseCode: http.StatusOK}, evt))
	assert.Equal(t, `admin updated configuration`, evt.Data.(*model.CommonEvent).OperationDescription)
	assert.Equal(t, map[string]any{"auth_mode": "ldap_auth", "project_creation_restriction": "adminonly"}, r.Snapshot(&commonevent.Metadata{RequestMethod: http.MethodPut}))
	assert.Nil(t, r.Snapshot(&commonevent.Metadata{RequestMethod: http.MethodPost}))
}

func TestSnapshotWithoutRequestConnection(t *testing.T) {
	r := &resolver{}
	// a context with no ORM has no connection to reuse, so nothing is read
	assert.Nil(t, r.Snapshot(&commonevent.Metadata{Ctx: context.Background(), RequestMethod: http.MethodPut}))
	assert.Nil(t, r.Snapshot(&commonevent.Metadata{Ctx: context.Background(), RequestMethod: http.MethodGet}))
}
