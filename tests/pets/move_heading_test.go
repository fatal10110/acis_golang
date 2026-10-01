package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPetWalkTurnsTowardItsDestination pins a summon's heading on a walk
// start: the walk faces its destination before the movement is broadcast
// (CreatureMove.moveToLocation, setHeadingTo(destination) ahead of the
// MoveToLocation broadcast), so a stop mid-walk reports the walk's direction
// rather than the heading the summon had before it.
func TestPetWalkTurnsTowardItsDestination(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	const staleHeading = 12345
	petActor.SetHeading(staleHeading)

	// A fear landing walks the pet away from the monster.
	landPetFear(t, petActor, hostile, 1092, 6, 2)
	var origin, dest location.Location
	found := false
	for _, frame := range drainFrames(t, h.client) {
		if frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != petActor.ObjectID() {
			continue
		}
		dest = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		origin = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		found = true
		break
	}
	if !found {
		t.Fatal("fear landing sent no MoveToLocation for the pet")
	}
	want := origin.HeadingTo(dest)
	if want == staleHeading {
		t.Fatalf("test setup: the walk %+v -> %+v keeps the stale heading", origin, dest)
	}

	done := make(chan struct{})
	if !petActor.Queue().Post(func() { petActor.StopMove(); close(done) }) {
		t.Fatal("post StopMove: queue closed")
	}
	<-done
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeStopMove, "pet StopMove")
	r := wire.NewReader(frames[len(frames)-1][1:])
	if id := r.ReadInt32(); id != petActor.ObjectID() {
		t.Fatalf("StopMove object = %d, want the pet %d", id, petActor.ObjectID())
	}
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	if got := int(r.ReadInt32()); got != want {
		t.Fatalf("StopMove heading = %d, want %d toward the walk's destination (stale %d)", got, want, staleHeading)
	}
}
