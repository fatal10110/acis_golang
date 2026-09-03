package loginserver

import (
	"net"
	"time"
)

const (
	connectionHandshakeTimeout = time.Minute
	// loginClientIdleTimeout applies only to login clients; established
	// game-server links intentionally remain open while idle.
	loginClientIdleTimeout = 15 * time.Minute
	connectionWriteTimeout = time.Minute
)

func setLoginClientReadDeadline(conn net.Conn, authed bool) error {
	timeout := connectionHandshakeTimeout
	if authed {
		timeout = loginClientIdleTimeout
	}
	return conn.SetReadDeadline(time.Now().Add(timeout))
}

func setWriteDeadline(conn net.Conn) error {
	return conn.SetWriteDeadline(time.Now().Add(connectionWriteTimeout))
}
