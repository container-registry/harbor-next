package notification

import (
	"context"
	"testing"
	"time"

	"github.com/goharbor/harbor/src/pkg/notification"
	policy_model "github.com/goharbor/harbor/src/pkg/notification/policy/model"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
	"github.com/goharbor/harbor/src/pkg/notifier/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAMQPHandler_Handle(t *testing.T) {
	hookMgr := notification.HookManager
	defer func() {
		notification.HookManager = hookMgr
	}()
	notification.HookManager = &fakedHookManager{}

	handler := &AMQPHandler{}

	type args struct {
		event *event.Event
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "AMQPHandler_Handle Want Error 1",
			args: args{
				event: &event.Event{
					Topic: "amqp",
					Data:  nil,
				},
			},
			wantErr: true,
		},
		{
			name: "AMQPHandler_Handle Want Error 2",
			args: args{
				event: &event.Event{
					Topic: "amqp",
					Data:  &model.EventData{},
				},
			},
			wantErr: true,
		},
		{
			name: "AMQPHandler_Handle Want Error 3 (nil payload)",
			args: args{
				event: &event.Event{
					Topic: "amqp",
					Data: &model.HookEvent{
						Target: &policy_model.EventTarget{
							Type:    "amqp",
							Address: "amqp://broker:5672/vhost/harbor.events",
						},
						Payload: nil,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "AMQPHandler_Handle Want Error 4 (nil target)",
			args: args{
				event: &event.Event{
					Topic: "amqp",
					Data: &model.HookEvent{
						Target: nil,
						Payload: &model.Payload{
							Type: "pushImage",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "AMQPHandler_Handle Want Error 5 (address has no queue segment)",
			args: args{
				event: &event.Event{
					Topic: "amqp",
					Data: &model.HookEvent{
						Target: &policy_model.EventTarget{
							Type:    "amqp",
							Address: "amqp://broker:5672",
						},
						Payload: &model.Payload{
							Type: "pushImage",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "AMQPHandler_Handle 1",
			args: args{
				event: &event.Event{
					Topic: "amqp",
					Data: &model.HookEvent{
						PolicyID:  1,
						EventType: "pushImage",
						Target: &policy_model.EventTarget{
							Type:    "amqp",
							Address: "amqp://broker:5672/vhost/harbor.events",
						},
						Payload: &model.Payload{
							OccurAt:  time.Now().Unix(),
							Type:     "pushImage",
							Operator: "admin",
							EventData: &model.EventData{
								Resources: []*model.Resource{
									{
										Tag: "v9.0",
									},
								},
								Repository: &model.Repository{
									Name: "library/debian",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Handle(context.TODO(), tt.args.event.Data)
			if tt.wantErr {
				require.NotNil(t, err, "Error: %s", err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAMQPHandler_IsStateful(t *testing.T) {
	handler := &AMQPHandler{}
	assert.False(t, handler.IsStateful())
}

func TestAMQPHandler_Name(t *testing.T) {
	handler := &AMQPHandler{}
	assert.Equal(t, "AMQP", handler.Name())
}

func TestSplitAMQPAddress(t *testing.T) {
	tests := []struct {
		name          string
		address       string
		wantBrokerURL string
		wantQueue     string
		wantErr       bool
	}{
		{
			name:          "vhost and queue",
			address:       "amqp://broker:5672/vhost/harbor.events",
			wantBrokerURL: "amqp://broker:5672/vhost",
			wantQueue:     "harbor.events",
		},
		{
			name:          "default vhost, queue only",
			address:       "amqp://broker:5672/harbor.events",
			wantBrokerURL: "amqp://broker:5672/",
			wantQueue:     "harbor.events",
		},
		{
			name:    "no queue segment",
			address: "amqp://broker:5672",
			wantErr: true,
		},
		{
			name:    "malformed URL",
			address: "://not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brokerURL, queue, err := splitAMQPAddress(tt.address)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBrokerURL, brokerURL)
			assert.Equal(t, tt.wantQueue, queue)
		})
	}
}
