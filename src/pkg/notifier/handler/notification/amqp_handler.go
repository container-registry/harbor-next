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
	"net/url"
	"strings"

	"github.com/goharbor/harbor/src/common/job/models"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/pkg/notification"
	"github.com/goharbor/harbor/src/pkg/notifier/model"
)

const (
	// AMQPContentType defines the content type for AMQP messages
	AMQPContentType = "application/json"
)

// AMQPHandler preprocess event data to amqp and start the hook processing
type AMQPHandler struct {
}

// Name ...
func (a *AMQPHandler) Name() string {
	return "AMQP"
}

// Handle handles event to amqp
func (a *AMQPHandler) Handle(ctx context.Context, value any) error {
	if value == nil {
		return fmt.Errorf("AMQPHandler cannot handle nil value")
	}

	event, ok := value.(*model.HookEvent)
	if !ok || event == nil {
		return fmt.Errorf("invalid notification amqp event")
	}

	return a.process(ctx, event)
}

// IsStateful ...
func (a *AMQPHandler) IsStateful() bool {
	return false
}

func (a *AMQPHandler) process(ctx context.Context, event *model.HookEvent) error {
	if event.Payload == nil {
		return fmt.Errorf("invalid AMQP event: nil payload")
	}
	if event.Target == nil {
		return fmt.Errorf("invalid AMQP event: nil target")
	}

	brokerURL, queue, err := splitAMQPAddress(event.Target.Address)
	if err != nil {
		return fmt.Errorf("invalid AMQP target address: %v", err)
	}

	j := &models.JobData{
		Metadata: &models.JobMetadata{
			JobKind: job.KindGeneric,
		},
	}
	// Create an amqpJob to publish to amqp
	j.Name = job.AMQPJobVendorType

	// Convert payload to amqp format
	payload, err := a.convert(event.Payload)
	if err != nil {
		return fmt.Errorf("convert payload to amqp failed: %v", err)
	}

	j.Parameters = map[string]any{
		"payload":          payload,
		"broker_url":       brokerURL,
		"queue":            queue,
		"content_type":     AMQPContentType,
		"auth":             event.Target.AuthHeader,
		"skip_cert_verify": event.Target.SkipCertVerify,
	}
	return notification.HookManager.StartHook(ctx, event, j)
}

// splitAMQPAddress splits a normalized amqp(s):// target address (as
// rewritten by the webhook API's validateTargets) into the broker
// connection URL, including any vhost, and the destination queue name,
// which is the final path segment.
// e.g. amqp://broker:5672/vhost/queue -> ("amqp://broker:5672/vhost", "queue")
//
// Splitting is done on the escaped path so a queue name containing an
// encoded slash (%2F) is not mistaken for a path separator; only the
// extracted queue segment is then unescaped, since it is used verbatim as
// a routing key rather than re-embedded in a URL.
func splitAMQPAddress(address string) (brokerURL string, queue string, err error) {
	u, err := url.Parse(address)
	if err != nil {
		return "", "", err
	}
	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(segments) == 0 || segments[len(segments)-1] == "" {
		return "", "", fmt.Errorf("address %q has no queue name in its path", address)
	}
	queue, err = url.PathUnescape(segments[len(segments)-1])
	if err != nil {
		return "", "", fmt.Errorf("address %q has an invalid queue segment: %w", address, err)
	}
	vhost := strings.Join(segments[:len(segments)-1], "/")
	return u.Scheme + "://" + u.Host + "/" + vhost, queue, nil
}

func (a *AMQPHandler) convert(payLoad *model.Payload) (string, error) {
	// For AMQP, send the full payload as JSON
	payloadBytes, err := json.Marshal(payLoad)
	if err != nil {
		return "", fmt.Errorf("failed to marshal amqp payload: %v", err)
	}
	return string(payloadBytes), nil
}
