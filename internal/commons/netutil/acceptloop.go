// Package netutil holds small networking helpers shared across the game
// and login servers.
package netutil

import (
	"context"
	"net"

	"github.com/sirupsen/logrus"
)

// AcceptLoop accepts connections on ln until ctx is canceled or accepting
// fails, running handle on its own goroutine per connection. A panic in
// either the shutdown watcher or a connection's handle is recovered and
// logged rather than taking down the caller. The caller owns ln: AcceptLoop
// closes it on ctx cancellation but does not create it. log may be nil, in
// which case logrus.StandardLogger() is used.
func AcceptLoop(ctx context.Context, ln net.Listener, handle func(conn net.Conn), log *logrus.Logger) error {
	if log == nil {
		log = logrus.StandardLogger()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("accept loop shutdown watcher panic: %v", r)
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
					log.Errorf("accept loop connection handler panic: %v", r)
				}
			}()
			handle(conn)
		}()
	}
}
