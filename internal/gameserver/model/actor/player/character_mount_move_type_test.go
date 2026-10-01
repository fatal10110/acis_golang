package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
)

// TestWyvernRiderMovesAsFlyer pins that the live movement steps a wyvern
// rider as a flyer and a strider rider or a dismounted one on the ground
// (Player.setMount sets MoveType.FLY only for a wyvern and resets it to
// GROUND on dismount).
func TestWyvernRiderMovesAsFlyer(t *testing.T) {
	c, _ := riderAt(t, 70)
	attachIdleLive(t, c)
	c.refreshMoveSpeed()
	wantMoveType := func(when string, want move.MoveType) {
		t.Helper()
		if got := c.Move().MoveType(); got != want {
			t.Fatalf("%s: Move().MoveType() = %d, want %d", when, got, want)
		}
	}
	wantMoveType("on foot", move.MoveGround)

	if !c.Mount(wyvernNPCID, 77) {
		t.Fatal("Mount(wyvern) = false")
	}
	wantMoveType("on a wyvern", move.MoveFly)
	c.Dismount()
	wantMoveType("dismounted from the wyvern", move.MoveGround)

	if !c.Mount(striderNPCID, 88) {
		t.Fatal("Mount(strider) = false")
	}
	wantMoveType("on a strider", move.MoveGround)
}
