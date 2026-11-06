// Package debughttp serves optional pprof and expvar endpoints, and the IP
// ban API, for a running gameserver or loginserver process.
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

// Listen serves pprof and expvar on addr, and bans, when not nil, on
// BansPath and below. An empty addr is a no-op.
func Listen(addr string, bans http.Handler) (*http.Server, error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: routes(bans), ReadHeaderTimeout: 5 * time.Second}
	// Serve returns http.ErrServerClosed after Shutdown; the debug listener
	// is best-effort and has no caller to report a failure to.
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}

// routes is the debug listener's handler: bans, when not nil, on BansPath
// and below, and http.DefaultServeMux, which holds pprof and expvar, for
// everything else.
func routes(bans http.Handler) http.Handler {
	if bans == nil {
		return http.DefaultServeMux
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.DefaultServeMux)
	mux.Handle(BansPath, bans)
	mux.Handle(BansPath+"/", bans)
	return mux
}

// Shutdown stops srv when non-nil.
func Shutdown(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
