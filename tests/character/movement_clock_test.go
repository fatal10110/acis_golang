package character

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLatePositionUpdateWalksElapsedTime pins PlayerMove.updatePosition's
// elapsed-time step (PlayerMove.java:214-222 timePassed = Duration.between(
// _instant, now), _instant = now; :246 passedDistance = speed / (1000d /
// timePassed)): a position update that runs 150 ms after the walk started
// walks 150 ms worth, and the next one, 100 ms later, only those 100 ms.
func TestLatePositionUpdateWalksElapsedTime(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("pinning the time between position updates needs the driven clock")
	}
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	walk := character.WalkSpeed()

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(location.Location{X: 3_000, Y: 20, Z: 30}, spawn, 1))
	expectGroundClickAck(t, c)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	mover := srv.PlayerMove(t, objID)

	srv.Advance(t, 50*time.Millisecond)
	srv.TickPositions() // 150 ms after the walk started
	accurate := float64(spawn.X) + walk*0.15
	if got, want := mover.Position().X, int(math.Floor(accurate+0.5)); got != want {
		t.Fatalf("X after an update 150 ms into the walk = %d, want %d (150 ms at walk speed %v)", got, want, walk)
	}
	srv.TickPositions()
	accurate += walk * 0.1
	if got, want := mover.Position().X, int(math.Floor(accurate+0.5)); got != want {
		t.Fatalf("X after the next update 100 ms later = %d, want %d (100 ms at walk speed %v)", got, want, walk)
	}
}
