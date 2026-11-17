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

package feature

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		payload string
		desc    string
	}{
		{"disable succeeded", 200, `{"enabled":false}`, "update feature enable_portal_bookmarks, enabled: false"},
		{"enable failed", 403, `{"enabled":true}`, "update feature enable_portal_bookmarks, enabled: true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := &event.Event{}
			ce := &commonevent.Metadata{
				Username:       "admin",
				RequestMethod:  "PUT",
				RequestURL:     "/api/v2.0/features/enable_portal_bookmarks",
				RequestPayload: tt.payload,
				ResponseCode:   tt.code,
			}
			assert.NoError(t, ce.Resolve(evt))
			e, ok := evt.Data.(*model.CommonEvent)
			assert.True(t, ok)
			assert.Equal(t, "admin", e.Operator)
			assert.Equal(t, "feature", e.ResourceType)
			assert.Equal(t, "enable_portal_bookmarks", e.ResourceName)
			assert.Equal(t, "update", e.Operation)
			assert.Equal(t, tt.desc, e.OperationDescription)
			assert.Equal(t, tt.code == 200, e.IsSuccessful)
		})
	}
}
