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

package user

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/pkg/auditext/event"
	notifierevent "github.com/goharbor/harbor/src/pkg/notifier/event"
)

func newResolver() *userEventResolver {
	r := &userEventResolver{Resolver: event.Resolver{
		ResourceType:      rbac.ResourceUser.String(),
		SucceedCodes:      []int{http.StatusCreated, http.StatusOK},
		ShouldResolveName: true,
		IDToNameFunc:      userIDToName,
		ResourceIDPattern: urlPattern,
	}}
	r.Details = r.details
	return r
}

func stubUsers(users map[int]*models.User) func() {
	orig := getUserFn
	getUserFn = func(_ context.Context, id int) (*models.User, error) {
		if u, ok := users[id]; ok {
			return u, nil
		}
		return nil, errors.New("not found")
	}
	return func() { getUserFn = orig }
}

func resolve(t *testing.T, ce *commonevent.Metadata) *model.CommonEvent {
	t.Helper()
	evt := &notifierevent.Event{}
	assert.NoError(t, newResolver().Resolve(ce, evt))
	data, ok := evt.Data.(*model.CommonEvent)
	if !ok {
		t.Fatal("no event data")
	}
	return data
}

func TestUserDescriptions(t *testing.T) {
	alice := &models.User{UserID: 42, Username: "alice", Email: "alice@x.io", Realname: "Alice Doe", SysAdminFlag: false}
	defer stubUsers(map[int]*models.User{42: alice})()

	t.Run("create", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPost, RequestURL: "/api/v2.0/users",
			ResponseCode: http.StatusCreated, ResponseLocation: "/api/v2.0/users/42"})
		assert.Equal(t, "alice", data.ResourceName)
		assert.Equal(t, `admin created user "alice": id 42, email "alice@x.io", realname "Alice Doe", sysadmin false`, data.OperationDescription)
	})
	t.Run("create rejected names the requested user", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPost, RequestURL: "/api/v2.0/users",
			RequestPayload: `{"username":"alice","password":"hunter2"}`, ResponseCode: http.StatusConflict,
			ResponseBody: `{"errors":[{"code":"CONFLICT","message":"user alice already exists"}]}`})
		assert.False(t, data.IsSuccessful)
		assert.Equal(t, `admin created user "alice" (failed: 409 user alice already exists)`, data.OperationDescription)
	})
	t.Run("profile update with snapshot", func(t *testing.T) {
		before := &models.User{UserID: 42, Username: "alice", Email: "a@x.io", Realname: "A"}
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42",
			RequestPayload: `{"email":"alice@x.io","realname":"Alice Doe"}`, ResponseCode: http.StatusOK, Before: before})
		assert.Equal(t, `admin updated user "alice": email "a@x.io" -> "alice@x.io", realname "A" -> "Alice Doe"`, data.OperationDescription)
	})
	t.Run("profile update rejected diffs the request", func(t *testing.T) {
		before := &models.User{UserID: 42, Username: "alice", Email: "a@x.io", Realname: "Alice Doe"}
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42",
			RequestPayload: `{"email":"not-an-email","realname":"Alice Doe"}`, ResponseCode: http.StatusBadRequest, Before: before})
		assert.Equal(t, `admin updated user "alice": email "a@x.io" -> "not-an-email" (failed: 400 bad request)`, data.OperationDescription)
	})
	t.Run("profile update without snapshot", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42",
			RequestPayload: `{"email":"alice@x.io"}`, ResponseCode: http.StatusOK})
		assert.Equal(t, `admin updated user "alice": email "alice@x.io", realname "Alice Doe"`, data.OperationDescription)
	})
	t.Run("password never carries a value", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "alice", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42/password",
			RequestPayload: `{"old_password":"a","new_password":"b"}`, ResponseCode: http.StatusOK})
		assert.Equal(t, `alice updated user "alice": password changed`, data.OperationDescription)
		data = resolve(t, &commonevent.Metadata{Username: "alice", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42/password",
			RequestPayload: `{"old_password":"a","new_password":"b"}`, ResponseCode: http.StatusForbidden})
		assert.Equal(t, `alice updated user "alice" (failed: 403 forbidden)`, data.OperationDescription)
	})
	t.Run("sysadmin flag", func(t *testing.T) {
		before := &models.User{UserID: 42, Username: "alice", SysAdminFlag: false}
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42/sysadmin",
			RequestPayload: `{"sysadmin_flag":true}`, ResponseCode: http.StatusOK, Before: before})
		assert.Equal(t, `admin updated user "alice": sysadmin false -> true`, data.OperationDescription)
		data = resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42/sysadmin",
			RequestPayload: `{"sysadmin_flag":true}`, ResponseCode: http.StatusOK})
		assert.Equal(t, `admin updated user "alice": sysadmin true`, data.OperationDescription)
	})
	t.Run("delete with snapshot", func(t *testing.T) {
		before := &models.User{UserID: 42, Username: "alice", Email: "alice@x.io", SysAdminFlag: true}
		data := resolve(t, &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/users/42",
			ResourceName: "alice", ResponseCode: http.StatusOK, Before: before})
		assert.Equal(t, `admin deleted user "alice": id 42, email "alice@x.io", sysadmin true`, data.OperationDescription)
	})
	t.Run("snapshot", func(t *testing.T) {
		r := newResolver()
		assert.Equal(t, alice, r.Snapshot(&commonevent.Metadata{RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42"}))
		assert.Nil(t, r.Snapshot(&commonevent.Metadata{RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/7"}))
		assert.Nil(t, r.Snapshot(&commonevent.Metadata{RequestMethod: http.MethodPost, RequestURL: "/api/v2.0/users"}))
	})
}

func TestUserDescriptionsOnFailure(t *testing.T) {
	alice := &models.User{UserID: 42, Username: "alice", Email: "alice@x.io", SysAdminFlag: false}
	defer stubUsers(map[int]*models.User{42: alice})()

	t.Run("sysadmin flag refused", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "bob", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42/sysadmin",
			RequestPayload: `{"sysadmin_flag":true}`, ResponseCode: http.StatusForbidden, Before: alice,
			ResponseBody: `{"errors":[{"code":"FORBIDDEN","message":"forbidden"}]}`})
		assert.False(t, data.IsSuccessful)
		assert.Equal(t, `bob updated user "alice": sysadmin false -> true (failed: 403 forbidden)`, data.OperationDescription)
	})
	t.Run("delete refused keeps the facts", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "alice", RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/users/42",
			ResourceName: "alice", ResponseCode: http.StatusForbidden, Before: alice,
			ResponseBody: `{"errors":[{"code":"FORBIDDEN","message":"cannot delete self"}]}`})
		assert.False(t, data.IsSuccessful)
		assert.Equal(t, `alice deleted user "alice": id 42, email "alice@x.io", sysadmin false (failed: 403 cannot delete self)`, data.OperationDescription)
	})
	t.Run("password refused with an unparseable body", func(t *testing.T) {
		data := resolve(t, &commonevent.Metadata{Username: "alice", RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/users/42/password",
			RequestPayload: `{"old_password":"a","new_password":"b"}`, ResponseCode: http.StatusInternalServerError, ResponseBody: "<html>oops</html>"})
		assert.Equal(t, `alice updated user "alice" (failed: 500 internal server error)`, data.OperationDescription)
		assert.NotContains(t, data.OperationDescription, "new_password")
	})
}
