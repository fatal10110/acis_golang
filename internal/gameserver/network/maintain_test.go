package network

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
)

// recordingListener hands every connection it accepts to accepted as well,
// so a test can drop a link from the login server's side.
type recordingListener struct {
	net.Listener
	accepted chan net.Conn
}

func (l recordingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.accepted <- conn
	}
	return conn, err
}

// within receives from ch, failing the test after 5 s.
func within[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

// TestMaintainRelinksAfterTheLoginServerDropsTheLink drops an established
// link from the login server's side: the game server waits the reconnect
// delay before it dials again, then re-registers under the same server id.
// The login side admits at most one link connection per address at a time,
// so the reconnect is admitted only once the dropped one was released.
func TestMaintainRelinksAfterTheLoginServerDropsTheLink(t *testing.T) {
	guard := netutil.NewFloodGuard(netutil.FloodGuardConfig{
		Enabled:              true,
		FastConnectionLimit:  15,
		NormalConnectionTime: 700 * time.Millisecond,
		MaxConnectionsPerIP:  1,
	}, zerolog.Nop())
	accepted := make(chan net.Conn, 64)
	addr, servers, _ := newTestLoginServerWith(t, false, guard, func(ln net.Listener) net.Listener {
		return recordingListener{Listener: ln, accepted: accepted}
	})
	servers.Register(1, testHexID)

	ctx, cancel := context.WithCancel(context.Background())
	waits := make(chan time.Duration)
	elapse := make(chan time.Time)
	after := func(d time.Duration) <-chan time.Time {
		select {
		case waits <- d:
		case <-ctx.Done():
		}
		return elapse
	}
	links := make(chan *LoginLink, 4)
	auth := func() LoginServerAuth {
		return LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		maintain(ctx, addr, auth, LoginLinkHandlers{}, DefaultReconnectDelay, after, func(l *LoginLink) { links <- l }, zerolog.Nop())
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	first := within(t, links, "the first link")
	if first.ServerID != 1 {
		t.Fatalf("first link ServerID = %d, want 1", first.ServerID)
	}
	linkConn := within(t, accepted, "the first link connection")

	// While the link is held, the login side refuses a second connection
	// from the same address before sending anything.
	probe, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial probe: %v", err)
	}
	defer probe.Close()
	within(t, accepted, "the probe connection")
	if err := probe.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := probe.Read(make([]byte, 1)); n != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("probe while linked read %d, %v; want a close with no byte", n, err)
	}

	linkConn.Close()
	within(t, first.Done(), "the dropped link to end")
	if d := within(t, waits, "the reconnect wait"); d != DefaultReconnectDelay {
		t.Fatalf("reconnect wait = %v, want %v", d, DefaultReconnectDelay)
	}
	// maintain is parked on the wait it asked for: nothing has dialed.
	select {
	case conn := <-accepted:
		t.Fatalf("dialed before the reconnect delay elapsed: %v", conn.RemoteAddr())
	default:
	}

	// The login side releases the dropped connection and marks the server
	// offline just after it closes; a dial that beats either is refused
	// and waits the delay again.
	var second *LoginLink
	for attempt := 1; second == nil; attempt++ {
		if attempt > 32 {
			t.Fatal("no reconnect admitted after 32 delays")
		}
		elapse <- time.Time{}
		select {
		case second = <-links:
		case d := <-waits:
			if d != DefaultReconnectDelay {
				t.Fatalf("reconnect wait after a refused dial = %v, want %v", d, DefaultReconnectDelay)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the reconnect")
		}
	}
	if second.ServerID != 1 || second.ServerName != "Bartz" {
		t.Fatalf("relinked as %d/%q, want 1/Bartz", second.ServerID, second.ServerName)
	}
	if entry, ok := servers.Get(1); !ok || !entry.Authed || entry.Port != 7777 {
		t.Fatalf("registry entry after the relink = %+v, %v; want server 1 online", entry, ok)
	}
}
