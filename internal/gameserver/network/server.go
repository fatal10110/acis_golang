package network

import (
	"context"
	"net"

	"github.com/sirupsen/logrus"
)

// Serve accepts game-client connections on ln until ctx is canceled or
// accepting fails. Each connection gets its own goroutine running
// handle; the caller owns ln (Serve closes it on ctx cancellation but
// does not create it, so tests can bind an ephemeral port). log may be
// nil, in which case logrus.StandardLogger() is used.
func Serve(ctx context.Context, ln net.Listener, handle func(ctx context.Context, conn *Conn), log *logrus.Logger) error {
	if log == nil {
		log = logrus.StandardLogger()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("game listener shutdown watcher panic: %v", r)
			}
		}()
		<-ctx.Done()
		ln.Close()
	}()

	for {
		raw, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}

		if tcp, ok := raw.(*net.TCPConn); ok {
			tcp.SetNoDelay(true)
		}

		conn := newConn(raw, log)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("game connection handler panic: %v", r)
				}
				conn.Close()
			}()
			handle(ctx, conn)
		}()
	}
}
