package gameservertest

import (
	"testing"
	"time"
)

// LoginAccountOnline reports whether the login server lists account as in
// play on this server, once it has read every message this server sent it
// so far.
func (s *Server) LoginAccountOnline(tb testing.TB, account string) bool {
	tb.Helper()
	s.LoginEntry(tb)
	id, ok := s.login.servers.AccountServerID(account)
	return ok && id == 1
}

// AwaitLoginLogout waits until the login server no longer lists account as
// in play on this server, failing tb if that takes longer than
// loginLinkSyncWait (a failure guard, not a pacing delay). The logout is
// reported from the connection's own goroutine as it ends, so there is
// nothing to read on the test's side.
func (s *Server) AwaitLoginLogout(tb testing.TB, account string) {
	tb.Helper()
	for deadline := time.Now().Add(loginLinkSyncWait); s.LoginAccountOnline(tb, account); {
		if time.Now().After(deadline) {
			tb.Fatalf("login server still lists %s in play", account)
		}
		time.Sleep(time.Millisecond)
	}
}

// ListOnLogin makes the login server list account as in play on this
// server without any connection here claiming it, as a stale roster entry
// would.
func (s *Server) ListOnLogin(account string) {
	s.login.servers.AddOnlineAccount(1, account)
}

// AwaitLoginKick returns the account of the next kick request from the
// login server this server handled, failing tb if none comes within
// loginLinkSyncWait.
func (s *Server) AwaitLoginKick(tb testing.TB) string {
	tb.Helper()
	select {
	case account := <-s.shutdown.kicked:
		return account
	case <-time.After(loginLinkSyncWait):
		tb.Fatal("login server sent no kick request")
		return ""
	}
}
