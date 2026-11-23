package loginserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// While one login's account lookup is held on the database, another login
// completes and its game server still validates the session.
func TestClientLinkHeldAccountLookupDoesNotBlockOtherLogins(t *testing.T) {
	hash := mustHashPassword(t, "s3cret")
	accounts := newFakeAccountStore(model.NewAccount("slow", hash, 0, 1), model.NewAccount("fast", hash, 0, 1))
	hold := newCallHold("Account", "slow")
	hold.armed.Store(true)
	accounts.gate = hold.gate
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	servers.Register(1, testHexID)
	gs := dialGameServer(t, serveGameServerLinkFor(t, servers, sessions))
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("game server registration failed")
	}
	auto := link.ServerTypeAuto
	servers.ApplyStatus(1, link.ServerStatus{Status: &auto})

	slow := dialLoginClient(t, addr)
	slow.gameGuard()
	slow.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "slow", "s3cret"))
	hold.waitEntered(t)

	fast := dialLoginClient(t, addr)
	k1, k2 := fast.login(l, "fast", "s3cret")
	key := fast.serverLogin(k1, k2, 1)
	gs.sendPlayerAuthRequest("fast", key)
	if account, ok := gs.readPlayerAuthResponse(); account != "fast" || !ok {
		t.Fatalf("PlayerAuthResponse = %q, %v, want fast, true", account, ok)
	}

	close(hold.release)
	if reply := slow.read(); reply[0] != serverpackets.OpcodeLoginOk {
		t.Fatalf("held login opcode = %#x, want LoginOk (%#x)", reply[0], serverpackets.OpcodeLoginOk)
	}
}

// Every account-store call a login makes carries a deadline no later than
// loginDBTimeout; a call that runs into it fails the request with the same
// reply as any other database failure.
func TestClientLinkDBCallTimeoutRepliesAsDBFailure(t *testing.T) {
	hash := mustHashPassword(t, "s3cret")
	tests := []struct {
		name       string
		op         string
		login      string
		autoCreate bool
		// wantFail is the LoginFail reason, or zero when the login goes on
		// to PlayOk.
		wantFail serverpackets.LoginFailReason
	}{
		{"account lookup counts as an unknown account", "Account", "player1", false, serverpackets.LoginFailUserOrPassWrong},
		{"account creation", "CreateAccount", "newbie", true, serverpackets.LoginFailAccessFailed},
		{"last-active write", "SetLastActive", "player1", false, serverpackets.LoginFailAccessFailed},
		{"last-server write is only logged", "SetLastServer", "player1", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := newFakeAccountStore(model.NewAccount("player1", hash, 0, 1))
			var (
				mu        sync.Mutex
				calls     int
				remaining time.Duration
				bounded   bool
			)
			accounts.gate = func(ctx context.Context, op, _ string) error {
				if op != tt.op {
					return nil
				}
				deadline, ok := ctx.Deadline()
				mu.Lock()
				calls++
				remaining, bounded = time.Until(deadline), ok
				mu.Unlock()
				return context.DeadlineExceeded
			}
			addr, l, servers, _, _ := newTestClientLink(t, accounts, tt.autoCreate)
			markOnlineAuto(t, servers, 2)

			c := dialLoginClient(t, addr)
			if tt.wantFail != 0 {
				c.gameGuard()
				c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, tt.login, "s3cret"))
				c.expectLoginFail(tt.wantFail)
			} else {
				k1, k2 := c.login(l, tt.login, "s3cret")
				c.serverLogin(k1, k2, 2)
			}

			mu.Lock()
			defer mu.Unlock()
			if calls != 1 {
				t.Fatalf("%s calls = %d, want 1", tt.op, calls)
			}
			if !bounded || remaining <= 0 || remaining > loginDBTimeout {
				t.Fatalf("%s deadline: set %v, %v left, want at most %v", tt.op, bounded, remaining, loginDBTimeout)
			}
		})
	}
}

// heldAccessLevelStore holds every SetAccessLevel until release and records
// the calls it completed.
type heldAccessLevelStore struct {
	entered chan struct{}
	release chan struct{}

	mu      sync.Mutex
	writes  []string
	levels  []int
	bounded bool
}

func (s *heldAccessLevelStore) SetAccessLevel(ctx context.Context, login string, level int) error {
	s.entered <- struct{}{}
	<-s.release
	_, ok := ctx.Deadline()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, login)
	s.levels = append(s.levels, level)
	s.bounded = ok
	return nil
}

// A game server's access-level change held on the database does not stall
// its link: a PlayerAuthRequest sent after it is still answered, and the
// write completes, with a deadline, once the database answers.
func TestGameServerLinkHeldAccessLevelWriteDoesNotBlockPlayerAuth(t *testing.T) {
	store := &heldAccessLevelStore{entered: make(chan struct{}, 1), release: make(chan struct{})}
	addr, _, servers, sessions, _ := newTestLinkWithRegistrationStore(t, false, nil, nil, func(l *GameServerLink) {
		l.accounts = store
	})
	servers.Register(1, testHexID)
	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("registration failed, want success")
	}
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	sessions.Put("player2", key, 1)

	admitOn(t, gs, sessions, 1, "player1")
	gs.sendPlayerInGame("player1")
	gs.sendChangeAccessLevel(-1, "player1")
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("SetAccessLevel was never called")
	}

	gs.sendPlayerAuthRequest("player2", key)
	if account, ok := gs.readPlayerAuthResponse(); account != "player2" || !ok {
		t.Fatalf("PlayerAuthResponse = %q, %v, want player2, true", account, ok)
	}

	close(store.release)
	waitUntil(t, "the held access-level write to complete", func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.writes) == 1
	})
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.writes[0] != "player1" || store.levels[0] != -1 || !store.bounded {
		t.Fatalf("writes = %v levels %v (deadline set %v), want [player1] at -1 with a deadline", store.writes, store.levels, store.bounded)
	}
}
