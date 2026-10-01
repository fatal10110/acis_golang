package loginserver

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// --- client link + game server link sharing one registry and roster ---

// newTestLinkPair boots a ClientLink and a GameServerLink over real
// sockets, sharing one registry, session store, and link roster, so the
// account-eviction path can be exercised end to end.
func newTestLinkPair(t *testing.T, accounts *fakeAccountStore) (clientAddr string, l *ClientLink, gsAddr string, servers *manager.ServerRegistry, sessions *manager.SessionStore) {
	t.Helper()

	dir := t.TempDir()
	namesPath := filepath.Join(dir, "serverNames.xml")
	if err := os.WriteFile(namesPath, []byte(`<?xml version='1.0'?><list>
		<server id="1" name="Bartz" />
	</list>`), 0o644); err != nil {
		t.Fatalf("write serverNames.xml: %v", err)
	}
	names, err := manager.LoadServerNames(namesPath)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}

	gsKeys, err := manager.NewRSAKeyPool()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}
	keyPair, err := commoncrypt.NewLoginKeyPair()
	if err != nil {
		t.Fatalf("NewLoginKeyPair: %v", err)
	}

	servers = manager.NewServerRegistry()
	sessions = manager.NewSessionStore()
	bans := manager.NewIPBanList(zerolog.Nop())
	roster := NewLinkRoster()

	gsLink := NewGameServerLink(servers, names, gsKeys, sessions, bans, nil, nil, false, nil, roster, zerolog.Nop())
	l = &ClientLink{
		accounts:           accounts,
		servers:            servers,
		sessions:           sessions,
		bans:               bans,
		roster:             roster,
		loginTryBeforeBan:  DefaultLoginTryBeforeBan,
		loginBlockAfterBan: DefaultLoginBlockAfterBan,
		log:                zerolog.Nop(),
		newKeyPair:         func() *commoncrypt.LoginKeyPair { return keyPair },
		newSessionKey:      func() ([]byte, error) { return testSessionKey, nil },
		newSessionID:       func() int32 { return testInitSessionID },
	}

	gsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go gsLink.Serve(ctx, gsLn)

	clientLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go l.Serve(ctx, clientLn)

	return clientLn.Addr().String(), l, gsLn.Addr().String(), servers, sessions
}

// TestClientLinkLoginForAccountOnGameServerRejectsAndKicks drives the
// account-already-in-play rejection: the second login is closed with
// REASON_ACCOUNT_IN_USE and the linked game server is asked to evict the
// live session, while the first login client stays untouched.
func TestClientLinkLoginForAccountOnGameServerRejectsAndKicks(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	clientAddr, l, gsAddr, servers, sessions := newTestLinkPair(t, accounts)

	servers.Register(1, testHexID)
	gs := dialGameServer(t, gsAddr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 100, testHexID)
	if ok, _, _, failReason := gs.readAuthResult(); !ok {
		t.Fatalf("game server auth failed with reason %d", failReason)
	}

	first := dialLoginClient(t, clientAddr)
	key1, key2 := first.login(l, "player1", "s3cret")

	gs.sendPlayerInGame("player1")
	// PlayerInGame is applied on the link's goroutine; wait for it, or the
	// second login can take the still-mapped double-login path instead.
	waitUntil(t, "player1 online on server 1", func() bool {
		_, online := servers.AccountServerID("player1")
		return online
	})

	second := dialLoginClient(t, clientAddr)
	second.gameGuard()
	second.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := second.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccountInUse {
		t.Fatalf("reason = %d, want REASON_ACCOUNT_IN_USE (%d)", reason, serverpackets.LoginFailAccountInUse)
	}
	second.expectClosed()

	if account := gs.readKickPlayer(); account != "player1" {
		t.Fatalf("KickPlayer account = %q, want player1", account)
	}

	if _, ok := sessions.Get("player1"); !ok {
		t.Fatal("session for player1 was dropped, want it kept for its holder")
	}
	first.send(encodeRequestServerList(key1, key2))
	if reply := first.read(); reply[0] != serverpackets.OpcodeServerList {
		t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
	}
}

// TestClientLinkDoubleLoginClosesBothClients drives the double-login
// collision while the account is still mapped on this server: neither the
// previous holder nor the new client survives, both close with
// REASON_ACCOUNT_IN_USE and the mapping is dropped.
func TestClientLinkDoubleLoginClosesBothClients(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)

	first := dialLoginClient(t, addr)
	first.login(l, "player1", "s3cret")

	second := dialLoginClient(t, addr)
	second.gameGuard()
	second.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := second.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccountInUse {
		t.Fatalf("reason = %d, want REASON_ACCOUNT_IN_USE (%d)", reason, serverpackets.LoginFailAccountInUse)
	}
	second.expectClosed()

	evicted := first.read()
	if evicted[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", evicted[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, evicted); reason != serverpackets.LoginFailAccountInUse {
		t.Fatalf("reason = %d, want REASON_ACCOUNT_IN_USE (%d)", reason, serverpackets.LoginFailAccountInUse)
	}
	first.expectClosed()

	if _, ok := sessions.Get("player1"); ok {
		t.Fatal("session for player1 remained mapped after both clients were closed")
	}
}

// TestClientLinkEvictionWriteDoesNotBlockOtherLogins drives a double-login
// collision whose previous holder never drains its socket: evicting that
// holder blocks on its write, but the eviction must happen outside the
// login mapping lock, so a login for any other account still completes.
func TestClientLinkEvictionWriteDoesNotBlockOtherLogins(t *testing.T) {
	accounts := newFakeAccountStore(
		model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1),
		model.NewAccount("player2", mustHashPassword(t, "s3cret"), 0, 1),
	)
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)

	// A previous holder for player1 whose connection nobody reads. net.Pipe
	// is synchronous and unbuffered: writing the eviction LoginFail to it
	// blocks until a read consumes the bytes, which never happens here.
	stalled, holderEnd := net.Pipe()
	t.Cleanup(func() { stalled.Close(); holderEnd.Close() })
	crypt, err := logincrypt.NewLoginCrypt(testSessionKey)
	if err != nil {
		t.Fatalf("NewLoginCrypt: %v", err)
	}
	if l.holders == nil {
		l.holders = make(map[string]*clientConn)
	}
	l.holders["player1"] = &clientConn{conn: stalled, crypt: crypt}
	sessions.Put("player1", link.SessionKey{LoginKey1: 1, LoginKey2: 2})

	// The colliding re-authentication for player1 runs concurrently; its
	// own goroutine may wait on the stalled eviction write.
	go func() {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer conn.Close()
		cipher, err := commoncrypt.NewBlowfishCipher(testSessionKey)
		if err != nil {
			return
		}
		send := func(payload []byte) {
			buf := make([]byte, commoncrypt.PaddedSize(len(payload)+4))
			copy(buf, payload)
			commoncrypt.AppendChecksum(buf)
			commoncrypt.EncryptBlocks(cipher, buf)
			_ = wire.WriteFrame(conn, buf)
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := wire.ReadFrame(conn); err != nil { // Init
			return
		}
		send(encodeAuthGameGuard(testInitSessionID))
		send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
		// Stay connected so the server-side write to this client is not
		// cut short by an early close.
		buf := make([]byte, 1)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
		}
	}()

	witness := dialLoginClient(t, addr)
	witness.gameGuard()
	witness.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player2", "s3cret"))
	reply := witness.read()
	if reply[0] != serverpackets.OpcodeLoginOk {
		t.Fatalf("opcode = %#x, want LoginOk (%#x): another account's login was blocked by the stalled eviction write", reply[0], serverpackets.OpcodeLoginOk)
	}
}

// TestClientLinkEvictionOfStalledHolderDoesNotDelayNewClient drives a
// double-login collision whose previous holder never drains its socket: the
// new client is still refused with AccountInUse and closed at once, while
// the holder is left to close with its own AccountInUse.
func TestClientLinkEvictionOfStalledHolderDoesNotDelayNewClient(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)

	// net.Pipe is synchronous: a write to it blocks until a read that
	// never happens here.
	stalled, holderEnd := net.Pipe()
	t.Cleanup(func() { stalled.Close(); holderEnd.Close() })
	crypt, err := logincrypt.NewLoginCrypt(testSessionKey)
	if err != nil {
		t.Fatalf("NewLoginCrypt: %v", err)
	}
	holder := &clientConn{conn: stalled, crypt: crypt}
	l.authMu.Lock()
	l.holders = map[string]*clientConn{"player1": holder}
	sessions.Put("player1", link.SessionKey{LoginKey1: 1, LoginKey2: 2})
	l.authMu.Unlock()

	c := dialLoginClient(t, addr)
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	c.expectLoginFail(serverpackets.LoginFailAccountInUse)

	final := holder.closeRequest.Load()
	if final == nil {
		t.Fatal("previous holder was not asked to close")
	}
	if (*final)[0] != serverpackets.OpcodeLoginFail || loginFailReason(t, *final) != serverpackets.LoginFailAccountInUse {
		t.Fatalf("previous holder's final packet = % X, want LoginFail AccountInUse", *final)
	}
	if _, ok := sessions.Get("player1"); ok {
		t.Fatal("session for player1 remained mapped")
	}
}
