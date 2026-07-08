package network

import (
	"context"
	"net"

	"github.com/sirupsen/logrus"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
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

	return netutil.AcceptLoop(ctx, ln, func(raw net.Conn) {
		if tcp, ok := raw.(*net.TCPConn); ok {
			tcp.SetNoDelay(true)
		}
		conn := newConn(raw, log)
		defer conn.Close()
		handle(ctx, conn)
	}, log)
}
