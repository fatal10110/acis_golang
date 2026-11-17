package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// poleBandDoorX puts the door's center past the pole's 66-unit range plus
// the player's radius, but within the door's own 16-unit collision radius
// of it.
const poleBandDoorX = 74

// TestPoleSwingReachesDoorByItsRadius pins the pole splash scan to widen by
// the door's collision radius as by any creature's (MathUtil.checkIfInRange
// adds every Creature's radius, a door's 16 included): during a siege, a door
// whose center stands just past the pole's range plus the attacker's radius,
// straight ahead, still takes the splash.
func TestPoleSwingReachesDoorByItsRadius(t *testing.T) {
	t.Parallel()
	origin := location.Location{X: 0, Y: 0, Z: 0}
	tmpl := combatDoorTemplate(door.KindDoor)
	tmpl.Position = location.Location{X: origin.X + poleBandDoorX, Y: origin.Y, Z: origin.Z}
	tmpl.Coordinates = []location.Point{
		{X: tmpl.Position.X - 8, Y: -8},
		{X: tmpl.Position.X + 8, Y: -8},
		{X: tmpl.Position.X + 8, Y: 8},
		{X: tmpl.Position.X - 8, Y: 8},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacterAt("Newbie", 5, 0, origin),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{poleCount()})),
		gameservertest.WithItemTemplates(poleCatalog()),
		gameservertest.WithDoors(tmpl),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	pole := srv.GiveItem(t, objID, poleWeaponID, 1)
	seedKnownSkill(t, srv, objID, poleCountSkillID, 1)
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(pole)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)

	gate, ok := srv.WorldObjects.Door(tmpl.ID)
	if !ok {
		t.Fatal("door not spawned")
	}
	srv.BeginDoorSiege(t, gate.DoorID())
	steadyHits(t, srv, objID)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	px, py, pz := player.Position()
	if (location.Location{X: px, Y: py, Z: pz}) != origin {
		t.Fatalf("player at (%d, %d, %d), want %+v", px, py, pz, origin)
	}
	gx, gy, gz := gate.Position()
	d := (location.Location{X: px, Y: py, Z: pz}).Distance3D(location.Location{X: gx, Y: gy, Z: gz})
	bare := float64(player.PhysicalAttackRange()) + player.CollisionRadius()
	if d <= bare || d > bare+door.CollisionRadius {
		t.Fatalf("door %v away, want it in (%v, %v]: out of the bare reach, inside it widened by the door's radius", d, bare, bare+door.CollisionRadius)
	}

	// The monster stands straight ahead between the player and the door.
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: origin.X + 40, Y: origin.Y, Z: origin.Z})
	drainUntilQuiet(t, c)
	startMelee(t, srv, objID, hostile.ObjectID())
	hp := hostile.CurrentHP()
	srv.AdvanceUntil(t, "the swing lands on the monster", func() bool { return hostile.CurrentHP() < hp || hostile.Dead() })
	drainUntilQuiet(t, c)
	if gate.CurrentHP() >= combatDoorMaxHP {
		t.Fatalf("door HP %v after the swing, want the splash to hit it", gate.CurrentHP())
	}
}
