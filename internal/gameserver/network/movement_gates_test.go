package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestMoveLivePlayerRejectsBeyond9900Units pins MoveBackwardToLocation.java:
// 109-114: a target farther than 9900 units from the packet's own origin is
// rejected with ActionFailed instead of starting a walk.
func TestMoveLivePlayerRejectsBeyond9900Units(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{log: zerolog.Nop()}
	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 10000, Y: 0, Z: 0} // 10000 > 9900 cap
	gcl.moveLivePlayer(live, target, origin)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcodes = %x, want [ActionFailed]", got)
	}
}

// TestMoveLivePlayerRejectsZeroMoveSpeed pins MoveBackwardToLocation.java:
// 82-87: a fully overloaded player (weight-penalty band 4,
// PlayerStatus.getMoveSpeed() == 0) rejects the walk with both ActionFailed
// and CANT_MOVE_TOO_ENCUMBERED, distinct from the arrow-key (MoveMovement
// == 0) rejection the caller already handles.
func TestMoveLivePlayerRejectsZeroMoveSpeed(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	live.Character.SetWeightLimitMultiplier(1)
	live.Character.Inventory().AddNew(9500, 100000, 999) // Heavy Ingot, weight 10 each
	live.Character.Inventory().UpdateWeight()
	live.Character.RefreshWeightPenalty()
	if got := live.Character.WeightPenalty(); got != 4 {
		t.Fatalf("WeightPenalty() = %d, want 4 (fully overloaded)", got)
	}
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{log: zerolog.Nop()}
	target := location.Location{X: 100, Y: 0, Z: 0}
	origin := location.Location{X: 0, Y: 0, Z: 0}
	gcl.moveLivePlayer(live, target, origin)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 2 ||
		got[0] != serverpackets.OpcodeActionFailed || got[1] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcodes = %x, want [ActionFailed, SystemMessage]", got)
	}
}
