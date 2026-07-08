//go:build integration

package sql

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

// gameserversSchema mirrors aCis_datapack/sql/gameservers.sql verbatim.
const gameserversSchema = "CREATE TABLE IF NOT EXISTS `gameservers` (\n" +
	"  `server_id` int(11) NOT NULL default '0',\n" +
	"  `hexid` varchar(50) NOT NULL default '',\n" +
	"  `host` varchar(50) NOT NULL default '',\n" +
	"  PRIMARY KEY (`server_id`)\n" +
	")"

func newGameServerIntegrationStore(t *testing.T) *GameServerStore {
	t.Helper()
	ctx := context.Background()

	container, err := mariadb.Run(ctx, "mariadb:11")
	if err != nil {
		t.Fatalf("start mariadb container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate mariadb container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(ctx, gameserversSchema); err != nil {
		t.Fatalf("create gameservers table: %v", err)
	}
	return NewGameServerStore(db)
}

func TestGameServerStore_PersistenceRoundTrip(t *testing.T) {
	store := newGameServerIntegrationStore(t)
	ctx := context.Background()

	want := model.NewGameServer(2, []byte{0x00, 0x80, 0x01}, "")
	if err := store.CreateGameServer(ctx, want); err != nil {
		t.Fatalf("CreateGameServer() unexpected error: %v", err)
	}

	got, err := store.GameServer(ctx, 2)
	if err != nil {
		t.Fatalf("GameServer() unexpected error: %v", err)
	}
	if got.ID != want.ID || got.Host != want.Host || !bytes.Equal(got.HexID, want.HexID) {
		t.Fatalf("GameServer() after create = %+v hex=%x, want %+v hex=%x", got, got.HexID, want, want.HexID)
	}
	all, err := store.GameServers(ctx)
	if err != nil {
		t.Fatalf("GameServers() unexpected error: %v", err)
	}
	if len(all) != 1 || all[2].ID != want.ID || !bytes.Equal(all[2].HexID, want.HexID) {
		t.Fatalf("GameServers() = %+v, want one row for id 2 hex %x", all, want.HexID)
	}

	if err := store.SetGameServerHost(ctx, 2, "127.0.0.1"); err != nil {
		t.Fatalf("SetGameServerHost() unexpected error: %v", err)
	}

	reloaded, err := NewGameServerStore(store.db).GameServer(ctx, 2)
	if err != nil {
		t.Fatalf("GameServer() after reload unexpected error: %v", err)
	}
	if reloaded.Host != "127.0.0.1" || !bytes.Equal(reloaded.HexID, want.HexID) {
		t.Fatalf("GameServer() after reload = %+v hex=%x, want host 127.0.0.1 hex %x", reloaded, reloaded.HexID, want.HexID)
	}

	_, err = store.GameServer(ctx, 99)
	if !errors.Is(err, ErrGameServerNotFound) {
		t.Fatalf("GameServer() missing row error = %v, want ErrGameServerNotFound", err)
	}
	if err := store.SetGameServerHost(ctx, 99, "127.0.0.1"); !errors.Is(err, ErrGameServerNotFound) {
		t.Fatalf("SetGameServerHost() missing row error = %v, want ErrGameServerNotFound", err)
	}
}
