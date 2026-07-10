// Package netutil holds small networking helpers shared across the game
// and login servers.
package netutil

import (
	"context"
	"net"

	"github.com/rs/zerolog"
)

// AcceptLoop accepts connections on ln until ctx is canceled or accepting
// fails, running handle on its own goroutine per connection. A panic in
// either the shutdown watcher or a connection's handle is recovered and
// logged rather than taking down the caller. The caller owns ln: AcceptLoop
// closes it on ctx cancellation but does not create it. log may be nil, in
// The zero logger disables logging.
func AcceptLoop(ctx context.Context, ln net.Listener, handle func(conn net.Conn), log zerolog.Logger) error {

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("accept loop shutdown watcher panic")
			}
		}()
		<-ctx.Done()
		ln.Close()
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
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Msg("accept loop connection handler panic")
				}
			}()
			handle(conn)
		}()
	}
}
