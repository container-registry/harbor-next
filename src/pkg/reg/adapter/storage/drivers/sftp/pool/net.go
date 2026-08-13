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
	"net"
	"time"
)

// Conn wraps a net.Conn, and sets a deadline for every read
// and write operation.
type conn struct {
	net.Conn

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (c *conn) Read(b []byte) (int, error) {
	err := c.Conn.SetReadDeadline(time.Now().Add(c.ReadTimeout))

	if err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *conn) Write(b []byte) (int, error) {
	err := c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))

	if err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
}
