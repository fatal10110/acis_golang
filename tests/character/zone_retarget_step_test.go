package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestZoneEnterCountsSameCellRetargets pins the retarget catch-up as a zone
// revalidation step: a move request that retargets a walk in flight first
// runs updatePosition(true) (PlayerMove.java:114), which ends in
// revalidateZone(false) (:321) even when less than one cell was covered
// (timePassed is at least 1 ms, :218-219). A player whose first position
// update takes it into a water zone, then retargets three times on the same
// cell, has not entered the zone; the fourth retarget is the fifth counted
// call and enters it, with the entry UserInfo. The world has a flat floor at
// the spawn height, so a catch-up keeps the player on the same location.
func TestZoneEnterCountsSameCellRetargets(t *testing.T) {
	t.Parallel()
	srv, character, objID := bootBesideWater(t, gameservertest.WithGeo(gameservertest.FlatGeo{Z: besideWaterSpawn.Z}))
	spawn := besideWaterSpawn
	mover := srv.PlayerMove(t, objID)
	c := srv.Client
	c.Send(encodeMoveBackwardToLocation(location.Location{X: spawn.X + 3_000, Y: spawn.Y, Z: spawn.Z}, spawn, 1))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", reply[0])
	}
	tickFrames(t, srv)
	if at := mover.Position(); at.X < spawn.X+5 || character.InWater() {
		t.Fatalf("after one update: at %+v, in water %v; want inside the zone, not entered yet", at, character.InWater())
	}

	for retarget := 1; retarget <= 4; retarget++ {
		before := mover.Position()
		dest := location.Location{X: spawn.X + 3_000, Y: spawn.Y + retarget%2, Z: spawn.Z}
		frames := testsupport.SyncBarrierFrames(t, c, func() {
			c.Send(encodeMoveBackwardToLocation(dest, before, 1))
			c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
		}, serverpackets.OpcodeItemList)
		if at := mover.Position(); at != before {
			t.Fatalf("retarget %d moved the player from %+v to %+v, want a same-cell catch-up", retarget, before, at)
		}
		if firstOpcode(frames, serverpackets.OpcodeMoveToLocation) < 0 {
			t.Fatalf("retarget %d frames = %x, want MoveToLocation", retarget, frames)
		}
		userInfo := firstOpcode(frames, serverpackets.OpcodeUserInfo)
		if retarget < 4 {
			if character.InWater() || mover.MoveType() != move.MoveGround || userInfo >= 0 {
				t.Fatalf("retarget %d (step %d): in water %v, move type %d, frames %x; want the zone entered on the fifth step only",
					retarget, retarget+1, character.InWater(), mover.MoveType(), frames)
			}
			continue
		}
		if !character.InWater() || mover.MoveType() != move.MoveSwim || userInfo < 0 {
			t.Fatalf("retarget %d (step 5): in water %v, move type %d, frames %x; want the zone entered with its UserInfo",
				retarget, character.InWater(), mover.MoveType(), frames)
		}
	}
}
