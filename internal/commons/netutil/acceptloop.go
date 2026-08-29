// Package netutil holds small networking helpers shared across the game
// and login servers.
package netutil

import (
	"context"
	"net"
	"sync"

	"github.com/rs/zerolog"
)

// AcceptLoop accepts connections on ln until ctx is canceled or accepting
// fails, running handle on its own goroutine per connection. On cancellation
// it closes the listener and every accepted connection, then waits for all
// handlers to return. AcceptLoop applies no timeout of its own, so callers
// that need a bounded shutdown must impose one. Accepted TCP connections
// have TCP_NODELAY enabled, matching the selector configuration shared by
// both servers. A panic in either the shutdown
// watcher or a connection's handle is recovered and logged rather than taking
// down the caller. The caller owns ln: AcceptLoop closes it on ctx cancellation
// but does not create it. A zero-value logger disables logging.
func AcceptLoop(ctx context.Context, ln net.Listener, handle func(conn net.Conn), log zerolog.Logger) error {
	var handlers sync.WaitGroup
	var connsMu sync.Mutex
	conns := make(map[net.Conn]struct{})
	done := make(chan struct{})
	defer func() {
		connsMu.Lock()
		pending := make([]net.Conn, 0, len(conns))
		for conn := range conns {
			pending = append(pending, conn)
		}
		connsMu.Unlock()
		for _, conn := range pending {
			conn.Close()
		}
		handlers.Wait()
	}()
	defer close(done)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("accept loop shutdown watcher panic")
			}
		}()
		select {
		case <-ctx.Done():
			ln.Close()
		case <-done:
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
					continue
				}
				return err
			}
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.SetNoDelay(true)
		}
		connsMu.Lock()
		conns[conn] = struct{}{}
		handlers.Add(1)
		connsMu.Unlock()
		go func(conn net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Msg("accept loop connection handler panic")
				}
				connsMu.Lock()
				delete(conns, conn)
				connsMu.Unlock()
				handlers.Done()
			}()
			handle(conn)
		}(conn)
	}
}
