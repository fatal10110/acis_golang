package loginserver

import (
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

func TestClientLinkServerList(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 2))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerList(key1, key2))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeServerList {
		t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
	}
	if count := reply[1]; count != 1 {
		t.Fatalf("server count = %d, want 1", count)
	}
	if last := reply[2]; last != 2 {
		t.Fatalf("last server = %d, want 2 (account.LastServer)", last)
	}
	if id := reply[3]; id != 7 {
		t.Fatalf("server id = %d, want 7", id)
	}
}

func TestClientLinkServerEntriesUseAdvertisedStatusForOnlineByte(t *testing.T) {
	servers := manager.NewServerRegistry()
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)

	l := &ClientLink{servers: servers}
	entries := l.serverEntries(0, net.ParseIP("127.0.0.1"))
	if len(entries) != 1 {
		t.Fatalf("serverEntries length = %d, want 1", len(entries))
	}
	if entries[0].Online {
		t.Fatal("Online = true while status is Down")
	}

	auto := link.ServerTypeAuto
	servers.ApplyStatus(7, link.ServerStatus{Status: &auto})

	entries = l.serverEntries(0, net.ParseIP("127.0.0.1"))
	if len(entries) != 1 || !entries[0].Online {
		t.Fatalf("serverEntries = %+v, want one online entry after Status=Auto", entries)
	}
}

func TestClientLinkServerEntriesFallbackToLoopbackForNonIPv4Host(t *testing.T) {
	servers := manager.NewServerRegistry()
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "not-an-ip", net.ParseIP("127.0.0.1"), 7777, 100)

	l := &ClientLink{servers: servers}
	entries := l.serverEntries(0, net.ParseIP("127.0.0.1"))
	if len(entries) != 1 {
		t.Fatalf("serverEntries length = %d, want 1", len(entries))
	}
	if want := [4]byte{127, 0, 0, 1}; entries[0].IP != want {
		t.Fatalf("server entry IP = %v, want %v", entries[0].IP, want)
	}
}

func TestClientLinkShowLicenceOffRepliesServerListAfterLogin(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 2))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) { l.skipLicenceCheck = true })
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)

	c := dialLoginClient(t, addr)
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeServerList {
		t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
	}
	if count := reply[1]; count != 1 {
		t.Fatalf("server count = %d, want 1", count)
	}
	if last := reply[2]; last != 2 {
		t.Fatalf("last server = %d, want 2 (account.LastServer)", last)
	}
}

func TestClientLinkShowLicenceOffSkipsSessionKeyCheckOnServerLogin(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) { l.skipLicenceCheck = true })
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeServerList {
		t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
	}

	c.send(encodeRequestServerLogin(0x1a2b3c4d, -0x5e6f7081, 7))
	reply = c.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
}

func TestClientLinkPlayLoginSuccess(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	playKey1, playKey2 := r.ReadInt32(), r.ReadInt32()

	full, ok := sessions.Get("player1")
	if !ok {
		t.Fatal("expected session to remain stored after PlayOk")
	}
	want := link.SessionKey{LoginKey1: key1, LoginKey2: key2, PlayKey1: playKey1, PlayKey2: playKey2}
	if full != want {
		t.Fatalf("stored session = %+v, want %+v", full, want)
	}

	acc, _ := accounts.get("player1")
	if acc.LastServer != 7 {
		t.Fatalf("account.LastServer = %d, want 7", acc.LastServer)
	}
}

func TestClientLinkDisconnectBeforeGameServerJoinReleasesSession(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
	if _, ok := sessions.Get("player1"); !ok {
		t.Fatal("expected session to be stored after LoginOk")
	}

	if err := c.conn.Close(); err != nil {
		t.Fatalf("close login client: %v", err)
	}
	waitSessionMissing(t, sessions, "player1")

	c = dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
}

func TestClientLinkDisconnectAfterPlayOkKeepsSessionForGameServer(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")
	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	want := link.SessionKey{
		LoginKey1: key1,
		LoginKey2: key2,
		PlayKey1:  r.ReadInt32(),
		PlayKey2:  r.ReadInt32(),
	}

	if err := c.conn.Close(); err != nil {
		t.Fatalf("close login client: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	got, ok := sessions.Get("player1")
	if !ok {
		t.Fatal("expected session to remain after PlayOk")
	}
	if got != want {
		t.Fatalf("stored session = %+v, want %+v", got, want)
	}
}

func TestClientLinkPlayLoginUnknownServerFails(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerLogin(key1, key2, 9))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayFail {
		t.Fatalf("opcode = %#x, want PlayFail (%#x)", reply[0], serverpackets.OpcodePlayFail)
	}
	if reason := reply[1]; reason != byte(serverpackets.PlayFailTooManyPlayers) {
		t.Fatalf("PlayFail reason = %#x, want TooManyPlayers (%#x)", reason, serverpackets.PlayFailTooManyPlayers)
	}
	c.expectClosed()
}

func TestClientLinkServerListBadSessionKeyClosesWithAccessFailed(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerList(0x11223344, 0x55667788))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := reply[1]; reason != byte(serverpackets.LoginFailAccessFailed) {
		t.Fatalf("LoginFail reason = %#x, want AccessFailed (%#x)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()
}

func TestClientLinkServerLoginBadSessionKeyClosesWithAccessFailed(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerLogin(0x11223344, 0x55667788, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := reply[1]; reason != byte(serverpackets.LoginFailAccessFailed) {
		t.Fatalf("LoginFail reason = %#x, want AccessFailed (%#x)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()
}

func TestClientLinkServerLoginDownServerRejected(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayFail || reply[1] != byte(serverpackets.PlayFailTooManyPlayers) {
		t.Fatalf("reply opcode %#x reason %#x, want PlayFail TooManyPlayers", reply[0], reply[1])
	}
	c.expectClosed()
}

func TestClientLinkServerLoginGMOnlyAdmitsOnlySuperiorAccounts(t *testing.T) {
	gmOnly := link.ServerTypeGMOnly
	accounts := newFakeAccountStore(
		model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 7),
		model.NewAccount("gm", mustHashPassword(t, "s3cret"), 1, 7),
	)
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)
	servers.ApplyStatus(7, link.ServerStatus{Status: &gmOnly})

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")
	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayFail || reply[1] != byte(serverpackets.PlayFailTooManyPlayers) {
		t.Fatalf("access level 0: reply opcode %#x reason %#x, want PlayFail TooManyPlayers", reply[0], reply[1])
	}
	c.expectClosed()

	gmc := dialLoginClient(t, addr)
	key1, key2 = gmc.login(l, "gm", "s3cret")
	gmc.send(encodeRequestServerLogin(key1, key2, 7))
	if reply := gmc.read(); reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("access level 1: opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
}

func TestClientLinkServerLoginFullServerAdmitsOnlySuperiorAccounts(t *testing.T) {
	full := link.ServerTypeNormal
	accounts := newFakeAccountStore(
		model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 7),
		model.NewAccount("gm", mustHashPassword(t, "s3cret"), 1, 7),
	)
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 1)
	servers.ApplyStatus(7, link.ServerStatus{Status: &full})
	servers.AddOnlineAccount(7, "someoneelse")

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")
	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayFail || reply[1] != byte(serverpackets.PlayFailTooManyPlayers) {
		t.Fatalf("full server access level 0: reply opcode %#x reason %#x, want PlayFail TooManyPlayers", reply[0], reply[1])
	}
	c.expectClosed()

	servers.RemoveOnlineAccount(7, "someoneelse")
	c = dialLoginClient(t, addr)
	key1, key2 = c.login(l, "gm", "s3cret")
	c.send(encodeRequestServerLogin(key1, key2, 7))
	if reply := c.read(); reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("full server access level 1: opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
}

func TestClientLinkServerEntriesMaskGMOnlyForPlainAccounts(t *testing.T) {
	gmOnly := link.ServerTypeGMOnly
	servers := manager.NewServerRegistry()
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "93.184.216.34", net.ParseIP("10.0.0.5"), 7777, 100)
	servers.ApplyStatus(7, link.ServerStatus{Status: &gmOnly})

	l := &ClientLink{servers: servers}

	entries := l.serverEntries(0, net.ParseIP("203.0.113.50"))
	if len(entries) != 1 || entries[0].Online {
		t.Fatalf("plain account entries = %+v, want GM-only masked down", entries)
	}

	entries = l.serverEntries(1, net.ParseIP("203.0.113.50"))
	if len(entries) != 1 || !entries[0].Online {
		t.Fatalf("GM account entries = %+v, want online entry", entries)
	}
}

func TestClientLinkServerEntriesPointLocalClientsAtConnectionIP(t *testing.T) {
	servers := manager.NewServerRegistry()
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "93.184.216.34", net.ParseIP("192.168.1.10"), 7777, 100)

	l := &ClientLink{servers: servers}

	local := [4]byte{192, 168, 1, 10}
	if entries := l.serverEntries(0, net.ParseIP("127.0.0.1")); entries[0].IP != local {
		t.Fatalf("local client entry IP = %v, want %v (connection IP)", entries[0].IP, local)
	}
	remote := [4]byte{93, 184, 216, 34}
	if entries := l.serverEntries(0, net.ParseIP("203.0.113.50")); entries[0].IP != remote {
		t.Fatalf("remote client entry IP = %v, want %v (advertised host)", entries[0].IP, remote)
	}
}
