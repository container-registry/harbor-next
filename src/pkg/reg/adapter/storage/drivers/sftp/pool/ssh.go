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

package sshpool

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/goharbor/harbor/src/lib/log"

	"golang.org/x/crypto/ssh"
)

// SSHConfig defines the configuration options of the SSH connection.
type SSHConfig struct {
	User string
	Host string
	Port int
	Auth []ssh.AuthMethod

	//MaxSessions is a maximum number of sessions per connection Default is 10
	MaxSessions int

	// Timeout is the maximum amount of time for the TCP connection to establish.
	Timeout time.Duration

	// TCPKeepAlive specifies whether to send TCP keepalive messages
	// to the other side.
	TCPKeepAlive bool
	// TCPKeepAlivePeriod specifies the TCP keepalive frequency.
	TCPKeepAlivePeriod time.Duration

	HostKeyCallback func(hostname string, remote net.Addr, key ssh.PublicKey) error
}

// String returns a hash string generated from the SSH config parameters.
func (c *SSHConfig) String() string {
	return fmt.Sprintf(
		"%s@%s:%d%v",
		c.User,
		c.Host,
		c.Port,
		c.Auth,
	)
}

// SSHConn is a wrapper around the standard ssh.Client which implements some additional
// parameters required for the connection pool work properly.
// Parameters such as last access time, reference counter etc.
type SSHConn struct {
	client *ssh.Client

	cfg  SSHConfig
	hash string

	ctx    context.Context
	cancel func()

	// Protects access to fields below
	mu      sync.Mutex
	lastErr error

	accessTime time.Time

	sessionLimit chan struct{}
}

// NewSSHConn creates and configures new SSH connection according to the given SSH config.
//
// Also in a separate goroutine a new function will be fired up. That function will send
// SSH keepalive messages every minute.
func NewSSHConn(ctx context.Context, cfg SSHConfig) (*SSHConn, error) {
	if ctx == nil {
		ctx = context.TODO()
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Debugf("create a new SSH connection for %s", addr)

	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = 10
	}

	// TCP connection
	tcpConn, err := func() (c net.Conn, err error) {
		if cfg.Timeout == 0 {
			c, err = net.Dial("tcp", addr)
			if err != nil {
				return nil, err
			}
		} else {
			c, err = net.DialTimeout("tcp", addr, cfg.Timeout)
			if err != nil {
				return nil, err
			}
		}

		if err := c.(*net.TCPConn).SetKeepAlive(cfg.TCPKeepAlive); err != nil {
			return nil, err
		}
		if cfg.TCPKeepAlive {
			if err := c.(*net.TCPConn).SetKeepAlivePeriod(cfg.TCPKeepAlivePeriod); err != nil {
				return nil, err
			}
		}

		if cfg.Timeout > 0 {
			// wrap a connection
			return &conn{c, cfg.Timeout, cfg.Timeout}, nil
		}

		return c, nil
	}()

	if err != nil {
		return nil, err
	}

	clientConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            cfg.Auth,
		Timeout:         cfg.Timeout,
		HostKeyCallback: cfg.HostKeyCallback,
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh new client conn: %w", err)
	}

	// SSH client
	client := ssh.NewClient(clientConn, chans, reqs)
	var clientOk bool
	defer func() {
		if !clientOk {
			_ = client.Close()
		}
	}()
	clientOk = true

	ctx, cancel := context.WithCancel(ctx)
	con := &SSHConn{
		client:       client,
		cfg:          cfg,
		hash:         cfg.String(),
		ctx:          ctx,
		cancel:       cancel,
		accessTime:   time.Now(),
		sessionLimit: make(chan struct{}, cfg.MaxSessions),
	}

	// This regularly sends keepalive packets
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()

		for {
			select {
			case <-con.ctx.Done():
				return
			case <-t.C:
			}

			if _, _, err := client.Conn.SendRequest("keepalive@golang.org", true, nil); err != nil {
				con.mu.Lock()
				con.lastErr = err
				con.mu.Unlock()
				return
			}
		}
	}()

	return con, nil
}

// Close closes a connection and all its resources.
func (c *SSHConn) Close() error {
	c.cancel()
	return c.client.Close()
}

// Hash returns a hash string generated from the SSH config parameters.
func (c *SSHConn) Hash() string {
	return c.hash
}

// AccessTime returns last access time to this connection.
func (c *SSHConn) AccessTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessTime
}

// NewSession opens and configures a new session for this SSH connection.
//
// If `envs` is not nil then it will be applied to any command executed via this session.
func (c *SSHConn) NewSession(envs map[string]string) (*ssh.Session, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	for k, v := range envs {
		if err := session.Setenv(k, v); err != nil {
			session.Close()
			return nil, err
		}
	}

	return session, nil
}

// RefCount returns the reference count of this connection,
// which can be interpreted as the number of active sessions.
func (c *SSHConn) RefCount() int {
	return len(c.sessionLimit)
}

// IncrRefCount increments the reference counter.
func (c *SSHConn) IncrRefCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionLimit <- struct{}{}

	c.accessTime = time.Now()
}

// DecrRefCount decrements the reference counter.
func (c *SSHConn) DecrRefCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	<-c.sessionLimit
	c.accessTime = time.Now()
}

// Err returns an error that broke this connection.
func (c *SSHConn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}
