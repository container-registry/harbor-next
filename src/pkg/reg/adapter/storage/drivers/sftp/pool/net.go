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
