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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	ctlevent "github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/lib/config"
	ext "github.com/goharbor/harbor/src/pkg/auditext/event"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

const resourceType = "feature"

var urlPattern = regexp.MustCompile(`^/api/v2\.0/features/([^/]{1,255})$`)

func init() {
	commonevent.RegisterResolver(urlPattern.String(), &resolver{})
}

// resolver records feature toggles from PUT /api/v2.0/features/{feature_key}
type resolver struct{}

func (r *resolver) Resolve(ce *commonevent.Metadata, evt *event.Event) error {
	m := urlPattern.FindStringSubmatch(ce.RequestURL)
	if ce.RequestMethod != http.MethodPut || len(m) < 2 {
		return nil
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	desc := fmt.Sprintf("update feature %s", m[1])
	if json.Unmarshal([]byte(ce.RequestPayload), &body) == nil && body.Enabled != nil {
		desc = fmt.Sprintf("%s, enabled: %t", desc, *body.Enabled)
	}
	evt.Topic = ctlevent.TopicCommonEvent
	evt.Data = &model.CommonEvent{
		Operator:             ce.Username,
		ResourceType:         resourceType,
		ResourceName:         m[1],
		Operation:            "update",
		OperationDescription: desc,
		IsSuccessful:         ce.ResponseCode == http.StatusOK,
		OccurAt:              time.Now(),
	}
	return nil
}

func (r *resolver) PreCheck(ctx context.Context, _ string, method string) (bool, string) {
	if method != http.MethodPut {
		return false, ""
	}
	return config.AuditLogEventEnabled(ctx, fmt.Sprintf("%v_%v", ext.MethodToOperation(method), resourceType)), ""
}
