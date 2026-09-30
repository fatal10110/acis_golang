package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// flatGeo is gameservertest.Geo on flat ground at the fixture height, so a
// walk stopping mid-leg keeps its Z.
type flatGeo struct{ gameservertest.Geo }

func (flatGeo) Height(int, int, int) int16 { return hostileZ }

// TestAttackApproachTracksMovedTarget pins the player's pawn walk
// (PlayerMove.updatePosition, PlayerMove.java:234 and :323): each position
// update re-aims the approach at where the target stands now, and the walk
// ends at the first step strictly within the attack range of it (2D), not
// on the cell the target stood on when the approach started.
func TestAttackApproachTracksMovedTarget(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(flatGeo{}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "MoveToPawn"), serverpackets.OpcodeMoveToPawn, "approach")

	mover := srv.PlayerMove(t, objID)
	for range 3 {
		srv.TickPositions()
	}
	moved := location.Location{X: hostileX + 500, Y: hostileY + 300, Z: hostileZ}
	hostile.TeleportTo(moved)
	for i := 0; mover.Moving(); i++ {
		if i == 300 {
			t.Fatalf("approach still under way at %+v after 30 s of position updates", mover.Position())
		}
		srv.TickPositions()
	}

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	reach := player.PhysicalAttackRange()
	pos := mover.Position()
	// One position update covers well under 30 units at the fixture's speed.
	if !pos.In2DRadius(moved, reach) || pos.In2DRadius(moved, reach-30) {
		t.Fatalf("approach ended at %+v, %.1f from the moved target; want the first step within the attack range %d",
			pos, pos.Distance2D(moved), reach)
	}
	drainUntilQuiet(t, c)
}
