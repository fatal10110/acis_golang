package network

import (
	"context"
	"net"
)

// Serve accepts game-client connections on ln until ctx is canceled or
// accepting fails. Each connection gets its own goroutine running
// handle; the caller owns ln (Serve closes it on ctx cancellation but
// does not create it, so tests can bind an ephemeral port).
func Serve(ctx context.Context, ln net.Listener, handle func(ctx context.Context, conn *Conn)) error {
	go func() {
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

		conn := newConn(raw)
		go func() {
			defer conn.Close()
			handle(ctx, conn)
		}()
	}
}
