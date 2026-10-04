// Package netutil holds small networking helpers shared across the game
// and login servers.
package netutil

import (
	"context"
	"errors"
	"expvar"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

var openConnections = expvar.NewInt("connections")

const (
	acceptRetryMin = 5 * time.Millisecond
	acceptRetryMax = time.Second
)

// AcceptLoop accepts connections on ln until ctx is canceled or the
// listener is closed, running handle on its own goroutine per connection.
// On cancellation it closes the listener and every accepted connection,
// then waits for all handlers to return. Every Accept error other than
// net.ErrClosed is logged at Warn and retried indefinitely, with
// exponential backoff from 5ms to 1s: AcceptLoop never gives up on a
// listener that is still open, so an unrecoverable listener keeps the
// process alive and reports itself once per retry rather than returning.
// AcceptLoop applies no timeout of its own, so callers that need a bounded
// shutdown must impose one.
// Accepted TCP connections have TCP_NODELAY enabled, matching the selector
// configuration shared by both servers. A panic in either the shutdown
// watcher or a connection's handle is recovered and logged rather than taking
// down the caller. The caller owns ln: AcceptLoop closes it on ctx cancellation
// but does not create it. A zero-value logger disables logging.
func AcceptLoop(ctx context.Context, ln net.Listener, handle func(conn net.Conn), log zerolog.Logger) error {
	return AcceptLoopWithCloseGrace(ctx, ln, handle, 0, log)
}

// AcceptLoopWithCloseGrace is AcceptLoop for handlers that end their own
// connection when ctx is canceled, with a last frame to flush: on
// cancellation it waits up to grace for the handlers to return before it
// force-closes the connections still open, so that close cannot cut off a
// final write. A zero grace force-closes at once, as AcceptLoop does.
func AcceptLoopWithCloseGrace(ctx context.Context, ln net.Listener, handle func(conn net.Conn), grace time.Duration, log zerolog.Logger) error {
	var handlers sync.WaitGroup
	var connsMu sync.Mutex
	conns := make(map[net.Conn]struct{})
	done := make(chan struct{})
	defer func() {
		if grace > 0 && ctx.Err() != nil {
			waitTimeout(&handlers, grace)
		}
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

	var retryDelay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			if retryDelay == 0 {
				retryDelay = acceptRetryMin
			} else {
				retryDelay *= 2
				if retryDelay > acceptRetryMax {
					retryDelay = acceptRetryMax
				}
			}
			log.Warn().Err(err).Dur("retry_in", retryDelay).Msg("accept loop accept error; retrying")
			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		retryDelay = 0
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetNoDelay(true) // best-effort latency tuning
		}
		connsMu.Lock()
		conns[conn] = struct{}{}
		handlers.Add(1)
		connsMu.Unlock()
		openConnections.Add(1)
		go func(conn net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Msg("accept loop connection handler panic")
				}
				connsMu.Lock()
				delete(conns, conn)
				connsMu.Unlock()
				openConnections.Add(-1)
				handlers.Done()
			}()
			handle(conn)
		}(conn)
	}
}

// waitTimeout waits for wg, giving up after d.
func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	waited := make(chan struct{})
	go func() {
		wg.Wait()
		close(waited)
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-waited:
	case <-timer.C:
	}
}
