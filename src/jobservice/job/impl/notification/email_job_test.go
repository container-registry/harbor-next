package notification

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/jobservice/job"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
)

func TestEmailJobMaxFails(t *testing.T) {
	rep := &EmailJob{}
	t.Run("default max fails", func(t *testing.T) {
		assert.Equal(t, uint(3), rep.MaxFails())
	})

	t.Run("user defined max fails", func(t *testing.T) {
		t.Setenv(maxFails, "15")
		assert.Equal(t, uint(15), rep.MaxFails())
	})

	t.Run("user defined wrong max fails", func(t *testing.T) {
		t.Setenv(maxFails, "abc")
		assert.Equal(t, uint(3), rep.MaxFails())
	})
}

func TestEmailJobMaxCurrency(t *testing.T) {
	rep := &EmailJob{}
	assert.Equal(t, uint(1), rep.MaxCurrency())
}

func TestEmailJobShouldRetry(t *testing.T) {
	rep := &EmailJob{}
	assert.True(t, rep.ShouldRetry())
}

func TestEmailJobValidate(t *testing.T) {
	rep := &EmailJob{}
	assert.NotNil(t, rep.Validate(nil))

	jp := job.Parameters{
		"subject": "Test Subject",
		"body":    "Test Body",
		"to":      "user@example.com",
		"address": "smtp.example.com",
		"from":    "harbor@example.com",
	}
	assert.Nil(t, rep.Validate(jp))
}

// capturedMail records what a fake SMTP server received.
type capturedMail struct {
	mu      sync.Mutex
	from    string
	rcptTo  []string
	message string
}

func (c *capturedMail) snapshot() (string, []string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.from, append([]string(nil), c.rcptTo...), c.message
}

// startFakeSMTP runs a minimal SMTP server that speaks just enough of the
// protocol for smtp.SendMail: greeting, EHLO/HELO, MAIL FROM, RCPT TO, DATA
// and QUIT. It records the envelope and message so the send path can be
// asserted without reaching a real mail server.
func startFakeSMTP(t *testing.T) (addr string, captured *capturedMail) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	captured = &capturedMail{}
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		write := func(s string) bool {
			if _, err := w.WriteString(s + "\r\n"); err != nil {
				return false
			}
			return w.Flush() == nil
		}

		if !write("220 fake.smtp ESMTP ready") {
			return
		}

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimRight(line, "\r\n")
			upper := strings.ToUpper(cmd)

			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				if !write("250 fake.smtp") {
					return
				}
			case strings.HasPrefix(upper, "MAIL FROM:"):
				captured.mu.Lock()
				captured.from = strings.TrimSpace(cmd[len("MAIL FROM:"):])
				captured.mu.Unlock()
				if !write("250 OK") {
					return
				}
			case strings.HasPrefix(upper, "RCPT TO:"):
				captured.mu.Lock()
				captured.rcptTo = append(captured.rcptTo, strings.TrimSpace(cmd[len("RCPT TO:"):]))
				captured.mu.Unlock()
				if !write("250 OK") {
					return
				}
			case upper == "DATA":
				if !write("354 End data with <CR><LF>.<CR><LF>") {
					return
				}
				var body strings.Builder
				for {
					dl, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if dl == ".\r\n" || dl == ".\n" {
						break
					}
					body.WriteString(dl)
				}
				captured.mu.Lock()
				captured.message = body.String()
				captured.mu.Unlock()
				if !write("250 OK queued") {
					return
				}
			case upper == "QUIT":
				write("221 Bye")
				return
			case upper == "RSET":
				if !write("250 OK") {
					return
				}
			default:
				if !write("250 OK") {
					return
				}
			}
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	return ln.Addr().String(), captured
}

func TestEmailJobRun(t *testing.T) {
	ctx := &mockjobservice.MockJobContext{}
	logger := &mockjobservice.MockJobLogger{}
	ctx.On("GetLogger").Return(logger)

	t.Run("sends the message with headers, body and every recipient", func(t *testing.T) {
		addr, captured := startFakeSMTP(t)

		rep := &EmailJob{}
		err := rep.Run(ctx, job.Parameters{
			"subject": "Harbor scan finished",
			"body":    "artifact scan completed",
			// A comma separated list with padding, which execute splits and trims.
			"to":      "first@example.com, second@example.com",
			"address": addr,
			"from":    "harbor@example.com",
		})
		require.NoError(t, err)

		from, rcptTo, message := captured.snapshot()
		assert.Equal(t, "<harbor@example.com>", from)
		assert.Equal(t, []string{"<first@example.com>", "<second@example.com>"}, rcptTo)
		assert.Contains(t, message, "From: harbor@example.com")
		assert.Contains(t, message, "To: first@example.com,second@example.com")
		assert.Contains(t, message, "Subject: Harbor scan finished")
		assert.Contains(t, message, "MIME-Version: 1.0")
		assert.Contains(t, message, `Content-Type: text/plain; charset="UTF-8"`)
		assert.Contains(t, message, "artifact scan completed")
	})

	t.Run("reports an error when the SMTP address is empty", func(t *testing.T) {
		rep := &EmailJob{}
		err := rep.Run(ctx, job.Parameters{
			"subject": "s",
			"body":    "b",
			"to":      "user@example.com",
			"address": "",
			"from":    "harbor@example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SMTP server not configured")
	})

	t.Run("reports an error when the SMTP server is unreachable", func(t *testing.T) {
		// Bind then immediately close to obtain a port nothing is listening on.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		deadAddr := ln.Addr().String()
		require.NoError(t, ln.Close())

		rep := &EmailJob{}
		err = rep.Run(ctx, job.Parameters{
			"subject": "s",
			"body":    "b",
			"to":      "user@example.com",
			"address": deadAddr,
			"from":    "harbor@example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send email")
	})
}
