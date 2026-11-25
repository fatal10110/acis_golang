package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestWalkOutOfTownKeepsOverlappingPeace pins #4957: a town whose combat
// rule (ZoneTown = 2) raises no peace takes none away on the way out, so a
// player walking from the town onto the peace zone overlapping it stays at
// peace: no compass change, and attacks stay barred.
func TestWalkOutOfTownKeepsOverlappingPeace(t *testing.T) {
	t.Parallel()
	townForm, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	peaceForm, err := zone.NewCuboid(-100, 500, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	town := zone.NewTown(1, townForm)
	town.CombatRule = 2
	zones.Add(town)
	zones.Add(zone.NewPeace(2, peaceForm))
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read()
	c.Read()
	c.Send(encodeEnterWorld())
	assertCompassCodes(t, readEnterWorldBurst(t, c), 0x0c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("player has no live character")
	}

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 300, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	expectGroundClickAck(t, c)
	if frame := c.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", frame[0])
	}
	waitForWorldPosition(t, srv, objID, target)
	assertCompassCodes(t, readUntilQuiet(c))
	if !character.InPeaceZone() {
		t.Fatal("leaving the town stripped the overlapping peace zone's peace")
	}
}
