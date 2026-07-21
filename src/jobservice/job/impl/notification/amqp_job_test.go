package notification

import (
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/jobservice/job"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
)

// fakeAMQPChannel is a test double for amqpChannel.
type fakeAMQPChannel struct {
	publishErr  error
	closed      bool
	publishedTo string
	published   amqp.Publishing
}

func (c *fakeAMQPChannel) Publish(_, key string, _, _ bool, msg amqp.Publishing) error {
	c.publishedTo = key
	c.published = msg
	return c.publishErr
}

func (c *fakeAMQPChannel) Close() error {
	c.closed = true
	return nil
}

// fakeAMQPConnection is a test double for amqpConnection.
type fakeAMQPConnection struct {
	channel    *fakeAMQPChannel
	channelErr error
	closed     bool
}

func (c *fakeAMQPConnection) Channel() (amqpChannel, error) {
	if c.channelErr != nil {
		return nil, c.channelErr
	}
	return c.channel, nil
}

func (c *fakeAMQPConnection) Close() error {
	c.closed = true
	return nil
}

// withFakeDialAMQP overrides dialAMQP for the duration of a test.
func withFakeDialAMQP(t *testing.T, fn func(brokerURL string, skipCertVerify bool) (amqpConnection, error)) {
	t.Helper()
	original := dialAMQP
	dialAMQP = fn
	t.Cleanup(func() { dialAMQP = original })
}

func TestAMQPJobMaxFails(t *testing.T) {
	rep := &AMQPJob{}
	t.Run("default max fails", func(t *testing.T) {
		assert.Equal(t, uint(3), rep.MaxFails())
	})

	t.Run("user defined max fails", func(t *testing.T) {
		t.Setenv(maxFailsAMQP, "15")
		assert.Equal(t, uint(15), rep.MaxFails())
	})

	t.Run("user defined wrong max fails", func(t *testing.T) {
		t.Setenv(maxFailsAMQP, "abc")
		assert.Equal(t, uint(3), rep.MaxFails())
	})
}

func TestAMQPJobShouldRetry(t *testing.T) {
	rep := &AMQPJob{}
	assert.True(t, rep.ShouldRetry())
}

func TestAMQPJobValidate(t *testing.T) {
	rep := &AMQPJob{}
	assert.NotNil(t, rep.Validate(nil))

	t.Run("valid", func(t *testing.T) {
		jp := job.Parameters{
			"payload":      "amqp payload",
			"queue":        "harbor.events",
			"broker_url":   "amqp://broker:5672/vhost",
			"content_type": "application/json",
		}
		assert.Nil(t, rep.Validate(jp))
	})

	t.Run("missing broker_url", func(t *testing.T) {
		jp := job.Parameters{
			"payload": "amqp payload",
			"queue":   "harbor.events",
		}
		assert.NotNil(t, rep.Validate(jp))
	})
}

func TestAMQPJobRun(t *testing.T) {
	ch := &fakeAMQPChannel{}
	conn := &fakeAMQPConnection{channel: ch}
	withFakeDialAMQP(t, func(_ string, _ bool) (amqpConnection, error) {
		return conn, nil
	})

	rep := &AMQPJob{}
	params := map[string]any{
		"payload":      "amqp payload",
		"queue":        "harbor.events",
		"broker_url":   "amqp://broker:5672/vhost",
		"content_type": "application/json",
	}
	err := rep.Run(&mockjobservice.MockJobContext{}, params)
	require.NoError(t, err)
	assert.Equal(t, "harbor.events", ch.publishedTo)
	assert.Equal(t, "application/json", ch.published.ContentType)
	assert.Equal(t, "amqp payload", string(ch.published.Body))
	assert.True(t, ch.closed)
	assert.True(t, conn.closed)
}

func TestAMQPJobRunDialError(t *testing.T) {
	withFakeDialAMQP(t, func(_ string, _ bool) (amqpConnection, error) {
		return nil, errors.New("connection refused")
	})

	rep := &AMQPJob{}
	params := map[string]any{
		"payload":    "amqp payload",
		"queue":      "harbor.events",
		"broker_url": "amqp://broker:5672/vhost",
	}
	err := rep.Run(&mockjobservice.MockJobContext{}, params)
	require.Error(t, err)
}

func TestAMQPJobRunChannelError(t *testing.T) {
	conn := &fakeAMQPConnection{channelErr: errors.New("channel error")}
	withFakeDialAMQP(t, func(_ string, _ bool) (amqpConnection, error) {
		return conn, nil
	})

	rep := &AMQPJob{}
	params := map[string]any{
		"payload":    "amqp payload",
		"queue":      "harbor.events",
		"broker_url": "amqp://broker:5672/vhost",
	}
	err := rep.Run(&mockjobservice.MockJobContext{}, params)
	require.Error(t, err)
	assert.True(t, conn.closed)
}

func TestAMQPJobRunPublishError(t *testing.T) {
	ch := &fakeAMQPChannel{publishErr: errors.New("publish failed")}
	conn := &fakeAMQPConnection{channel: ch}
	withFakeDialAMQP(t, func(_ string, _ bool) (amqpConnection, error) {
		return conn, nil
	})

	rep := &AMQPJob{}
	params := map[string]any{
		"payload":    "amqp payload",
		"queue":      "harbor.events",
		"broker_url": "amqp://broker:5672/vhost",
	}
	err := rep.Run(&mockjobservice.MockJobContext{}, params)
	require.Error(t, err)
	assert.True(t, ch.closed)
	assert.True(t, conn.closed)
}

func TestAMQPJobRunWithAuth(t *testing.T) {
	ch := &fakeAMQPChannel{}
	conn := &fakeAMQPConnection{channel: ch}
	var dialedURL string
	withFakeDialAMQP(t, func(brokerURL string, _ bool) (amqpConnection, error) {
		dialedURL = brokerURL
		return conn, nil
	})

	rep := &AMQPJob{}
	params := map[string]any{
		"payload":    "amqp payload",
		"queue":      "harbor.events",
		"broker_url": "amqp://broker:5672/vhost",
		"auth":       "user:pass",
	}
	err := rep.Run(&mockjobservice.MockJobContext{}, params)
	require.NoError(t, err)
	assert.Equal(t, "amqp://user:pass@broker:5672/vhost", dialedURL)
}

func TestInjectAMQPCredentials(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := injectAMQPCredentials("amqp://broker:5672/vhost", "user:pass")
		require.NoError(t, err)
		assert.Equal(t, "amqp://user:pass@broker:5672/vhost", got)
	})

	t.Run("missing colon", func(t *testing.T) {
		_, err := injectAMQPCredentials("amqp://broker:5672/vhost", "notacredential")
		require.Error(t, err)
	})

	t.Run("invalid broker URL", func(t *testing.T) {
		_, err := injectAMQPCredentials("://not-a-url", "user:pass")
		require.Error(t, err)
	})
}
