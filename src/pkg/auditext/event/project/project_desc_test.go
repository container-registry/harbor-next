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

package project

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	mockproject "github.com/goharbor/harbor/src/testing/controller/project"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

func TestDiffMetadata(t *testing.T) {
	d := desc.New("admin", "update", "project").Name("myproj")
	diffMetadata(d,
		map[string]string{"public": "false", "severity": "low", "retention_id": "7", "auto_scan": "true"},
		map[string]string{"public": "true", "severity": "critical", "auto_scan": "true", "prevent_vul": "true"})
	assert.Equal(t, `admin updated project "myproj": prevent_vul set true, public false -> true, retention_id removed (was "7"), severity "low" -> "critical"`, d.String())
}

func TestAttemptedMetadata(t *testing.T) {
	before := map[string]string{"public": "false", "retention_id": "7"}
	tests := []struct {
		name string
		ce   *commonevent.Metadata
		want map[string]string
	}{
		{"project PUT", &commonevent.Metadata{RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/projects/myproj",
			RequestPayload: `{"public":true,"metadata":{"severity":"high"}}`},
			map[string]string{"public": "true", "retention_id": "7", "severity": "high"}},
		{"metadata POST", &commonevent.Metadata{RequestMethod: http.MethodPost, RequestURL: "/api/v2.0/projects/myproj/metadatas/",
			RequestPayload: `{"auto_scan":"true"}`},
			map[string]string{"public": "false", "retention_id": "7", "auto_scan": "true"}},
		{"metadata PUT", &commonevent.Metadata{RequestMethod: http.MethodPut, RequestURL: "/api/v2.0/projects/myproj/metadatas/public",
			RequestPayload: `{"public":"true"}`},
			map[string]string{"public": "true", "retention_id": "7"}},
		{"metadata DELETE", &commonevent.Metadata{RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/projects/myproj/metadatas/retention_id"},
			map[string]string{"public": "false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, attemptedMetadata(tt.ce, before))
		})
	}
	assert.Equal(t, map[string]string{"public": "false", "retention_id": "7"}, before, "input must not be modified")
}

func TestResolveWithSnapshot(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)
	mockCtrl := &mockproject.Controller{}
	oldCtrl := project.Ctl
	project.Ctl = mockCtrl
	defer func() { project.Ctl = oldCtrl }()

	before := &proModels.Project{ProjectID: 5, Name: "myproj", Metadata: map[string]string{"public": "false", "severity": "low"}}
	stored := &proModels.Project{ProjectID: 5, Name: "myproj", Metadata: map[string]string{"public": "true", "severity": "low"}}
	mockCtrl.On("Get", mock.Anything, "myproj").Return(stored, nil)

	r := &resolver{}
	t.Run("success diffs stored state", func(t *testing.T) {
		ce := &commonevent.Metadata{Ctx: context.Background(), Username: "admin", RequestURL: "/api/v2.0/projects/myproj",
			RequestMethod: http.MethodPut, RequestPayload: `{"public":true}`, ResponseCode: http.StatusOK, Before: before}
		evt := &event.Event{}
		assert.NoError(t, r.Resolve(ce, evt))
		data := evt.Data.(*model.CommonEvent)
		assert.True(t, data.IsSuccessful)
		assert.Equal(t, `admin updated project "myproj": public false -> true`, data.OperationDescription)
	})
	t.Run("failure diffs the request against the snapshot", func(t *testing.T) {
		ce := &commonevent.Metadata{Ctx: context.Background(), Username: "admin", RequestURL: "/api/v2.0/projects/myproj",
			RequestMethod: http.MethodPut, RequestPayload: `{"metadata":{"severity":"bogus"}}`, ResponseCode: http.StatusBadRequest,
			ResponseBody: `{"errors":[{"code":"BAD_REQUEST","message":"invalid severity"}]}`, Before: before}
		evt := &event.Event{}
		assert.NoError(t, r.Resolve(ce, evt))
		data := evt.Data.(*model.CommonEvent)
		assert.False(t, data.IsSuccessful)
		assert.Equal(t, `admin updated project "myproj": severity "low" -> "bogus" (failed: 400 invalid severity)`, data.OperationDescription)
	})
	t.Run("snapshot loads the project", func(t *testing.T) {
		got := r.Snapshot(&commonevent.Metadata{Ctx: context.Background(), RequestURL: "/api/v2.0/projects/myproj", RequestMethod: http.MethodPut})
		assert.Equal(t, stored, got)
		assert.Nil(t, r.Snapshot(&commonevent.Metadata{Ctx: context.Background(), RequestURL: "/api/v2.0/other", RequestMethod: http.MethodPut}))
	})
}

func TestResolveMetadataRoutes(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 2)
	mockCtrl := &mockproject.Controller{}
	oldCtrl := project.Ctl
	project.Ctl = mockCtrl
	defer func() { project.Ctl = oldCtrl }()

	before := &proModels.Project{ProjectID: 5, Name: "myproj", Metadata: map[string]string{"public": "false", "prevent_vul": "false", "retention_id": "7"}}
	stored := &proModels.Project{ProjectID: 5, Name: "myproj", Metadata: map[string]string{"public": "false", "prevent_vul": "false", "retention_id": "7"}}
	mockCtrl.On("Get", mock.Anything, "myproj").Return(stored, nil)
	r := &resolver{}

	tests := []struct {
		name    string
		method  string
		url     string
		payload string
		code    int
		body    string
		after   map[string]string
		want    string
	}{
		{"metadata added", http.MethodPost, "/api/v2.0/projects/myproj/metadatas/", `{"auto_scan":"true"}`, http.StatusOK, "",
			map[string]string{"public": "false", "prevent_vul": "false", "retention_id": "7", "auto_scan": "true"},
			`alice updated project "myproj": auto_scan set true`},
		{"metadata add refused", http.MethodPost, "/api/v2.0/projects/myproj/metadatas/", `{"auto_scan":"true"}`, http.StatusConflict,
			`{"errors":[{"code":"CONFLICT","message":"key exists"}]}`, nil,
			`alice updated project "myproj": auto_scan set true (failed: 409 key exists)`},
		{"metadata changed", http.MethodPut, "/api/v2.0/projects/myproj/metadatas/prevent_vul", `{"prevent_vul":"true"}`, http.StatusOK, "",
			map[string]string{"public": "false", "prevent_vul": "true", "retention_id": "7"},
			`alice updated project "myproj": prevent_vul false -> true`},
		{"metadata change refused", http.MethodPut, "/api/v2.0/projects/myproj/metadatas/prevent_vul", `{"prevent_vul":"bogus"}`, http.StatusBadRequest,
			`{"errors":[{"code":"BAD_REQUEST","message":"invalid value"}]}`, nil,
			`alice updated project "myproj": prevent_vul false -> "bogus" (failed: 400 invalid value)`},
		{"metadata removed", http.MethodDelete, "/api/v2.0/projects/myproj/metadatas/retention_id", "", http.StatusOK, "",
			map[string]string{"public": "false", "prevent_vul": "false"},
			`alice updated project "myproj": retention_id removed (was "7")`},
		{"metadata remove refused", http.MethodDelete, "/api/v2.0/projects/myproj/metadatas/retention_id", "", http.StatusNotFound,
			`{"errors":[{"code":"NOT_FOUND","message":"not found"}]}`, nil,
			`alice updated project "myproj": retention_id removed (was "7") (failed: 404 not found)`},
		{"nothing changed", http.MethodPut, "/api/v2.0/projects/myproj", `{"metadata":{"public":"false"}}`, http.StatusOK, "", nil,
			`alice updated project "myproj"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored.Metadata = before.Metadata
			if tt.after != nil {
				stored.Metadata = tt.after
			}
			ce := &commonevent.Metadata{Ctx: context.Background(), Username: "alice", RequestURL: tt.url, RequestMethod: tt.method,
				RequestPayload: tt.payload, ResponseCode: tt.code, ResponseBody: tt.body, Before: before}
			evt := &event.Event{}
			assert.NoError(t, r.Resolve(ce, evt))
			data := evt.Data.(*model.CommonEvent)
			assert.Equal(t, tt.code == http.StatusOK, data.IsSuccessful)
			assert.Equal(t, "update", data.Operation)
			assert.Equal(t, int64(5), data.ProjectID)
			assert.Equal(t, tt.want, data.OperationDescription)
		})
	}
}
