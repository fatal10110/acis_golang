package network

import (
	"context"
	"net"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
)

// ShutdownCloseGrace is how long a stopping Serve lets its connections end
// themselves, each flushing its last frame (ServerClose for a player in the
// world), before it force-closes the ones still open.
const ShutdownCloseGrace = 2 * time.Second

// Serve accepts game-client connections on ln until ctx is canceled or
// accepting fails. Each connection gets its own goroutine running
// handle; the caller owns ln (Serve closes it on ctx cancellation but
// does not create it, so tests can bind an ephemeral port). On
// cancellation it waits up to ShutdownCloseGrace for the handlers before
// closing the remaining connections. The zero logger disables logging.
func Serve(ctx context.Context, ln net.Listener, handle func(ctx context.Context, conn *Conn), log zerolog.Logger) error {
	return netutil.AcceptLoopWithCloseGrace(ctx, ln, func(raw net.Conn) {
		conn := newConn(raw, log)
		defer conn.Close()
		handle(ctx, conn)
	}, ShutdownCloseGrace, log)
}
