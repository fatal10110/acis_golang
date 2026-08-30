package loginserver

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/dbtest"
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
	gs.sendGameServerAuth(1, false, false, "gs.example.com", 7777, 300, testHexID)

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
	time.Sleep(100 * time.Millisecond)

	got, err := accounts.Account(ctx, "player1")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if got.AccessLevel != -1 {
		t.Fatalf("AccessLevel = %d, want -1", got.AccessLevel)
	}
}
