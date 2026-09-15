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
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common/rbac"
	ctlevent "github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	ext "github.com/goharbor/harbor/src/pkg/auditext/event"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

func init() {
	var configureEventResolver = &resolver{}
	commonevent.RegisterResolver(`^/api/v2\.0/configurations$`, configureEventResolver)
}

// userCfgsFn reads the stored user configuration; overridable for tests
var userCfgsFn = func(ctx context.Context) map[string]any {
	mgr := config.DefaultMgr()
	if mgr == nil {
		return nil
	}
	// Runs inside the request before the handler, so it must reuse the request's
	// own connection rather than take a second one from the pool (#850).
	if _, err := orm.FromContext(ctx); err != nil {
		return nil
	}
	return mgr.GetUserCfgs(ctx)
}

// resolver used to resolve the configuration event
type resolver struct {
}

// Snapshot keeps the configuration as it was before the update.
func (c *resolver) Snapshot(m *commonevent.Metadata) any {
	if m.RequestMethod != http.MethodPut {
		return nil
	}
	return userCfgsFn(m.Ctx)
}

func (c *resolver) Resolve(ce *commonevent.Metadata, evt *event.Event) error {
	e := &model.CommonEvent{}
	e.Operation = "update"
	e.Operator = ce.Username
	e.ResourceType = rbac.ResourceConfiguration.String()
	e.ResourceName = rbac.ResourceConfiguration.String()
	e.OccurAt = time.Now()
	if ce.ResponseCode == http.StatusOK {
		e.IsSuccessful = true
	}

	d := desc.New(ce.Username, "update", rbac.ResourceConfiguration.String())
	if before, ok := ce.Before.(map[string]any); ok && before != nil {
		var req map[string]any
		if json.Unmarshal([]byte(ce.RequestPayload), &req) == nil {
			var after map[string]any
			if e.IsSuccessful {
				after = userCfgsFn(ce.Ctx)
			}
			diffConfig(d, req, before, after)
		}
	}
	if !e.IsSuccessful {
		d.Error(ce.ResponseCode, ce.ResponseBody)
	}
	e.OperationDescription = d.String()

	evt.Topic = ctlevent.TopicCommonEvent
	evt.Data = e
	return nil
}

// diffConfig records a change for every key the request carried. The new
// value is the stored one when the update succeeded (after != nil) and the
// requested one when it was rejected. Secrets only ever say "changed", and
// only when the stored value really did change.
func diffConfig(d *desc.Description, req, before, after map[string]any) {
	keys := make([]string, 0, len(req))
	for k := range req {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		from, had := before[k]
		if isSecret(k) {
			if after != nil && !desc.Equal(from, after[k]) {
				d.Secret(k)
			}
			continue
		}
		to := req[k]
		if after != nil {
			if v, ok := after[k]; ok {
				to = v
			}
		}
		if had {
			d.Changed(k, from, to)
		} else {
			d.Added(k, to)
		}
	}
}

// isSecret is true for keys typed as passwords in the config metadata and,
// because uaa_client_secret is only a StringType there, for any key that
// names a password, secret or token.
func isSecret(key string) bool {
	if item, ok := metadata.Instance().GetByName(key); ok {
		if _, isPwd := item.ItemType.(*metadata.PasswordType); isPwd {
			return true
		}
	}
	k := strings.ToLower(key)
	return strings.Contains(k, "password") || strings.Contains(k, "secret") || strings.Contains(k, "token")
}

func (c *resolver) PreCheck(ctx context.Context, _ string, method string) (bool, string) {
	if method != http.MethodPut {
		return false, ""
	}
	return config.AuditLogEventEnabled(ctx, fmt.Sprintf("%v_%v", ext.MethodToOperation(method), rbac.ResourceConfiguration.String())), ""
}
