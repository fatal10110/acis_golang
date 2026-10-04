package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// spawnLairNPCs places a raid-related monster and a plain one inside the
// boss zone of bootInBossZone, both away from their spawn point outside it.
func spawnLairNPCs(t *testing.T, srv *gameservertest.Server) (raid, plain *npc.Hostile) {
	t.Helper()
	home := location.Location{X: 5_000, Y: 20, Z: 30}
	raid = srv.SpawnMovingHostileNPCAt(t, "Monster", home, location.Location{X: 500, Y: 20, Z: 30})
	raid.SetRaidRelated(true)
	plain = srv.SpawnMovingHostileNPCAt(t, "Monster", home, location.Location{X: 600, Y: 20, Z: 30})
	if !raid.InsideZone(zone.FlagBoss) || !plain.InsideZone(zone.FlagBoss) {
		t.Fatal("NPCs spawned inside the boss zone are not in it")
	}
	drainQuiet(t, srv.Client)
	return raid, plain
}

// TestBossZoneLastPlayableOutSendsRaidHome pins BossZone.onExit's playable
// branch (BossZone.java:162-174): once the last playable leaves the lair,
// every raid-related monster still inside returns to its spawn point, and a
// monster that is not raid-related stays.
func TestBossZoneLastPlayableOutSendsRaidHome(t *testing.T) {
	t.Parallel()
	srv, boss, objID := bootInBossZone(t)
	raid, plain := spawnLairNPCs(t, srv)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	// The player walks out of the lair, staying on the grid near it: a
	// teleport would take it off the grid and put the lair's region to
	// sleep, which sends its monsters back to peace.
	_, y, z := srv.PlayerPosition(t, objID)
	for range zone.StepsPerRevalidation {
		player.SyncPosition(location.Location{X: 1_200, Y: y, Z: z})
	}
	for _, a := range boss.Occupants() {
		if a.Class() == zone.ClassPlayer {
			t.Fatal("player that walked out of the boss zone is still in it")
		}
	}

	srv.AdvanceUntil(t, "raid monster heads home", raid.IsMoving)
	if plain.IsMoving() {
		t.Fatal("a monster that is not raid-related left for home too")
	}
}

// walkOut walks h out of the boss zone of bootInBossZone to at, one movement
// step at a time, as its move controller does.
func walkOut(h *npc.Hostile, at location.Location) {
	for range zone.StepsPerRevalidation {
		h.Move().SetPosition(at)
		h.SyncPosition(at)
	}
}

// TestBossZoneRaidLeavingGoesHome pins BossZone.onExit's attackable branch
// (BossZone.java:176-177): a raid-related monster that walks out of the lair
// returns to its spawn point; any other monster does not.
func TestBossZoneRaidLeavingGoesHome(t *testing.T) {
	t.Parallel()
	srv, _, _ := bootInBossZone(t)
	raid, plain := spawnLairNPCs(t, srv)

	walkOut(plain, location.Location{X: 1_500, Y: 20, Z: 30})
	walkOut(raid, location.Location{X: 1_600, Y: 20, Z: 30})
	if raid.InsideZone(zone.FlagBoss) || plain.InsideZone(zone.FlagBoss) {
		t.Fatal("NPCs that walked out of the boss zone are still in it")
	}

	srv.AdvanceUntil(t, "raid monster heads home", raid.IsMoving)
	if plain.IsMoving() {
		t.Fatal("a monster that is not raid-related left for home too")
	}
}

// TestBossZoneRaidTeleportedOutStays pins the same branch for a teleport:
// the reference runs returnHome inside teleportTo, at the old position and
// while the teleport disables movement, so a raid-related monster teleported
// out of the lair does not set off for home on landing.
func TestBossZoneRaidTeleportedOutStays(t *testing.T) {
	t.Parallel()
	srv, _, _ := bootInBossZone(t)
	raid, _ := spawnLairNPCs(t, srv)

	raid.TeleportTo(location.Location{X: 1_600, Y: 20, Z: 30})
	if raid.InsideZone(zone.FlagBoss) {
		t.Fatal("raid monster teleported out of the boss zone is still in it")
	}

	srv.Advance(t, time.Second)
	if raid.IsMoving() {
		t.Fatal("raid monster teleported out of its lair walked home on landing")
	}
}
