package network

import "net"

// remoteIP is the address of the client at the other end of s, without its
// port; "" when s has no connection.
func (s *Session) remoteIP() string {
	if s == nil || s.conn == nil || s.conn.Conn == nil {
		return ""
	}
	addr := s.conn.RemoteAddr()
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
