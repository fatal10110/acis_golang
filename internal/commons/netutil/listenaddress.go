package netutil

import (
	"net"
	"strconv"
)

// ListenAddress returns the listener address for a configured host: "*"
// binds every address and an empty host binds the loopback address only.
// A host name stays as written and resolves at bind.
func ListenAddress(host string, port int) string {
	switch host {
	case "*":
		host = ""
	case "":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
