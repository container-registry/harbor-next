package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/jobservice/job"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
)

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

	jp := job.Parameters{
		"payload":      "amqp payload",
		"queue":        "harbor.events",
		"content_type": "application/json",
	}
	assert.Nil(t, rep.Validate(jp))
}

func TestAMQPJobRun(t *testing.T) {
	rep := &AMQPJob{}

	params := map[string]any{
		"payload":      "amqp payload",
		"queue":        "harbor.events",
		"content_type": "application/json",
	}
	err := rep.Run(&mockjobservice.MockJobContext{}, params)
	assert.Nil(t, err)
}
