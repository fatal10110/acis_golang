package loginserver

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// callHold holds the fake account store's call op for login, once armed,
// until release: entered is closed when the first held call starts.
type callHold struct {
	op, login string
	armed     atomic.Bool
	entered   chan struct{}
	enterOnce sync.Once
	release   chan struct{}
}

func newCallHold(op, login string) *callHold {
	return &callHold{op: op, login: login, entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *callHold) gate(ctx context.Context, op, login string) error {
	if op != h.op || login != h.login || !h.armed.Load() {
		return nil
	}
	h.enterOnce.Do(func() { close(h.entered) })
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *callHold) waitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s(%s) was never called", h.op, h.login)
	}
}

// serverLogin sends RequestServerLogin for serverID and requires PlayOk,
// returning the session it completes.
func (f *fakeLoginClient) serverLogin(key1, key2 int32, serverID byte) link.SessionKey {
	f.t.Helper()
	f.send(encodeRequestServerLogin(key1, key2, serverID))
	reply := f.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		f.t.Fatalf("server login opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	return link.SessionKey{LoginKey1: key1, LoginKey2: key2, PlayKey1: r.ReadInt32(), PlayKey2: r.ReadInt32()}
}

// expectOnlySession requires sessions to hold exactly want for account.
func expectOnlySession(t *testing.T, sessions interface {
	Get(string) (link.SessionKey, bool)
}, account string, want link.SessionKey,
) {
	t.Helper()
	if got, ok := sessions.Get(account); !ok || got != want {
		t.Fatalf("session for %s = %+v (stored %v), want %+v", account, got, ok, want)
	}
}

// A second login for an account collides with its holder at every point of
// the holder's server choice. Each ordering is forced by holding one store
// call: whichever side wins, both clients end closed, no session is left
// behind, and a third login right after is accepted.
func TestClientLinkDoubleLoginRaceLeavesNoSession(t *testing.T) {
	hash := mustHashPassword(t, "s3cret")
	const serverID = 2 // the account last played on server 1, so a choice of 2 writes it

	tests := []struct {
		name string
		op   string
		run  func(t *testing.T, l *ClientLink, addr string, hold *callHold)
	}{
		{
			// The holder passed the key and server checks and waits on its
			// last-server write when the second login evicts it: its PlayOk
			// never goes out.
			name: "evicted between server check and PlayOk",
			op:   "SetLastServer",
			run: func(t *testing.T, l *ClientLink, addr string, hold *callHold) {
				a := dialLoginClient(t, addr)
				k1, k2 := a.login(l, "player1", "s3cret")
				hold.armed.Store(true)
				a.send(encodeRequestServerLogin(k1, k2, serverID))
				hold.waitEntered(t)

				b := dialLoginClient(t, addr)
				b.gameGuard()
				b.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
				b.expectLoginFail(serverpackets.LoginFailAccountInUse)

				close(hold.release)
				a.expectLoginFail(serverpackets.LoginFailAccountInUse)
			},
		},
		{
			// The second login waits on its account lookup while the holder
			// receives PlayOk and stays connected.
			name: "second login resolves after PlayOk",
			op:   "Account",
			run: func(t *testing.T, l *ClientLink, addr string, hold *callHold) {
				a := dialLoginClient(t, addr)
				k1, k2 := a.login(l, "player1", "s3cret")
				hold.armed.Store(true)

				b := dialLoginClient(t, addr)
				b.gameGuard()
				b.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
				hold.waitEntered(t)

				a.serverLogin(k1, k2, serverID)
				close(hold.release)
				b.expectLoginFail(serverpackets.LoginFailAccountInUse)
				a.expectLoginFail(serverpackets.LoginFailAccountInUse)
			},
		},
		{
			// As above, but the holder hangs up after PlayOk, so its
			// teardown and the second login race.
			name: "holder hangs up after PlayOk",
			op:   "Account",
			run: func(t *testing.T, l *ClientLink, addr string, hold *callHold) {
				a := dialLoginClient(t, addr)
				k1, k2 := a.login(l, "player1", "s3cret")
				hold.armed.Store(true)

				b := dialLoginClient(t, addr)
				b.gameGuard()
				b.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
				hold.waitEntered(t)

				a.serverLogin(k1, k2, serverID)
				a.conn.Close()
				close(hold.release)
				b.expectLoginFail(serverpackets.LoginFailAccountInUse)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := newFakeAccountStore(model.NewAccount("player1", hash, 0, 1))
			hold := newCallHold(tt.op, "player1")
			accounts.gate = hold.gate
			addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) {
				l.loginTimeout = time.Hour
			})
			markOnlineAuto(t, servers, serverID)

			tt.run(t, l, addr, hold)

			if _, ok := sessions.Get("player1"); ok {
				t.Fatal("a session for player1 outlived the collision")
			}
			c := dialLoginClient(t, addr)
			k1, k2 := c.login(l, "player1", "s3cret")
			key := c.serverLogin(k1, k2, serverID)
			expectOnlySession(t, sessions, "player1", key)
		})
	}
}

// A connection replaced as its account's holder (its session was claimed by
// the game server, then the account logged in again elsewhere) never drops
// the replacement's session when it goes away, even past the login timeout.
func TestClientLinkReplacedHolderKeepsReplacementSession(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) {
		l.loginTimeout = time.Hour
	})
	servers.Register(1, testHexID)
	gs := dialGameServer(t, serveGameServerLinkFor(t, servers, sessions))
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("game server registration failed")
	}
	auto := link.ServerTypeAuto
	servers.ApplyStatus(1, link.ServerStatus{Status: &auto})

	a := dialLoginClient(t, addr)
	k1, k2 := a.login(l, "player1", "s3cret")
	first := a.serverLogin(k1, k2, 1)
	gs.sendPlayerAuthRequest("player1", first)
	if _, ok := gs.readPlayerAuthResponse(); !ok {
		t.Fatal("game server could not claim the first session")
	}
	l.authMu.Lock()
	holder := l.holders["player1"]
	l.authMu.Unlock()

	b := dialLoginClient(t, addr)
	k1, k2 = b.login(l, "player1", "s3cret")
	second := b.serverLogin(k1, k2, 1)

	// The replaced connection's teardown, run late and past the timeout.
	l.releaseSession(holder, time.Now().Add(2*time.Hour))

	expectOnlySession(t, sessions, "player1", second)
	gs.sendPlayerAuthRequest("player1", second)
	if _, ok := gs.readPlayerAuthResponse(); !ok {
		t.Fatal("game server could not claim the replacement session")
	}
}

// A session whose client hung up after PlayOk is kept for the game server
// only until the login timeout: the purge tick past it drops the session,
// and the account can log in again.
func TestClientLinkJoinedSessionExpiresAtLoginTimeout(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	// The purge loop's own ticker never fires during the test.
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) {
		l.loginTimeout = time.Hour
	})
	markOnlineAuto(t, servers, 1)

	a := dialLoginClient(t, addr)
	k1, k2 := a.login(l, "player1", "s3cret")
	key := a.serverLogin(k1, k2, 1)
	a.conn.Close()
	waitUntil(t, "the closed connection's session to be kept unclaimed", func() bool {
		l.authMu.Lock()
		defer l.authMu.Unlock()
		_, ok := l.unclaimed["player1"]
		return ok
	})

	l.sweep(time.Now())
	expectOnlySession(t, sessions, "player1", key)

	l.sweep(time.Now().Add(2 * time.Hour))
	if _, ok := sessions.Get("player1"); ok {
		t.Fatal("joined session outlived the login timeout")
	}
	l.authMu.Lock()
	left := len(l.unclaimed)
	l.authMu.Unlock()
	if left != 0 {
		t.Fatalf("unclaimed sessions after expiry = %d, want 0", left)
	}

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
}
