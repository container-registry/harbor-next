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
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/jobservice/logger"
	"github.com/goharbor/harbor/src/lib/errors"
)

// maxFailsAMQP is the env var controlling how many times an AMQP
// notification job may be retried, independent of the webhook job setting.
const maxFailsAMQP = "JOBSERVICE_AMQP_JOB_MAX_RETRY"

// amqpChannel is the subset of *amqp.Channel used to publish a message,
// abstracted so tests can substitute a fake broker connection.
type amqpChannel interface {
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Close() error
}

// amqpConnection is the subset of *amqp.Connection used to obtain a channel.
type amqpConnection interface {
	Channel() (amqpChannel, error)
	Close() error
}

type realAMQPConnection struct {
	*amqp.Connection
}

func (c realAMQPConnection) Channel() (amqpChannel, error) {
	return c.Connection.Channel()
}

// dialAMQP opens a connection to the broker at brokerURL. Overridden in
// tests to avoid depending on a real broker.
var dialAMQP = func(brokerURL string, skipCertVerify bool) (amqpConnection, error) {
	var conn *amqp.Connection
	var err error
	if strings.HasPrefix(brokerURL, "amqps://") {
		conn, err = amqp.DialTLS(brokerURL, &tls.Config{InsecureSkipVerify: skipCertVerify})
	} else {
		conn, err = amqp.Dial(brokerURL)
	}
	if err != nil {
		return nil, err
	}
	return realAMQPConnection{conn}, nil
}

// AMQPJob implements the job interface, which publishes notifications to an
// AMQP broker.
type AMQPJob struct {
	logger logger.Interface
}

// MaxFails returns that how many times this job can fail.
func (aj *AMQPJob) MaxFails() (result uint) {
	// Default max fails count is 3
	result = 3
	if maxFails, exist := os.LookupEnv(maxFailsAMQP); exist {
		mf, err := strconv.ParseUint(maxFails, 10, 32)
		if err != nil {
			logger.Warningf("Fetch amqp job maxFails error: %s", err.Error())
			return result
		}
		result = uint(mf)
	}
	return result
}

// MaxCurrency is implementation of same method in Interface.
func (aj *AMQPJob) MaxCurrency() uint {
	return 1
}

// ShouldRetry ...
func (aj *AMQPJob) ShouldRetry() bool {
	return true
}

// Validate implements the interface in job/Interface
func (aj *AMQPJob) Validate(params job.Parameters) error {
	if params == nil {
		// Params are required
		return errors.New("missing parameter of amqp job")
	}

	for _, name := range []string{"payload", "queue", "broker_url"} {
		if err := validateNonEmptyStringParam(params, name); err != nil {
			return err
		}
	}
	return nil
}

func validateNonEmptyStringParam(params job.Parameters, name string) error {
	val, ok := params[name]
	if !ok {
		return errors.Errorf("missing job parameter '%s'", name)
	}
	if val == nil {
		return errors.Errorf("malformed job parameter '%s', got nil", name)
	}
	str, ok := val.(string)
	if !ok {
		return errors.Errorf("malformed job parameter '%s', expecting string but got %s", name, fmt.Sprintf("%T", val))
	}
	if str == "" {
		return errors.Errorf("malformed job parameter '%s', expecting non-empty string", name)
	}
	return nil
}

// Run implements the interface in job/Interface
func (aj *AMQPJob) Run(ctx job.Context, params job.Parameters) error {
	if err := aj.init(ctx, params); err != nil {
		return err
	}

	aj.logger.Info("start to run amqp job")

	err := aj.execute(params)
	if err != nil {
		aj.logger.Errorf("exit amqp job, error: %s", err)
	} else {
		aj.logger.Info("success to run amqp job")
	}
	return err
}

// init amqp job
func (aj *AMQPJob) init(ctx job.Context, _ map[string]any) error {
	aj.logger = ctx.GetLogger()
	return nil
}

// execute connects to the configured AMQP broker and publishes the payload
// to the target queue via the default exchange, using the queue name as the
// routing key -- the standard AMQP pattern for delivering directly to a
// single declared queue.
func (aj *AMQPJob) execute(params map[string]any) error {
	payload := params["payload"].(string)
	queue := params["queue"].(string)
	brokerURL := params["broker_url"].(string)

	contentType, _ := params["content_type"].(string)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	skipCertVerify, _ := params["skip_cert_verify"].(bool)

	if auth, ok := params["auth"].(string); ok && auth != "" {
		var err error
		brokerURL, err = injectAMQPCredentials(brokerURL, auth)
		if err != nil {
			return fmt.Errorf("failed to apply AMQP credentials: %w", err)
		}
	}

	conn, err := dialAMQP(brokerURL, skipCertVerify)
	if err != nil {
		return fmt.Errorf("failed to connect to AMQP broker: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open AMQP channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Publish("", queue, false, false, amqp.Publishing{
		ContentType: contentType,
		Body:        []byte(payload),
	}); err != nil {
		return fmt.Errorf("failed to publish to AMQP queue %s: %w", queue, err)
	}

	aj.logger.Infof("published to AMQP queue %s", queue)
	return nil
}

// injectAMQPCredentials sets the userinfo (user:password, from the
// notification target's auth header) on a broker URL that had its
// credentials stripped during webhook target validation.
func injectAMQPCredentials(brokerURL, auth string) (string, error) {
	u, err := url.Parse(brokerURL)
	if err != nil {
		return "", fmt.Errorf("invalid broker URL: %w", err)
	}
	user, pass, found := strings.Cut(auth, ":")
	if !found {
		return "", errors.New("malformed auth header, expecting 'user:password'")
	}
	u.User = url.UserPassword(user, pass)
	return u.String(), nil
}
