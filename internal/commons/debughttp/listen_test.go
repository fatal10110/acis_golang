package debughttp

import (
	"context"
	"net"
	"testing"
)

// A -debug-addr that cannot be bound must surface as an error so the binary's
// fx start hook fails boot instead of running without the endpoint.
func TestListenBusyAddrFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	srv, err := Listen(ln.Addr().String())
	if err == nil {
		_ = Shutdown(context.Background(), srv)
		t.Fatalf("Listen(%s) on a bound address: got nil error", ln.Addr())
	}
	if srv != nil {
		t.Fatal("Listen on a bound address returned a server")
	}
}
