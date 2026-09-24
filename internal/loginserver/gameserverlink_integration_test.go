package loginserver

import (
	"context"
	"database/sql"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/link"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	_ "github.com/go-sql-driver/mysql"
)

// accountsSchema mirrors aCis_datapack/sql/accounts.sql verbatim.
const accountsSchema = "CREATE TABLE IF NOT EXISTS `accounts` (\n" +
	"	`login` VARCHAR(45) NOT NULL DEFAULT '',\n" +
	"	`password` VARCHAR(60) NOT NULL DEFAULT '',\n" +
	"	`last_active` BIGINT NOT NULL DEFAULT 0,\n" +
	"	`access_level` INT(3) NOT NULL DEFAULT 0,\n" +
	"	`last_server` INT(4) NOT NULL DEFAULT 1,\n" +
	"	PRIMARY KEY (`login`)\n" +
	")"

// gameserversSchema mirrors aCis_datapack/sql/gameservers.sql verbatim.
const gameserversSchema = "CREATE TABLE IF NOT EXISTS `gameservers` (\n" +
	"  `server_id` int(11) NOT NULL default '0',\n" +
	"  `hexid` varchar(50) NOT NULL default '',\n" +
	"  `host` varchar(50) NOT NULL default '',\n" +
	"  PRIMARY KEY (`server_id`)\n" +
	")"

func newIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	return dbtest.NewDB(t, accountsSchema, gameserversSchema)
}

func TestGameServerLinkFreshRegistrationPersistsToDB(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	gameServers := loginsql.NewGameServerStore(db)

	addr, _, servers, _, _ := newTestLinkCommon(t, true, loginsql.NewAccountStore(db), gameServers)

	gs := dialGameServer(t, addr)
	gs.handshake()
	// An IP literal resolves without a DNS query, so the AuthResponse wait
	// never depends on the host resolver's retry timeout.
	gs.sendGameServerAuth(1, false, false, "127.0.0.1", 7777, 300, testHexID)

	ok, id, name, _ := gs.readAuthResult()
	if !ok || id != 1 || name != "Bartz" {
		t.Fatalf("readAuthResult() = ok=%v id=%d name=%q, want ok=true id=1 name=Bartz", ok, id, name)
	}

	entry, exists := servers.Get(1)
	if !exists || !entry.Authed {
		t.Fatalf("registry entry after auth = %+v", entry)
	}

	stored, err := gameServers.GameServer(ctx, 1)
	if err != nil {
		t.Fatalf("GameServer(1): %v", err)
	}
	if stored.ID != 1 {
		t.Fatalf("stored.ID = %d, want 1", stored.ID)
	}
	if stored.Host != "127.0.0.1" {
		t.Fatalf("stored.Host = %q, want 127.0.0.1", stored.Host)
	}
}

// TestGameServerLinkHostResolution drives registration with a stubbed
// resolver: a resolved host is stored as its address, and a host that fails
// to resolve or resolves to nothing falls back to the connection IP in both
// the registry and the persisted gameservers row.
func TestGameServerLinkHostResolution(t *testing.T) {
	const advertised = "gs.invalid"
	tests := []struct {
		name     string
		resolved []string
		err      error
		wantHost string
	}{
		{name: "resolved", resolved: []string{"10.1.2.3", "10.1.2.4"}, wantHost: "10.1.2.3"},
		{name: "lookup error", err: &net.DNSError{Err: "no such host", Name: advertised, IsNotFound: true}, wantHost: "127.0.0.1"},
		{name: "empty result", wantHost: "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newIntegrationDB(t)
			gameServers := loginsql.NewGameServerStore(db)

			var looked []string
			addr, _, servers, _, _ := newTestLinkCommon(t, true, loginsql.NewAccountStore(db), gameServers, func(l *GameServerLink) {
				l.lookupHost = func(host string) ([]string, error) {
					looked = append(looked, host)
					return tt.resolved, tt.err
				}
			})

			gs := dialGameServer(t, addr)
			gs.handshake()
			gs.sendGameServerAuth(1, false, false, advertised, 7777, 300, testHexID)
			if ok, id, _, _ := gs.readAuthResult(); !ok || id != 1 {
				t.Fatalf("readAuthResult() = ok=%v id=%d, want ok=true id=1", ok, id)
			}

			entry, exists := servers.Get(1)
			if !exists || entry.Host != tt.wantHost {
				t.Fatalf("registry entry = %+v (exists=%v), want Host %q", entry, exists, tt.wantHost)
			}
			if len(looked) != 1 || looked[0] != advertised {
				t.Fatalf("lookupHost calls = %q, want [%q]", looked, advertised)
			}
			stored, err := gameServers.GameServer(context.Background(), 1)
			if err != nil {
				t.Fatalf("GameServer(1): %v", err)
			}
			if stored.Host != tt.wantHost {
				t.Fatalf("stored.Host = %q, want %q", stored.Host, tt.wantHost)
			}
		})
	}
}

// TestGameServerLinkRejectedAuthSkipsHostResolution proves a registration
// that is refused never resolves the advertised host.
func TestGameServerLinkRejectedAuthSkipsHostResolution(t *testing.T) {
	var lookups atomic.Int32
	addr, _, _, _, _ := newTestLinkCommon(t, false, nil, nil, func(l *GameServerLink) {
		l.lookupHost = func(string) ([]string, error) {
			lookups.Add(1)
			return nil, &net.DNSError{Err: "no such host", Name: "gs.invalid", IsNotFound: true}
		}
	})

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "gs.invalid", 7777, 300, testHexID)
	if ok, _, _, reason := gs.readAuthResult(); ok || reason != byte(link.ReasonWrongHexID) {
		t.Fatalf("readAuthResult() = ok=%v reason=%d, want ok=false reason=%d", ok, reason, link.ReasonWrongHexID)
	}
	if n := lookups.Load(); n != 0 {
		t.Fatalf("lookupHost calls = %d, want 0 on a rejected registration", n)
	}
}

func TestGameServerLinkChangeAccessLevelUpdatesDB(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	accounts := loginsql.NewAccountStore(db)
	if _, err := accounts.CreateAccount(ctx, "player1", "hash", time.UnixMilli(1_700_000_000_000)); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	addr, _, servers, _, _ := newTestLinkCommon(t, false, accounts, loginsql.NewGameServerStore(db))
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("registration failed, want success")
	}

	gs.sendChangeAccessLevel(-1, "player1")

	waitUntil(t, "AccessLevel = -1", func() bool {
		got, err := accounts.Account(ctx, "player1")
		if err != nil {
			t.Fatalf("Account: %v", err)
		}
		return got.AccessLevel == -1
	})
}
