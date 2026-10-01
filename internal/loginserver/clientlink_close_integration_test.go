package loginserver

import (
	"errors"
	"net"
	"os"
	"slices"
	"testing"
	"time"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"

	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// holdPurgeSnapshot takes the purge lock and copies the registered
// connections, as purgeStale does before it kicks them. While the lock is
// held, a finished handler cannot get past unregistering itself, which
// pins the window between a handler's final packet and its cleanup that a
// purge kick or an eviction can race. release drops the lock.
func holdPurgeSnapshot(t *testing.T, l *ClientLink) (stale []*clientConn, release func()) {
	t.Helper()
	l.purgeMu.Lock()
	for c := range l.purgeable {
		stale = append(stale, c)
	}
	released := false
	release = func() {
		if !released {
			released = true
			l.purgeMu.Unlock()
		}
	}
	t.Cleanup(release)
	return stale, release
}

// fullServer links server id at capacity, so a plain account's
// RequestServerLogin is answered with PlayFail TooManyPlayers.
func fullServer(t *testing.T, servers *manager.ServerRegistry, id int) {
	t.Helper()
	normal := link.ServerTypeNormal
	servers.Register(id, []byte{0x01})
	servers.MarkOnline(id, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 1)
	if _, ok := servers.ApplyStatus(id, link.ServerStatus{Status: &normal}); !ok {
		t.Fatalf("ApplyStatus(%d) = false", id)
	}
	servers.AddOnlineAccount(id, "someoneelse")
}

func expectPlayFailTooManyPlayers(t *testing.T, reply []byte) {
	t.Helper()
	if reply[0] != serverpackets.OpcodePlayFail || reply[1] != byte(serverpackets.PlayFailTooManyPlayers) {
		t.Fatalf("reply opcode %#x reason %#x, want PlayFail TooManyPlayers", reply[0], reply[1])
	}
}

// TestClientLinkPurgeKickAfterHandlerFinalPacketSendsNothing races a purge
// kick against a handler's own final reply: the purge copied the
// connection before the handler answered RequestServerLogin with PlayFail,
// and kicks it afterwards. The handler's close came first, so the client
// receives PlayFail and then EOF, never the kick's LoginFail.
func TestClientLinkPurgeKickAfterHandlerFinalPacketSendsNothing(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 7))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	fullServer(t, servers, 7)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	stale, release := holdPurgeSnapshot(t, l)
	if len(stale) != 1 {
		t.Fatalf("purge snapshot holds %d connections, want 1", len(stale))
	}

	c.send(encodeRequestServerLogin(key1, key2, 7))
	expectPlayFailTooManyPlayers(t, c.read())

	for _, conn := range stale {
		conn.kick()
	}
	release()
	c.expectClosed()
}

// TestClientLinkEvictionAfterHandlerFinalPacketSendsNothing races a
// duplicate login's eviction against the holder's own final reply: the
// holder was answered PlayFail, but before its cleanup ran a second login
// for the account found it still mapped and evicted it. The holder
// receives PlayFail and then EOF, never the eviction's LoginFail; the new
// client is still refused with AccountInUse.
func TestClientLinkEvictionAfterHandlerFinalPacketSendsNothing(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 7))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	fullServer(t, servers, 7)

	first := dialLoginClient(t, addr)
	key1, key2 := first.login(l, "player1", "s3cret")

	_, release := holdPurgeSnapshot(t, l)

	first.send(encodeRequestServerLogin(key1, key2, 7))
	expectPlayFailTooManyPlayers(t, first.read())

	second := dialLoginClient(t, addr)
	second.gameGuard()
	second.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := second.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("second login opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccountInUse {
		t.Fatalf("second login reason = %d, want AccountInUse (%d)", reason, serverpackets.LoginFailAccountInUse)
	}
	second.expectClosed()

	release()
	first.expectClosed()
}

// stallPeer floods c's connection with request and never reads a reply,
// until the server stops reading it: its handler is then blocked writing a
// reply into a socket whose buffers are full. A write that cannot complete
// within half a second marks that point.
func stallPeer(t *testing.T, c *fakeLoginClient, request []byte) {
	t.Helper()
	buf := make([]byte, commoncrypt.PaddedSize(len(request)+4))
	copy(buf, request)
	commoncrypt.AppendChecksum(buf)
	commoncrypt.EncryptBlocks(c.cipher, buf)
	frame, err := wire.FrameBytes(buf)
	if err != nil {
		t.Fatalf("FrameBytes: %v", err)
	}
	batch := slices.Repeat(frame, 64)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		c.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		if _, err := c.conn.Write(batch); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return
			}
			t.Fatalf("flood write: %v", err)
		}
	}
	t.Fatal("server kept reading a peer that never drains its replies")
}

// TestClientLinkSweepDoesNotWaitOnStalledPeer stalls one authed peer, whose
// handler is blocked writing to it, and runs a purge-loop tick once both it
// and an idle authed peer are past the login timeout. The tick returns at
// once, the idle peer is still purged with LoginFail AccessFailed and then
// EOF, and the same tick's failed-attempt sweep still runs.
func TestClientLinkSweepDoesNotWaitOnStalledPeer(t *testing.T) {
	accounts := newFakeAccountStore(
		model.NewAccount("stalled", mustHashPassword(t, "s3cret"), 0, 1),
		model.NewAccount("idle", mustHashPassword(t, "s3cret"), 0, 1),
	)
	// The purge loop's own ticker never fires during the test; the test runs
	// its tick directly, at a time past the login timeout.
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) {
		l.loginTimeout = time.Hour
		l.failedAttempts = map[string]failedAttempt{"192.0.2.9": {count: 1, last: time.Now()}}
	})
	// Large ServerList replies fill the stalled peer's socket quickly.
	for id := 1; id <= 100; id++ {
		markOnlineAuto(t, servers, id)
	}

	stalled := dialLoginClient(t, addr)
	key1, key2 := stalled.login(l, "stalled", "s3cret")
	stallPeer(t, stalled, encodeRequestServerList(key1, key2))

	idle := dialLoginClient(t, addr)
	idle.login(l, "idle", "s3cret")

	swept := make(chan struct{})
	go func() {
		l.sweep(time.Now().Add(2 * time.Hour))
		close(swept)
	}()
	select {
	case <-swept:
	case <-time.After(2 * time.Second):
		t.Fatal("purge tick blocked on a stalled peer")
	}

	idle.expectLoginFail(serverpackets.LoginFailAccessFailed)
	waitSessionMissing(t, sessions, "idle")
	if n := failedAttemptCount(l); n != 0 {
		t.Fatalf("failed attempts tracked = %d after the tick, want 0", n)
	}
}
