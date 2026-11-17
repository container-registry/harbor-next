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
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/rbac"
	ctlevent "github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	ext "github.com/goharbor/harbor/src/pkg/auditext/event"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
	"github.com/goharbor/harbor/src/server/v2.0/models"
)

func init() {
	var configureEventResolver = &resolver{}
	commonevent.RegisterResolver(`^/api/v2\.0/configurations$`, configureEventResolver)
}

// resolver used to resolve the configuration event
type resolver struct {
}

func (c *resolver) Resolve(ce *commonevent.Metadata, evt *event.Event) error {
	e := &model.CommonEvent{}
	e.Operation = "update"
	e.Operator = ce.Username
	e.ResourceType = rbac.ResourceConfiguration.String()
	e.ResourceName = rbac.ResourceConfiguration.String()
	e.OccurAt = time.Now()
	e.OperationDescription = "update configuration"
	before, _ := ce.Before.(map[string]string)
	if ce.ResponseCode == http.StatusOK {
		e.IsSuccessful = true
	}
	if changes := describeChanges(ce.RequestPayload, before); len(changes) > 0 {
		e.OperationDescription += ": " + strings.Join(changes, ", ")
	}
	evt.Topic = ctlevent.TopicCommonEvent
	evt.Data = e
	return nil
}

// maxValueLength caps each value shown in a change.
const maxValueLength = 64

// sensitiveWords mark setting names whose values are never shown.
var sensitiveWords = []string{"password", "passwd", "pwd", "secret", "token", "key", "cert", "credential", "private"}

// Snapshot reads the current values of the settings in the request before the
// update overwrites them.
func (c *resolver) Snapshot(ctx context.Context, payload string) any {
	return currentValues(ctx, requestedSettings(payload))
}

// currentValues reads the stored values of the given settings. Sensitive
// values are never read.
func currentValues(ctx context.Context, cfgs map[string]json.RawMessage) map[string]string {
	if len(cfgs) == 0 {
		return nil
	}
	current := config.GetCfgManager(ctx).GetUserCfgs(ctx)
	values := make(map[string]string, len(cfgs))
	for k := range cfgs {
		if v, ok := current[k]; ok && !sensitive(k) {
			values[k] = formatValue(v)
		}
	}
	return values
}

// requestedSettings returns the settings the update API applies from the
// request: it decodes the body into models.Configurations, which drops keys it
// does not accept (such as core_url) and nulls, and trims robot_name_prefix.
func requestedSettings(payload string) map[string]json.RawMessage {
	var conf models.Configurations
	if err := json.Unmarshal([]byte(payload), &conf); err != nil {
		return nil
	}
	buf, err := json.Marshal(&conf)
	if err != nil {
		return nil
	}
	var cfgs map[string]json.RawMessage
	if err := json.Unmarshal(buf, &cfgs); err != nil {
		return nil
	}
	for k := range cfgs {
		if _, known := metadata.Instance().GetByName(k); !known {
			delete(cfgs, k)
		}
	}
	if raw, ok := cfgs[common.RobotNamePrefix]; ok {
		var prefix string
		if json.Unmarshal(raw, &prefix) == nil {
			cfgs[common.RobotNamePrefix], _ = json.Marshal(strings.TrimSpace(prefix))
		}
	}
	return cfgs
}

// describeChanges lists "key old -> new" for each setting the request changes,
// and only "key changed" for sensitive ones. The new value comes from this
// request, so a concurrent save cannot show up in its entry.
func describeChanges(payload string, before map[string]string) []string {
	cfgs := requestedSettings(payload)
	keys := make([]string, 0, len(cfgs))
	for k := range cfgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	changes := make([]string, 0, len(keys))
	for _, k := range keys {
		if sensitive(k) {
			changes = append(changes, k+" changed")
			continue
		}
		var v any
		if err := json.Unmarshal(cfgs[k], &v); err != nil {
			continue
		}
		next := formatValue(v)
		old, known := before[k]
		switch {
		case !known:
			changes = append(changes, k+" -> "+displayValue(next))
		case old != next:
			changes = append(changes, k+" "+displayValue(old)+" -> "+displayValue(next))
		}
	}
	return changes
}

// sensitive reports settings the metadata types as passwords or maps, whose
// JSON may nest credentials, or whose name says they hold a secret.
func sensitive(key string) bool {
	if item, ok := metadata.Instance().GetByName(key); ok {
		switch item.ItemType.(type) {
		case *metadata.PasswordType, *metadata.MapType, *metadata.StringToStringMapType:
			return true
		}
	}
	lower := strings.ToLower(key)
	for _, w := range sensitiveWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

func formatValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// displayValue quotes values that would blur the list and truncates long ones.
func displayValue(s string) string {
	if utf8.RuneCountInString(s) > maxValueLength {
		s = string([]rune(s)[:maxValueLength-3]) + "..."
	}
	if s == "" || strings.ContainsAny(s, " ,") || strings.Contains(s, "->") ||
		strings.IndexFunc(s, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 {
		return strconv.Quote(s)
	}
	return s
}

func (c *resolver) PreCheck(ctx context.Context, _ string, method string) (bool, string) {
	if method != http.MethodPut {
		return false, ""
	}
	return config.AuditLogEventEnabled(ctx, fmt.Sprintf("%v_%v", ext.MethodToOperation(method), rbac.ResourceConfiguration.String())), ""
}
