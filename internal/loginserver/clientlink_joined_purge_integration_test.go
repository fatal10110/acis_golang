package loginserver

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// TestClientLinkSweepPurgesJoinedSessionPastLoginTimeout covers the client
// that has already been handed PlayOk: it stays registered until the game
// server claims its session, so a client that sits on its connection past the
// login timeout is purged with LoginFail AccessFailed like any other, and its
// session is dropped with it.
func TestClientLinkSweepPurgesJoinedSessionPastLoginTimeout(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("joined", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) {
		l.loginTimeout = time.Nanosecond
	})
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "joined", "s3cret")
	c.send(encodeRequestServerLogin(key1, key2, 7))
	if reply := c.read(); reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}

	// The login timeout is a nanosecond, so the connection is already past
	// it and the disconnect, which reads the real clock, drops the session.
	l.sweep(time.Now())
	c.expectLoginFail(serverpackets.LoginFailAccessFailed)
	waitSessionMissing(t, sessions, "joined")
}
