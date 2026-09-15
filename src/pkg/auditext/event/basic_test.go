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

package event

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ctlevent "github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

func TestEventResolver_PreCheck(t *testing.T) {
	type fields struct {
		ResourceType        string
		SucceedCodes        []int
		SensitiveAttributes []string
		ShouldResolveName   bool
		IDToNameFunc        ResolveIDToNameFunc
		ResourceIDPattern   string
	}
	type args struct {
		ctx    context.Context
		url    string
		method string
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		wantCapture      bool
		wantResourceName string
	}{
		{"test normal", fields{ResourceIDPattern: `/api/v2.0/tests/(\d+)`, ResourceType: "test", SucceedCodes: []int{200}, ShouldResolveName: true, IDToNameFunc: func(context.Context, string) string { return "test" }}, args{context.Background(), "/api/v2.0/tests/123", "DELETE"}, true, "test"},
		{"test resource name", fields{ResourceIDPattern: `/api/v2.0/tests/(\d+)`, ResourceType: "test", SucceedCodes: []int{200}, ShouldResolveName: true, IDToNameFunc: func(context.Context, string) string { return "test_resource_name" }}, args{context.Background(), "/api/v2.0/tests/234", "DELETE"}, true, "test_resource_name"},
		{"test no resource name", fields{ResourceIDPattern: `/api/v2.0/tests/(\d+)`, ResourceType: "test", SucceedCodes: []int{200}, ShouldResolveName: true}, args{context.Background(), "/api/v2.0/tests/234", "GET"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Resolver{
				ResourceIDPattern:   tt.fields.ResourceIDPattern,
				ResourceType:        tt.fields.ResourceType,
				SucceedCodes:        tt.fields.SucceedCodes,
				SensitiveAttributes: tt.fields.SensitiveAttributes,
				ShouldResolveName:   tt.fields.ShouldResolveName,
				IDToNameFunc:        tt.fields.IDToNameFunc,
			}
			gotCapture, gotResourceName := e.PreCheck(tt.args.ctx, tt.args.url, tt.args.method)
			if gotCapture != tt.wantCapture {
				t.Errorf("EventResolver.PreCheck() gotCapture = %v, want %v", gotCapture, tt.wantCapture)
			}
			if gotResourceName != tt.wantResourceName {
				t.Errorf("EventResolver.PreCheck() gotResourceName = %v, want %v", gotResourceName, tt.wantResourceName)
			}
		})
	}
}

func TestResolver_Resolve_Description(t *testing.T) {
	r := &Resolver{
		ResourceType:      "registry",
		SucceedCodes:      []int{http.StatusCreated, http.StatusOK},
		ResourceIDPattern: `^/api/v2.0/registries/(\d+)$`,
		Details: func(_ *commonevent.Metadata, _ *model.CommonEvent, d *desc.Description) {
			d.Set("type", "harbor")
		},
	}
	evt := &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "admin", RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/registries/3", ResponseCode: http.StatusOK}, evt))
	data := evt.Data.(*model.CommonEvent)
	assert.True(t, data.IsSuccessful)
	assert.Equal(t, `admin deleted registry "3": type "harbor"`, data.OperationDescription)

	evt = &event.Event{}
	assert.NoError(t, r.Resolve(&commonevent.Metadata{Username: "admin", RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/registries/3", ResponseCode: http.StatusNotFound,
		ResponseBody: `{"errors":[{"code":"NOT_FOUND","message":"registry 3 not found"}]}`}, evt))
	data = evt.Data.(*model.CommonEvent)
	assert.False(t, data.IsSuccessful)
	assert.Equal(t, `admin deleted registry "3": type "harbor" (failed: 404 registry 3 not found)`, data.OperationDescription)
}

// The richer description is the only change: every other field of the
// event, and whether an event fires at all, is the same as before.
func TestResolver_Resolve_OnlyDescriptionChanged(t *testing.T) {
	r := &Resolver{
		ResourceType:      "registry",
		SucceedCodes:      []int{http.StatusCreated, http.StatusOK},
		ResourceIDPattern: `^/api/v2.0/registries/(\d+)$`,
		Details:           func(_ *commonevent.Metadata, _ *model.CommonEvent, d *desc.Description) { d.Set("type", "harbor") },
	}
	plain := &Resolver{ResourceType: "registry", SucceedCodes: []int{http.StatusCreated, http.StatusOK}, ResourceIDPattern: `^/api/v2.0/registries/(\d+)$`}
	for _, code := range []int{http.StatusOK, http.StatusCreated, http.StatusBadRequest, http.StatusForbidden, http.StatusInternalServerError} {
		ce := &commonevent.Metadata{Username: "admin", RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/registries/3", ResponseCode: code, ResponseBody: "boom"}
		got, want := &event.Event{}, &event.Event{}
		assert.NoError(t, r.Resolve(ce, got))
		assert.NoError(t, plain.Resolve(ce, want))
		gotData, wantData := got.Data.(*model.CommonEvent), want.Data.(*model.CommonEvent)
		assert.Equal(t, ctlevent.TopicCommonEvent, got.Topic)
		assert.Equal(t, wantData.IsSuccessful, gotData.IsSuccessful, "op_result for %d", code)
		assert.Equal(t, wantData.Operation, gotData.Operation)
		assert.Equal(t, wantData.ResourceType, gotData.ResourceType)
		assert.Equal(t, wantData.ResourceName, gotData.ResourceName)
		assert.Equal(t, wantData.Operator, gotData.Operator)
		assert.Equal(t, wantData.ProjectID, gotData.ProjectID)
		assert.WithinDuration(t, wantData.OccurAt, gotData.OccurAt, time.Second)
		assert.NotEmpty(t, gotData.OperationDescription)
		assert.LessOrEqual(t, len(gotData.OperationDescription), desc.MaxLen)
	}
	// no Details hook: the generic line still stands on its own
	evt := &event.Event{}
	assert.NoError(t, plain.Resolve(&commonevent.Metadata{Username: "admin", RequestMethod: http.MethodDelete, RequestURL: "/api/v2.0/registries/3", ResponseCode: http.StatusOK}, evt))
	assert.Equal(t, `admin deleted registry "3"`, evt.Data.(*model.CommonEvent).OperationDescription)
}
