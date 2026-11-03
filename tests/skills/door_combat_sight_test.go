package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAutoAttackOnClosedDoorInGeodataLowersHP drives an auto-attack at a
// closed door whose geodata blocker stands in a real geodata engine. The
// door is left out of the attacker's sight query to it
// (GeoEngine.canSeeTarget), so its own blocker never hides it: the click
// swings and the hit lowers the door's HP.
func TestAutoAttackOnClosedDoorInGeodataLowersHP(t *testing.T) {
	t.Parallel()
	const (
		doorX  = 200
		doorY  = 20
		height = 200
	)
	tmpl := combatDoorTemplate(door.KindDoor)
	sight := sightDoorTemplate(doorX, doorY, gameservertest.SpawnZ, height)
	tmpl.Position, tmpl.Coordinates, tmpl.Height = sight.Position, sight.Coordinates, sight.Height

	eng := sightEngine(t, doorX, doorY, gameservertest.SpawnZ, 0, 0)
	geo := &engineSightGeo{eng: eng}
	srv, objID, gate := bootDoorCombat(t, tmpl, nil, gameservertest.WithGeo(geo))
	if gate.Opened() {
		t.Fatal("fixture: the door is open, want it closed")
	}
	eng.AddObject(gate)

	// The fixture only means something if the closed door blocks a plain
	// sight query to itself from where the player stands.
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player := obj.(interface {
		Position() (int, int, int)
		CollisionHeight() float64
	})
	px, py, pz := player.Position()
	if eng.CanSeeActor(px, py, pz, player.CollisionHeight(), doorX, doorY, gameservertest.SpawnZ, gate.CollisionHeight()) {
		t.Fatal("fixture: the closed door does not block a plain sight query")
	}

	srv.BeginDoorSiege(t, gate.DoorID())
	steadyHits(t, srv, objID)
	attackDoor(t, srv.Client, gate)
	hit := readDoorHit(t, srv.Client, objID, gate)

	if hit.swing <= 0 || hit.hp != combatDoorMaxHP-hit.swing {
		t.Fatalf("hit %d left door HP %d, want a positive hit taken off %d", hit.swing, hit.hp, combatDoorMaxHP)
	}
}
