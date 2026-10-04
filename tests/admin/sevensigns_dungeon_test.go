package admin

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// storeInDungeon stores objectID in a Seven Signs dungeon.
func storeInDungeon(t *testing.T, srv *gameservertest.Server, objectID int32) {
	t.Helper()
	if _, err := srv.DB.Exec("UPDATE characters SET isin7sdungeon = 1 WHERE obj_Id = ?", objectID); err != nil {
		t.Fatalf("seed dungeon membership: %v", err)
	}
}

// inDungeon reports whether the online objectID is in a Seven Signs
// dungeon.
func inDungeon(t *testing.T, srv *gameservertest.Server, objectID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objectID)
	if !ok {
		t.Fatalf("player %d not online", objectID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return c.In7sDungeon()
}

// addDungeonPlayer seeds "Player" on account player2, stored in a Seven
// Signs dungeon and signed up for Dawn, so the competition lets it stay,
// then dials it and enters the world.
func addDungeonPlayer(t *testing.T, srv *gameservertest.Server) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	storeInDungeon(t, srv, ch.ID)
	if err := srv.SevenSigns.SetPlayerInfo(context.Background(), ch.ID, sevensigns.Dawn, sevensigns.Strife); err != nil {
		t.Fatalf("sign up: %v", err)
	}
	c := srv.DialClient(t, "player2", 1)
	enterWorld(t, c)
	if !inDungeon(t, srv, ch.ID) {
		t.Fatal("Dawn member left the dungeon at login")
	}
	return c, ch.ID
}

// spawnTown is a restart table whose one point covers the fixture spawn.
func spawnTown(town location.Location) *restart.Table {
	return &restart.Table{Points: []restart.Point{{
		Name:       "TestTown",
		Points:     []location.Location{town},
		ChaoPoints: []location.Location{town},
		MapRegions: []location.Point{{
			X: (spawnX-world.MinX)/world.TileSize + world.TileXMin,
			Y: (spawnY-world.MinY)/world.TileSize + world.TileYMin,
		}},
	}}}
}

// TestAdminSendHomeLeavesSevenSignsDungeon pins //sendhome's dungeon exit
// (AdminTeleport.java:158-159): the player sent to town leaves its Seven
// Signs dungeon.
func TestAdminSendHomeLeavesSevenSignsDungeon(t *testing.T) {
	t.Parallel()
	town := location.Location{X: 20000, Y: 20000, Z: 300}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithRestartPoints(spawnTown(town)))
	gm := srv.Client
	enterWorld(t, gm)
	player, playerID := addDungeonPlayer(t, srv)
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("sendhome Player"))
	if at := readTeleport(t, player, playerID); !nearTown(at, town) {
		t.Fatalf("//sendhome Player destination = %v, want within 20 of %v", at, town)
	}
	if inDungeon(t, srv, playerID) {
		t.Fatal("sent home still in the dungeon")
	}
}

// TestAdminJailLeavesSevenSignsDungeon pins the jail's dungeon exit
// (Punishment.java:138): a player jailed leaves its Seven Signs dungeon on
// the way to the jail.
func TestAdminJailLeavesSevenSignsDungeon(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(punishPages(t)),
		gameservertest.WithZones(shippedJailZones(t)))
	gm := srv.Client
	enterWorld(t, gm)
	player, playerID := addDungeonPlayer(t, srv)
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("jail Player 30"))
	if at := teleportOf(t, settle(t, player), playerID); at != jailAt {
		t.Fatalf("jail teleport = %v, want %v", at, jailAt)
	}
	if inDungeon(t, srv, playerID) {
		t.Fatal("jailed still in the dungeon")
	}
}

// TestGameMasterStaysInSevenSignsDungeon pins the game master exemption of
// Player.onPlayerEnter and teleLosingCabalFromDungeons: a game master in a
// dungeon it has no sign-up for stays there at login and through a period
// change, where anybody else would be sent to town.
func TestGameMasterStaysInSevenSignsDungeon(t *testing.T) {
	t.Parallel()
	var change func()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithRestartPoints(spawnTown(location.Location{X: 20000, Y: 20000, Z: 300})),
		gameservertest.WithSevenSignsTimer(func(_ time.Duration, fn func()) *time.Timer {
			change = fn
			return nil
		}))
	storeInDungeon(t, srv, gmID)
	gm := srv.Client
	start := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	start.WriteInt32(0)
	start.WriteUint16(0)
	start.WriteInt32(0)
	start.WriteInt32(0)
	start.WriteInt32(0)
	gm.Send(start.Bytes())
	gm.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	noTeleport(t, settle(t, gm))
	if !inDungeon(t, srv, gmID) {
		t.Fatal("game master left the dungeon at login")
	}

	change()
	noTeleport(t, settle(t, gm))
	if !inDungeon(t, srv, gmID) {
		t.Fatal("game master left the dungeon on the period change")
	}
}
