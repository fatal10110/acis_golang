package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHostileWalkTurnsTowardItsDestination pins a monster's heading on a
// walk start: the return-home walk faces its spawn point before the movement
// is broadcast (CreatureMove.moveToLocation, setHeadingTo(destination) ahead
// of the MoveToLocation broadcast), so a stop mid-walk reports the walk's
// direction rather than the heading the monster had before it.
func TestHostileWalkTurnsTowardItsDestination(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	away := location.Location{X: hostileX, Y: hostileY + 500, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, away)
	drainUntilQuiet(t, c)
	const staleHeading = 12345
	hostile.SetHeading(staleHeading)
	want := away.HeadingTo(home)

	onQueue := func(what string, fn func()) {
		t.Helper()
		done := make(chan struct{})
		if !hostile.Queue().Post(func() { fn(); close(done) }) {
			t.Fatalf("post %s: queue closed", what)
		}
		<-done
	}
	started := false
	onQueue("ReturnHome", func() { started = hostile.ReturnHome() })
	if !started {
		t.Fatal("ReturnHome() = false outside drift range")
	}
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "return-home MoveToLocation")

	onQueue("StopMove", hostile.StopMove)
	stop := readUntil(t, c, serverpackets.OpcodeStopMove, "StopMove")
	r := wireReader(stop[0][1:])
	if id := r.ReadInt32(); id != hostile.ObjectID() {
		t.Fatalf("StopMove object = %d, want %d", id, hostile.ObjectID())
	}
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	if got := int(r.ReadInt32()); got != want {
		t.Fatalf("StopMove heading = %d, want %d toward the spawn point (stale %d)", got, want, staleHeading)
	}
}
