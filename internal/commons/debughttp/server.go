// Package debughttp serves optional pprof and expvar endpoints for a
// running gameserver or loginserver process.
package debughttp

import (
	"context"
	"expvar"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"
	"time"
)

func init() {
	expvar.Publish("players-online", expvar.Func(func() any {
		f, _ := playersOnlineFn.Load().(func() int)
		if f == nil {
			return 0
		}
		return f()
	}))
}

var playersOnlineFn atomic.Value

// SetPlayersOnlineFunc supplies the live player count published as
// players-online. The login server leaves this unset, so the value stays 0.
func SetPlayersOnlineFunc(f func() int) {
	playersOnlineFn.Store(f)
}

// Listen serves pprof and expvar on addr. An empty addr is a no-op.
func Listen(addr string) (*http.Server, error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	return srv, nil
}

// Shutdown stops srv when non-nil.
func Shutdown(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
