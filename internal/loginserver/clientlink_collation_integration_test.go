package loginserver

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// accentedPlayer1 is a spelling of player1 that the accounts table's
// accent-insensitive collation treats as equal to it (#5046).
const accentedPlayer1 = "plàyer1"

// newCollationStore returns an account store over a real database holding
// the account player1 / s3cret.
func newCollationStore(t *testing.T) *loginsql.AccountStore {
	t.Helper()
	store := loginsql.NewAccountStore(newIntegrationDB(t))
	if _, err := store.CreateAccount(context.Background(), "player1", mustHashPassword(t, "s3cret"), time.Now()); err != nil {
		t.Fatalf("CreateAccount(player1): %v", err)
	}
	return store
}

func withAccounts(store accountStore) func(*ClientLink) {
	return func(l *ClientLink) { l.accounts = store }
}

func expectAccountInUse(t *testing.T, c *fakeLoginClient) {
	t.Helper()
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccountInUse {
		t.Fatalf("reason = %d, want REASON_ACCOUNT_IN_USE (%d)", reason, serverpackets.LoginFailAccountInUse)
	}
	c.expectClosed()
}

// TestClientLinkCollationEqualSpellingUsesStoredLogin logs in with a
// spelling the database matches to player1: the session is keyed by the
// stored login, never by the typed spelling.
func TestClientLinkCollationEqualSpellingUsesStoredLogin(t *testing.T) {
	store := newCollationStore(t)
	addr, l, _, sessions, _ := newTestClientLink(t, nil, false, withAccounts(store))

	c := dialLoginClient(t, addr)
	c.login(l, accentedPlayer1, "s3cret")

	if _, ok := sessions.Get("player1"); !ok {
		t.Fatal("no session under the stored login player1")
	}
	if _, ok := sessions.Get(accentedPlayer1); ok {
		t.Fatalf("session issued under the typed spelling %q", accentedPlayer1)
	}
}

// TestClientLinkCollationEqualSpellingRefusedWhileCanonicalHeld logs in as
// player1, then with a collation-equal spelling: the second login collides
// with the first as the same account and both close as in use.
func TestClientLinkCollationEqualSpellingRefusedWhileCanonicalHeld(t *testing.T) {
	store := newCollationStore(t)
	addr, l, _, sessions, _ := newTestClientLink(t, nil, false, withAccounts(store))

	first := dialLoginClient(t, addr)
	first.login(l, "player1", "s3cret")

	second := dialLoginClient(t, addr)
	second.gameGuard()
	second.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, accentedPlayer1, "s3cret"))
	expectAccountInUse(t, second)
	expectAccountInUse(t, first)

	if _, ok := sessions.Get("player1"); ok {
		t.Fatal("session for player1 kept after the double login, want it dropped")
	}
	if _, ok := sessions.Get(accentedPlayer1); ok {
		t.Fatalf("session issued under the typed spelling %q", accentedPlayer1)
	}
}

// TestClientLinkCollationEqualSpellingRefusedWhileCanonicalInGame puts
// player1 in play on a linked game server, then logs in with a
// collation-equal spelling: the login is refused as in use and the game
// server is asked to evict player1.
func TestClientLinkCollationEqualSpellingRefusedWhileCanonicalInGame(t *testing.T) {
	store := newCollationStore(t)
	clientAddr, l, gsAddr, servers, sessions := newTestLinkPair(t, store)

	servers.Register(1, testHexID)
	gs := dialGameServer(t, gsAddr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 100, testHexID)
	if ok, _, _, failReason := gs.readAuthResult(); !ok {
		t.Fatalf("game server auth failed with reason %d", failReason)
	}

	first := dialLoginClient(t, clientAddr)
	first.login(l, "player1", "s3cret")
	gs.sendPlayerInGame("player1")
	waitUntil(t, "player1 online on server 1", func() bool {
		_, online := servers.AccountServerID("player1")
		return online
	})

	second := dialLoginClient(t, clientAddr)
	second.gameGuard()
	second.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, accentedPlayer1, "s3cret"))
	expectAccountInUse(t, second)

	if account := gs.readKickPlayer(); account != "player1" {
		t.Fatalf("KickPlayer account = %q, want player1", account)
	}
	if _, ok := sessions.Get(accentedPlayer1); ok {
		t.Fatalf("session issued under the typed spelling %q", accentedPlayer1)
	}
}

// TestGameServerRefusesSessionKeysUnderCollationEqualSpelling logs in and
// joins a server with a collation-equal spelling of player1, then presents
// the issued keys to the session check: the typed spelling is refused, the
// stored login player1 is accepted (#5046, #5464).
func TestGameServerRefusesSessionKeysUnderCollationEqualSpelling(t *testing.T) {
	store := newCollationStore(t)
	clientAddr, l, gsAddr, servers, _ := newTestLinkPair(t, store)

	servers.Register(1, testHexID)
	gs := dialGameServer(t, gsAddr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 100, testHexID)
	if ok, _, _, failReason := gs.readAuthResult(); !ok {
		t.Fatalf("game server auth failed with reason %d", failReason)
	}
	auto := link.ServerTypeAuto
	servers.ApplyStatus(1, link.ServerStatus{Status: &auto})

	c := dialLoginClient(t, clientAddr)
	loginKey1, loginKey2 := c.login(l, accentedPlayer1, "s3cret")
	keys := c.serverLogin(loginKey1, loginKey2, 1)

	gs.sendPlayerAuthRequest(accentedPlayer1, keys)
	if account, ok := gs.readPlayerAuthResponse(); account != accentedPlayer1 || ok {
		t.Fatalf("PlayerAuthResponse under %q = %q, %v, want %q, false", accentedPlayer1, account, ok, accentedPlayer1)
	}
	gs.sendPlayerAuthRequest("player1", keys)
	if account, ok := gs.readPlayerAuthResponse(); account != "player1" || !ok {
		t.Fatalf("PlayerAuthResponse under player1 = %q, %v, want player1, true", account, ok)
	}
}
