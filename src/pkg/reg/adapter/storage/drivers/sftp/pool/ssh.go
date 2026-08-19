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

	"github.com/pkg/sftp"
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
	client *sftp.Client

	cfg  SSHConfig
	hash string

	ctx    context.Context
	cancel func()

	// Protects access to fields below
	mu      sync.Mutex
	lastErr error

	accessTime time.Time
}

func (p *SSHPool) NewSFTPClient(cfg *SSHConfig) (*sftp.Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = 10
	}

	config := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            cfg.Auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Connect to SSH
	sshClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial failed: %w", err)
	}

	// Create SFTP client
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("sftp client creation failed: %w", err)
	}

	// Automatically close after 1 minute
	go func() {
		time.Sleep(1 * time.Minute)
		log.Debug("auto-closing sftp connection after 1 minute")
		sftpClient.Close()
		sshClient.Close()
	}()

	return sftpClient, nil
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

	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = 10
	}

	config := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            cfg.Auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// TCP connection
	sshclient, err := func() (c *ssh.Client, err error) {
		c, err = ssh.Dial("tcp", addr, config)
		if err != nil {
			return nil, err
		}

		return c, nil
	}()

	if err != nil {
		return nil, err
	}

	// SSH client
	client, err := sftp.NewClient(sshclient)
	if err != nil {
		log.Debug("failed sftp client creation")
		return nil, fmt.Errorf("sftp client creation failed: %v", err)
	}
	var clientOk bool
	defer func() {
		if !clientOk {
			client.Close()
		}
	}()
	clientOk = true

	ctx, cancel := context.WithCancel(ctx)
	con := &SSHConn{
		client:     client,
		cfg:        cfg,
		hash:       cfg.String(),
		ctx:        ctx,
		cancel:     cancel,
		accessTime: time.Now(),
	}

	// Cancel the connection after 1 minute
	go func() {
		select {
		case <-time.After(1 * time.Minute):
			log.Debug("closing connection after 1 minute")
			cancel()
		case <-ctx.Done():
			// context cancelled externally
		}
	}()

	// Keepalive ticker
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()

		for {
			select {
			case <-con.ctx.Done():
				log.Debug("keepalive loop exiting, context done")
				con.client.Close()
				return
			case <-t.C:
				_, err := con.client.Stat(".")
				if err != nil {
					log.Debugf("keepalive failed: %v", err)
					cancel()
					return
				}
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

// Err returns an error that broke this connection.
func (c *SSHConn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}
