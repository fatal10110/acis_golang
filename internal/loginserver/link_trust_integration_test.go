package loginserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

var secondHexID = []byte{0x05, 0x06, 0x07, 0x08}

// authGameServer links a fake game server as id with hexID, failing the test
// unless the login server accepts it.
func authGameServer(t *testing.T, addr string, id byte, hexID []byte) *fakeGameServer {
	t.Helper()
	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(id, false, false, "*", 7777, 300, hexID)
	if ok, got, _, reason := gs.readAuthResult(); !ok || got != id {
		t.Fatalf("game server %d auth = ok=%v id=%d reason=%d, want ok=true id=%d", id, ok, got, reason, id)
	}
	return gs
}

// TestSessionKeysValidateOnlyOnTheChosenServer: the keys a client got for
// server 1 are refused when linked server 2 presents them, and the refusal
// leaves them for server 1, which is admitted (#4655).
func TestSessionKeysValidateOnlyOnTheChosenServer(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	servers.Register(1, testHexID)
	servers.Register(2, secondHexID)
	gsAddr := serveGameServerLinkFor(t, servers, sessions)
	gsA := authGameServer(t, gsAddr, 1, testHexID)
	gsB := authGameServer(t, gsAddr, 2, secondHexID)
	up := link.ServerTypeAuto
	servers.ApplyStatus(1, link.ServerStatus{Status: &up})

	c := dialLoginClient(t, addr)
	loginKey1, loginKey2 := c.login(l, "player1", "s3cret")
	c.send(encodeRequestServerLogin(loginKey1, loginKey2, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("server login opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	playKey1, playKey2 := r.ReadInt32(), r.ReadInt32()
	key := link.SessionKey{LoginKey1: loginKey1, LoginKey2: loginKey2, PlayKey1: playKey1, PlayKey2: playKey2}

	gsB.sendPlayerAuthRequest("player1", key)
	if account, ok := gsB.readPlayerAuthResponse(); account != "player1" || ok {
		t.Fatalf("PlayerAuthResponse on server 2 = %q, %v, want player1, false", account, ok)
	}
	gsA.sendPlayerAuthRequest("player1", key)
	if account, ok := gsA.readPlayerAuthResponse(); account != "player1" || !ok {
		t.Fatalf("PlayerAuthResponse on server 1 = %q, %v, want player1, true", account, ok)
	}
}

// admitOn admits account on the fake game server gs (linked as id) the way a
// client entering it is: the session issued at login is bound to id when the
// client picks it, and gs presents the keys in a PlayerAuthRequest.
func admitOn(t *testing.T, gs *fakeGameServer, sessions *manager.SessionStore, id int, account string) {
	t.Helper()
	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: int32(id), PlayKey2: 44}
	sessions.Put(account, key, manager.NoServer)
	if !sessions.Bind(account, key, id) {
		t.Fatalf("Bind(%s, %d) = false", account, id)
	}
	gs.sendPlayerAuthRequest(account, key)
	if got, ok := gs.readPlayerAuthResponse(); got != account || !ok {
		t.Fatalf("PlayerAuthResponse = %q, %v, want %s, true", got, ok, account)
	}
}

// TestChangeAccessLevelOnlyForAccountsAdmittedOnTheLink: a linked server's
// ChangeAccessLevel is applied only to an account it admitted with session
// keys issued for it. Names it merely reports online through PlayerInGame,
// whether offline or admitted on another server, are ignored and logged
// (#4655).
func TestChangeAccessLevelOnlyForAccountsAdmittedOnTheLink(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	accounts := loginsql.NewAccountStore(db)
	for _, login := range []string{"offline1", "victim1", "player1"} {
		if _, err := accounts.CreateAccount(ctx, login, "hash", time.UnixMilli(1_700_000_000_000)); err != nil {
			t.Fatalf("CreateAccount(%s): %v", login, err)
		}
	}

	var logs syncBuffer
	addr, _, servers, sessions, _ := newTestLinkCommon(t, false, accounts, loginsql.NewGameServerStore(db), func(l *GameServerLink) {
		l.log = zerolog.New(&logs)
	})
	servers.Register(1, testHexID)
	servers.Register(2, secondHexID)
	gs := authGameServer(t, addr, 1, testHexID)
	rogue := authGameServer(t, addr, 2, secondHexID)

	admitOn(t, gs, sessions, 1, "victim1")
	admitOn(t, rogue, sessions, 2, "player1")

	// One connection's requests are handled in order, so once the last
	// change is applied the earlier ones were already decided.
	rogue.sendPlayerInGame("offline1", "victim1")
	waitUntil(t, "offline1 and victim1 on server 2's roster", func() bool {
		return servers.OnlineAccountCount(2) == 2
	})
	rogue.sendChangeAccessLevel(8, "offline1")
	rogue.sendChangeAccessLevel(8, "victim1")
	rogue.sendChangeAccessLevel(8, "player1")
	waitUntil(t, "player1 AccessLevel = 8", func() bool {
		got, err := accounts.Account(ctx, "player1")
		if err != nil {
			t.Fatalf("Account: %v", err)
		}
		return got.AccessLevel == 8
	})

	for _, login := range []string{"offline1", "victim1"} {
		got, err := accounts.Account(ctx, login)
		if err != nil {
			t.Fatalf("Account(%s): %v", login, err)
		}
		if got.AccessLevel != 0 {
			t.Errorf("%s AccessLevel = %d, want 0 (request ignored)", login, got.AccessLevel)
		}
		if !ignoredChangeLogged(logs.String(), login) {
			t.Errorf("no log for the ignored change of %s; logs:\n%s", login, logs.String())
		}
	}
}

// TestChangeAccessLevelIgnoredAfterLogoutOrAdmissionElsewhere: a server's
// admission ends when it reports the account logged out, or when another
// server admits it, so a later ChangeAccessLevel from it is ignored (#4655).
func TestChangeAccessLevelIgnoredAfterLogoutOrAdmissionElsewhere(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	accounts := loginsql.NewAccountStore(db)
	for _, login := range []string{"left1", "moved1", "sync1"} {
		if _, err := accounts.CreateAccount(ctx, login, "hash", time.UnixMilli(1_700_000_000_000)); err != nil {
			t.Fatalf("CreateAccount(%s): %v", login, err)
		}
	}

	var logs syncBuffer
	addr, _, servers, sessions, _ := newTestLinkCommon(t, false, accounts, loginsql.NewGameServerStore(db), func(l *GameServerLink) {
		l.log = zerolog.New(&logs)
	})
	servers.Register(1, testHexID)
	servers.Register(2, secondHexID)
	first := authGameServer(t, addr, 1, testHexID)
	second := authGameServer(t, addr, 2, secondHexID)

	admitOn(t, first, sessions, 1, "left1")
	admitOn(t, first, sessions, 1, "moved1")
	admitOn(t, first, sessions, 1, "sync1")
	admitOn(t, second, sessions, 2, "moved1")
	// A late logout from the first server leaves the second's admission.
	first.sendPlayerLogout("moved1")
	first.sendPlayerLogout("left1")

	first.sendChangeAccessLevel(8, "left1")
	first.sendChangeAccessLevel(8, "moved1")
	first.sendChangeAccessLevel(8, "sync1")
	waitUntil(t, "sync1 AccessLevel = 8", func() bool {
		got, err := accounts.Account(ctx, "sync1")
		if err != nil {
			t.Fatalf("Account: %v", err)
		}
		return got.AccessLevel == 8
	})
	for _, login := range []string{"left1", "moved1"} {
		got, err := accounts.Account(ctx, login)
		if err != nil {
			t.Fatalf("Account(%s): %v", login, err)
		}
		if got.AccessLevel != 0 {
			t.Errorf("%s AccessLevel = %d, want 0 (request ignored)", login, got.AccessLevel)
		}
		if !ignoredChangeLogged(logs.String(), login) {
			t.Errorf("no log for the ignored change of %s; logs:\n%s", login, logs.String())
		}
	}

	second.sendChangeAccessLevel(9, "moved1")
	waitUntil(t, "moved1 AccessLevel = 9 from the server holding it", func() bool {
		got, err := accounts.Account(ctx, "moved1")
		if err != nil {
			t.Fatalf("Account: %v", err)
		}
		return got.AccessLevel == 9
	})
}

// ignoredChangeLogged reports whether logs hold the warning for an ignored
// access-level change of account.
func ignoredChangeLogged(logs, account string) bool {
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "change access level: ignored") && strings.Contains(line, `"account":"`+account+`"`) {
			return true
		}
	}
	return false
}

// TestGameServerLinkRegistrationWriteFailureRefusesLink: when the new
// registration cannot be stored, the link is closed with no AuthResponse and
// neither the registry nor the roster keeps an entry for it, while an entry
// that existed before is left alone (#5048).
func TestGameServerLinkRegistrationWriteFailureRefusesLink(t *testing.T) {
	tests := []struct {
		name      string
		alternate bool
		seed      bool
		refusedID int
	}{
		{name: "fresh id", refusedID: 1},
		{name: "alternate id", alternate: true, seed: true, refusedID: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeRegistrationStore{err: errors.New("database down")}
			addr, l, servers, _, _ := newTestLinkWithRegistrationStore(t, true, nil, store)
			if tt.seed {
				servers.Register(1, secondHexID)
			}

			gs := dialGameServer(t, addr)
			gs.handshake()
			gs.sendGameServerAuth(1, tt.alternate, false, "*", 7777, 300, testHexID)
			gs.expectClosed()

			if _, ok := servers.Get(tt.refusedID); ok {
				t.Fatalf("registry entry %d kept after a failed registration write", tt.refusedID)
			}
			if tt.seed {
				if e, ok := servers.Get(1); !ok || e.Authed {
					t.Fatalf("seeded entry 1 = %+v, %v, want it kept offline", e, ok)
				}
			}
			l.roster.mu.RLock()
			n := len(l.roster.conns)
			l.roster.mu.RUnlock()
			if n != 0 {
				t.Fatalf("roster has %d entries, want none", n)
			}
		})
	}
}
