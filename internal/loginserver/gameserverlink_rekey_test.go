package loginserver

import (
	"errors"
	"os"
	"testing"
	"time"
)

// TestGameServerLinkDropsBlowFishKeyAfterRegistration presents a second
// Blowfish key on a registered link, right after the roster encrypted a
// kick to it from another goroutine: the login server closes the link
// instead of switching keys under the roster's sends.
func TestGameServerLinkDropsBlowFishKeyAfterRegistration(t *testing.T) {
	addr, l, servers, _, _ := newTestLink(t, true)
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, id, _, _ := gs.readAuthResult(); !ok || id != 1 {
		t.Fatalf("readAuthResult() = ok=%v id=%d, want ok=true id=1", ok, id)
	}

	if !l.roster.kick(1, "acc1") {
		t.Fatal("kick not delivered to the registered link")
	}
	if got := gs.readKickPlayer(); got != "acc1" {
		t.Fatalf("KickPlayer account = %q, want acc1", got)
	}

	gs.sendBlowFishKey([]byte("fedcba9876543210"))
	// Closed means the read ends on the close, not on the deadline.
	if err := gs.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := gs.conn.Read(make([]byte, 1)); n != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read after the second key = %d, %v; want the link closed", n, err)
	}
}
