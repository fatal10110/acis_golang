package loginserver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// A spelling the accounts table's collation matches to a stored login but
// that differs from it is refused at login as an unknown account (#5464):
// the game server keys the session by the typed name, so such a login could
// never enter the game.

// accentedPlayer1 is a spelling of player1 that the accounts table's
// accent-insensitive collation treats as equal to it.
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

// sendCollationVariantLogin opens a new client and logs in as
// accentedPlayer1 with password, expecting the UserOrPassWrong refusal and
// the connection closing.
func sendCollationVariantLogin(t *testing.T, addr string, l *ClientLink, password string) {
	t.Helper()
	c := dialLoginClient(t, addr)
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, accentedPlayer1, password))
	c.expectLoginFail(serverpackets.LoginFailUserOrPassWrong)
}

// holds reports whether l holds a login session for account.
func holds(l *ClientLink, account string) bool {
	l.authMu.Lock()
	defer l.authMu.Unlock()
	_, ok := l.holders[account]
	return ok
}

// failedAttemptsFor returns the failures l counts from ip under account.
func failedAttemptsFor(l *ClientLink, ip, account string) int {
	l.failedMu.Lock()
	defer l.failedMu.Unlock()
	return l.failedAttempts[ip].byAccount[account]
}

// TestClientLinkCollationEqualSpellingRefused logs in with a spelling the
// database matches to player1, with the right and a wrong password, with
// auto-creation off and on: every attempt is refused alike with
// UserOrPassWrong, counts as a failed attempt under the typed spelling, and
// leaves no session or holder behind.
func TestClientLinkCollationEqualSpellingRefused(t *testing.T) {
	for _, tc := range []struct {
		name       string
		password   string
		autoCreate bool
	}{
		{"right password", "s3cret", false},
		{"wrong password", "wrong", false},
		{"right password, auto-create", "s3cret", true},
		{"wrong password, auto-create", "wrong", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCollationStore(t)
			addr, l, _, sessions, _ := newTestClientLink(t, nil, tc.autoCreate, withAccounts(store))

			sendCollationVariantLogin(t, addr, l, tc.password)

			for _, account := range []string{"player1", accentedPlayer1} {
				if _, ok := sessions.Get(account); ok {
					t.Errorf("session issued under %q", account)
				}
				if holds(l, account) {
					t.Errorf("login holder kept under %q", account)
				}
			}
			if got := failedAttemptsFor(l, "127.0.0.1", accentedPlayer1); got != 1 {
				t.Errorf("failed attempts under %q = %d, want 1", accentedPlayer1, got)
			}
			if got := failedAttemptsFor(l, "127.0.0.1", "player1"); got != 0 {
				t.Errorf("failed attempts under player1 = %d, want 0", got)
			}
			if account, err := store.Account(context.Background(), accentedPlayer1); err != nil || account.Login != "player1" {
				t.Errorf("Account(%q) = %q, %v, want the stored row player1", accentedPlayer1, account.Login, err)
			}
		})
	}
}

// TestClientLinkCollationEqualSpellingFailuresReachBan repeats the refused
// spelling until the address is banned: each refusal counts like an unknown
// account's.
func TestClientLinkCollationEqualSpellingFailuresReachBan(t *testing.T) {
	store := newCollationStore(t)
	addr, l, _, _, bans := newTestClientLink(t, nil, false, withAccounts(store))
	ip := net.ParseIP("127.0.0.1")

	for i := 1; i <= DefaultLoginTryBeforeBan; i++ {
		if bans.IsBanned(ip) {
			t.Fatalf("IP banned before failure %d", i)
		}
		sendCollationVariantLogin(t, addr, l, "s3cret")
	}
	if !bans.IsBanned(ip) {
		t.Fatal("refused spellings did not reach the IP ban")
	}
}

// TestClientLinkCollationEqualSpellingLeavesCanonicalHeld logs in as
// player1, then with a collation-equal spelling: the spelling is refused
// as unknown and player1's login session is untouched.
func TestClientLinkCollationEqualSpellingLeavesCanonicalHeld(t *testing.T) {
	store := newCollationStore(t)
	addr, l, _, sessions, _ := newTestClientLink(t, nil, false, withAccounts(store))

	first := dialLoginClient(t, addr)
	first.login(l, "player1", "s3cret")
	before, ok := sessions.Get("player1")
	if !ok {
		t.Fatal("no session for player1 after its login")
	}

	sendCollationVariantLogin(t, addr, l, "s3cret")

	after, ok := sessions.Get("player1")
	if !ok || after != before {
		t.Fatalf("session for player1 = %+v, %v after the refused spelling, want %+v kept", after, ok, before)
	}
	if !holds(l, "player1") {
		t.Fatal("login holder for player1 dropped by the refused spelling")
	}
	if _, ok := sessions.Get(accentedPlayer1); ok {
		t.Fatalf("session issued under the typed spelling %q", accentedPlayer1)
	}
}

// TestClientLinkCollationEqualSpellingLeavesCanonicalInGame puts player1 in
// play on a linked game server, then logs in with a collation-equal
// spelling: the spelling is refused as unknown, player1 stays online and
// the game server is not asked to evict it. The game server's next frame is
// the answer to its own session check, which refuses the typed spelling.
func TestClientLinkCollationEqualSpellingLeavesCanonicalInGame(t *testing.T) {
	store := newCollationStore(t)
	clientAddr, l, gsAddr, servers, sessions := newTestLinkPair(t, store)

	servers.Register(1, testHexID)
	gs := dialGameServer(t, gsAddr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 100, testHexID)
	if ok, _, _, failReason := gs.readAuthResult(); !ok {
		t.Fatalf("game server auth failed with reason %d", failReason)
	}
	auto := link.ServerTypeAuto
	servers.ApplyStatus(1, link.ServerStatus{Status: &auto})

	first := dialLoginClient(t, clientAddr)
	loginKey1, loginKey2 := first.login(l, "player1", "s3cret")
	keys := first.serverLogin(loginKey1, loginKey2, 1)
	gs.sendPlayerInGame("player1")
	waitUntil(t, "player1 online on server 1", func() bool {
		_, online := servers.AccountServerID("player1")
		return online
	})

	sendCollationVariantLogin(t, clientAddr, l, "s3cret")

	if serverID, online := servers.AccountServerID("player1"); !online || serverID != 1 {
		t.Fatalf("player1 online = %v on server %d after the refused spelling, want online on 1", online, serverID)
	}
	gs.sendPlayerAuthRequest(accentedPlayer1, keys)
	if account, ok := gs.readPlayerAuthResponse(); account != accentedPlayer1 || ok {
		t.Fatalf("PlayerAuthResponse under %q = %q, %v, want %q, false", accentedPlayer1, account, ok, accentedPlayer1)
	}
	if _, ok := sessions.Get(accentedPlayer1); ok {
		t.Fatalf("session issued under the typed spelling %q", accentedPlayer1)
	}
}
