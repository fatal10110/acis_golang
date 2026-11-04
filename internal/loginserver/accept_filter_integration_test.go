package loginserver

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestClientLinkAcceptFilterRefusesBurstBeforeInit drives the login-client
// listener behind the accept filter on a fake clock: the first four
// connections inside a second each get Init; the fifth is closed before
// Init and the ban holds while attempts keep coming, until a sweep after
// 300 s of silence; another host is unaffected throughout.
func TestClientLinkAcceptFilterRefusesBurstBeforeInit(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	advance := func(d time.Duration) time.Time {
		clock.Add(int64(d))
		return now()
	}
	filter := netutil.NewAcceptFilter()
	addr, _, _, _, _ := newTestClientLinkOn(t, func(ln net.Listener) net.Listener {
		return netutil.FilterAccepts(ln, filter, now)
	}, newFakeAccountStore(), false)

	refused := func(what string) {
		t.Helper()
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("%s: dial: %v", what, err)
		}
		t.Cleanup(func() { conn.Close() })
		// The first read is the close: no Init frame came before it.
		(&fakeLoginClient{t: t, conn: conn}).expectClosed()
	}

	dialLoginClient(t, addr)
	for i := 2; i <= 4; i++ {
		advance(200 * time.Millisecond)
		dialLoginClient(t, addr)
	}
	advance(200 * time.Millisecond)
	refused("fifth burst connection")
	advance(999 * time.Millisecond)
	refused("attempt while banned")
	advance(10 * time.Second)
	refused("slow attempt while banned")

	other := &net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.2")}}
	conn, err := other.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial from another host: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := wire.ReadFrame(conn); err != nil {
		t.Fatalf("another host: read Init frame: %v", err)
	}

	filter.Sweep(advance(300*time.Second + time.Millisecond))
	dialLoginClient(t, addr)
}
