package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// instantTeleportPlayer has a script on the character's own queue jump it
// to to, and returns the frames its client gets.
func instantTeleportPlayer(t *testing.T, srv *gameservertest.Server, objID int32, to location.Location) [][]byte {
	t.Helper()
	obj, ok := srv.State.Object(objID)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	p := script.PlayerOf(obj.(attackable.Combatant))
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		p.InstantTeleportTo(script.Loc{X: to.X, Y: to.Y, Z: to.Z})
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	srv.Settle(t)
	return readUntilQuiet(srv.Client)
}

// stepToward sends the character walking from from to to and runs one
// position update, returning the frames its client gets.
func stepToward(t *testing.T, srv *gameservertest.Server, from, to location.Location) [][]byte {
	t.Helper()
	srv.Client.Send(encodeMoveBackwardToLocation(to, from, 1))
	srv.Settle(t)
	srv.TickPositions()
	return readUntilQuiet(srv.Client)
}

// TestInstantTeleportChangesZonesAtTheNextStep pins Creature.instantTeleportTo,
// which sets the position with no region edit and revalidates no zone: a
// character that jumps into or out of a peace zone in another region keeps
// the zones it had, with no compass update, until its next position
// update. That step moves it to the region of its position, whose zones it
// then enters (Creature.setRegion), and the zones of the region it left,
// which it leaves.
func TestInstantTeleportChangesZonesAtTheNextStep(t *testing.T) {
	t.Parallel()
	t.Run("into a peace zone", func(t *testing.T) {
		t.Parallel()
		peace := newCountedPeace(t, 5_000, 8_000, -2_000, 2_000)
		zones := zone.NewIndex()
		zones.Add(peace)
		srv, character, objID := bootInZones(t, zones)
		x, y, z := srv.PlayerPosition(t, objID)
		to := location.Location{X: x + 6_000, Y: y, Z: z}

		assertCompassCodes(t, instantTeleportPlayer(t, srv, objID, to))
		if character.InPeaceZone() {
			t.Fatal("in a peace zone at the jump, want it entered at the next step")
		}
		peace.assert(t, "after the jump", 0, 0, false)

		assertCompassCodes(t, stepToward(t, srv, to, location.Location{X: to.X, Y: to.Y + 1_000, Z: to.Z}), 0x0c) // PEACEZONE
		if !character.InPeaceZone() {
			t.Fatal("not in the peace zone after the first step")
		}
		peace.assert(t, "after the step", 1, 0, true)
	})
	t.Run("out of a peace zone", func(t *testing.T) {
		t.Parallel()
		peace := newCountedPeace(t, -2_000, 2_000, -2_000, 2_000)
		zones := zone.NewIndex()
		zones.Add(peace)
		srv, character, objID := bootInZones(t, zones)
		peace.assert(t, "at the spawn", 1, 0, true)
		x, y, z := srv.PlayerPosition(t, objID)
		to := location.Location{X: x + 6_000, Y: y, Z: z}

		assertCompassCodes(t, instantTeleportPlayer(t, srv, objID, to))
		if !character.InPeaceZone() {
			t.Fatal("left the peace zone at the jump, want it left at the next step")
		}
		peace.assert(t, "after the jump", 1, 0, true)

		assertCompassCodes(t, stepToward(t, srv, to, location.Location{X: to.X, Y: to.Y + 1_000, Z: to.Z}), 0x0f) // GENERALZONE
		if character.InPeaceZone() {
			t.Fatal("still in the peace zone after the first step")
		}
		peace.assert(t, "after the step", 1, 1, false)
	})
}
