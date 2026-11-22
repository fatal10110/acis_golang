// Package debughttp serves optional pprof and expvar endpoints, and the
// operator APIs mounted on them, for a running gameserver or loginserver
// process.
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

// Route mounts Handler on Path and every path below it.
type Route struct {
	Path    string
	Handler http.Handler
}

// Listen serves pprof and expvar on addr, and each route on its path and
// below. An empty addr is a no-op.
func Listen(addr string, routes ...Route) (*http.Server, error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: mux(routes), ReadHeaderTimeout: 5 * time.Second}
	// Serve returns http.ErrServerClosed after Shutdown; the debug listener
	// is best-effort and has no caller to report a failure to.
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}

// mux is the debug listener's handler: each route on its path and below,
// and http.DefaultServeMux, which holds pprof and expvar, for everything
// else.
func mux(routes []Route) http.Handler {
	if len(routes) == 0 {
		return http.DefaultServeMux
	}
	m := http.NewServeMux()
	m.Handle("/", http.DefaultServeMux)
	for _, r := range routes {
		m.Handle(r.Path, r.Handler)
		m.Handle(r.Path+"/", r.Handler)
	}
	return m
}

// Shutdown stops srv when non-nil.
func Shutdown(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
