package loginserver

import (
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// Failed logins are counted per address and account (owner-approved
// divergence, #5047): a good login clears only its own account's failures,
// and the address is banned once its failures across all accounts reach
// the threshold.

// failLogin sends one RequestAuthLogin that must be refused with reason.
func failLogin(t *testing.T, addr string, l *ClientLink, user, password string, reason serverpackets.LoginFailReason) {
	t.Helper()
	c := dialLoginClient(t, addr)
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, user, password))
	c.expectLoginFail(reason)
}

// succeedLogin logs user in, then drops the connection and waits for the
// login session to be released so the account can log in again.
func succeedLogin(t *testing.T, addr string, l *ClientLink, sessions *manager.SessionStore, user, password string) {
	t.Helper()
	c := dialLoginClient(t, addr)
	c.login(l, user, password)
	if err := c.conn.Close(); err != nil {
		t.Fatalf("close login client: %v", err)
	}
	waitSessionMissing(t, sessions, user)
}

// Wrong passwords against one account, each followed by a good login on
// another account from the same address, still reach the ban (#5047).
func TestClientLinkSuccessOnOtherAccountKeepsFailedCount(t *testing.T) {
	accounts := newFakeAccountStore(
		model.NewAccount("victim", mustHashPassword(t, "right"), 0, 1),
		model.NewAccount("own", mustHashPassword(t, "mine"), 0, 1),
	)
	addr, l, _, sessions, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	for i := 1; i <= DefaultLoginTryBeforeBan; i++ {
		if i > 1 {
			succeedLogin(t, addr, l, sessions, "own", "mine")
		}
		if bans.IsBanned(ip) {
			t.Fatalf("IP banned before failure %d", i)
		}
		failLogin(t, addr, l, "victim", "wrong", serverpackets.LoginFailPasswordWrong)
	}
	if !bans.IsBanned(ip) {
		t.Fatal("successful logins on another account cleared the victim account's failures")
	}
}

// An unknown account with auto-creation off counts under the requested
// name, so successes on a real account do not clear it either (#5047).
func TestClientLinkSuccessDoesNotClearUnknownAccountFailures(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("own", mustHashPassword(t, "mine"), 0, 1))
	addr, l, _, sessions, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	for i := 1; i <= DefaultLoginTryBeforeBan; i++ {
		if i > 1 {
			succeedLogin(t, addr, l, sessions, "own", "mine")
		}
		if bans.IsBanned(ip) {
			t.Fatalf("IP banned before failure %d", i)
		}
		failLogin(t, addr, l, "ghost", "any", serverpackets.LoginFailUserOrPassWrong)
	}
	if !bans.IsBanned(ip) {
		t.Fatal("successful logins cleared the unknown account's failures")
	}
}

// A player who mistypes and then logs in correctly is never banned, however
// often that repeats (#5047).
func TestClientLinkMistypeThenCorrectLoginIsNotBanned(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("own", mustHashPassword(t, "mine"), 0, 1))
	addr, l, _, sessions, bans := newTestClientLink(t, accounts, false)
	ip := net.ParseIP("127.0.0.1")

	for range DefaultLoginTryBeforeBan + 1 {
		failLogin(t, addr, l, "own", "mnie", serverpackets.LoginFailPasswordWrong)
		succeedLogin(t, addr, l, sessions, "own", "mine")
	}
	if bans.IsBanned(ip) {
		t.Fatal("IP banned although every mistype was followed by a good login")
	}
	if n := failedAttemptCount(l); n != 0 {
		t.Fatalf("tracked addresses after a clearing success = %d, want 0", n)
	}
}

// One failure on each of many accounts still bans at the threshold: the
// per-account split does not let an address spread guesses (#5047).
func TestClientLinkFailuresAcrossAccountsShareTheThreshold(t *testing.T) {
	l, bans := newFailedAttemptLink()
	ip := net.ParseIP("192.0.2.7")
	now := time.Now()

	for i := range DefaultLoginTryBeforeBan - 1 {
		l.recordFailedAttempt(ip, string(rune('a'+i)), now)
	}
	if bans.IsBanned(ip) {
		t.Fatal("banned below the threshold")
	}
	l.recordFailedAttempt(ip, "z", now)
	if !bans.IsBanned(ip) {
		t.Fatal("failures spread over accounts never reached the ban")
	}
	if n := failedAttemptCount(l); n != 0 {
		t.Fatalf("tracked addresses after the ban = %d, want 0", n)
	}
}

// A success clears only its own account's failures; the rest keep counting
// toward the address's ban (#5047).
func TestClientLinkClearDropsOnlyThatAccount(t *testing.T) {
	l, bans := newFailedAttemptLink()
	ip := net.ParseIP("192.0.2.8")
	now := time.Now()

	l.recordFailedAttempt(ip, "own", now)
	for range DefaultLoginTryBeforeBan - 2 {
		l.recordFailedAttempt(ip, "victim", now)
	}
	l.clearFailedAttempts(ip, "own")
	l.clearFailedAttempts(ip, "unrelated")
	if got := failedAttemptCount(l); got != 1 {
		t.Fatalf("tracked addresses = %d, want 1", got)
	}
	l.recordFailedAttempt(ip, "victim", now)
	if bans.IsBanned(ip) {
		t.Fatal("the cleared account's failure still counted toward the ban")
	}
	l.recordFailedAttempt(ip, "victim", now)
	if !bans.IsBanned(ip) {
		t.Fatal("the remaining account's failures did not reach the ban")
	}

	// Clearing the only account with failures forgets the address.
	other := net.ParseIP("192.0.2.9")
	l.recordFailedAttempt(other, "own", now)
	l.clearFailedAttempts(other, "own")
	if got := failedAttemptCount(l); got != 0 {
		t.Fatalf("tracked addresses after clearing the last account = %d, want 0", got)
	}
}
