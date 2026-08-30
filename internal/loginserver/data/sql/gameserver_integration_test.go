package sql

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	_ "github.com/go-sql-driver/mysql"
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
	return NewGameServerStore(dbtest.NewDB(t, gameserversSchema))
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

func TestGameServerStore_GameServersSkipsInvalidHexID(t *testing.T) {
	for _, invalidHexID := range []string{"", "null"} {
		t.Run(invalidHexID, func(t *testing.T) {
			store := newGameServerIntegrationStore(t)
			ctx := context.Background()

			if err := store.CreateGameServer(ctx, model.NewGameServer(1, []byte{1, 2, 3}, "good")); err != nil {
				t.Fatalf("CreateGameServer() unexpected error: %v", err)
			}
			if _, err := store.db.ExecContext(ctx, "INSERT INTO gameservers (hexid, server_id, host) VALUES (?, ?, ?)", invalidHexID, 2, "bad"); err != nil {
				t.Fatalf("insert invalid gameserver: %v", err)
			}

			servers, err := store.GameServers(ctx)
			if err == nil {
				t.Fatal("GameServers() error = nil, want invalid hex id error")
			}
			if len(servers) != 1 || servers[1].Host != "good" || !bytes.Equal(servers[1].HexID, []byte{1, 2, 3}) {
				t.Fatalf("GameServers() = %+v, want valid gameserver 1", servers)
			}
		})
	}
}
