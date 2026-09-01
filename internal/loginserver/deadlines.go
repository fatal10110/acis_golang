package loginserver

import (
	"net"
	"time"
)

const (
	connectionHandshakeTimeout = time.Minute
	connectionIdleTimeout      = 15 * time.Minute
	connectionWriteTimeout     = time.Minute
)

func setReadDeadline(conn net.Conn, authed bool) error {
	timeout := connectionHandshakeTimeout
	if authed {
		timeout = connectionIdleTimeout
	}
	return conn.SetReadDeadline(time.Now().Add(timeout))
}

func setWriteDeadline(conn net.Conn) error {
	return conn.SetWriteDeadline(time.Now().Add(connectionWriteTimeout))
}
