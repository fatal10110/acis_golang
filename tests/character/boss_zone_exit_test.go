package character

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootInBossZone boots a player standing inside a boss zone that has
// already admitted it (BossZone.allowPlayerEntry, then the entry consumes
// the deadline).
func bootInBossZone(t *testing.T) (*gameservertest.Server, *zone.Boss, int32) {
	t.Helper()
	form, err := zone.NewCuboid(-1000, 1000, -1000, 1000, -10000, 10000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	set := commons.NewStatSet()
	set.Set("InvadeTime", "600000")
	boss, err := zone.NewBoss(1, form, set)
	if err != nil {
		t.Fatalf("build boss zone: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(boss)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	objID := srv.SoleObjectID(t)
	boss.AllowEntry(objID, time.Minute)
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)
	if !slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("permitted player lost its permission on entry")
	}
	return srv, boss, objID
}

// TestBossZoneTeleportOutRevokesPermission pins BossZone.onExit's online
// branch for a teleport (BossZone.java:146-158): the player is still
// connected (Player.isOnline), so walking or teleporting out drops the
// permission. Being off the grid mid-teleport must not read as offline.
func TestBossZoneTeleportOutRevokesPermission(t *testing.T) {
	t.Parallel()
	srv, boss, objID := bootInBossZone(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	x, y, z := srv.PlayerPosition(t, objID)

	// The teleport runs to completion on this goroutine, zone exit included.
	player.TeleportTo(x+5000, y, z, 0)
	if slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("player teleported out of the boss zone kept its entry permission")
	}
}

// TestBossZoneLogoutKeepsPermission is the disconnect branch
// (BossZone.java:148-151): a player logging out inside keeps its permission
// and gets a fresh re-entry window.
func TestBossZoneLogoutKeepsPermission(t *testing.T) {
	t.Parallel()
	srv, boss, objID := bootInBossZone(t)

	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if !srv.Client.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	if !slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("player logging out inside the boss zone lost its entry permission")
	}
}
