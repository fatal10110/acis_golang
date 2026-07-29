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
// handlers to return. A panic in either the shutdown watcher or a connection's
// handle is recovered and logged rather than taking down the caller. The
// caller owns ln: AcceptLoop closes it on ctx cancellation but does not create
// it. A zero-value logger disables logging.
func AcceptLoop(ctx context.Context, ln net.Listener, handle func(conn net.Conn), log zerolog.Logger) error {
	var handlers sync.WaitGroup
	var connsMu sync.Mutex
	conns := make(map[net.Conn]struct{})
	stopping := false
	done := make(chan struct{})
	defer close(done)
	defer handlers.Wait()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("accept loop shutdown watcher panic")
			}
		}()
		select {
		case <-ctx.Done():
			connsMu.Lock()
			stopping = true
			ln.Close()
			for conn := range conns {
				conn.Close()
			}
			connsMu.Unlock()
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
				return err
			}
		}
		connsMu.Lock()
		if stopping {
			connsMu.Unlock()
			conn.Close()
			continue
		}
		conns[conn] = struct{}{}
		handlers.Add(1)
		connsMu.Unlock()
		go func(conn net.Conn) {
			defer func() {
				connsMu.Lock()
				delete(conns, conn)
				connsMu.Unlock()
				handlers.Done()
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Msg("accept loop connection handler panic")
				}
			}()
			handle(conn)
		}(conn)
	}
}
