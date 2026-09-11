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

package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/job/models"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/pkg/notification"
	"github.com/goharbor/harbor/src/pkg/notifier/model"
)

const (
	// EmailSubjectTemplate defines email subject template
	EmailSubjectTemplate = "Harbor Event: %s"
	// EmailBodyTemplate defines email body template
	EmailBodyTemplate = `Harbor Event Notification

Event Type: %s
Occurred At: %d
Operator: %s

Event Data:
%s
`
)

// EmailHandler preprocess event data to email and start the hook processing
type EmailHandler struct {
}

// Name ...
func (e *EmailHandler) Name() string {
	return "Email"
}

// Handle handles event to email
func (e *EmailHandler) Handle(ctx context.Context, value any) error {
	if value == nil {
		return fmt.Errorf("EmailHandler cannot handle nil value")
	}

	event, ok := value.(*model.HookEvent)
	if !ok || event == nil {
		return fmt.Errorf("invalid notification email event")
	}

	return e.process(ctx, event)
}

// IsStateful ...
func (e *EmailHandler) IsStateful() bool {
	return false
}

func (e *EmailHandler) process(ctx context.Context, event *model.HookEvent) error {
	if event.Payload == nil {
		return fmt.Errorf("invalid email event: nil payload")
	}
	if event.Target == nil {
		return fmt.Errorf("invalid email event: nil target")
	}

	j := &models.JobData{
		Metadata: &models.JobMetadata{
			JobKind: job.KindGeneric,
		},
	}
	j.Name = job.EmailJobVendorType

	subject, body, err := e.convert(event.Payload)
	if err != nil {
		return fmt.Errorf("convert payload to email failed: %v", err)
	}

	j.Parameters = map[string]any{
		"subject": subject,
		"body":    body,
		"to":      event.Target.Address,
	}

	mgr := config.DefaultMgr()
	if err := mgr.Load(ctx); err != nil {
		j.Parameters["address"] = ""
		j.Parameters["from"] = ""
	} else {
		j.Parameters["address"] = mgr.Get(ctx, common.EmailHost).GetString()
		j.Parameters["from"] = mgr.Get(ctx, common.EmailFrom).GetString()

		if username := mgr.Get(ctx, common.EmailUsername).GetString(); username != "" {
			j.Parameters["username"] = username
		}
		if password := mgr.Get(ctx, common.EmailPassword).GetPassword(); password != "" {
			j.Parameters["password"] = password
		}
		if port := mgr.Get(ctx, common.EmailPort).GetInt(); port > 0 {
			j.Parameters["port"] = port
		}
		if useSSL := mgr.Get(ctx, common.EmailSSL).GetBool(); useSSL {
			j.Parameters["use_ssl"] = useSSL
		}
		if insecure := mgr.Get(ctx, common.EmailInsecure).GetBool(); insecure {
			j.Parameters["insecure_skip_verify"] = insecure
		}
	}

	if event.Target.SkipCertVerify {
		j.Parameters["insecure_skip_verify"] = true
	}

	return notification.HookManager.StartHook(ctx, event, j)
}

func (e *EmailHandler) convert(payLoad *model.Payload) (string, string, error) {
	eventData, err := json.MarshalIndent(payLoad.EventData, "", "\t")
	if err != nil {
		return "", "", fmt.Errorf("marshal from eventData %v failed: %v", payLoad.EventData, err)
	}

	subject := fmt.Sprintf(EmailSubjectTemplate, payLoad.Type)
	body := fmt.Sprintf(EmailBodyTemplate, payLoad.Type, payLoad.OccurAt, payLoad.Operator, string(eventData))

	return subject, body, nil
}
