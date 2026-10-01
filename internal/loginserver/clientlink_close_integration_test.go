package loginserver

import (
	"net"
	"testing"

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
