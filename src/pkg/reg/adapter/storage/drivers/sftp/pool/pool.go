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
	"io"

	"github.com/pkg/sftp"

	"github.com/goharbor/harbor/src/lib/log"

	"sort"
	"sync"
	"time"
)

// PoolConfig defines configuration options of the pool.
type PoolConfig struct {
	// GCInterval specifies the frequency of Garbage Collector.
	GCInterval time.Duration
}

type SSHPool struct {
	PoolConfig

	ctx    context.Context
	cancel func()

	// Protects access to fields below
	mu    sync.Mutex
	table map[string]*SSHConn
}

// NewPool creates a new pool of connections and starts GC. If no configuration
// is specified (nil), defaults values are used.
func NewPool(cfg *PoolConfig) *SSHPool {
	ctx, cancel := context.WithCancel(context.Background())

	if cfg == nil {
		cfg = &PoolConfig{GCInterval: 30 * time.Second}
	}

	p := SSHPool{
		PoolConfig: *cfg,
		ctx:        ctx,
		cancel:     cancel,
		table:      make(map[string]*SSHConn),
	}

	if p.GCInterval > 0 {
		go p.collect()
	}
	return &p
}

// Collect removes broken and the oldest connections from the pool.
func (p *SSHPool) collect() {
	t := time.NewTicker(p.GCInterval)
	defer t.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
		}

		needClose := func() []io.Closer {
			var out []io.Closer

			p.mu.Lock()
			defer p.mu.Unlock()

			// Releasing broken connections
			for hash, c := range p.table {
				if err := c.Err(); err != nil {
					delete(p.table, hash)
					out = append(out, c)
				}
			}

			// Releasing the oldest connections
			s := make([]*SSHConn, 0, len(p.table))
			for _, c := range p.table {
				s = append(s, c)
			}

			sort.SliceStable(s, func(i, j int) bool { return s[i].AccessTime().Unix() > s[j].AccessTime().Unix() })

			for _, c := range s {
				if c.RefCount() > 0 {
					log.Debugf("skip from cleaning because connection has still %d active sessions", c.RefCount())
					// do not gc connections with open sessions
					continue
				}
				delete(p.table, c.Hash())
				out = append(out, c)
			}
			return out
		}()

		for _, c := range needClose {
			log.Debugf("closing connection automatically")
			_ = c.Close()
		}
	}
}

// NewSFTPSession creates and configures a new session reusing an existing
// SSH connection if possible.
// IMPORTANT release function does not close the session, close it manually expplicitly or via readerCloser
// If no connection exists, or there are any problems with connection
// a new connection will be created and added to the pool. After this
// a new session will be set up.
func (p *SSHPool) NewSFTPSession(cfg *SSHConfig) (*sftp.Client, func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var err error
	con, found := p.table[cfg.String()]
	if !found {
		// create a new connection if limit is full
		con, err = NewSSHConn(p.ctx, *cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("new sftp conn: %w", err)
		}
		p.table[con.Hash()] = con
	}

	// increment and maybe sleep
	con.IncrRefCount()

	// create sftp session here
	session, err := sftp.NewClient(con.client,
		sftp.UseConcurrentWrites(false),
		sftp.UseConcurrentReads(false),
	)

	if err != nil {
		//failed, give up and decrement
		con.DecrRefCount()
		return nil, nil, fmt.Errorf("new client error: %w", err)
	}

	// all good, provide session and it's closer
	return session, con.DecrRefCount, nil
}

// CloseConn closes and removes a connection corresponding to the given config
// from the pool.
func (p *SSHPool) CloseConn(cfg *SSHConfig) {
	hash := cfg.String()

	p.mu.Lock()
	defer p.mu.Unlock()

	if c, found := p.table[hash]; found {
		_ = c.Close()
		delete(p.table, hash)
	}
}

// Close closes the pool, thus destroying all connections.
// The pool cannot be used anymore after this call.
func (p *SSHPool) Close() {
	p.cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, c := range p.table {
		// It's ok, that we use here a blocking way
		// since pool cannot be used after it's closed.
		_ = c.Close()
	}

	// Clearing the connection table.
	p.table = nil
}

// ActiveConns returns the number of connections handled by the pool thus far.
func (p *SSHPool) ActiveConns() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.table)
}
